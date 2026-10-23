package schema

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
)

type Sub struct {
	X int `json:"x"`
}

type P struct {
	Message string   `json:"message"`
	N       int      `json:"n" default:"0"`
	Tags    []string `json:"tags,omitempty"`
	Sub     *Sub     `json:"sub,omitempty"`
}

type Status string

func (Status) JSONSchema(s *jsonschema.Schema) { s.Enum = []any{"A", "B", "C"} }

type Constrained struct {
	S   string         `json:"s" default:"xyz"`
	I   int            `json:"i" default:"5"`
	J   int            `json:"j" default:"5"`
	F   float64        `json:"f" default:"1.0"`
	B   bool           `json:"b" default:"true"`
	E   Status         `json:"e" default:"A"`
	Two string         `json:"two" default:"x"`
	L   []int          `json:"l" default:"[1]"`
	D   map[string]any `json:"d" default:"{}"`
	M   float64        `json:"m" default:"1.0"`
}

func (*Constrained) JSONSchema(s *jsonschema.Schema) {
	one, three, five, two := 1, 3, 5, 2
	ge, le, gt, lt, half := 1.0, 10.0, 1.0, 10.0, 0.5
	s.Properties["s"].MinLength, s.Properties["s"].MaxLength, s.Properties["s"].Pattern = &three, &five, "^[a-z]+$"
	s.Properties["i"].Minimum, s.Properties["i"].Maximum = &ge, &le
	s.Properties["j"].ExclusiveMinimum, s.Properties["j"].ExclusiveMaximum = &gt, &lt
	s.Properties["two"].Enum = []any{"x", "y"}
	s.Properties["l"].MinItems, s.Properties["l"].MaxItems = &one, &two
	s.Properties["m"].MultipleOf = &half
}

func issues(t *testing.T, typ reflect.Type, raw string, prefix ...any) []Issue {
	t.Helper()
	v, err := NewValidator(typ)
	if err != nil {
		t.Fatal(err)
	}
	if prefix == nil {
		prefix = []any{}
	}
	_, got := v.Decode([]byte(raw), prefix)
	return got
}

func check(t *testing.T, typ reflect.Type, raw, want string, prefix ...any) {
	t.Helper()
	got, _ := json.Marshal(issues(t, typ, raw, prefix...))
	if got := string(got); got != want && (want != "" || got != "null") {
		t.Errorf("%s\n got: %s\nwant: %s", raw, got, want)
	}
}

