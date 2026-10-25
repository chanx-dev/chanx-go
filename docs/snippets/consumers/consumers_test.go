package consumers

import (
	"testing"
	"time"

	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/chanxtest"
)

func TestFrames(t *testing.T) {
	for name, consumer := range map[string]any{
		"struct":   &ChatConsumer{Rooms: &Rooms{Banner: "Welcome to the lobby"}},
		"function": NewChatHub("Welcome to the lobby"),
	} {
		t.Run(name, func(t *testing.T) {
			e := chanx.New(chanx.Config{})
			e.Handle("/ws/chat/{room}", consumer)
			a := chanxtest.Connect(t, e, "/ws/chat/lobby")
			a.Expect(`{"action": "welcome", "payload": "Welcome to the lobby"}`)
			b := chanxtest.Connect(t, e, "/ws/chat/lobby")
			b.Expect(`{"action": "welcome", "payload": "Welcome to the lobby"}`)
			a.SendText(`{"action": "chat", "payload": {"message": "hi"}}`)
			b.Expect(`{"action": "chat_notification", "payload": {"message": "hi"}}`)
			a.ExpectNothing(100 * time.Millisecond) // ExcludeSelf
			a.SendText(`{"action": "ping", "ref": "42"}`)
			a.Expect(`{"action": "pong", "payload": null, "version": 1, "ref": "42"}`)
			if name == "struct" {
				a.SendText(`{"action": "report", "payload": {"reason": " "}, "ref": "r1"}`)
				a.Expect(`{"action": "error", "payload": {"detail": "Failed to process message for report"}, "version": 1, "ref": "r1"}`)
				b.Close()
				a.Expect(`{"action": "chat_notification", "payload": {"message": "someone left"}}`)
			}
		})
	}
}
