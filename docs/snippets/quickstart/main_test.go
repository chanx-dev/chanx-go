package main

import (
	"testing"

	"go.chanx.dev/v2/chanxtest"
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
	a.Expect(`{"action": "error", "payload": {"type": "https://chanx.dev/errors/validation-failed", "title": "Message failed validation", "detail": "1 validation error", "errors": [{"keyword": "required", "pointer": "/payload", "detail": "Field required", "property": "text"}]}}`)

	a.SendText(`{"action": "ping", "ref": "1"}`)
	a.Expect(`{"action": "pong", "payload": null, "version": 1, "ref": "1"}`)
}
