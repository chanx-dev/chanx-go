package main

import (
	"testing"

	"go.chanx.dev/chanxtest"
)

// The frames the quick start shows.
func TestQuickStart(t *testing.T) {
	e := newEngine()
	a := chanxtest.Connect(t, e, "/ws/chat/lobby")
	b := chanxtest.Connect(t, e, "/ws/chat/lobby")

	a.SendText(`{"action": "chat", "payload": {"text": "hi"}}`)
	a.Expect(`{"action": "chat_notification", "payload": {"text": "hi"}}`)
	b.Expect(`{"action": "chat_notification", "payload": {"text": "hi"}}`)

	a.SendText(`{"action": "chat", "payload": {"txt": "hi"}}`)
	a.Expect(`{"action": "error", "payload": [{"type": "missing", "loc": ["chat", "payload", "text"], "msg": "Field required"}]}`)

	a.SendText(`{"action": "ping", "ref": "1"}`)
	a.Expect(`{"action": "pong", "payload": null, "version": 1, "ref": "1"}`)
}
