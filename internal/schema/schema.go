// Package schema infers JSON Schemas from Go types in the shape chanx documents
// use (titles, $refs to named models, anyOf for nullable fields, defaults),
// validates instances against them and reports problems as the protocol's
// validation issues. The same schema validates frames and documents them.
package schema

import (
	"encoding"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/jsonschema-go/jsonschema"
)

// Hook lets a type adjust its inferred schema: constraints Go tags cannot
// express (enum, minLength, format...). The adjusted schema is also the one that
// validates.
type Hook interface {
	JSONSchema(s *jsonschema.Schema)
}

var (
	hookType     = reflect.TypeFor[Hook]()
	timeType     = reflect.TypeFor[time.Time]()
	rawType      = reflect.TypeFor[json.RawMessage]()
	marshalerTyp = reflect.TypeFor[json.Marshaler]()
	textMarshTyp = reflect.TypeFor[encoding.TextMarshaler]()
	bytesType    = reflect.TypeFor[[]byte]()
)

// Generator builds schemas, collecting named types as definitions.
type Generator struct {
	// RefPrefix is prepended to definition names in $ref: "#/$defs/" or
	// "#/components/schemas/".
	RefPrefix string
	// Defs holds the definitions by name; Order lists them as first seen.
	Defs  map[string]*jsonschema.Schema
	Order []string
	// Rename resolves a name already taken by another type; nil appends a counter.
	Rename func(t reflect.Type, name string) string

	names map[reflect.Type]string
	taken map[string]reflect.Type
}

// NewGenerator returns a generator writing refs under refPrefix.
func NewGenerator(refPrefix string) *Generator {
	return &Generator{
		RefPrefix: refPrefix,
		Defs:      map[string]*jsonschema.Schema{},
		names:     map[reflect.Type]string{},
		taken:     map[string]reflect.Type{},
	}
}

// Reserve claims a definition name for t before any type is lifted, so a
// lifted type of the same name gets renamed instead.
func (g *Generator) Reserve(name string, t reflect.Type) {
	if _, ok := g.taken[name]; !ok {
		g.taken[name] = t
	}
}

// NameOf returns the definition name a type got, if it was lifted.
func (g *Generator) NameOf(t reflect.Type) (string, bool) {
	n, ok := g.names[t]
	return n, ok
}

// For returns the schema of t, referencing definitions for named structs and
// named types with a Hook.
func (g *Generator) For(t reflect.Type) *jsonschema.Schema {
	if t.Kind() == reflect.Pointer {
		for t.Elem().Kind() == reflect.Pointer { // encoding/json reads **T as *T
			t = t.Elem()
		}
		inner := g.For(t.Elem())
		if isAny(inner) {
			return inner
		}
		return &jsonschema.Schema{AnyOf: []*jsonschema.Schema{inner, {Type: "null"}}}
	}
	if g.lifted(t) {
		return &jsonschema.Schema{Ref: g.RefPrefix + g.define(t)}
	}
	return g.inline(t)
}

// Define returns t's own schema as a definition body (title included), lifting
// it when it is named.
func (g *Generator) Define(t reflect.Type) (string, *jsonschema.Schema) {
	name := g.define(t)
	return name, g.Defs[name]
}

func (g *Generator) lifted(t reflect.Type) bool {
	if t.Name() == "" || t == timeType {
		return false
	}
	if t.Kind() == reflect.Struct {
		return !opaque(t)
	}
	return implements(t, hookType)
}

func implements(t, iface reflect.Type) bool {
	return t.Implements(iface) || reflect.PointerTo(t).Implements(iface)
}

// opaque types marshal themselves; without a Hook their shape is unknown.
func opaque(t reflect.Type) bool {
	if t == timeType || implements(t, hookType) {
		return false
	}
	return implements(t, marshalerTyp) || implements(t, textMarshTyp)
}

func (g *Generator) define(t reflect.Type) string {
	if name, ok := g.names[t]; ok {
		return name
	}
	name := t.Name()
	if prev, ok := g.taken[name]; ok && prev != t {
		if g.Rename != nil {
			name = g.Rename(t, name)
		}
		for i := 2; g.taken[name] != nil; i++ {
			name = t.Name() + itoa(i)
		}
	}
	g.names[t] = name
	g.taken[name] = t
	g.Order = append(g.Order, name)
	s := &jsonschema.Schema{} // placeholder first, so recursive types terminate
	g.Defs[name] = s
	*s = *g.inline(t)
	s.Title = name
	return name
}

