package chanx

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"go.chanx.dev/layers/v2"
	"go.chanx.dev/v2/internal/schema"
	"go.chanx.dev/v2/protocol"
)

// stamp is the envelope written on an outgoing frame.
type stamp struct {
	ref     *string
	topic   string
	inTopic bool // the topic may be "", as a client can send it
	seq     *int64
	camel   bool
}

// encodeFrame writes {"action", "payload", "version", "topic", "ref", "seq"} in
// that order. payload is any JSON-able value; its floats keep a decimal point, as
// on a layer-forwarded frame.
func encodeFrame(action string, payload any, st stamp) ([]byte, error) {
	tree, err := frameTree(reflect.ValueOf(payload))
	if err != nil {
		return nil, err
	}
	var p []byte
	if st.camel {
		p, err = camelJSON(tree)
	} else {
		p, err = json.Marshal(jsonFloats(tree))
	}
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString(`{"action":`)
	a, _ := json.Marshal(action)
	b.Write(a)
	b.WriteString(`,"payload":`)
	b.Write(p)
	if st.ref != nil || st.inTopic {
		b.WriteString(`,"version":`)
		b.WriteString(strconv.Itoa(protocol.Version))
	}
	if st.inTopic {
		t, _ := json.Marshal(st.topic)
		b.WriteString(`,"topic":`)
		b.Write(t)
	}
	if st.ref != nil {
		r, _ := json.Marshal(*st.ref)
		b.WriteString(`,"ref":`)
		b.Write(r)
	}
	if st.inTopic && st.seq != nil {
		b.WriteString(`,"seq":`)
		b.WriteString(strconv.FormatInt(*st.seq, 10))
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// encodeRaw writes a layer-forwarded message (a group message) as it came.
func encodeRaw(v any, camel bool) ([]byte, error) {
	if camel {
		return camelJSON(v)
	}
	return json.Marshal(jsonFloats(v))
}

func camelJSON(v any) ([]byte, error) {
	data, err := json.Marshal(jsonFloats(v))
	if err != nil {
		return nil, err
	}
	generic, err := decodeGeneric(data)
	if err != nil {
		return nil, err
	}
	return json.Marshal(protocol.CamelizeKeys(generic))
}

func decodeGeneric(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// jsonFloats makes integral float64 values from a layer map look like floats on
// the wire, as Python writes them (2.0 and 1e+16, where Go writes 2 and
// 10000000000000000), so clients still decode a float.
func jsonFloats(v any) any {
	switch x := v.(type) {
	case float64:
		switch {
		case x != math.Trunc(x) || math.IsInf(x, 0):
			return x
		case math.Abs(x) < 1e16:
			return json.Number(strconv.FormatFloat(x, 'f', 1, 64))
		default:
			return json.Number(strconv.FormatFloat(x, 'e', -1, 64))
		}
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = jsonFloats(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = jsonFloats(val)
		}
		return out
	case object:
		out := make(object, len(x))
		for i, f := range x {
			out[i] = member{f.name, jsonFloats(f.value)}
		}
		return out
	}
	return v
}

// object is a struct in a frame: its fields in declaration order, as
// encoding/json and Pydantic write them.
type object []member

type member struct {
	name  string
	value any
}

func (o object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(f.name)
		v, err := json.Marshal(f.value)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// layerMessage is the plain map a layer carries for a message, never camelized.
func layerMessage(m Message) (map[string]any, *MessageInfo, error) {
	info, payload, err := payloadOf(m)
	if err != nil {
		return nil, nil, err
	}
	p, err := plain(reflect.ValueOf(payload))
	if err != nil {
		return nil, nil, fmt.Errorf("chanx: %s payload: %w", info.Type, err)
	}
	return map[string]any{"action": info.Action, "payload": p}, info, nil
}

var (
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
)

// plain converts a Go value to the map/slice/scalar tree encoding/json would
// produce, without a JSON round trip: ints stay int64 and floats stay float64,
// so a receiver in any language gets 4 and 2.0 back, not 4.0 and 2.
func plain(v reflect.Value) (any, error) { return toTree(v, false) }

// frameTree is plain for a frame: structs keep their field order and a type
// marshaling itself keeps its exact JSON.
func frameTree(v reflect.Value) (any, error) { return toTree(v, true) }

func toTree(v reflect.Value, frame bool) (any, error) {
	if !v.IsValid() {
		return nil, nil
	}
	t := v.Type()
	if t.Implements(jsonMarshalerType) || t.Implements(textMarshalerType) ||
		(v.CanAddr() && (reflect.PointerTo(t).Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType))) {
		if (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) && v.IsNil() {
			return nil, nil
		}
		if frame {
			data, err := json.Marshal(v.Interface())
			return json.RawMessage(data), err
		}
		return viaJSON(v)
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil, nil
		}
		return toTree(v.Elem(), frame)
	case reflect.Bool:
		return v.Bool(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := v.Uint()
		if u <= math.MaxInt64 {
			return int64(u), nil
		}
		return u, nil
	case reflect.Float32, reflect.Float64:
		return v.Float(), nil
	case reflect.String:
		return v.String(), nil
	case reflect.Slice:
		if v.IsNil() {
			return nil, nil
		}
		if t.Elem().Kind() == reflect.Uint8 {
			if frame {
				data, err := json.Marshal(v.Interface())
				return json.RawMessage(data), err
			}
			return viaJSON(v) // base64, as encoding/json
		}
		fallthrough
	case reflect.Array:
		out := make([]any, v.Len())
		for i := range out {
			e, err := toTree(v.Index(i), frame)
			if err != nil {
				return nil, err
			}
			out[i] = e
		}
		return out, nil
	case reflect.Map:
		if v.IsNil() {
			return nil, nil
		}
		out := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			key, ok := mapKey(iter.Key())
			if !ok {
				return viaJSON(v)
			}
			e, err := toTree(iter.Value(), frame)
			if err != nil {
				return nil, err
			}
			out[key] = e
		}
		return out, nil
	case reflect.Struct:
		var fields object
		for _, f := range schema.Fields(t) {
			_, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
			fv, ok := fieldByIndex(v, f.Index)
			if !ok {
				continue
			}
			if strings.Contains(","+opts+",", ",omitempty,") && isEmpty(fv) {
				continue
			}
			if strings.Contains(","+opts+",", ",omitzero,") && fv.IsZero() {
				continue
			}
			if strings.Contains(","+opts+",", ",string,") {
				if s, ok := quoted(fv); ok {
					fields = append(fields, member{f.Name, s})
					continue
				}
			}
			e, err := toTree(fv, frame)
			if err != nil {
				return nil, err
			}
			fields = append(fields, member{f.Name, e})
		}
		if frame {
			return fields, nil
		}
		out := make(map[string]any, len(fields))
		for _, f := range fields {
			out[f.name] = f.value
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported type %s", t)
}

// mapKey renders a map key as encoding/json does for string and integer keys.
func mapKey(k reflect.Value) (string, bool) {
	switch k.Kind() {
	case reflect.String:
		return k.String(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(k.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(k.Uint(), 10), true
	}
	return "", false
}

// quoted applies the ",string" option: a number or bool written as a string, a
// string written as its own JSON encoding.
func quoted(v reflect.Value) (string, bool) {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return "", false
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.String:
		data, err := json.Marshal(v.Interface())
		return string(data), err == nil
	}
	return "", false
}

func fieldByIndex(v reflect.Value, index []int) (reflect.Value, bool) {
	for i, x := range index {
		if i > 0 && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return reflect.Value{}, false
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v, true
}

func isEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Interface, reflect.Pointer:
		return v.IsZero()
	}
	return false
}

func viaJSON(v reflect.Value) (any, error) {
	data, err := json.Marshal(v.Interface())
	if err != nil {
		return nil, err
	}
	g, err := decodeGeneric(data)
	if err != nil {
		return nil, err
	}
	return layers.Normalize(g), nil
}
