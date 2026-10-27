// Package goclient holds the examples of the Go client guide: a chat server, its
// AsyncAPI document and the client chanx-codegen generates from it.
package goclient

import (
	"errors"

	chanx "go.chanx.dev/v2"
)

//go:generate go run go.chanx.dev/v2/cmd/chanx-codegen -o chatclient asyncapi.json

type ChatPayload struct {
	Message string `json:"message"`
}

type ChatMessage struct {
	chanx.BaseMessage `action:"chat"`
	Payload           ChatPayload `json:"payload"`
}

type ChatNotification struct {
	chanx.BaseMessage `action:"chat_notification"`
	Payload           ChatPayload `json:"payload"`
}

type JoinPayload struct {
	Name string `json:"name"`
}

type JoinMessage struct {
	chanx.BaseMessage `action:"join"`
	Payload           JoinPayload `json:"payload"`
}

type WelcomePayload struct {
	Room    string `json:"room"`
	Greeter string `json:"greeter" default:"bot"`
}

type WelcomeMessage struct {
	chanx.BaseMessage `action:"welcome"`
	Payload           WelcomePayload `json:"payload"`
}

type Headline struct {
	chanx.BaseMessage `action:"headline"`
	Payload           string `json:"payload"`
}

type NewsTopic struct {
	chanx.BaseTopic `pattern:"news:{tag}"`
}

func (NewsTopic) Passthrough() []chanx.Message { return []chanx.Message{Headline{}} }

type ChatConsumer struct{}

func (ChatConsumer) Groups(c *chanx.Conn) []string { return []string{"room." + c.Param("room")} }

func (ChatConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{&NewsTopic{}} }

func (ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

func (ChatConsumer) HandleJoin(ctx *chanx.Context, m JoinMessage) (WelcomeMessage, error) {
	if m.Payload.Name == "" {
		return WelcomeMessage{}, errors.New("a name is required")
	}
	return WelcomeMessage{Payload: WelcomePayload{Room: ctx.Conn.Param("room"), Greeter: "bot"}}, nil
}

func (ChatConsumer) HandleChat(ctx *chanx.Context, m ChatMessage) error {
	return ctx.Broadcast(ChatNotification{Payload: m.Payload})
}

func (ChatConsumer) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{"HandleChat": {Replies: []chanx.Message{ChatNotification{}}}}
}

func NewEngine() *chanx.Engine {
	e := chanx.New(chanx.Config{})
	e.Handle("/ws/chat/{room}", ChatConsumer{}, chanx.Name("chat"))
	return e
}
