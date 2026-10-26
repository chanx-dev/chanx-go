package sandbox

import (
	"context"
	"testing"
	"time"

	"go.chanx.dev/v2/chanxtest"
)

func TestSandbox(t *testing.T) {
	set, _ := Layers("")
	e := New(set, nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = e.Shutdown(ctx)
	})

	sys := chanxtest.Connect(t, e, "/ws/system")
	sys.Expect(`{"action":"system_echo","payload":{"message":"🔧 System: Connection established!"}}`)
	sys.SendText(`{"action":"user_message","payload":{"message":"hi"},"ref":"1"}`)
	sys.Expect(`{"action":"system_echo","payload":{"message":"🔧 System Echo: hi"},"version":1,"ref":"1"}`)

	// Chat: the join broadcast reaches the joiner too, mixins and passthrough work.
	a := chanxtest.Connect(t, e, "/ws/chat")
	a.Expect(`{"action":"chat_notification","payload":{"message":"📢 Someone joined the chat"}}`)
	b := chanxtest.Connect(t, e, "/ws/chat")
	b.Expect(`{"action":"chat_notification","payload":{"message":"📢 Someone joined the chat"}}`)
	a.Expect(`{"action":"chat_notification","payload":{"message":"📢 Someone joined the chat"}}`)
	a.SendText(`{"action":"extra_request","payload":"x"}`)
	a.Expect(`{"action":"extra_response","payload":"x any extra thing"}`)
	pub := e.Publisher().Layer("chat")
	_ = pub.BroadcastEvent(context.Background(), UserJoinedNotification{Payload: ChatPayload{Message: "j"}}, "chat_room")
	_ = pub.BroadcastEvent(context.Background(), ExtraPassthroughMessage{Payload: "p"}, "chat_room")
	_ = pub.BroadcastEvent(context.Background(), SystemNotify{Payload: "sys"}, "chat_room")
	for _, c := range []*chanxtest.Client{a, b} {
		c.Expect(`{"action":"user_joined_notification","payload":{"message":"j"}}`)
		c.Expect(`{"action":"extra_passthrough","payload":"p"}`)
		c.Expect(`{"action":"chat_notification","payload":{"message":"sys"}}`)
	}
	b.Close()
	a.Expect(`{"action":"chat_notification","payload":{"message":"❌ Someone left the chat."}}`)

	// Room chat: defaults and the nullable room name.
	r := chanxtest.Connect(t, e, "/ws/room/lobby")
	r.SendText(`{"action":"room_chat","payload":{"message":"yo"}}`)
	r.Expect(`{"action":"room_notification","payload":{"message":"💬 yo","room_name":"lobby","status":"PENDING"}}`)

	// Background jobs: the worker answers through the layer.
	j := chanxtest.Connect(t, e, "/ws/background_jobs")
	j.Receive() // connected
	j.SendText(`{"action":"job","payload":{"type":"translate","content":"hola"}}`)
	if f := j.Receive(); f.Payload().(map[string]any)["status"] != "queuing" {
		t.Fatal(f)
	}
	j.Receive() // queued
	j.Expect(`{"action":"job_status","payload":{"status":"result","message":"🌐 Translated: hola"}}`)

	// Topics on the hub and on the dedicated route.
	h := chanxtest.Connect(t, e, "/ws/topics")
	h.Subscribe("room:lobby")
	h.Subscribe("presence:ann")
	d := chanxtest.Connect(t, e, "/ws/topics/room/lobby")
	d.ReceiveTopic("room:lobby")
	d.SendText(`{"action":"post","payload":{"body":"b"}}`)
	h.Expect(`{"action":"room_event","payload":{"body":"b","author":"anonymous"},"version":1,"topic":"room:lobby"}`)
	d.ReceiveTopic("room:lobby")
}
