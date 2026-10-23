package chanx_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/chanxtest"
	"github.com/huynguyengl99/chanx-go/layers"
	"github.com/huynguyengl99/chanx-go/layers/inmemory"
)

// A chat consumer with a mixin, a topic hub and a dedicated topic route.

type ChatPayload struct {
	Message string `json:"message"`
}

type ChatMessage struct {
	chanx.BaseMessage `action:"chat" description:"Send a chat message to the room"`
	Payload           ChatPayload `json:"payload"`
}

type ChatNotificationPayload struct {
	Room    string `json:"room"`
	Message string `json:"message"`
}

type ChatNotification struct {
	chanx.BaseMessage `action:"chat_notification"`
	Payload           ChatNotificationPayload `json:"payload"`
}

type ReportPayload struct {
	Reason string `json:"reason"`
}

type ReportMessage struct {
	chanx.BaseMessage `action:"report"`
	Payload           ReportPayload `json:"payload"`
}

type ReportedMessage struct {
	chanx.BaseMessage `action:"reported"`
}

type AnnouncementPayload struct {
	Text string `json:"text"`
}

type AnnouncementEvent struct {
	chanx.BaseMessage `action:"announcement_event"`
	Payload           AnnouncementPayload `json:"payload"`
}

type AnnouncementMessage struct {
	chanx.BaseMessage `action:"announcement"`
	Payload           AnnouncementPayload `json:"payload"`
}

type Moderation struct{}

func (Moderation) Report(ctx *chanx.Context, m ReportMessage) (ReportedMessage, error) {
	if strings.TrimSpace(m.Payload.Reason) == "" {
		return ReportedMessage{}, errors.New("empty reason")
	}
	return ReportedMessage{}, nil
}

type ChatConsumer struct {
	Moderation
	Banner string
}

func (c *ChatConsumer) Authenticate(ctx *chanx.Context, r *chanx.Request) (any, error) {
	switch r.Query.Get("token") {
	case "bad":
		return nil, chanx.Unauthenticated("Unauthorized")
	case "banned":
		return nil, chanx.Forbidden("Banned")
	case "broken":
		return nil, errors.New("database down")
	case "panic":
		panic("database down")
	}
	return "user", nil
}

func (c *ChatConsumer) Groups(conn *chanx.Conn) []string {
	return []string{"room." + conn.Param("room")}
}

func (c *ChatConsumer) OnConnect(ctx *chanx.Context) error {
	return ctx.Push(AnnouncementMessage{Payload: AnnouncementPayload{Text: c.Banner}})
}

func (c *ChatConsumer) Ping(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

func (c *ChatConsumer) Chat(ctx *chanx.Context, m ChatMessage) error {
	return ctx.Broadcast(ChatNotification{Payload: ChatNotificationPayload{
		Room: ctx.Conn.Param("room"), Message: m.Payload.Message,
	}}, chanx.ExcludeSelf())
}

func (c *ChatConsumer) Announcement(ctx *chanx.EventContext, e AnnouncementEvent) (AnnouncementMessage, error) {
	return AnnouncementMessage{Payload: e.Payload}, nil
}

func (c *ChatConsumer) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{"Chat": {Replies: []chanx.Message{ChatNotification{}}}}
}

type PostPayload struct {
	Body   string `json:"body"`
	Author string `json:"author" default:"anonymous"`
	Likes  int    `json:"likes" default:"0"`
}

type PostMessage struct {
	chanx.BaseMessage `action:"post"`
	Payload           PostPayload `json:"payload"`
}

type EchoMessage struct {
	chanx.BaseMessage `action:"echo"`
	Payload           PostPayload `json:"payload"`
}

type PostedMessage struct {
	chanx.BaseMessage `action:"posted"`
	Payload           PostPayload `json:"payload"`
}

type RoomEvent struct {
	chanx.BaseMessage `action:"room_event"`
	Payload           PostPayload `json:"payload"`
}

type RosterPayload struct {
	Room    string `json:"room"`
	Members int    `json:"members"`
}

type RosterMessage struct {
	chanx.BaseMessage `action:"roster"`
	Payload           RosterPayload `json:"payload"`
}

