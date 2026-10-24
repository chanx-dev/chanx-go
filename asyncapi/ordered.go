package asyncapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/goccy/go-yaml"
)

// Map is a JSON object that keeps its insertion order, in JSON and YAML alike.
type Map struct {
	keys []string
	vals map[string]any
}

// NewMap returns an empty ordered map.
func NewMap() *Map { return &Map{vals: map[string]any{}} }

// Set adds or replaces a key, keeping its first position.
func (m *Map) Set(k string, v any) *Map {
	if _, ok := m.vals[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.vals[k] = v
	return m
}

// Get returns a value.
func (m *Map) Get(k string) (any, bool) {
	v, ok := m.vals[k]
	return v, ok
}

// Has reports whether k is set.
func (m *Map) Has(k string) bool { _, ok := m.vals[k]; return ok }

// Keys returns the keys in order.
func (m *Map) Keys() []string { return append([]string(nil), m.keys...) }

// Len returns the number of keys.
func (m *Map) Len() int { return len(m.keys) }

// Sorted returns a copy with keys sorted.
func (m *Map) Sorted() *Map {
	out := NewMap()
	keys := m.Keys()
	sort.Strings(keys)
	for _, k := range keys {
		out.Set(k, m.vals[k])
	}
	return out
}

func (m *Map) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		b.Write(kb)
		b.WriteByte(':')
		vb, err := marshalValue(m.vals[k])
		if err != nil {
			return nil, err
		}
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// marshalValue writes integral floats as Python does (a default of 1.0 stays
// 1.0, where Go writes 1), as YAML already does.
func marshalValue(v any) ([]byte, error) {
	switch x := v.(type) {
	case float64:
		switch {
		case x != math.Trunc(x) || math.IsInf(x, 0):
		case math.Abs(x) < 1e16:
			return []byte(strconv.FormatFloat(x, 'f', 1, 64)), nil
		default:
			return []byte(strconv.FormatFloat(x, 'e', -1, 64)), nil
		}
	case []any:
		var b bytes.Buffer
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			eb, err := marshalValue(e)
			if err != nil {
				return nil, err
			}
			b.Write(eb)
		}
		b.WriteByte(']')
		return b.Bytes(), nil
	}
	return json.Marshal(v)
}

// MarshalYAML keeps the order in YAML.
func (m *Map) MarshalYAML() (any, error) {
	out := make(yaml.MapSlice, 0, len(m.keys))
	for _, k := range m.keys {
		out = append(out, yaml.MapItem{Key: k, Value: m.vals[k]})
	}
	return out, nil
}

// ordered decodes JSON into Maps and slices, keeping key order.
func ordered(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			m := NewMap()
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, ok := kt.(string)
				if !ok {
					return nil, fmt.Errorf("asyncapi: object key %v", kt)
				}
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				m.Set(k, v)
			}
			_, err := dec.Token()
			return m, err
		case '[':
			out := []any{}
			for dec.More() {
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			}
			_, err := dec.Token()
			return out, err
		}
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i, nil
		}
		f, _ := t.Float64()
		return f, nil
	}
	return tok, nil
}

// toOrdered converts any JSON-able value (schemas included) into Maps.
func toOrdered(v any) (any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return ordered(data)
}
