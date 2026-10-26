// Package consumers holds the consumer examples of the Consumers guide.
package consumers

import (
	"errors"
	"strings"

	chanx "go.chanx.dev"
)

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

type ReportPayload struct {
	Reason string `json:"reason"`
}

type ReportMessage struct {
	chanx.BaseMessage `action:"report"`
	Payload           ReportPayload `json:"payload"`
}

type ReportedMessage struct {
	chanx.BaseMessage `action:"reported"`
}

type WelcomeMessage struct {
	chanx.BaseMessage `action:"welcome"`
	Payload           string `json:"payload"`
}

// Rooms stands in for a service the consumer depends on.
type Rooms struct{ Banner string }

// #region mixin
// Moderation is a mixin: embed it, and its handlers come along.
type Moderation struct{}

func (Moderation) HandleReport(ctx *chanx.Context, m ReportMessage) (ReportedMessage, error) {
	if strings.TrimSpace(m.Payload.Reason) == "" {
		return ReportedMessage{}, errors.New("empty reason")
	}
	return ReportedMessage{}, nil
}

// #endregion mixin

// #region consumer
type ChatConsumer struct {
	Moderation        // its HandleReport is promoted
	Rooms      *Rooms // dependencies are plain fields
}

// Groups is called once per connection: join the room named by the path.
func (c *ChatConsumer) Groups(conn *chanx.Conn) []string {
	return []string{"room." + conn.Param("room")}
}

// OnConnect runs once the connection is set up and its groups are joined.
func (c *ChatConsumer) OnConnect(ctx *chanx.Context) error {
	return ctx.Push(WelcomeMessage{Payload: c.Rooms.Banner})
}

// OnDisconnect runs while the groups are still joined, so it can still broadcast.
func (c *ChatConsumer) OnDisconnect(ctx *chanx.Context) {
	_ = ctx.Broadcast(ChatNotification{Payload: ChatPayload{Message: "someone left"}})
}

// HandleChat replies nothing directly: it broadcasts to the room.
func (c *ChatConsumer) HandleChat(ctx *chanx.Context, m ChatMessage) error {
	return ctx.Broadcast(ChatNotification{Payload: m.Payload}, chanx.ExcludeSelf())
}

// HandlePing replies: the return value goes back to the sender, echoing its ref.
func (c *ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

// Operations documents handlers in AsyncAPI, keyed by method name.
func (c *ChatConsumer) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{
		"HandleChat": {
			Summary: "Send a chat message",
			Replies: []chanx.Message{ChatNotification{}}, // what HandleChat broadcasts
		},
	}
}

// Emits lists what the consumer sends outside any handler.
func (c *ChatConsumer) Emits() []chanx.Message { return []chanx.Message{WelcomeMessage{}} }

// #endregion consumer

// #region function
// NewChatHub builds a consumer from functions, for closures and dynamic setups.
func NewChatHub(banner string) *chanx.Consumer {
	hub := chanx.NewConsumer("chat_hub")
	hub.Groups = func(conn *chanx.Conn) []string { return []string{"room." + conn.Param("room")} }
	hub.OnConnect = func(ctx *chanx.Context) error { return ctx.Push(WelcomeMessage{Payload: banner}) }
	hub.Emits = []chanx.Message{WelcomeMessage{}, ChatNotification{}}

	chanx.Handle(hub, func(ctx *chanx.Context, m ChatMessage) error {
		return ctx.Broadcast(ChatNotification{Payload: m.Payload}, chanx.ExcludeSelf())
	}, chanx.Summary("Send a chat message"))
	chanx.On(hub, func(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
		return chanx.PongMessage{}, nil
	})
	return hub
}

// #endregion function
