package chattest

import (
	"testing"
	"time"

	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/chanxtest"
	"github.com/huynguyengl99/chanx-go/layers"
	"github.com/huynguyengl99/chanx-go/layers/inmemory"
)

// #region client
func TestChat(t *testing.T) {
	engine := NewEngine(chanx.Config{})
	alice := chanxtest.Connect(t, engine, "/ws/chat/lobby")
	bob := chanxtest.Connect(t, engine, "/ws/chat/lobby")

	alice.Send(ChatMessage{Payload: ChatPayload{Message: "hi"}})
	bob.Expect(`{"action": "chat_notification", "payload": {"message": "hi"}}`)

	note, before := chanxtest.ReceiveUntilMessage[ChatNotification](alice)
	if note.Payload.Message != "hi" || len(before) != 0 {
		t.Fatalf("alice got %+v after %v", note, before)
	}
}

// #endregion client

// #region errors
func TestValidation(t *testing.T) {
	c := chanxtest.Connect(t, NewEngine(chanx.Config{}), "/ws/chat/lobby")
	c.SendText(`{"action": "chat", "payload": {}}`)
	c.Expect(`{"action": "error", "payload": [{"type": "missing", "loc": ["chat", "payload", "message"], "msg": "Field required"}]}`)
	c.ExpectNothing(50 * time.Millisecond) // and nothing else
}

// #endregion errors

// #region request
func TestRequest(t *testing.T) {
	c := chanxtest.Connect(t, NewEngine(chanx.Config{SendCompletion: true}), "/ws/chat/lobby")
	frames := c.Request(chanx.PingMessage{}) // a fresh ref; collects until complete
	if len(frames) != 1 || frames[0].Action() != "pong" {
		t.Fatalf("got %v", frames)
	}
}

// #endregion request

// #region spy
func TestChatIsAudited(t *testing.T) {
	spy := chanxtest.Spy(inmemory.New())
	engine := NewEngine(chanx.Config{Layers: layers.Set{"default": spy}})
	c := chanxtest.Connect(t, engine, "/ws/chat/lobby")

	c.Send(ChatMessage{Payload: ChatPayload{Message: "hi"}})
	c.ReceiveUntil("chat_notification")

	posted := chanxtest.BroadcastEvents[MessagePosted](spy)
	if len(posted) != 1 || posted[0].Group != "audit" || posted[0].Event.Payload.Message != "hi" {
		t.Fatalf("published %+v", posted)
	}
}

// #endregion spy
