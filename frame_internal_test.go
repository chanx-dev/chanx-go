package chanx

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

type inner struct {
	N int `json:"n"`
}

type Embedded struct {
	E string `json:"e"`
}

type sample struct {
	Embedded
	Int      int              `json:"int"`
	Uint     uint64           `json:"uint"`
	Float    float64          `json:"float"`
	Str      string           `json:"str"`
	Bool     bool             `json:"bool"`
	Ptr      *inner           `json:"ptr"`
	NilPtr   *inner           `json:"nil_ptr"`
	Skip     string           `json:"-"`
	Empty    string           `json:"empty,omitempty"`
	Zero     inner            `json:"zero,omitzero"`
	List     []int            `json:"list"`
	NilList  []int            `json:"nil_list"`
	Arr      [2]string        `json:"arr"`
	Map      map[string]inner `json:"map"`
	IntMap   map[int]string   `json:"int_map"`
	Bytes    []byte           `json:"bytes"`
	Time     time.Time        `json:"time"`
	Iface    any              `json:"iface"`
	AsString int              `json:"as_string,string"`
	unexport int
}

// plain must give the values encoding/json would, with ints kept apart from
// floats, so a receiver decoding the layer message sees 4 and 2.0, not 4.0 and 2.
func TestPlainKeepsNumberKinds(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got, err := plain(reflect.ValueOf(sample{
		Embedded: Embedded{E: "e"}, Int: 4, Uint: 7, Float: 2.0, Str: "s", Bool: true,
		Ptr: &inner{N: 1}, List: []int{1}, Arr: [2]string{"a", "b"},
		Map: map[string]inner{"k": {N: 2}}, IntMap: map[int]string{3: "x"},
		Bytes: []byte("hi"), Time: ts, Iface: 1.5, unexport: 1,
	}))
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	want := map[string]any{
		"e": "e", "int": int64(4), "uint": int64(7), "float": 2.0, "str": "s", "bool": true,
		"ptr": map[string]any{"n": int64(1)}, "nil_ptr": nil, "list": []any{int64(1)},
		"nil_list": nil, "arr": []any{"a", "b"}, "map": map[string]any{"k": map[string]any{"n": int64(2)}},
		"int_map": map[string]any{"3": "x"}, "bytes": "aGk=", "time": "2026-01-02T03:04:05Z",
		"iface": 1.5, "as_string": "0",
	}
	for k, v := range want {
		if !reflect.DeepEqual(m[k], v) {
			t.Errorf("%s: got %T %v, want %T %v", k, m[k], m[k], v, v)
		}
	}
	for _, k := range []string{"Skip", "-", "empty", "zero", "unexport"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s should be left out", k)
		}
	}
	if f, ok := m["float"].(float64); !ok || f != 2.0 {
		t.Errorf("float kind lost: %T", m["float"])
	}

	// Same JSON values as encoding/json gives, number kinds aside.
	value := sample{Int: 4, Float: 2.5, AsString: 9, Map: map[string]inner{"k": {N: 2}}, IntMap: map[int]string{3: "x"}, Time: ts}
	viaPlain, _ := plain(reflect.ValueOf(value))
	a, _ := json.Marshal(viaPlain)
	b, _ := json.Marshal(value)
	var x, y any
	_ = json.Unmarshal(a, &x)
	_ = json.Unmarshal(b, &y)
	if !reflect.DeepEqual(x, y) {
		t.Errorf("plain differs from encoding/json:\n plain %s\n  json %s", a, b)
	}
}

func TestJSONFloatsKeepDecimalPoint(t *testing.T) {
	data, err := encodeRaw(map[string]any{"a": 2.0, "b": []any{3.0, 1.5, 1.5e17, -3e16}, "c": int64(4)}, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"a":2.0,"b":[3.0,1.5,1.5e+17,-3e+16],"c":4}` { // as Python's json.dumps
		t.Fatal(string(data))
	}
}

type quote struct {
	Symbol string          `json:"symbol"`
	Price  float64         `json:"price"`
	Ratio  float32         `json:"ratio"`
	Count  int             `json:"count"`
	Extra  map[string]any  `json:"extra"`
	At     time.Time       `json:"at"`
	Raw    json.RawMessage `json:"raw"`
	Inner  inner           `json:"inner"`
}

// A frame keeps its floats a float (100.0, not 100) as Pydantic writes them, and
// its fields in declaration order; a type marshaling itself keeps its own JSON.
func TestEncodeFrameFloats(t *testing.T) {
	q := quote{
		Symbol: "ACME", Price: 100, Ratio: 0.5, Count: 3,
		Extra: map[string]any{"b": 2.0, "a": []any{1.0, 2}},
		At:    time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC),
		Raw:   json.RawMessage(`{"z":1,"y":2}`),
		Inner: inner{N: 7},
	}
	want := `{"action":"quote","payload":{"symbol":"ACME","price":100.0,"ratio":0.5,"count":3,` +
		`"extra":{"a":[1.0,2],"b":2.0},"at":"2026-10-25T09:00:00Z","raw":{"z":1,"y":2},"inner":{"n":7}}}`
	got, err := encodeFrame("quote", q, stamp{})
	if err != nil || string(got) != want {
		t.Fatalf("got  %s (%v)\nwant %s", got, err, want)
	}

	camel, err := encodeFrame("quote", struct {
		RoomName string  `json:"room_name"`
		Price    float64 `json:"price"`
	}{"lobby", 2}, stamp{camel: true})
	if err != nil || string(camel) != `{"action":"quote","payload":{"price":2.0,"roomName":"lobby"}}` {
		t.Fatalf("camel: %s (%v)", camel, err)
	}

	for payload, want := range map[any]string{nil: "null", 4.0: "4.0", 2.5: "2.5", "x": `"x"`, 7: "7"} {
		got, err := encodeFrame("x", payload, stamp{})
		if err != nil || string(got) != `{"action":"x","payload":`+want+`}` {
			t.Errorf("%v: %s (%v)", payload, got, err)
		}
	}
	got, _ = encodeFrame("x", []float64{1, 2.5}, stamp{})
	if string(got) != `{"action":"x","payload":[1.0,2.5]}` {
		t.Errorf("slice: %s", got)
	}
}