// Expected values recorded from Pydantic 2.
func TestPydanticErrors(t *testing.T) {
	p := reflect.TypeFor[P]()
	check(t, p, `{}`, `[{"type":"missing","loc":["payload","message"],"msg":"Field required"}]`, "payload")
	check(t, p, `null`, `[{"type":"model_type","loc":["chat","payload"],"msg":"Input should be a valid dictionary or instance of P"}]`, "chat", "payload")
	check(t, p, `"x"`, `[{"type":"model_type","loc":["payload"],"msg":"Input should be a valid dictionary or instance of P"}]`, "payload")
	check(t, p, `{"message":1,"n":"x","tags":"a"}`,
		`[{"type":"string_type","loc":["message"],"msg":"Input should be a valid string"},`+
			`{"type":"int_parsing","loc":["n"],"msg":"Input should be a valid integer, unable to parse string as an integer"},`+
			`{"type":"list_type","loc":["tags"],"msg":"Input should be a valid list"}]`)
	check(t, p, `{"message":"a","n":1.5,"tags":[1]}`,
		`[{"type":"int_from_float","loc":["n"],"msg":"Input should be a valid integer, got a number with a fractional part"},`+
			`{"type":"string_type","loc":["tags",0],"msg":"Input should be a valid string"}]`)
	check(t, p, `{"message":"a","sub":{"x":"q"}}`, `[{"type":"int_parsing","loc":["sub","x"],"msg":"Input should be a valid integer, unable to parse string as an integer"}]`)
	check(t, p, `{"message":"a","sub":5}`, `[{"type":"model_type","loc":["sub"],"msg":"Input should be a valid dictionary or instance of Sub"}]`)
	check(t, p, `{"message":"a","n":2.0}`, ``)

	c := reflect.TypeFor[Constrained]()
	for raw, want := range map[string]string{
		`{"s":"ab"}`:        `[{"type":"string_too_short","loc":["s"],"msg":"String should have at least 3 characters"}]`,
		`{"s":"abcdef"}`:    `[{"type":"string_too_long","loc":["s"],"msg":"String should have at most 5 characters"}]`,
		`{"s":"AB1"}`:       `[{"type":"string_pattern_mismatch","loc":["s"],"msg":"String should match pattern '^[a-z]+$'"}]`,
		`{"i":0}`:           `[{"type":"greater_than_equal","loc":["i"],"msg":"Input should be greater than or equal to 1"}]`,
		`{"i":11}`:          `[{"type":"less_than_equal","loc":["i"],"msg":"Input should be less than or equal to 10"}]`,
		`{"j":1}`:           `[{"type":"greater_than","loc":["j"],"msg":"Input should be greater than 1"}]`,
		`{"j":10}`:          `[{"type":"less_than","loc":["j"],"msg":"Input should be less than 10"}]`,
		`{"f":"x"}`:         `[{"type":"float_parsing","loc":["f"],"msg":"Input should be a valid number, unable to parse string as a number"}]`,
		`{"b":"x"}`:         `[{"type":"bool_parsing","loc":["b"],"msg":"Input should be a valid boolean, unable to interpret input"}]`,
		`{"b":2}`:           `[{"type":"bool_parsing","loc":["b"],"msg":"Input should be a valid boolean, unable to interpret input"}]`,
		`{"b":[1]}`:         `[{"type":"bool_type","loc":["b"],"msg":"Input should be a valid boolean"}]`,
		`{"e":"Z"}`:         `[{"type":"enum","loc":["e"],"msg":"Input should be 'A', 'B' or 'C'"}]`,
		`{"two":"z"}`:       `[{"type":"literal_error","loc":["two"],"msg":"Input should be 'x' or 'y'"}]`,
		`{"l":[]}`:          `[{"type":"too_short","loc":["l"],"msg":"List should have at least 1 item after validation, not 0"}]`,
		`{"l":[1,2,3]}`:     `[{"type":"too_long","loc":["l"],"msg":"List should have at most 2 items after validation, not 3"}]`,
		`{"d":1}`:           `[{"type":"dict_type","loc":["d"],"msg":"Input should be a valid dictionary"}]`,
		`{"d":[1]}`:         `[{"type":"dict_type","loc":["d"],"msg":"Input should be a valid dictionary"}]`,
		`{"m":0.3}`:         `[{"type":"multiple_of","loc":["m"],"msg":"Input should be a multiple of 0.5"}]`,
		`{"i":null}`:        `[{"type":"int_type","loc":["i"],"msg":"Input should be a valid integer"}]`,
		`{"s":null}`:        `[{"type":"string_type","loc":["s"],"msg":"Input should be a valid string"}]`,
		`{"i":"1.5"}`:       `[{"type":"int_parsing","loc":["i"],"msg":"Input should be a valid integer, unable to parse string as an integer"}]`,
		`{"l":null}`:        `[{"type":"list_type","loc":["l"],"msg":"Input should be a valid list"}]`,
		`{"s":1.5}`:         `[{"type":"string_type","loc":["s"],"msg":"Input should be a valid string"}]`,
		`{"f":null}`:        `[{"type":"float_type","loc":["f"],"msg":"Input should be a valid number"}]`,
		`{"s":"abc","i":3}`: ``,
	} {
		check(t, c, raw, want)
	}
}

func TestDefaults(t *testing.T) {
	v, err := NewValidator(reflect.TypeFor[Constrained]())
	if err != nil {
		t.Fatal(err)
	}
	got, issues := v.Decode([]byte(`{"i":7}`), nil)
	if issues != nil {
		t.Fatal(issues)
	}
	c := got.Interface().(Constrained)
	if c.S != "xyz" || c.I != 7 || c.F != 1.0 || !c.B || c.E != "A" || len(c.L) != 1 || c.D == nil {
		t.Fatalf("defaults not applied: %+v", c)
	}
}

