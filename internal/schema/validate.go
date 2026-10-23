package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	santhosh "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// Issue is one validation problem: its type, location and message.
type Issue struct {
	Type string `json:"type"`
	Loc  []any  `json:"loc"`
	Msg  string `json:"msg"`
}

// Validator checks JSON values against the schema of a Go type and decodes them.
type Validator struct {
	t        reflect.Type
	Schema   *jsonschema.Schema // with $defs
	compiled *santhosh.Schema
}

// NewValidator compiles the schema of t.
func NewValidator(t reflect.Type) (*Validator, error) {
	g := NewGenerator("#/$defs/")
	root := g.For(t)
	doc := *root
	if len(g.Defs) > 0 {
		doc.Defs = g.Defs
	}
	raw, err := json.Marshal(&doc)
	if err != nil {
		return nil, err
	}
	inst, err := santhosh.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	c := santhosh.NewCompiler()
	c.DefaultDraft(santhosh.Draft2020)
	if err := c.AddResource("chanx.json", inst); err != nil {
		return nil, err
	}
	compiled, err := c.Compile("chanx.json")
	if err != nil {
		return nil, fmt.Errorf("schema of %s: %w", t, err)
	}
	return &Validator{t: t, Schema: &doc, compiled: compiled}, nil
}

// Decode validates raw and decodes it into a new value of the validator's type,
// filling `default` tags first and running an optional Validate() error method.
// Issue locations start with prefix.
func (v *Validator) Decode(raw []byte, prefix []any) (reflect.Value, []Issue) {
	out := reflect.New(v.t)
	inst, err := santhosh.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return out.Elem(), []Issue{{"json_invalid", slices.Clone(prefix), "Invalid JSON: " + err.Error()}}
	}
	if issues := v.Check(inst, prefix); len(issues) > 0 {
		return out.Elem(), issues
	}
	ApplyDefaults(out)
	data, err := json.Marshal(integral(inst))
	if err == nil {
		err = json.Unmarshal(data, out.Interface())
	}
	if err != nil {
		return out.Elem(), []Issue{{"value_error", slices.Clone(prefix), "Value error, " + err.Error()}}
	}
	if err := callValidate(out); err != nil {
		return out.Elem(), []Issue{{"value_error", slices.Clone(prefix), "Value error, " + err.Error()}}
	}
	return out.Elem(), nil
}

func callValidate(ptr reflect.Value) error {
	if val, ok := ptr.Interface().(interface{ Validate() error }); ok {
		return val.Validate()
	}
	if val, ok := ptr.Elem().Interface().(interface{ Validate() error }); ok {
		return val.Validate()
	}
	return nil
}

// Check validates a decoded instance (json.Number numbers) without decoding it.
func (v *Validator) Check(inst any, prefix []any) []Issue {
	err := v.compiled.Validate(inst)
	if err == nil {
		return nil
	}
	var ve *santhosh.ValidationError
	if !errors.As(err, &ve) {
		return []Issue{{"value_error", slices.Clone(prefix), err.Error()}}
	}
	m := mapper{t: v.t, inst: inst, prefix: prefix}
	issues := m.mapErr(ve)
	m.sort(issues)
	return dedupe(issues)
}

// integral rewrites integral numbers ("1.0", "1e3") as integers: valid for an
// int field, but refused by encoding/json.
func integral(v any) any {
	switch x := v.(type) {
	case json.Number:
		s := string(x)
		if strings.ContainsAny(s, ".eE") {
			if f, err := x.Float64(); err == nil && f == math.Trunc(f) && math.Abs(f) < 1<<53 {
				return json.Number(strconv.FormatInt(int64(f), 10))
			}
		}
		return x
	case map[string]any:
		for k, val := range x {
			x[k] = integral(val)
		}
	case []any:
		for i, val := range x {
			x[i] = integral(val)
		}
	}
	return v
}

type mapper struct {
	t      reflect.Type
	inst   any
	prefix []any
}

func (m mapper) loc(path []string) []any {
	out := slices.Clone(m.prefix)
	at := m.inst
	for _, seg := range path {
		if arr, ok := at.([]any); ok {
			n, _ := strconv.Atoi(seg)
			out = append(out, n)
			if n < len(arr) {
				at = arr[n]
			}
			continue
		}
		out = append(out, seg)
		if obj, ok := at.(map[string]any); ok {
			at = obj[seg]
		}
	}
	return out
}

func (m mapper) valueAt(path []string) any {
	at := m.inst
	for _, seg := range path {
		switch x := at.(type) {
		case []any:
			n, _ := strconv.Atoi(seg)
			if n >= len(x) {
				return nil
			}
			at = x[n]
		case map[string]any:
			at = x[seg]
		default:
			return nil
		}
	}
	return at
}

