// Package vectors reads the golden vectors in testdata. A copy of layers/internal/vectors:
// go.chanx.dev/layers/redis/v2 is not under go.chanx.dev/layers/v2, so it cannot import that one.
package vectors

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// Load decodes a vector file into v.
func Load(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(v)
}

// Value turns the typed JSON the generator writes back into Go values: {"$i"} is
// an int64, {"$f"} a float64, {"$b"} bytes.
func Value(v any) any {
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 1 {
			if s, ok := x["$i"].(string); ok {
				i, _ := strconv.ParseInt(s, 10, 64)
				return i
			}
			if n, ok := x["$f"].(json.Number); ok {
				f, _ := n.Float64()
				return f
			}
			if s, ok := x["$b"].(string); ok {
				b, _ := base64.StdEncoding.DecodeString(s)
				return b
			}
		}
		out := map[string]any{}
		for k, val := range x {
			out[k] = Value(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = Value(val)
		}
		return out
	}
	return v
}

// Diff compares two decoded values keeping ints, floats and bytes distinct.
func Diff(path string, got, want any) string {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok || len(g) != len(w) {
			return fmt.Sprintf("%s: got %T %v, want %v", path, got, got, want)
		}
		for k, v := range w {
			if d := Diff(path+"."+k, g[k], v); d != "" {
				return d
			}
		}
		return ""
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return fmt.Sprintf("%s: got %T %v, want %v", path, got, got, want)
		}
		for i := range w {
			if d := Diff(fmt.Sprintf("%s[%d]", path, i), g[i], w[i]); d != "" {
				return d
			}
		}
		return ""
	case []byte:
		g, ok := got.([]byte)
		if !ok || !bytes.Equal(g, w) {
			return fmt.Sprintf("%s: got %T %v, want bytes %v", path, got, got, w)
		}
		return ""
	}
	if got != want {
		return fmt.Sprintf("%s: got %T %v, want %T %v", path, got, got, want, want)
	}
	return ""
}
