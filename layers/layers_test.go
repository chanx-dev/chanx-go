package layers_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"go.chanx.dev/layers/v2"
)

func TestInt(t *testing.T) {
	for _, c := range []struct {
		in   any
		want int64
		ok   bool
	}{
		{int64(5), 5, true}, {uint64(5), 5, true}, {7, 7, true}, {json.Number("9"), 9, true},
		{3.0, 3, true}, {3.5, 0, false}, {"3", 0, false}, {nil, 0, false}, {uint64(1) << 63, 0, false},
		{int8(-2), -2, true}, {uint16(9), 9, true}, {float32(4), 4, true}, {float32(4.5), 0, false},
		{seq(11), 11, true}, {json.Number("1.5"), 0, false}, {true, 0, false},
	} {
		got, ok := layers.Int(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("Int(%#v) = %v, %v", c.in, got, ok)
		}
	}
}

type seq int

func TestNormalize(t *testing.T) {
	got := layers.Normalize(map[any]any{
		"i8": int8(-1), "u32": uint32(7), "u64": uint64(1) << 63, "f32": float32(0.5), "n": json.Number("2"),
		"nf": json.Number("2.5"), "named": seq(3), "s": "x", "b": true, "nil": nil,
		"list": []any{uint8(1), map[string]any{"x": int16(2)}},
	})
	want := map[string]any{
		"i8": int64(-1), "u32": int64(7), "u64": uint64(1) << 63, "f32": 0.5, "n": int64(2),
		"nf": 2.5, "named": int64(3), "s": "x", "b": true, "nil": nil,
		"list": []any{int64(1), map[string]any{"x": int64(2)}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("\n got %#v\nwant %#v", got, want)
	}
}

func TestNames(t *testing.T) {
	if err := layers.ValidGroupName("room.lobby-1_x"); err != nil {
		t.Error(err)
	}
	if err := layers.ValidChannelName("specific.abc!def"); err != nil {
		t.Error(err)
	}
	for _, bad := range []string{"", "a b", "a!b", strings.Repeat("x", 100)} {
		if err := layers.ValidGroupName(bad); !errors.Is(err, layers.ErrInvalidName) {
			t.Errorf("ValidGroupName(%q) = %v", bad, err)
		}
	}
	if err := layers.ValidChannelName("a!b!c"); err == nil {
		t.Error("two '!' accepted")
	}
}

func TestSet(t *testing.T) {
	if _, err := (layers.Set{}).Get(""); err == nil {
		t.Fatal("empty set returned a layer")
	}
}
