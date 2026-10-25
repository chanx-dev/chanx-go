package messages

import (
	"testing"

	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/chanxtest"
)

// The frames the Messages guide shows.
func TestFrames(t *testing.T) {
	e := chanx.New(chanx.Config{})
	e.Handle("/ws/room", Room{})
	c := chanxtest.Connect(t, e, "/ws/room")

	c.SendText(`{"action": "post", "payload": {"body": "hello"}}`)
	c.Expect(`{"action": "post", "payload": {"body": "hello", "author": "anonymous"}}`)
	c.SendText(`{"action": "post", "payload": {"body": 42}}`)
	c.Expect(`{"action": "error", "payload": [{"type": "string_type", "loc": ["post", "payload", "body"], "msg": "Input should be a valid string"}]}`)
	c.SendText(`{"action": "mood", "payload": {"mood": "angry"}}`)
	c.Expect(`{"action": "error", "payload": [{"type": "enum", "loc": ["mood", "payload", "mood"], "msg": "Input should be 'happy' or 'sad'"}]}`)
	c.SendText(`{"action": "signup", "payload": {"username": "al", "password": "x"}}`)
	c.Expect(`{"action": "error", "payload": [{"type": "string_too_short", "loc": ["signup", "payload", "username"], "msg": "String should have at least 3 characters"}]}`)
	c.SendText(`{"action": "signup", "payload": {"username": "ann", "password": "ann123"}}`)
	c.Expect(`{"action": "error", "payload": [{"type": "value_error", "loc": ["signup", "payload"], "msg": "Value error, password must not contain the username"}]}`)
	c.SendText(`{"action": "dance"}`)
	c.Expect(`{"action": "error", "payload": [{"type": "union_tag_invalid", "loc": [], "msg": "Input tag 'dance' found using 'action' does not match any of the expected tags: 'mood', 'post', 'signup', 'typing'"}]}`)
}
