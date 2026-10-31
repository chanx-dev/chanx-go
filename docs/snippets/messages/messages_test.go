package messages

import (
	"testing"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"
)

// The frames the Messages guide shows.
func TestFrames(t *testing.T) {
	e := chanx.New(chanx.Config{})
	e.Handle("/ws/room", Room{})
	c := chanxtest.Connect(t, e, "/ws/room")

	c.SendText(`{"action": "post", "payload": {"body": "hello"}}`)
	c.Expect(`{"action": "post", "payload": {"body": "hello", "author": "anonymous"}}`)
	c.SendText(`{"action": "post", "payload": {"body": 42}}`)
	c.Expect(`{"action": "error", "payload": {"type": "https://chanx.dev/errors/validation-failed", "title": "Message failed validation", "detail": "1 validation error", "errors": [{"keyword": "type", "pointer": "/payload/body", "detail": "Input should be a valid string"}]}}`)
	c.SendText(`{"action": "mood", "payload": {"mood": "angry"}}`)
	c.Expect(`{"action": "error", "payload": {"type": "https://chanx.dev/errors/validation-failed", "title": "Message failed validation", "detail": "1 validation error", "errors": [{"keyword": "enum", "pointer": "/payload/mood", "detail": "Input should be 'happy' or 'sad'"}]}}`)
	c.SendText(`{"action": "signup", "payload": {"username": "al", "password": "x"}}`)
	c.Expect(`{"action": "error", "payload": {"type": "https://chanx.dev/errors/validation-failed", "title": "Message failed validation", "detail": "1 validation error", "errors": [{"keyword": "minLength", "pointer": "/payload/username", "detail": "String should have at least 3 characters"}]}}`)
	c.SendText(`{"action": "signup", "payload": {"username": "ann", "password": "ann123"}}`)
	c.Expect(`{"action": "error", "payload": {"type": "https://chanx.dev/errors/validation-failed", "title": "Message failed validation", "detail": "1 validation error", "errors": [{"keyword": "custom", "pointer": "/payload", "detail": "Value error, password must not contain the username"}]}}`)
	c.SendText(`{"action": "dance"}`)
	c.Expect(`{"action": "error", "payload": {"type": "https://chanx.dev/errors/validation-failed", "title": "Message failed validation", "detail": "1 validation error", "errors": [{"keyword": "enum", "pointer": "/action", "detail": "Input tag 'dance' found using 'action' does not match any of the expected tags: 'mood', 'post', 'signup', 'typing'"}]}}`)
}
