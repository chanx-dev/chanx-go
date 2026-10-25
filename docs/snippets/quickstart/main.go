// Command quickstart is the chat server of the quick start guide.
package main

import (
	"log"
	"net/http"

	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/asyncapi"
	chanxhttp "github.com/huynguyengl99/chanx-go/nethttp"
)

// #region messages
type ChatPayload struct {
	Text string `json:"text"`
}

// ChatMessage is what a client sends.
type ChatMessage struct {
	chanx.BaseMessage `action:"chat" description:"Send a message to the room"`
	Payload           ChatPayload `json:"payload"`
}

// ChatNotification is what everyone in the room receives.
type ChatNotification struct {
	chanx.BaseMessage `action:"chat_notification"`
	Payload           ChatPayload `json:"payload"`
}

// #endregion messages

// #region consumer
type ChatConsumer struct{}

// Groups puts each connection in its room, from the {room} path parameter.
func (ChatConsumer) Groups(c *chanx.Conn) []string { return []string{"room." + c.Param("room")} }

// Emits declares what the consumer sends besides replies.
func (ChatConsumer) Emits() []chanx.Message { return []chanx.Message{ChatNotification{}} }

func (ChatConsumer) HandleChat(ctx *chanx.Context, m ChatMessage) error {
	return ctx.Broadcast(ChatNotification{Payload: m.Payload})
}

func (ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

// #endregion consumer

// #region main
func newEngine() *chanx.Engine {
	engine := chanx.New(chanx.Config{}) // an in-memory layer; Redis for several processes
	engine.Handle("/ws/chat/{room}", ChatConsumer{}, chanx.Name("chat"))
	return engine
}

func main() {
	engine := newEngine()
	mux := http.NewServeMux()
	chanxhttp.Mount(mux, engine)
	chanxhttp.MountDocs(mux, engine, "/asyncapi", asyncapi.Info{Title: "Chat"})
	log.Println("listening on http://localhost:8000")
	log.Fatal(http.ListenAndServe(":8000", mux))
}

// #endregion main
