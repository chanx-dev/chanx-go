package goclient

import (
	"bytes"
	"os"
	"testing"

	"go.chanx.dev/v2/asyncapi"
	"go.chanx.dev/v2/codegen"
)

// asyncapi.json and chatclient/ are what the server documents and what
// chanx-codegen generates from it; UPDATE=1 rewrites the document.
func TestGenerated(t *testing.T) {
	doc, err := asyncapi.JSON(NewEngine(), asyncapi.Info{Title: "Chat", Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("UPDATE") != "" {
		_ = os.WriteFile("asyncapi.json", doc, 0o644)
	}
	if committed, _ := os.ReadFile("asyncapi.json"); !bytes.Equal(committed, doc) {
		t.Fatal("asyncapi.json is stale: UPDATE=1 go test ./snippets/goclient, then go generate ./snippets/goclient")
	}
	files, err := codegen.Generate(doc, codegen.Options{Package: "chatclient"})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range files {
		if got, _ := os.ReadFile("chatclient/" + name); !bytes.Equal(got, want) {
			t.Errorf("chatclient/%s is stale: go generate ./snippets/goclient", name)
		}
	}
}
