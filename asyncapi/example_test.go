package asyncapi_test

import (
	"fmt"
	"net/http"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/asyncapi"
)

type AskMessage struct {
	chanx.BaseMessage `action:"ask" description:"Ask a question."`
	Payload           string `json:"payload"`
}

type AnswerMessage struct {
	chanx.BaseMessage `action:"answer"`
	Payload           string `json:"payload"`
}

type OracleConsumer struct{}

func (OracleConsumer) HandleAsk(ctx *chanx.Context, m AskMessage) (AnswerMessage, error) {
	return AnswerMessage{Payload: "42"}, nil
}

// The document is built from the handlers that serve traffic, so it cannot drift
// from the server.
func ExampleGenerate() {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/oracle", OracleConsumer{})

	doc, _ := asyncapi.Generate(engine, asyncapi.Info{Title: "Oracle"})
	channels, _ := doc.Get("channels")
	operations, _ := doc.Get("operations")
	fmt.Println(channels.(*asyncapi.Map).Keys(), operations.(*asyncapi.Map).Keys())
	// Output: [oracle] [handle_ask]
}

// HTMLHandler serves the interactive page; JSONHandler and YAMLHandler serve the
// document client generators read. The adapters' MountDocs registers all three.
func ExampleHTMLHandler() {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/oracle", OracleConsumer{})

	info := asyncapi.Info{Title: "Oracle", Version: "1.0.0"}
	mux := http.NewServeMux()
	mux.Handle("GET /asyncapi", asyncapi.HTMLHandler(engine, info))
	mux.Handle("GET /asyncapi.json", asyncapi.JSONHandler(engine, info))
	mux.Handle("GET /asyncapi.yaml", asyncapi.YAMLHandler(engine, info))
}
