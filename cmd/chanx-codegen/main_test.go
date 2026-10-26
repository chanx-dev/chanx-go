package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRun(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sandboxgen")
	if err := run([]string{"-o", out, "../../examples/sandbox/testdata/python_asyncapi.json"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"messages.go", "channels.go"} {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil || len(b) == 0 {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for _, args := range [][]string{{}, {"a", "b"}, {"-o", out, "missing.json"}, {"-o", filepath.Join(out, "bad-name"), "../../examples/sandbox/testdata/python_asyncapi.json"}} {
		if err := run(args); err == nil {
			t.Errorf("%v: no error", args)
		}
	}
}