func itoa(i int) string {
	b := []byte{}
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func isAny(s *jsonschema.Schema) bool {
	b, _ := json.Marshal(s)
	return string(b) == "true"
}

func (g *Generator) inline(t reflect.Type) *jsonschema.Schema {
	s := g.base(t)
	if implements(t, hookType) {
		callHook(t, s)
	}
	return s
}

func callHook(t reflect.Type, s *jsonschema.Schema) {
	v := reflect.New(t)
	if h, ok := v.Interface().(Hook); ok {
		h.JSONSchema(s)
		return
	}
	v.Elem().Interface().(Hook).JSONSchema(s)
}

func (g *Generator) base(t reflect.Type) *jsonschema.Schema {
	switch {
	case t == timeType:
		return &jsonschema.Schema{Type: "string", Format: "date-time"}
	case t == rawType:
		return &jsonschema.Schema{}
	case t == bytesType:
		return &jsonschema.Schema{Type: "string", Format: "binary"}
	case opaque(t):
		return &jsonschema.Schema{}
	}
	switch t.Kind() {
	case reflect.Bool:
		return &jsonschema.Schema{Type: "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return &jsonschema.Schema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &jsonschema.Schema{Type: "number"}
	case reflect.String:
		return &jsonschema.Schema{Type: "string"}
	case reflect.Interface:
		return &jsonschema.Schema{}
	case reflect.Pointer:
		return g.For(t)
	case reflect.Slice:
		return &jsonschema.Schema{Type: "array", Items: g.For(t.Elem())}
	case reflect.Array:
		n := t.Len()
		return &jsonschema.Schema{Type: "array", Items: g.For(t.Elem()), MinItems: &n, MaxItems: &n}
	case reflect.Map:
		if t.Key().Kind() != reflect.String && !implements(t.Key(), textMarshTyp) {
			return &jsonschema.Schema{Type: "object"}
		}
		return &jsonschema.Schema{Type: "object", AdditionalProperties: g.For(t.Elem())}
	case reflect.Struct:
		return g.object(t)
	}
	return &jsonschema.Schema{} // chan, func: not JSON; encoding/json reports it
}

// Field is a JSON-visible struct field, with embedded structs flattened as
// encoding/json does.
type Field struct {
	Name     string // JSON name
	Index    []int
	Type     reflect.Type
	Tag      reflect.StructTag
	Optional bool // omitempty, omitzero or a default
	Default  string
	HasDef   bool
}

// Fields lists t's JSON fields in declaration order. When several fields share
// a JSON name, encoding/json's rule picks one: the shallowest, then the only
// tagged one at that depth; a tie drops the name.
func Fields(t reflect.Type) []Field {
	type candidate struct {
		Field
		depth  int
		tagged bool
	}
	var all []candidate
	var walk func(t reflect.Type, index []int)
	walk = func(t reflect.Type, index []int) {
		for i := range t.NumField() {
			f := t.Field(i)
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, opts, _ := strings.Cut(tag, ",")
			idx := append(slices.Clone(index), i)
			ft := f.Type
			if f.Anonymous && name == "" {
				et := ft
				if et.Kind() == reflect.Pointer {
					et = et.Elem()
				}
				if et.Kind() == reflect.Struct {
					walk(et, idx)
					continue
				}
			}
			if !f.IsExported() {
				continue
			}
			tagged := name != ""
			if !tagged {
				name = f.Name
			}
			def, hasDef := f.Tag.Lookup("default")
			optional := hasDef || strings.Contains(","+opts+",", ",omitempty,") || strings.Contains(","+opts+",", ",omitzero,")
			all = append(all, candidate{
				Field:  Field{Name: name, Index: idx, Type: ft, Tag: f.Tag, Optional: optional, Default: def, HasDef: hasDef},
				depth:  len(index),
				tagged: tagged,
			})
		}
	}
	walk(t, nil)

	byName := map[string][]int{}
	for i, c := range all {
		byName[c.Name] = append(byName[c.Name], i)
	}
	winner := map[string]int{} // JSON name to its index in all, -1 when dropped
	for name, idxs := range byName {
		minDepth := all[idxs[0]].depth
		for _, i := range idxs {
			minDepth = min(minDepth, all[i].depth)
		}
		var shallow, tagged []int
		for _, i := range idxs {
			if all[i].depth == minDepth {
				shallow = append(shallow, i)
				if all[i].tagged {
					tagged = append(tagged, i)
				}
			}
		}
		switch {
		case len(shallow) == 1:
			winner[name] = shallow[0]
		case len(tagged) == 1:
			winner[name] = tagged[0]
		default:
			winner[name] = -1
		}
	}
	var out []Field
	for i, c := range all {
		if winner[c.Name] == i {
			out = append(out, c.Field)
		}
	}
	return out
}

func (g *Generator) object(t reflect.Type) *jsonschema.Schema {
	s := &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{}}
	for _, f := range Fields(t) {
		prop := g.For(f.Type)
		if prop.Ref != "" {
			prop = &jsonschema.Schema{Ref: prop.Ref} // a reference carries no title
		} else {
			cp := *prop
			prop = &cp
			prop.Title = Title(f.Name)
		}
		if d := f.Tag.Get("description"); d != "" && prop.Ref == "" {
			prop.Description = d
		} else if d := f.Tag.Get("jsonschema"); d != "" && prop.Ref == "" {
			prop.Description = d
		}
		switch {
		case f.HasDef:
			prop.Default = DefaultJSON(f.Type, f.Default)
		case f.Optional && nullable(f.Type):
			prop.Default = json.RawMessage("null")
		}
		s.Properties[f.Name] = prop
		s.PropertyOrder = append(s.PropertyOrder, f.Name)
		if !f.Optional {
			s.Required = append(s.Required, f.Name)
		}
	}
	return s
}

func nullable(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Interface:
		return true
	}
	return false
}

// DefaultJSON renders a `default` tag as JSON: strings as given, anything else
// parsed as JSON ("0", "true", "[]", "null").
func DefaultJSON(t reflect.Type, def string) json.RawMessage {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.String {
		b, _ := json.Marshal(def)
		return b
	}
	if !json.Valid([]byte(def)) {
		b, _ := json.Marshal(def)
		return b
	}
	return json.RawMessage(def)
}

// Title is the default field title: underscores to spaces, then each word
// capitalized, a letter after a digit too ("room_name" -> "Room Name").
func Title(name string) string {
	r := []rune(strings.ReplaceAll(name, "_", " "))
	prevCased := false
	for i, c := range r {
		if unicode.IsLetter(c) {
			if prevCased {
				r[i] = unicode.ToLower(c)
			} else {
				r[i] = unicode.ToUpper(c)
			}
			prevCased = true
		} else {
			prevCased = false
		}
	}
	return string(r)
}