type RoomTopic struct {
	chanx.BaseTopic `pattern:"room:{room_name}"`
	left            chan string
}

func (*RoomTopic) Passthrough() []chanx.Message { return []chanx.Message{RoomEvent{}} }

func (*RoomTopic) Authorize(ctx *chanx.Context, p chanx.Params) (bool, error) {
	return p["room_name"] != "secret", nil
}

func (*RoomTopic) OnSubscribe(ctx *chanx.Context) error {
	return ctx.Send(RosterMessage{Payload: RosterPayload{Room: ctx.Params["room_name"], Members: 1}})
}

func (t *RoomTopic) OnUnsubscribe(ctx *chanx.Context) error {
	if t.left != nil {
		t.left <- ctx.Topic
	}
	return nil
}

func (*RoomTopic) Post(ctx *chanx.Context, m PostMessage) error {
	return ctx.BroadcastTopic(RoomEvent{Payload: m.Payload})
}

func (*RoomTopic) Echo(ctx *chanx.Context, m EchoMessage) (PostedMessage, error) {
	return PostedMessage{Payload: m.Payload}, nil
}

func (*RoomTopic) Emits() []chanx.Message { return []chanx.Message{RosterMessage{}} }

type TopicHubConsumer struct{ room *RoomTopic }

func (h *TopicHubConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{h.room} }

