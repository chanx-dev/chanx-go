package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"

	"go.chanx.dev/v2/asyncapi"
)

// TestAsyncAPIParity compares the document with the one Python chanx generates
// for sandbox_fastapi, recorded in testdata/python_asyncapi.json.
func TestAsyncAPIParity(t *testing.T) {
	set, _ := Layers("")
	e := New(set, nil)
	got, err := asyncapi.JSON(e, Info)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/python_asyncapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var g, w any
	_ = json.Unmarshal(got, &g)
	_ = json.Unmarshal(want, &w)
	var diffs []string
	diff("", g, w, &diffs)
	for _, d := range diffs {
		t.Error(d)
	}
	if len(diffs) > 0 && os.Getenv("WRITE_GO_ASYNCAPI") != "" {
		_ = os.WriteFile("testdata/go_asyncapi.json", got, 0o644)
	}
}

func diff(path string, got, want any, out *[]string) {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: got %v, want an object", path, got))
			return
		}
		keys := map[string]bool{}
		for k := range w {
			keys[k] = true
		}
		for k := range g {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			gv, gok := g[k]
			wv, wok := w[k]
			switch {
			case !gok:
				*out = append(*out, fmt.Sprintf("%s/%s: missing, want %s", path, k, js(wv)))
			case !wok:
				*out = append(*out, fmt.Sprintf("%s/%s: unexpected %s", path, k, js(gv)))
			default:
				diff(path+"/"+k, gv, wv, out)
			}
		}
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			*out = append(*out, fmt.Sprintf("%s: got %s, want %s", path, js(got), js(want)))
			return
		}
		for i := range w {
			diff(fmt.Sprintf("%s[%d]", path, i), g[i], w[i], out)
		}
	default:
		if !reflect.DeepEqual(got, want) {
			*out = append(*out, fmt.Sprintf("%s: got %s, want %s", path, js(got), js(want)))
		}
	}
}

func js(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