type Room struct {
	Message  string  `json:"message"`
	RoomName *string `json:"room_name,omitempty"`
	Status   Status  `json:"status" default:"A"`
	Data     any     `json:"data,omitempty"`
	Items    []Sub   `json:"items,omitempty"`
}

func TestPydanticShape(t *testing.T) {
	g := NewGenerator("#/components/schemas/")
	name, s := g.Define(reflect.TypeFor[Room]())
	if name != "Room" {
		t.Fatal(name)
	}
	gotRaw, _ := json.Marshal(s)
	want := `{"type":"object","title":"Room","required":["message"],"properties":{` +
		`"message":{"type":"string","title":"Message"},` +
		`"room_name":{"title":"Room Name","default":null,"anyOf":[{"type":"string"},{"type":"null"}]},` +
		`"status":{"$ref":"#/components/schemas/Status","default":"A"},` +
		`"data":{"title":"Data","default":null},` +
		`"items":{"type":"array","title":"Items","items":{"$ref":"#/components/schemas/Sub"}}}}`
	var got, exp any
	_ = json.Unmarshal(gotRaw, &got)
	_ = json.Unmarshal([]byte(want), &exp)
	if !reflect.DeepEqual(got, exp) {
		t.Fatalf("\n got %s\nwant %s", gotRaw, want)
	}
	st, _ := json.Marshal(g.Defs["Status"])
	if string(st) != `{"type":"string","title":"Status","enum":["A","B","C"]}` {
		t.Fatal(string(st))
	}
}

