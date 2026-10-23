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
	"github.com/huynguyengl99/chanx-go/transport"
)

type pingOnly struct{}

func (pingOnly) Ping(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

func TestMaxFrameSize(t *testing.T) {
	e := chanx.New(chanx.Config{MaxFrameSize: 64})
	e.Handle("/ws", pingOnly{})
	c := chanxtest.Connect(t, e, "/ws")
	c.SendText(`{"action":"ping"}`)
	c.Expect(`{"action":"pong","payload":null}`)
	c.SendText(`{"action":"ping","ref":"` + strings.Repeat("x", 100) + `"}`)
	if code := c.WaitClosed(); code != transport.StatusMessageTooBig {
		t.Fatalf("close code %d, want 1009", code)
	}
}

// A connection served while Shutdown starts is either refused or closed by it.
func TestShutdownWhileConnecting(t *testing.T) {
	for range 200 {
		e := chanx.New(chanx.Config{})
		r := e.Handle("/ws", pingOnly{})
		server, client := transport.Pipe()
		served := make(chan struct{})
		go func() {
			defer close(served)
			e.Serve(context.Background(), r, server, &chanx.Request{Path: "/ws"})
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := e.Shutdown(ctx)
		cancel()
		if err != nil {
			t.Fatalf("Shutdown left a connection open: %v", err)
		}
		<-served
		_ = client.Close(transport.StatusNormalClosure, "")
	}
}

type preflightConsumer struct{ pingOnly }

func (preflightConsumer) Authenticate(ctx *chanx.Context, r *chanx.Request) (any, error) {
	ctx.Logger().Info("authenticating")
	if ctx.Conn.Context() == nil || ctx.Publisher() == nil {
		return nil, errors.New("incomplete context")
	}
	return "ann", nil
}

func TestPreflightContext(t *testing.T) {
	e := chanx.New(chanx.Config{RejectBeforeUpgrade: true})
	e.Handle("/ws", preflightConsumer{})
	c, err := chanxtest.Dial(t, e, "/ws")
	if err != nil {
		t.Fatalf("pre-upgrade Authenticate could not use its context: %v", err)
	}
	if c.Auth.Action() != "authentication" {
		t.Fatal(c.Auth)
	}
}

type slowJoiner struct{ joined chan error }

type JoinLater struct {
	chanx.BaseMessage `action:"join_later"`
}

func (s slowJoiner) Join(ctx *chanx.Context, _ JoinLater) error {
	time.Sleep(300 * time.Millisecond) // past DisconnectTimeout
	s.joined <- ctx.Conn.Join(context.Background(), "late")
	return nil
}

// A handler outliving the disconnect timeout cannot rejoin after cleanup.
func TestJoinAfterTeardown(t *testing.T) {
	s := slowJoiner{joined: make(chan error, 1)}
	e := chanx.New(chanx.Config{DisconnectTimeout: 50 * time.Millisecond})
	e.Handle("/ws", s)
	c := chanxtest.Connect(t, e, "/ws")
	c.SendText(`{"action":"join_later"}`)
	time.Sleep(20 * time.Millisecond)
	c.Close()
	if err := <-s.joined; !errors.Is(err, chanx.ErrClosed) {
		t.Fatalf("Join after teardown = %v, want ErrClosed", err)
	}
}

type AlertTopic struct {
	chanx.BaseTopic `pattern:"alert:{name}" layer:"other"`
}

func (AlertTopic) Passthrough() []chanx.Message { return []chanx.Message{AlertEvent{}} }

type AlertEvent struct {
	chanx.BaseMessage `action:"alert"`
	Payload           string `json:"payload"`
}

type alertHub struct{}

func (alertHub) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{&AlertTopic{}} }

type Raise struct {
	chanx.BaseMessage `action:"raise"`
}

type raiser struct{}

func (raiser) Raise(ctx *chanx.Context, _ Raise) error {
	return chanx.BroadcastTopic[AlertTopic](ctx, ctx.Publisher(), "alert:fire", AlertEvent{Payload: "fire"})
}

// ctx.Publisher reaches every layer: a topic's layer tag picks its own.
func TestContextPublisherLayers(t *testing.T) {
	e := chanx.New(chanx.Config{Layers: layers.Set{"default": inmemory.New(), "other": inmemory.New()}})
	e.Handle("/ws/alerts", alertHub{}, chanx.Layer("other"))
	e.Handle("/ws/raise", raiser{})
	sub := chanxtest.Connect(t, e, "/ws/alerts")
	sub.Subscribe("alert:fire")
	r := chanxtest.Connect(t, e, "/ws/raise")
	r.SendText(`{"action":"raise"}`)
	sub.Expect(`{"action":"alert","payload":"fire","version":1,"topic":"alert:fire"}`)
}

func TestWildcardRoutesRefused(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(error).Error(), "{rest...}") {
			t.Fatalf("got %v", r)
		}
	}()
	chanx.New(chanx.Config{}).Handle("/ws/{rest...}", pingOnly{})
}