func (*TopicHubConsumer) Ping(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

func newEngine(t *testing.T, cfg chanx.Config) (*chanx.Engine, *RoomTopic) {
	t.Helper()
	e := chanx.New(cfg)
	room := &RoomTopic{left: make(chan string, 10)}
	e.Handle("/ws/chat/{room}", &ChatConsumer{Banner: "welcome"})
	e.Handle("/ws/topics", &TopicHubConsumer{room: room})
	e.Handle("/ws/topics/room/{room_name}", chanx.TopicConsumer(room))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := e.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	return e, room
}

func dialChat(t *testing.T, e *chanx.Engine, room string) *chanxtest.Client {
	t.Helper()
	c := chanxtest.Connect(t, e, "/ws/chat/"+room)
	if !chanxtest.JSONEqual(c.Auth.String(), `{"action":"authentication","payload":{"status_code":200,"status_text":"OK","data":null}}`) {
		t.Fatalf("auth: %s", c.Auth)
	}
	c.Expect(`{"action":"announcement","payload":{"text":"welcome"}}`)
	return c
}

func TestChat(t *testing.T) {
	e, _ := newEngine(t, chanx.Config{})
	alice := dialChat(t, e, "lobby")
	bob := dialChat(t, e, "lobby")
	carol := dialChat(t, e, "other")

	alice.SendText(`{"action":"ping","ref":"1"}`)
	alice.Expect(`{"version":1,"ref":"1","action":"pong","payload":null}`)

	alice.SendText(`{"action":"chat","payload":{"message":"hi"}}`)
	bob.Expect(`{"action":"chat_notification","payload":{"room":"lobby","message":"hi"}}`)
	alice.SendText(`{"action":"ping"}`)
	alice.Expect(`{"action":"pong","payload":null}`)

	bob.SendText(`{"action":"report","ref":"r1","payload":{"reason":"spam"}}`)
	bob.Expect(`{"version":1,"ref":"r1","action":"reported","payload":null}`)
	bob.SendText(`{"action":"report","ref":"r2","payload":{"reason":" "}}`)
	bob.Expect(`{"version":1,"ref":"r2","action":"error","payload":{"detail":"Failed to process message for report"}}`)

	bob.SendText(`{"action":"nope"}`)
	bob.Expect(`{"action":"error","payload":[{"type":"union_tag_invalid","loc":[],"msg":"Input tag 'nope' found using 'action' does not match any of the expected tags: 'chat', 'ping', 'report'"}]}`)
	bob.SendText(`{"action":"ping","ref":3}`)
	bob.Expect(`{"action":"error","payload":[{"type":"string_type","loc":["ref"],"msg":"Input should be a valid string"}]}`)
	bob.SendText(`{"payload":1}`)
	bob.Expect(`{"action":"error","payload":[{"type":"union_tag_not_found","loc":[],"msg":"Unable to extract tag using discriminator 'action'"}]}`)
	bob.SendText(`{"action":"chat","ref":"v"}`)
	bob.Expect(`{"version":1,"ref":"v","action":"error","payload":[{"type":"missing","loc":["chat","payload"],"msg":"Field required"}]}`)
	bob.SendText(`{"action":"ping","payload":1}`)
	bob.Expect(`{"action":"error","payload":[{"type":"none_required","loc":["ping","payload"],"msg":"Input should be None"}]}`)
	bob.SendText(`{"action":true}`)
	bob.Expect(`{"action":"error","payload":[{"type":"union_tag_invalid","loc":[],"msg":"Input tag 'True' found using 'action' does not match any of the expected tags: 'chat', 'ping', 'report'"}]}`)
	bob.SendText(`{"action":"ping","version":"x","seq":1.5}`)
	bob.Expect(`{"action":"error","payload":[{"type":"int_parsing","loc":["version"],"msg":"Input should be a valid integer, unable to parse string as an integer"},{"type":"int_from_float","loc":["seq"],"msg":"Input should be a valid integer, got a number with a fractional part"}]}`)
	bob.SendText(`{"action":"ping","version":"12","ref":"lax"}`)
	bob.Expect(`{"version":1,"ref":"lax","action":"pong","payload":null}`)
	bob.SendText(`[1]`)
	bob.Expect(`{"action":"error","payload":[{"type":"model_type","loc":[],"msg":"Input should be a valid dictionary or instance of Envelope"}]}`)
	bob.SendText(`not json`)
	f := bob.Receive()
	if !strings.HasPrefix(fmt.Sprint(f.Payload()), "[map[loc:[] msg:Invalid JSON: ") {
		t.Fatalf("non-JSON frame: %s", f)
	}
	bob.SendBinary([]byte{1, 2})
	bob.Expect(`{"action":"error","payload":[{"type":"json_invalid","loc":[],"msg":"Invalid JSON: binary frames are not supported"}]}`)

	// An event through the layer; the event handler maps it.
	pub := e.Publisher()
	if err := pub.BroadcastEvent(context.Background(), AnnouncementEvent{Payload: AnnouncementPayload{Text: "deploy at 5"}}, "room.lobby"); err != nil {
		t.Fatal(err)
	}
	alice.Expect(`{"action":"announcement","payload":{"text":"deploy at 5"}}`)
	bob.Expect(`{"action":"announcement","payload":{"text":"deploy at 5"}}`)

	carol.SendText(`{"action":"ping"}`)
	carol.Expect(`{"action":"pong","payload":null}`)
}

func TestAuthFailure(t *testing.T) {
	e, _ := newEngine(t, chanx.Config{})
	for _, tc := range []struct {
		token string
		auth  string
		code  int
	}{
		{"bad", `{"action":"authentication","payload":{"status_code":401,"status_text":"Unauthorized","data":null}}`, 4401},
		{"banned", `{"action":"authentication","payload":{"status_code":403,"status_text":"Banned","data":null}}`, 4403},
		{"broken", `{"action":"authentication","payload":{"status_code":401,"status_text":"Unauthorized","data":null}}`, 4401},
		{"panic", `{"action":"authentication","payload":{"status_code":500,"status_text":"Internal Server Error","data":null}}`, 4500},
	} {
		c := chanxtest.Connect(t, e, "/ws/chat/lobby?token="+tc.token)
		if !chanxtest.JSONEqual(c.Auth.String(), tc.auth) {
			t.Fatalf("%s: got %s", tc.token, c.Auth)
		}
		if code := c.WaitClosed(); code != tc.code {
			t.Fatalf("%s: close code %d", tc.token, code)
		}
	}
}

func TestRejectBeforeUpgrade(t *testing.T) {
	e, _ := newEngine(t, chanx.Config{RejectBeforeUpgrade: true})
	_, err := chanxtest.Dial(t, e, "/ws/chat/lobby?token=banned")
	var ae *chanx.AuthError
	if !errors.As(err, &ae) || ae.Status != 403 {
		t.Fatalf("got %v", err)
	}
	dialChat(t, e, "lobby") // success still sends the authentication message
}

func TestTopics(t *testing.T) {
	e, room := newEngine(t, chanx.Config{})
	a := chanxtest.Connect(t, e, "/ws/topics")
	b := chanxtest.Connect(t, e, "/ws/topics")

	a.SendText(`{"action":"subscribe","topic":"room:lobby","ref":"s1"}`)
	a.Expect(`{"action":"subscribed","payload":null,"version":1,"topic":"room:lobby","ref":"s1"}`)
	a.Expect(`{"action":"roster","payload":{"room":"lobby","members":1},"version":1,"topic":"room:lobby"}`)
	a.SendText(`{"action":"subscribe","topic":"room:lobby","ref":"s2"}`)
	a.Expect(`{"action":"subscribed","payload":null,"version":1,"topic":"room:lobby","ref":"s2"}`)
	a.ExpectNothing(100 * time.Millisecond) // no second roster

	a.SendText(`{"action":"subscribe","topic":"room:secret"}`)
	a.Expect(`{"action":"error","payload":{"reason":"unauthorized","detail":"Not allowed to subscribe to 'room:secret'"},"version":1,"topic":"room:secret"}`)
	a.SendText(`{"action":"subscribe","topic":"nope:x"}`)
	a.Expect(`{"action":"error","payload":{"reason":"unknown_topic","detail":"Unknown topic 'nope:x'"},"version":1,"topic":"nope:x"}`)
	a.SendText(`{"action":"post","topic":"room:other","payload":{"body":"x"}}`)
	a.Expect(`{"action":"error","payload":{"reason":"not_subscribed","detail":"Not subscribed to 'room:other'"},"version":1,"topic":"room:other"}`)
	// "" is a topic, as Python's Envelope (str | None) reads it.
	a.SendText(`{"action":"subscribe","topic":""}`)
	a.Expect(`{"action":"error","payload":{"reason":"unknown_topic","detail":"Unknown topic ''"},"version":1,"topic":""}`)
	a.SendText(`{"action":"post","topic":"","payload":{"body":"x"}}`)
	a.Expect(`{"action":"error","payload":{"reason":"not_subscribed","detail":"Not subscribed to ''"},"version":1,"topic":""}`)

	b.Subscribe("room:lobby")
	b.ReceiveTopic("room:lobby") // roster

	// A post broadcasts to the topic, defaults filled.
	a.SendText(`{"action":"post","topic":"room:lobby","ref":"p","payload":{"body":"hello"}}`)
	for _, c := range []*chanxtest.Client{a, b} {
		c.Expect(`{"action":"room_event","payload":{"body":"hello","author":"anonymous","likes":0},"version":1,"topic":"room:lobby"}`)
	}
	a.SendText(`{"action":"echo","topic":"room:lobby","ref":"e","payload":{"body":"b","likes":2}}`)
	a.Expect(`{"action":"posted","payload":{"body":"b","author":"anonymous","likes":2},"version":1,"topic":"room:lobby","ref":"e"}`)
	a.SendText(`{"action":"echo","topic":"room:lobby","payload":{}}`)
	a.Expect(`{"action":"error","payload":[{"type":"missing","loc":["echo","payload","body"],"msg":"Field required"}],"version":1,"topic":"room:lobby"}`)

	// From outside, with seq.
	if err := chanx.BroadcastTopic[RoomTopic](context.Background(), e.Publisher(), "room:lobby", RoomEvent{Payload: PostPayload{Body: "ext"}}, chanx.Seq(7)); err != nil {
		t.Fatal(err)
	}
	a.Expect(`{"action":"room_event","payload":{"body":"ext","author":"","likes":0},"version":1,"topic":"room:lobby","seq":7}`)
	b.Expect(`{"action":"room_event","payload":{"body":"ext","author":"","likes":0},"version":1,"topic":"room:lobby","seq":7}`)

	b.Unsubscribe("room:lobby")
	if got := <-room.left; got != "room:lobby" {
		t.Fatal(got)
	}
	b.Unsubscribe("room:lobby") // twice is fine
	a.SendText(`{"action":"post","topic":"room:lobby","payload":{"body":"again"}}`)
	a.ReceiveTopic("room:lobby")
	b.ExpectNothing(100 * time.Millisecond)

	// Disconnecting runs OnUnsubscribe.
	a.Close()
	select {
	case got := <-room.left:
		if got != "room:lobby" {
			t.Fatal(got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnUnsubscribe not run on disconnect")
	}
}

func TestDedicatedTopicRoute(t *testing.T) {
	e, _ := newEngine(t, chanx.Config{})
	c := chanxtest.Connect(t, e, "/ws/topics/room/lobby")
	c.Expect(`{"action":"subscribed","payload":null,"version":1,"topic":"room:lobby"}`)
	c.ReceiveTopic("room:lobby") // roster
	c.SendText(`{"action":"echo","ref":"1","payload":{"body":"x"}}`)
	c.Expect(`{"action":"posted","payload":{"body":"x","author":"anonymous","likes":0},"version":1,"topic":"room:lobby","ref":"1"}`)

	hub := chanxtest.Connect(t, e, "/ws/topics")
	hub.Subscribe("room:lobby")
	hub.ReceiveTopic("room:lobby")
	c.SendText(`{"action":"post","payload":{"body":"both"}}`)
	c.ReceiveTopic("room:lobby")
	hub.ReceiveTopic("room:lobby")
}

func TestCompletion(t *testing.T) {
	e := chanx.New(chanx.Config{SendCompletion: true})
	e.Handle("/ws/chat/{room}", &ChatConsumer{Banner: "hi"})
	c := chanxtest.Connect(t, e, "/ws/chat/x")
	c.Receive() // announcement
	frames := c.Request(chanx.PingMessage{})
	if len(frames) != 1 || frames[0].Action() != "pong" {
		t.Fatalf("got %v", frames)
	}
	if err := e.Publisher().BroadcastEvent(context.Background(), AnnouncementEvent{Payload: AnnouncementPayload{Text: "x"}}, "room.x"); err != nil {
		t.Fatal(err)
	}
	c.Expect(`{"action":"announcement","payload":{"text":"x"}}`)
	c.Expect(`{"action":"event_complete","payload":null}`)
}

type CamelPayload struct {
	RoomName string `json:"room_name"`
}

type CamelMessage struct {
	chanx.BaseMessage `action:"camel"`
	Payload           CamelPayload `json:"payload"`
}

func TestCamelize(t *testing.T) {
	hub := chanx.NewConsumer("camel")
	chanx.On(hub, func(ctx *chanx.Context, m CamelMessage) (CamelMessage, error) { return m, nil })
	e := chanx.New(chanx.Config{Camelize: true})
	e.Handle("/ws", hub)
	c := chanxtest.Connect(t, e, "/ws")
	c.SendText(`{"action":"camel","payload":{"roomName":"x"}}`)
	c.Expect(`{"action":"camel","payload":{"roomName":"x"}}`)
}

func TestGenericConsumer(t *testing.T) {
	hub := chanx.NewConsumer("generic")
	chanx.On(hub, func(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
		return chanx.PongMessage{}, nil
	})
	chanx.Handle(hub, func(ctx *chanx.Context, m ChatMessage) error { return errors.New("boom") })
	chanx.OnEvent(hub, func(ctx *chanx.EventContext, e AnnouncementEvent) (AnnouncementMessage, error) {
		return AnnouncementMessage{Payload: e.Payload}, nil
	})
	hub.Groups = func(*chanx.Conn) []string { return []string{"g"} }
	e := chanx.New(chanx.Config{})
	e.Handle("/ws", hub)
	c := chanxtest.Connect(t, e, "/ws")
	c.SendText(`{"action":"ping","ref":"x"}`)
	c.Expect(`{"action":"pong","payload":null,"version":1,"ref":"x"}`)
	c.SendText(`{"action":"chat","payload":{"message":"m"}}`)
	c.Expect(`{"action":"error","payload":{"detail":"Failed to process message for chat"}}`)
	_ = e.Publisher().BroadcastEvent(context.Background(), AnnouncementEvent{Payload: AnnouncementPayload{Text: "t"}}, "g")
	c.Expect(`{"action":"announcement","payload":{"text":"t"}}`)
}

type single struct{}

func (single) Chat(ctx *chanx.Context, m ChatMessage) error { panic("kaboom") }

type empty struct{}

func TestSingleHandlerAndPanics(t *testing.T) {
	e := chanx.New(chanx.Config{})
	e.Handle("/ws/single", single{})
	e.Handle("/ws/empty", empty{})
	c := chanxtest.Connect(t, e, "/ws/single")
	c.SendText(`{"action":"nope","payload":{}}`)
	c.Expect(`{"action":"error","payload":[{"type":"literal_error","loc":["action"],"msg":"Input should be 'chat'"},{"type":"missing","loc":["payload","message"],"msg":"Field required"}]}`)
	c.SendText(`{"payload":{"message":"a"}}`) // with one handler, the action may be left out
	c.Expect(`{"action":"error","payload":{"detail":"Failed to process message for chat"}}`)
	d := chanxtest.Connect(t, e, "/ws/empty")
	d.SendText(`{"action":"x"}`)
	d.Expect(`{"action":"error","payload":{"detail":"Failed to process message"}}`)
}

type badConsumer struct{}

type One struct {
	chanx.BaseMessage `action:"one"`
}
type AlsoOne struct {
	chanx.BaseMessage `action:"one"`
}

func (badConsumer) A(ctx *chanx.Context, _ One) error     { return nil }
func (badConsumer) B(ctx *chanx.Context, _ AlsoOne) error { return nil }

type wrongSignature struct{}

func (wrongSignature) Oops(ctx *chanx.Context, s string) error { return nil }

type wrongHook struct{}

func (wrongHook) Groups() []string { return nil }

type noPattern struct{ chanx.BaseTopic }

type pointerHandlers struct{}

func (*pointerHandlers) HandleOne(ctx *chanx.Context, _ One) error { return nil }

type pointerHook struct{}

func (*pointerHook) OnConnect(ctx *chanx.Context) error { return nil }

type opsOnHook struct{ pingOnly }

func (opsOnHook) OnConnect(ctx *chanx.Context) error { return nil }

func (opsOnHook) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{"OnConnect": {Summary: "s"}}
}

type opsOnPlainMethod struct{ pingOnly }

func (opsOnPlainMethod) Helper() {}

func (opsOnPlainMethod) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{"Helper": {Summary: "s"}}
}

func TestCheck(t *testing.T) {
	for _, ok := range []any{&ChatConsumer{}, &pointerHandlers{}, &pointerHook{}} {
		if err := chanx.Check(ok); err != nil {
			t.Fatal(err)
		}
	}
	for v, want := range map[any]string{
		badConsumer{}:      `two message handlers for action "one": A() and B()`,
		wrongSignature{}:   "second parameter must be a message type",
		wrongHook{}:        "Groups is a reserved hook name",
		&noPattern{}:       "without a `pattern` tag",
		pointerHandlers{}:  "pointerHandlers.HandleOne has a pointer receiver",
		pointerHook{}:      "pointerHook.OnConnect has a pointer receiver",
		opsOnHook{}:        `Operations names "OnConnect", which is not a handler method`,
		opsOnPlainMethod{}: `Operations names "Helper", which is not a handler method`,
	} {
		if err := chanx.Check(v); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%T: got %v, want %q", v, err, want)
		}
	}
	e := chanx.New(chanx.Config{})
	func() {
		defer func() {
			if recover() == nil {
				t.Error("unknown layer accepted")
			}
		}()
		e.Handle("/x", &ChatConsumer{}, chanx.Layer("nope"))
	}()
}