func TestTitle(t *testing.T) {
	for in, want := range map[string]string{"room_name": "Room Name", "roomName": "Roomname", "x1y": "X1Y", "status_code": "Status Code"} {
		if got := Title(in); got != want {
			t.Errorf("Title(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPyRepr(t *testing.T) {
	for in, want := range map[string]string{`5`: `5`, `1.5`: `1.5`, `1.0`: `1.0`, `null`: `None`, `true`: `True`, `[1,"a"]`: `[1, 'a']`, `{"a":1}`: `{'a': 1}`, `"it's"`: `"it's"`} {
		var v any
		d := json.NewDecoder(stringsReader(in))
		d.UseNumber()
		_ = d.Decode(&v)
		if got := PyRepr(v); got != want {
			t.Errorf("PyRepr(%s) = %s, want %s", in, got, want)
		}
	}
}

type Opaque struct{ v int }

func (Opaque) MarshalJSON() ([]byte, error) { return []byte(`"x"`), nil }

func (o *Opaque) UnmarshalJSON([]byte) error { o.v = 1; return nil }

type Kinds struct {
	When    time.Time       `json:"when"`
	Raw     json.RawMessage `json:"raw"`
	Bytes   []byte          `json:"bytes"`
	Pair    [2]int          `json:"pair"`
	IntKeys map[int]string  `json:"int_keys"`
	Small   uint8           `json:"small"`
	Ratio   float32         `json:"ratio"`
	Custom  Opaque          `json:"custom"`
	Any     any             `json:"any"`
	Self    *Kinds          `json:"self,omitempty"`
	Skip    string          `json:"-"`
	NoTag   string
}

func TestKinds(t *testing.T) {
	g := NewGenerator("#/$defs/")
	_, s := g.Define(reflect.TypeFor[Kinds]())
	raw, _ := json.Marshal(s)
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	props := got["properties"].(map[string]any)
	check := func(name, want string) {
		t.Helper()
		b, _ := json.Marshal(props[name])
		if string(b) != want {
			t.Errorf("%s: got %s, want %s", name, b, want)
		}
	}
	check("when", `{"format":"date-time","title":"When","type":"string"}`)
	check("raw", `{"title":"Raw"}`)
	check("bytes", `{"format":"binary","title":"Bytes","type":"string"}`)
	check("pair", `{"items":{"type":"integer"},"maxItems":2,"minItems":2,"title":"Pair","type":"array"}`)
	check("int_keys", `{"title":"Int Keys","type":"object"}`)
	check("small", `{"title":"Small","type":"integer"}`)
	check("ratio", `{"title":"Ratio","type":"number"}`)
	check("custom", `{"title":"Custom"}`)
	check("self", `{"anyOf":[{"$ref":"#/$defs/Kinds"},{"type":"null"}],"default":null,"title":"Self"}`)
	check("NoTag", `{"title":"Notag","type":"string"}`)
	if _, ok := props["Skip"]; ok {
		t.Error(`json:"-" field documented`)
	}
	if name, ok := g.NameOf(reflect.TypeFor[Kinds]()); !ok || name != "Kinds" {
		t.Error(name)
	}
	// Recursive types validate too.
	v, err := NewValidator(reflect.TypeFor[Kinds]())
	if err != nil {
		t.Fatal(err)
	}
	if _, issues := v.Decode([]byte(`{"when":"2026-01-01T00:00:00Z","raw":1,"bytes":"aGk=","pair":[1,2],"int_keys":{},"small":1,"ratio":1.5,"custom":"x","any":null,"NoTag":"n","self":{"when":"2026-01-01T00:00:00Z","raw":1,"bytes":"","pair":[1,2],"int_keys":{},"small":1,"ratio":1,"custom":"x","any":1,"NoTag":"n"}}`), nil); issues != nil {
		t.Fatal(issues)
	}
}

type ByValue struct {
	N int `json:"n"`
}

func (v ByValue) Validate() error {
	if v.N < 0 {
		return errors.New("n must not be negative")
	}
	return nil
}

type ByPointer struct {
	N int `json:"n"`
}

func (v *ByPointer) Validate() error {
	if v.N > 9 {
		return errors.New("n must be a digit")
	}
	return nil
}

// Validate runs whether it has a value or a pointer receiver.
func TestValidateHook(t *testing.T) {
	check(t, reflect.TypeFor[ByValue](), `{"n":-1}`, `[{"type":"value_error","loc":[],"msg":"Value error, n must not be negative"}]`)
	check(t, reflect.TypeFor[ByValue](), `{"n":1}`, ``)
	check(t, reflect.TypeFor[ByPointer](), `{"n":10}`, `[{"type":"value_error","loc":[],"msg":"Value error, n must be a digit"}]`)
	check(t, reflect.TypeFor[ByPointer](), `{"n":1}`, ``)
}

type Inner struct {
	Name  int `json:"name"`
	Plain string
	Pick  string `json:"Kept"`
}

type OtherInner struct {
	Plain string
	Kept  int
}

type Shadowing struct {
	Inner
	OtherInner
	Name string `json:"name"`
}

// Fields follows encoding/json when names collide: the outer "name" wins, the
// tagged "Kept" beats the untagged one, and the tie on "Plain" drops it.
func TestShadowing(t *testing.T) {
	var got []string
	for _, f := range Fields(reflect.TypeFor[Shadowing]()) {
		got = append(got, f.Name)
	}
	raw, _ := json.Marshal(Shadowing{})
	var want map[string]any
	_ = json.Unmarshal(raw, &want)
	if len(got) != len(want) {
		t.Fatalf("fields %v, encoding/json %s", got, raw)
	}
	for _, name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("fields %v, encoding/json %s", got, raw)
		}
	}
	check(t, reflect.TypeFor[Shadowing](), `{"name":"x","Kept":"k"}`, "")
}

type DoublePointer struct {
	P **Sub `json:"p"`
}

// **T is read as *T, as encoding/json does.
func TestDoublePointer(t *testing.T) {
	check(t, reflect.TypeFor[DoublePointer](), `{"p":null}`, "")
	check(t, reflect.TypeFor[DoublePointer](), `{"p":{"x":1}}`, "")
	check(t, reflect.TypeFor[DoublePointer](), `{"p":{"x":"a"}}`, `[{"type":"int_parsing","loc":["p","x"],"msg":"Input should be a valid integer, unable to parse string as an integer"}]`)
}
