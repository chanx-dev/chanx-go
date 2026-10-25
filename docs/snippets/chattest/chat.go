// Package chattest holds the examples of the Testing guide.
package chattest

import chanx "github.com/huynguyengl99/chanx-go"

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

type MessagePosted struct {
	chanx.BaseMessage `action:"message_posted"`
	Payload           ChatPayload `json:"payload"`
}

type ChatConsumer struct{}

func (ChatConsumer) Groups(c *chanx.Conn) []string { return []string{"room." + c.Param("room")} }

func (ChatConsumer) Emits() []chanx.Message { return []chanx.Message{ChatNotification{}} }

// HandleChat broadcasts to the room and tells other services through an event.
func (ChatConsumer) HandleChat(ctx *chanx.Context, m ChatMessage) error {
	if err := ctx.Broadcast(ChatNotification{Payload: m.Payload}); err != nil {
		return err
	}
	return ctx.Publisher().BroadcastEvent(ctx, MessagePosted{Payload: m.Payload}, "audit")
}

func (ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

func NewEngine(cfg chanx.Config) *chanx.Engine {
	e := chanx.New(cfg)
	e.Handle("/ws/chat/{room}", ChatConsumer{})
	return e
}
