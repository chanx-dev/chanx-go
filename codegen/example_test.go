package codegen_test

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"

	chanx "go.chanx.dev"
	"go.chanx.dev/asyncapi"
	"go.chanx.dev/codegen"
)

type ChatMessage struct {
	chanx.BaseMessage `action:"chat"`
	Payload           string `json:"payload"`
}

type ChatConsumer struct{}

func (ChatConsumer) HandleChat(ctx *chanx.Context, m ChatMessage) (ChatMessage, error) {
	return m, nil
}

// Generate reads the document a chanx server serves at /asyncapi.json, Go or Python,
// and returns the client's files. Load fetches the document from a URL or a file.
func ExampleGenerate() {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/chat/{room}", ChatConsumer{})
	doc, err := asyncapi.Generate(engine, asyncapi.Info{Title: "Chat"})
	if err != nil {
		log.Fatal(err)
	}
	src, err := json.Marshal(doc)
	if err != nil {
		log.Fatal(err)
	}

	files, err := codegen.Generate(src, codegen.Options{Package: "chatclient"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(slices.Sorted(maps.Keys(files)))
	for line := range strings.Lines(string(files["channels.go"])) {
		if strings.HasPrefix(line, "func DialChat") || strings.HasPrefix(line, "func (c *ChatClient) RequestChat") {
			fmt.Print(line)
		}
	}
	// Output:
	// [channels.go messages.go]
	// func DialChat(ctx context.Context, baseURL string, room string, opts ...chanxclient.Option) (*ChatClient, error) {
	// func (c *ChatClient) RequestChat(ctx context.Context, m ChatMessage) (ChatMessage, error) {
}