// typeAt walks the Go type along an instance path.
func (m mapper) typeAt(path []string) reflect.Type {
	t := m.t
	for _, seg := range path {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		switch t.Kind() {
		case reflect.Struct:
			var next reflect.Type
			for _, f := range Fields(t) {
				if f.Name == seg {
					next = f.Type
					break
				}
			}
			if next == nil {
				return nil
			}
			t = next
		case reflect.Slice, reflect.Array, reflect.Map:
			t = t.Elem()
		default:
			return nil
		}
	}
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func (m mapper) mapErr(ve *santhosh.ValidationError) []Issue {
	switch k := ve.ErrorKind.(type) {
	case *kind.Group, *kind.Schema, *kind.Reference, *kind.AllOf:
		var out []Issue
		for _, c := range ve.Causes {
			out = append(out, m.mapErr(c)...)
		}
		return out
	case *kind.AnyOf:
		// A nullable field: report against the non-null branch.
		var rest []*santhosh.ValidationError
		for _, c := range ve.Causes {
			if t, ok := c.ErrorKind.(*kind.Type); ok && slices.Equal(t.Want, []string{"null"}) {
				continue
			}
			rest = append(rest, c)
		}
		if len(rest) == 0 {
			rest = ve.Causes
		}
		var out []Issue
		for _, c := range rest {
			out = append(out, m.mapErr(c)...)
		}
		return out
	case *kind.Required:
		loc := m.loc(ve.InstanceLocation)
		var out []Issue
		for _, name := range k.Missing {
			out = append(out, Issue{"missing", append(slices.Clone(loc), name), "Field required"})
		}
		return out
	case *kind.Type:
		return []Issue{m.typeIssue(ve.InstanceLocation, k.Want)}
	case *kind.Const:
		return []Issue{{"literal_error", m.loc(ve.InstanceLocation), "Input should be " + orList([]any{k.Want})}}
	case *kind.Enum:
		typ := "literal_error"
		if t := m.typeAt(ve.InstanceLocation); t != nil && t.PkgPath() != "" && t.Kind() != reflect.Struct {
			typ = "enum"
		}
		return []Issue{{typ, m.loc(ve.InstanceLocation), "Input should be " + orList(k.Want)}}
	case *kind.MinLength:
		return []Issue{{"string_too_short", m.loc(ve.InstanceLocation), fmt.Sprintf("String should have at least %s", plural(k.Want, "character"))}}
	case *kind.MaxLength:
		return []Issue{{"string_too_long", m.loc(ve.InstanceLocation), fmt.Sprintf("String should have at most %s", plural(k.Want, "character"))}}
	case *kind.Pattern:
		return []Issue{{"string_pattern_mismatch", m.loc(ve.InstanceLocation), fmt.Sprintf("String should match pattern '%s'", k.Want)}}
	case *kind.Minimum:
		return []Issue{{"greater_than_equal", m.loc(ve.InstanceLocation), "Input should be greater than or equal to " + ratText(k.Want)}}
	case *kind.ExclusiveMinimum:
		return []Issue{{"greater_than", m.loc(ve.InstanceLocation), "Input should be greater than " + ratText(k.Want)}}
	case *kind.Maximum:
		return []Issue{{"less_than_equal", m.loc(ve.InstanceLocation), "Input should be less than or equal to " + ratText(k.Want)}}
	case *kind.ExclusiveMaximum:
		return []Issue{{"less_than", m.loc(ve.InstanceLocation), "Input should be less than " + ratText(k.Want)}}
	case *kind.MultipleOf:
		return []Issue{{"multiple_of", m.loc(ve.InstanceLocation), "Input should be a multiple of " + ratText(k.Want)}}
	case *kind.MinItems:
		return []Issue{{"too_short", m.loc(ve.InstanceLocation), fmt.Sprintf("List should have at least %s after validation, not %d", plural(k.Want, "item"), k.Got)}}
	case *kind.MaxItems:
		return []Issue{{"too_long", m.loc(ve.InstanceLocation), fmt.Sprintf("List should have at most %s after validation, not %d", plural(k.Want, "item"), k.Got)}}
	}
	if len(ve.Causes) > 0 {
		var out []Issue
		for _, c := range ve.Causes {
			out = append(out, m.mapErr(c)...)
		}
		return out
	}
	return []Issue{{"value_error", m.loc(ve.InstanceLocation), "Value error, " + strings.TrimSpace(fmt.Sprint(ve.ErrorKind))}}
}

func (m mapper) typeIssue(path []string, want []string) Issue {
	loc := m.loc(path)
	got := m.valueAt(path)
	w := ""
	for _, x := range want {
		if x != "null" {
			w = x
			break
		}
	}
	if w == "" && len(want) > 0 {
		w = want[0]
	}
	switch w {
	case "string":
		return Issue{"string_type", loc, "Input should be a valid string"}
	case "integer":
		switch x := got.(type) {
		case string:
			if _, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64); err != nil {
				return Issue{"int_parsing", loc, "Input should be a valid integer, unable to parse string as an integer"}
			}
		case json.Number:
			if f, err := x.Float64(); err == nil && f != math.Trunc(f) {
				return Issue{"int_from_float", loc, "Input should be a valid integer, got a number with a fractional part"}
			}
		}
		return Issue{"int_type", loc, "Input should be a valid integer"}
	case "number":
		if s, ok := got.(string); ok {
			if _, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err != nil {
				return Issue{"float_parsing", loc, "Input should be a valid number, unable to parse string as a number"}
			}
		}
		return Issue{"float_type", loc, "Input should be a valid number"}
	case "boolean":
		switch x := got.(type) {
		case string:
			if !pyBoolString(x) {
				return Issue{"bool_parsing", loc, "Input should be a valid boolean, unable to interpret input"}
			}
		case json.Number:
			if x != "0" && x != "1" {
				return Issue{"bool_parsing", loc, "Input should be a valid boolean, unable to interpret input"}
			}
		}
		return Issue{"bool_type", loc, "Input should be a valid boolean"}
	case "array":
		return Issue{"list_type", loc, "Input should be a valid list"}
	case "null":
		return Issue{"none_required", loc, "Input should be None"}
	case "object":
		if t := m.typeAt(path); t != nil && t.Kind() == reflect.Struct {
			return Issue{"model_type", loc, "Input should be a valid dictionary or instance of " + t.Name()}
		}
		return Issue{"dict_type", loc, "Input should be a valid dictionary"}
	}
	return Issue{"value_error", loc, "Value error, invalid type"}
}