func TestSpy(t *testing.T) {
	spy := chanxtest.Spy(inmemory.New())
	e := chanx.New(chanx.Config{Layers: layers.Set{"default": spy}})
	ctx := context.Background()
	pub := e.Publisher()
	_ = chanx.BroadcastTopic[RoomTopic](ctx, pub, "room:a", RoomEvent{Payload: PostPayload{Body: "x", Likes: 4}}, chanx.Seq(2))
	_ = pub.BroadcastEvent(ctx, AnnouncementEvent{Payload: AnnouncementPayload{Text: "hi"}}, "g1", "g2")
	_ = pub.SendEvent(ctx, "specific.abc", AnnouncementEvent{Payload: AnnouncementPayload{Text: "direct"}})
	_ = pub.BroadcastEvent(ctx, ChatMessage{Payload: ChatPayload{Message: "other type"}}, "g1")

	topics := chanxtest.TopicBroadcasts[RoomTopic, RoomEvent](spy)
	if len(topics) != 1 || topics[0].Topic != "room:a" || *topics[0].Seq != 2 || topics[0].Event.Payload.Likes != 4 {
		t.Fatalf("%+v", topics)
	}
	events := chanxtest.BroadcastEvents[AnnouncementEvent](spy)
	if len(events) != 3 || events[0].Group != "g1" || events[1].Group != "g2" || events[2].Channel != "specific.abc" ||
		events[0].Event.Payload.Text != "hi" || events[2].Event.Payload.Text != "direct" {
		t.Fatalf("%+v", events)
	}
	if n := len(chanxtest.BroadcastEvents[ChatMessage](spy)); n != 1 {
		t.Fatalf("other event type: %d", n)
	}
	// The group name is the topic in the topic type's namespace.
	if g := spy.All()[0].Group; g != "RoomTopic.room-a" {
		t.Fatal(g)
	}
	spy.Reset()
	if len(spy.All()) != 0 {
		t.Fatal("Reset kept records")
	}
}

