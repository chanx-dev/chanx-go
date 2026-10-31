package protocol

import (
	"reflect"
	"testing"
)

func TestJSONPointer(t *testing.T) {
	if got := JSONPointer(nil); got != "" {
		t.Errorf("root: %q", got)
	}
	if got := JSONPointer([]any{"payload", "items", 0, "a/b~c"}); got != "/payload/items/0/a~1b~0c" {
		t.Errorf("escaped: %q", got)
	}
}

// The same cases as chanx's tests/core/test_errors.py.
func TestInstancePathDropsValidatorLabels(t *testing.T) {
	cases := []struct {
		loc     []any
		data    any
		path    []any
		missing any
	}{
		{[]any{"echo", "payload", "text"}, map[string]any{"payload": map[string]any{"text": 1}}, []any{"payload", "text"}, nil},
		{[]any{"echo", "payload", "text"}, map[string]any{"payload": map[string]any{}}, []any{"payload"}, "text"},
		{[]any{"v", "int"}, map[string]any{"v": []any{}}, []any{"v"}, nil},
		{[]any{"s", "Circle", "r"}, map[string]any{"s": map[string]any{"r": "x"}}, []any{"s", "r"}, nil},
		{[]any{"s", "Square", "side"}, map[string]any{"s": map[string]any{"r": "x"}}, []any{"s"}, "side"},
		{[]any{"scores", "a", "[key]"}, map[string]any{"scores": map[string]any{"a": 1}}, []any{"scores", "a"}, nil},
		{[]any{"items", 1, "n"}, map[string]any{"items": []any{map[string]any{}, map[string]any{"n": "x"}}}, []any{"items", 1, "n"}, nil},
	}
	for _, c := range cases {
		path, missing := InstancePath(c.loc, c.data)
		if !reflect.DeepEqual(path, c.path) || missing != c.missing {
			t.Errorf("%v: got %v, %v; want %v, %v", c.loc, path, missing, c.path, c.missing)
		}
	}
}