func pyBoolString(s string) bool {
	switch strings.ToLower(s) {
	case "0", "off", "f", "false", "n", "no", "1", "on", "t", "true", "y", "yes":
		return true
	}
	return false
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func ratText(r *big.Rat) string {
	if r == nil {
		return "?"
	}
	if r.IsInt() {
		return r.Num().String()
	}
	f, _ := r.Float64()
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// orList renders values as 'a', 'b' or 'c'.
func orList(values []any) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = PyRepr(v)
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " or " + parts[len(parts)-1]
}

// PyRepr renders a JSON value as Python's repr would show it.
func PyRepr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case string:
		return pyStr(x)
	case json.Number:
		return pyNumber(string(x))
	case float64:
		return pyNumber(strconv.FormatFloat(x, 'f', -1, 64))
	case int, int64:
		return fmt.Sprint(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = PyRepr(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = pyStr(k) + ": " + PyRepr(x[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return fmt.Sprint(v)
}

func pyStr(s string) string {
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
}

// pyNumber renders a JSON number for PyRepr: ints stay as written, floats keep
// a decimal point.
func pyNumber(s string) string {
	if !strings.ContainsAny(s, ".eE") {
		return s
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	if f == math.Trunc(f) && math.Abs(f) < 1e16 {
		return strconv.FormatFloat(f, 'f', 1, 64)
	}
	out := strconv.FormatFloat(f, 'g', -1, 64)
	if i := strings.Index(out, "e"); i >= 0 && len(out) > i+2 && (out[i+1] == '+' || out[i+1] == '-') && len(out[i+2:]) == 1 {
		out = out[:i+2] + "0" + out[i+2:] // 1e-07, not 1e-7
	}
	return out
}

// sort orders issues by field declaration order.
func (m mapper) sort(issues []Issue) {
	key := func(is Issue) []int {
		var out []int
		t := m.t
		for _, seg := range is.Loc[len(m.prefix):] {
			for t != nil && t.Kind() == reflect.Pointer {
				t = t.Elem()
			}
			switch s := seg.(type) {
			case int:
				out = append(out, s)
				if t != nil && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array) {
					t = t.Elem()
				}
			case string:
				pos := 1 << 20
				var next reflect.Type
				if t != nil && t.Kind() == reflect.Struct {
					for i, f := range Fields(t) {
						if f.Name == s {
							pos, next = i, f.Type
							break
						}
					}
				} else if t != nil && t.Kind() == reflect.Map {
					next = t.Elem()
				}
				out = append(out, pos)
				t = next
			}
		}
		return out
	}
	sort.SliceStable(issues, func(i, j int) bool { return slices.Compare(key(issues[i]), key(issues[j])) < 0 })
}

func dedupe(issues []Issue) []Issue {
	seen := map[string]bool{}
	out := issues[:0]
	for _, is := range issues {
		k := fmt.Sprint(is.Type, is.Loc, is.Msg)
		if !seen[k] {
			seen[k] = true
			out = append(out, is)
		}
	}
	return out
}

// ApplyDefaults fills `default` tags, recursively through nested structs, so
// absent fields keep them when JSON is decoded on top.
func ApplyDefaults(v reflect.Value) {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	for i := range v.NumField() {
		f, fv := v.Type().Field(i), v.Field(i)
		if !fv.CanSet() {
			continue
		}
		if def, ok := f.Tag.Lookup("default"); ok {
			_ = json.Unmarshal(DefaultJSON(f.Type, def), fv.Addr().Interface())
		}
		if fv.Kind() == reflect.Struct {
			ApplyDefaults(fv.Addr())
		}
	}
}