type adminConsumer struct{}

type WhoMessage struct {
	chanx.BaseMessage `action:"who"`
}

type IAm struct {
	chanx.BaseMessage `action:"i_am"`
	Payload           string `json:"payload"`
}

func (adminConsumer) Who(ctx *chanx.Context, _ WhoMessage) (IAm, error) {
	return IAm{Payload: "admin"}, nil
}

type roomConsumer struct{}

func (roomConsumer) Who(ctx *chanx.Context, _ WhoMessage) (IAm, error) {
	return IAm{Payload: "room " + ctx.Conn.Param("room")}, nil
}

func TestLookupMostSpecific(t *testing.T) {
	e := chanx.New(chanx.Config{})
	e.Handle("/ws/{room}", roomConsumer{})
	e.Handle("/ws/admin", adminConsumer{})
	for path, want := range map[string]string{"/ws/admin": "admin", "/ws/lobby": "room lobby"} {
		c := chanxtest.Connect(t, e, path)
		c.SendText(`{"action":"who"}`)
		c.Expect(`{"action":"i_am","payload":"` + want + `"}`)
	}
	if _, _, ok := e.Lookup("/ws/a/b"); ok {
		t.Fatal("a parameter matched two segments")
	}
}

type BodyPayload struct {
	Text string `json:"text"`
}

// A payload field with another Go name, tagged "payload".
type Renamed struct {
	chanx.BaseMessage `action:"renamed"`
	Body              BodyPayload `json:"payload"`
}

