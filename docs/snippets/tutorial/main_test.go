package main

import (
	"testing"

	"github.com/huynguyengl99/chanx-go/chanxtest"
)

// #region test
func TestRoom(t *testing.T) {
	engine := newEngine(nil)
	ann := chanxtest.Connect(t, engine, "/ws/rooms/lobby?token=t-ann")
	bob := chanxtest.Connect(t, engine, "/ws/rooms/lobby", chanxtest.Header("Authorization", "Bearer t-bob"))

	ann.Expect(`{"action": "presence", "payload": {"user": "bob", "online": true}}`)

	bob.Send(SendMessage{Payload: SendPayload{Text: "hi ann"}})
	for _, c := range []*chanxtest.Client{ann, bob} {
		posted, _ := chanxtest.ReceiveUntilMessage[PostedMessage](c)
		if posted.Payload.Author != "bob" || posted.Payload.Text != "hi ann" {
			t.Fatalf("got %+v", posted.Payload)
		}
	}

	bob.Close()
	ann.Expect(`{"action": "presence", "payload": {"user": "bob", "online": false}}`)
}

func TestBadToken(t *testing.T) {
	c := chanxtest.Connect(t, newEngine(nil), "/ws/rooms/lobby?token=nope")
	if code := c.WaitClosed(); code != 4401 {
		t.Fatalf("closed with %d", code)
	}
}

// #endregion test
