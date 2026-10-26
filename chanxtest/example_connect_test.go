package chanxtest_test

import (
	"testing"

	chanx "go.chanx.dev"
	"go.chanx.dev/chanxtest"
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

type ChatConsumer struct{}

func (ChatConsumer) Groups(*chanx.Conn) []string { return []string{"chat"} }

// Emits declares what HandleChat broadcasts, for the AsyncAPI document and for
// strict mode, which fails a test when the server sends an undeclared action.
func (ChatConsumer) Emits() []chanx.Message { return []chanx.Message{ChatNotification{}} }

func (ChatConsumer) HandleChat(ctx *chanx.Context, m ChatMessage) error {
	return ctx.Broadcast(ChatNotification{Payload: m.Payload})
}

func (ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

// testChat is a test of your consumer, as you would name it TestChat: Connect runs
// a connection in memory, with no network, and returns once it joined its groups.
func testChat(t *testing.T) {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/chat", ChatConsumer{})

	alice := chanxtest.Connect(t, engine, "/ws/chat")
	bob := chanxtest.Connect(t, engine, "/ws/chat")

	alice.Send(ChatMessage{Payload: ChatPayload{Message: "hi"}})
	bob.Expect(`{"action":"chat_notification","payload":{"message":"hi"}}`)
	note, _ := chanxtest.ReceiveUntilMessage[ChatNotification](alice)
	if note.Payload.Message != "hi" {
		t.Fatalf("alice got %q", note.Payload.Message)
	}

	// A bad payload never reaches the handler: the sender gets the validation error.
	alice.SendText(`{"action":"chat","payload":{}}`)
	alice.Expect(`{"action":"error","payload":[{"type":"missing","loc":["chat","payload","message"],"msg":"Field required"}]}`)

	// Request sends with a fresh ref and collects the frames answering it.
	if replies := alice.Request(chanx.PingMessage{}); replies[0].Action() != "pong" {
		t.Fatalf("got %v", replies)
	}
}

func ExampleConnect() {
	_ = testChat
}