func TestRenamedPayloadField(t *testing.T) {
	hub := chanx.NewConsumer("renamed")
	chanx.On(hub, func(ctx *chanx.Context, m Renamed) (Renamed, error) { return m, nil })
	e := chanx.New(chanx.Config{})
	e.Handle("/ws", hub)
	c := chanxtest.Connect(t, e, "/ws")
	c.Send(Renamed{Body: BodyPayload{Text: "hi"}})
	var got Renamed
	if err := c.Receive().Decode(&got); err != nil || got.Body.Text != "hi" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestEmitsSetAfterHandlers(t *testing.T) {
	hub := chanx.NewConsumer("late_emits")
	chanx.On(hub, func(ctx *chanx.Context, _ chanx.PingMessage) (chanx.Message, error) { return chanx.PongMessage{}, nil })
	hub.Emits = []chanx.Message{chanx.PongMessage{}}
	r := chanx.New(chanx.Config{}).Handle("/ws", hub)
	outs := r.Info().Consumer.Handlers[0].Outputs
	if len(outs) != 1 || outs[0].Action != "pong" {
		t.Fatalf("documented replies: %v", outs)
	}
}

type SlowMessage struct {
	chanx.BaseMessage `action:"slow"`
}

type FastMessage struct {
	chanx.BaseMessage `action:"fast"`
}

type Done struct {
	chanx.BaseMessage `action:"done"`
	Payload           string `json:"payload"`
}

type ordered struct{}

func (ordered) Slow(ctx *chanx.Context, _ SlowMessage) (Done, error) {
	time.Sleep(100 * time.Millisecond)
	return Done{Payload: "slow"}, nil
}

func (ordered) Fast(ctx *chanx.Context, _ FastMessage) (Done, error) {
	return Done{Payload: "fast"}, nil
}

func replies(t *testing.T, e *chanx.Engine) string {
	t.Helper()
	c := chanxtest.Connect(t, e, "/ws")
	c.SendText(`{"action":"slow"}`)
	c.SendText(`{"action":"fast"}`)
	var out []string
	for range 2 {
		out = append(out, c.Receive().Payload().(string))
	}
	return strings.Join(out, ",")
}

// Frames run concurrently by default; Sequential and
// MaxInFlight(1) keep their order.
func TestConcurrencyOptions(t *testing.T) {
	for name, c := range map[string]struct {
		opts []chanx.Option
		cfg  chanx.Config
		want string
	}{
		"default":       {want: "fast,slow"},
		"sequential":    {opts: []chanx.Option{chanx.Sequential()}, want: "slow,fast"},
		"max in flight": {opts: []chanx.Option{chanx.MaxInFlight(1)}, want: "slow,fast"},
		"config max":    {cfg: chanx.Config{MaxInFlight: 1}, want: "slow,fast"},
	} {
		t.Run(name, func(t *testing.T) {
			e := chanx.New(c.cfg)
			e.Handle("/ws", ordered{}, c.opts...)
			if got := replies(t, e); got != c.want {
				t.Fatalf("replies %s, want %s", got, c.want)
			}
		})
	}
}

type Notify struct {
	chanx.BaseMessage `action:"notify"`
	Payload           AnnouncementPayload `json:"payload"`
}

type eventErrors struct{ errs chan string }

func (eventErrors) Groups(*chanx.Conn) []string { return []string{"g"} }

func (eventErrors) OnNotify(ctx *chanx.EventContext, e Notify) (AnnouncementMessage, error) {
	if e.Payload.Text == "bad" {
		return AnnouncementMessage{}, errors.New("rejected")
	}
	return AnnouncementMessage{Payload: e.Payload}, nil
}

func (s eventErrors) OnEventError(ctx *chanx.EventContext, action string, err error) {
	s.errs <- action + ": " + err.Error()
}

func TestEventErrors(t *testing.T) {
	s := eventErrors{errs: make(chan string, 1)}
	e := chanx.New(chanx.Config{SendCompletion: true})
	e.Handle("/ws", s)
	c := chanxtest.Connect(t, e, "/ws")
	pub := e.Publisher()
	ctx := context.Background()

	// An invalid event is logged, never sent; nothing reaches the client.
	if err := pub.BroadcastMessage(ctx, []string{"g"}, nil); err == nil {
		t.Fatal("a nil message was accepted")
	}
	invalid := layers.Message{"type": "handle_channel_event", "event_data": map[string]any{"action": "notify", "payload": map[string]any{"text": 5}}}
	if err := e.Layers()["default"].GroupSend(ctx, "g", invalid); err != nil {
		t.Fatal(err)
	}
	c.ExpectNothing(150 * time.Millisecond)

	// A handler error goes to OnEventError; completion still follows.
	_ = pub.BroadcastEvent(ctx, Notify{Payload: AnnouncementPayload{Text: "bad"}}, "g")
	if got := <-s.errs; got != "notify: rejected" {
		t.Fatal(got)
	}
	c.Expect(`{"action":"event_complete","payload":null}`)

	_ = pub.BroadcastEvent(ctx, Notify{Payload: AnnouncementPayload{Text: "ok"}}, "g")
	c.Expect(`{"action":"announcement","payload":{"text":"ok"}}`)
	c.Expect(`{"action":"event_complete","payload":null}`)
}

func TestLaxEnvelopeIntegers(t *testing.T) {
	e := chanx.New(chanx.Config{})
	e.Handle("/ws", pingOnly{})
	c := chanxtest.Connect(t, e, "/ws")
	c.SendText(`{"action":"ping","ref":"a","version":true,"seq":" 7 "}`)
	c.Expect(`{"action":"pong","payload":null,"version":1,"ref":"a"}`)
	c.SendText(`{"action":"ping","seq":[1]}`)
	c.Expect(`{"action":"error","payload":[{"type":"int_type","loc":["seq"],"msg":"Input should be a valid integer"}]}`)
	c.SendText(`{"action":"ping","topic":5}`)
	c.Expect(`{"action":"error","payload":[{"type":"string_type","loc":["topic"],"msg":"Input should be a valid string"}]}`)
}

func TestFunctionConsumer(t *testing.T) {
	room := &RoomTopic{}
	hub := chanx.NewConsumer("fn_hub")
	chanx.On(hub, func(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
		return chanx.PongMessage{}, nil
	},
		chanx.Summary("ping"), chanx.OperationDescription("d"), chanx.OperationTags("t"), chanx.OperationName("op_ping"))
	chanx.HandleEvent(hub, func(ctx *chanx.EventContext, e AnnouncementEvent) error {
		return ctx.Push(AnnouncementMessage{Payload: e.Payload})
	}, chanx.Replies(AnnouncementMessage{}))
	hub.Passthrough = []chanx.Message{ChatNotification{}}
	hub.Topics = []chanx.TopicHandler{room}
	hub.Groups = func(*chanx.Conn) []string { return []string{"fn"} }
	e := chanx.New(chanx.Config{})
	r := e.Handle("/ws", hub)
	info := r.Info()
	if h := info.Consumer.Handlers[0]; h.Operation != "op_ping" || h.Summary != "ping" || h.Description != "d" || h.Tags[0] != "t" {
		t.Fatalf("%+v", h)
	}
	if len(info.Consumer.Events) != 2 || len(info.Topics) != 1 {
		t.Fatalf("events %d topics %d", len(info.Consumer.Events), len(info.Topics))
	}
	c := chanxtest.Connect(t, e, "/ws")
	_ = e.Publisher().BroadcastEvent(context.Background(), ChatNotification{Payload: ChatNotificationPayload{Room: "r", Message: "m"}}, "fn")
	c.Expect(`{"action":"chat_notification","payload":{"room":"r","message":"m"}}`)
	_ = e.Publisher().BroadcastEvent(context.Background(), AnnouncementEvent{Payload: AnnouncementPayload{Text: "x"}}, "fn")
	c.Expect(`{"action":"announcement","payload":{"text":"x"}}`)
	c.Subscribe("room:lobby")

	dup := chanx.NewConsumer("dup")
	chanx.Handle(dup, func(*chanx.Context, chanx.PingMessage) error { return nil })
	chanx.Handle(dup, func(*chanx.Context, chanx.PingMessage) error { return nil })
	if err := chanx.Check(dup); err == nil || !strings.Contains(err.Error(), `two message handlers for action "ping"`) {
		t.Fatalf("duplicate: %v", err)
	}
	twice := chanx.NewConsumer("twice")
	twice.Topics = []chanx.TopicHandler{&RoomTopic{}, &RoomTopic{}}
	if err := chanx.Check(twice); err == nil || !strings.Contains(err.Error(), "mounts topic pattern") {
		t.Fatalf("topic mounted twice: %v", err)
	}
	if err := chanx.Check(chanx.NewConsumer("")); err == nil {
		t.Fatal("a consumer without a name was accepted")
	}
}

type hooked struct{ pingOnly }

type Boom struct {
	chanx.BaseMessage `action:"boom"`
}

func (hooked) Boom(ctx *chanx.Context, _ Boom) error { return errors.New("kaboom") }

func (hooked) OnValidationError(ctx *chanx.Context, issues []chanx.Issue) {
	_ = ctx.Send(Done{Payload: "consumer validation: " + issues[0].Type})
}

func (hooked) OnHandlerError(ctx *chanx.Context, action string, err error) {
	_ = ctx.Send(Done{Payload: "consumer handler: " + action + ": " + err.Error()})
}

type plainBoom struct{ pingOnly }

func (plainBoom) Boom(ctx *chanx.Context, _ Boom) error { return errors.New("kaboom") }

func (plainBoom) Groups(*chanx.Conn) []string { return []string{"pb"} }

func (plainBoom) OnNotify(ctx *chanx.EventContext, e Notify) error { return errors.New("event failed") }

// The consumer's hooks win over Config's, which win over the default frames.
func TestErrorHooks(t *testing.T) {
	events := make(chan string, 1)
	e := chanx.New(chanx.Config{
		OnValidationError: func(ctx *chanx.Context, issues []chanx.Issue) {
			_ = ctx.Send(Done{Payload: "config validation: " + issues[0].Type})
		},
		OnHandlerError: func(ctx *chanx.Context, action string, err error) {
			_ = ctx.Send(Done{Payload: "config handler: " + action})
		},
		OnEventError: func(ctx *chanx.EventContext, action string, err error) { events <- action + ": " + err.Error() },
	})
	e.Handle("/ws/consumer", hooked{})
	e.Handle("/ws/config", plainBoom{})

	c := chanxtest.Connect(t, e, "/ws/consumer", chanxtest.Lenient())
	c.SendText(`{"action":"nope"}`)
	c.Expect(`{"action":"done","payload":"consumer validation: union_tag_invalid"}`)
	c.SendText(`{"action":"boom"}`)
	c.Expect(`{"action":"done","payload":"consumer handler: boom: kaboom"}`)

	d := chanxtest.Connect(t, e, "/ws/config", chanxtest.Lenient())
	d.SendText(`{"action":"nope"}`)
	d.Expect(`{"action":"done","payload":"config validation: union_tag_invalid"}`)
	d.SendText(`{"action":"boom"}`)
	d.Expect(`{"action":"done","payload":"config handler: boom"}`)
	_ = e.Publisher().BroadcastEvent(context.Background(), Notify{Payload: AnnouncementPayload{Text: "x"}}, "pb")
	select {
	case got := <-events:
		if got != "notify: event failed" {
			t.Fatal(got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Config.OnEventError not called")
	}
}

type panicGroups struct{ pingOnly }

func (panicGroups) Groups(*chanx.Conn) []string { panic("no groups") }

// A panicking hook is logged and never takes the process down: error hooks keep
// the connection, a connect hook closes it with 1011.
func TestPanickingHooks(t *testing.T) {
	e := chanx.New(chanx.Config{
		OnValidationError: func(*chanx.Context, []chanx.Issue) { panic("validation hook") },
		OnHandlerError:    func(*chanx.Context, string, error) { panic("handler hook") },
	})
	e.Handle("/ws", plainBoom{})
	e.Handle("/ws/groups", panicGroups{})

	c := chanxtest.Connect(t, e, "/ws", chanxtest.Lenient())
	c.SendText(`[]`) // rejected on the reader goroutine
	c.SendText(`{"action":"nope"}`)
	c.SendText(`{"action":"boom"}`)
	c.SendText(`{"action":"ping"}`)
	c.Expect(`{"action":"pong","payload":null}`)

	d := chanxtest.Connect(t, e, "/ws/groups")
	if code := d.WaitClosed(); code != 1011 {
		t.Fatalf("close code %d, want 1011", code)
	}
}

// A worker publishes with layers alone; a topic's layer tag picks its layer
// unless the publisher names one.
func TestStandalonePublisher(t *testing.T) {
	set := layers.Set{"default": inmemory.New(), "other": inmemory.New()}
	e := chanx.New(chanx.Config{Layers: set})
	e.Handle("/ws/alerts", alertHub{}, chanx.Layer("other"))
	e.Handle("/ws/chat/{room}", &ChatConsumer{Banner: "hi"})
	chat := chanxtest.Connect(t, e, "/ws/chat/x")
	chat.Receive() // banner
	alerts := chanxtest.Connect(t, e, "/ws/alerts")
	alerts.Subscribe("alert:a")

	worker := chanx.NewPublisher(set)
	ctx := context.Background()
	if err := worker.BroadcastEvent(ctx, AnnouncementEvent{Payload: AnnouncementPayload{Text: "w"}}, "room.x"); err != nil {
		t.Fatal(err)
	}
	chat.Expect(`{"action":"announcement","payload":{"text":"w"}}`)
	if err := chanx.BroadcastTopic[AlertTopic](ctx, worker, "alert:a", AlertEvent{Payload: "tag"}); err != nil {
		t.Fatal(err)
	}
	alerts.Expect(`{"action":"alert","payload":"tag","version":1,"topic":"alert:a"}`)
	// Named explicitly, the default layer is used and the subscriber on "other" hears nothing.
	if err := chanx.BroadcastTopic[AlertTopic](ctx, worker.Layer("default"), "alert:a", AlertEvent{Payload: "x"}); err != nil {
		t.Fatal(err)
	}
	alerts.ExpectNothing(150 * time.Millisecond)
	if err := worker.Layer("missing").BroadcastEvent(ctx, AnnouncementEvent{}, "g"); err == nil {
		t.Fatal("unknown layer accepted")
	}
}

type Store struct {
	chanx.BaseMessage `action:"store"`
	Payload           int `json:"payload"`
}

type Load struct {
	chanx.BaseMessage `action:"load"`
}

type Bye struct {
	chanx.BaseMessage `action:"bye"`
}

type stateful struct{}

func (stateful) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{&RoomTopic{}} }

func (stateful) Store(ctx *chanx.Context, m Store) error {
	ctx.Conn.SetValue("n", m.Payload)
	return nil
}

func (stateful) Load(ctx *chanx.Context, _ Load) (Done, error) {
	n, ok := chanx.ConnValue[int](ctx.Conn, "n")
	_, wrongType := chanx.ConnValue[string](ctx.Conn, "n")
	return Done{Payload: fmt.Sprintf("%d %v %v %v %s", n, ok, wrongType, ctx.Conn.Subscriptions(), ctx.Conn.Request.Param("room"))}, nil
}

func (stateful) Bye(ctx *chanx.Context, _ Bye) error {
	ctx.Close(4000, "bye")
	return nil
}

func TestConnState(t *testing.T) {
	e := chanx.New(chanx.Config{})
	e.Handle("/ws/{room}", stateful{}, chanx.Sequential()) // store before load
	c := chanxtest.Connect(t, e, "/ws/lobby")
	c.Subscribe("room:a")
	c.ReceiveTopic("room:a") // roster
	c.SendText(`{"action":"store","payload":7}`)
	c.SendText(`{"action":"load"}`)
	c.Expect(`{"action":"done","payload":"7 true false [room:a] lobby"}`)
	c.SendText(`{"action":"bye"}`)
	if code := c.WaitClosed(); code != 4000 {
		t.Fatal(code)
	}
}

// Route options override Config; ToGroups targets explicit groups.
func TestRouteOverrides(t *testing.T) {
	e := chanx.New(chanx.Config{SendCompletion: false, Camelize: false})
	e.Handle("/ws/complete", pingOnly{}, chanx.SendCompletion(true))
	e.Handle("/ws/camel", camelEcho{}, chanx.Camelize(true))
	e.Handle("/ws/plain", pingOnly{})
	c := chanxtest.Connect(t, e, "/ws/complete")
	c.SendText(`{"action":"ping"}`)
	c.ReceiveUntil("complete")
	p := chanxtest.Connect(t, e, "/ws/plain")
	p.SendText(`{"action":"ping"}`)
	p.Expect(`{"action":"pong","payload":null}`)
	p.ExpectNothing(100 * time.Millisecond)
	m := chanxtest.Connect(t, e, "/ws/camel")
	m.SendText(`{"action":"camel","payload":{"roomName":"r"}}`)
	m.Expect(`{"action":"camel","payload":{"roomName":"r"}}`)

	g := chanx.New(chanx.Config{})
	g.Handle("/ws/{room}", targeted{})
	a := chanxtest.Connect(t, g, "/ws/a")
	b := chanxtest.Connect(t, g, "/ws/b")
	a.SendText(`{"action":"shout_to","payload":"b"}`)
	b.Expect(`{"action":"done","payload":"from a"}`)
	a.ExpectNothing(100 * time.Millisecond)
}

type camelEcho struct{}

func (camelEcho) Camel(ctx *chanx.Context, m CamelMessage) (CamelMessage, error) { return m, nil }

type ShoutTo struct {
	chanx.BaseMessage `action:"shout_to"`
	Payload           string `json:"payload"`
}

type targeted struct{}

func (targeted) Groups(c *chanx.Conn) []string { return []string{"room." + c.Param("room")} }

func (targeted) ShoutTo(ctx *chanx.Context, m ShoutTo) error {
	return ctx.Broadcast(Done{Payload: "from " + ctx.Conn.Param("room")}, chanx.ToGroups("room."+m.Payload))
}

func (targeted) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{"ShoutTo": {Replies: []chanx.Message{Done{}}}}
}

type Defaults struct {
	Name  string `json:"name" default:"anon"`
	Count int    `json:"count" default:"3"`
}

func TestSmallHelpers(t *testing.T) {
	var d Defaults
	chanx.ApplyDefaults(&d)
	if d.Name != "anon" || d.Count != 3 {
		t.Fatalf("%+v", d)
	}
	if transport.CloseCode(&transport.CloseError{Code: 1009}) != 1009 || transport.CloseCode(errors.New("x")) != -1 {
		t.Fatal("CloseCode")
	}
	if chanx.ActionOf(chanx.PingMessage{}) != "ping" || chanx.ActionOf(nil) != "" {
		t.Fatal("ActionOf")
	}
	var ae *chanx.AuthError
	if !errors.As(chanx.Forbidden(""), &ae) || ae.Status != 403 || ae.Detail != "Forbidden" || ae.Error() != "chanx: 403 Forbidden" {
		t.Fatalf("%+v", ae)
	}
}