// Suppress records publishes without delivering them.
func TestSpySuppress(t *testing.T) {
	spy := chanxtest.Spy(inmemory.New(), chanxtest.Suppress())
	e := chanx.New(chanx.Config{Layers: layers.Set{"default": spy}})
	e.Handle("/ws/chat/{room}", &ChatConsumer{Banner: "hi"})
	c := chanxtest.Connect(t, e, "/ws/chat/x")
	c.Receive() // announcement
	_ = e.Publisher().BroadcastEvent(context.Background(), AnnouncementEvent{Payload: AnnouncementPayload{Text: "quiet"}}, "room.x")
	c.ExpectNothing(150 * time.Millisecond)
	if got := chanxtest.BroadcastEvents[AnnouncementEvent](spy); len(got) != 1 || got[0].Event.Payload.Text != "quiet" {
		t.Fatalf("%+v", got)
	}
}

func TestReceiveUntil(t *testing.T) {
	e := chanx.New(chanx.Config{SendCompletion: true})
	e.Handle("/ws/chat/{room}", &ChatConsumer{Banner: "hi"})
	c := chanxtest.Connect(t, e, "/ws/chat/x")
	c.Receive() // announcement

	c.SendText(`{"action":"ping","ref":"1"}`)
	frames := c.ReceiveUntil()
	if len(frames) != 2 || frames[0].Action() != "pong" || frames[1].Action() != "complete" {
		t.Fatalf("%v", frames)
	}

	// Layer messages keep their order: the event, then the group message.
	pub := e.Publisher()
	_ = pub.BroadcastEvent(context.Background(), AnnouncementEvent{Payload: AnnouncementPayload{Text: "news"}}, "room.x")
	_ = pub.BroadcastMessage(context.Background(), []string{"room.x"}, ChatNotification{Payload: ChatNotificationPayload{Room: "x", Message: "m"}})
	note, before := chanxtest.ReceiveUntilMessage[ChatNotification](c)
	if note.Payload.Message != "m" || len(before) != 2 || before[0].Action() != "announcement" || before[1].Action() != "event_complete" {
		t.Fatalf("%+v, frames before it: %v", note, before)
	}
	c.ReceiveUntil("group_complete")

	_ = e.Publisher().BroadcastEvent(context.Background(), AnnouncementEvent{Payload: AnnouncementPayload{Text: "typed"}}, "room.x")
	ann, _ := chanxtest.ReceiveUntilMessage[AnnouncementMessage](c)
	if ann.Payload.Text != "typed" {
		t.Fatalf("%+v", ann)
	}
}

func TestShutdown(t *testing.T) {
	e := chanx.New(chanx.Config{})
	e.Handle("/ws/chat/{room}", &ChatConsumer{})
	c := chanxtest.Connect(t, e, "/ws/chat/x")
	c.Receive()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if code := c.WaitClosed(); code != 1001 {
		t.Fatalf("close code %d", code)
	}
}
