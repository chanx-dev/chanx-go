package chanxtest_test

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"go.chanx.dev/layers/v2"
	"go.chanx.dev/layers/v2/inmemory"
	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"
)

// recorder is a testing.TB that records failures instead of failing, so the
// helpers' own failure paths can be checked. Fatal stops the goroutine, as the
// real one does.
type recorder struct {
	testing.TB
	mu     sync.Mutex
	failed []string
}

func (r *recorder) Helper() {}

func (r *recorder) add(msg string) {
	r.mu.Lock()
	r.failed = append(r.failed, msg)
	r.mu.Unlock()
}

func (r *recorder) Errorf(format string, args ...any) { r.add(fmt.Sprintf(format, args...)) }
func (r *recorder) Error(args ...any)                 { r.add(fmt.Sprint(args...)) }
func (r *recorder) Fatalf(format string, args ...any) {
	r.add(fmt.Sprintf(format, args...))
	runtime.Goexit()
}
func (r *recorder) Fatal(args ...any) { r.add(fmt.Sprint(args...)); runtime.Goexit() }

// failures runs fn with a recorder and returns what it reported.
func failures(t *testing.T, fn func(tb testing.TB)) []string {
	t.Helper()
	r := &recorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(r)
	}()
	<-done
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failed
}

func wantFailure(t *testing.T, got []string, substr string) {
	t.Helper()
	for _, f := range got {
		if strings.Contains(f, substr) {
			return
		}
	}
	t.Fatalf("want a failure containing %q, got %q", substr, got)
}

func shortTimeout(t *testing.T) {
	old := chanxtest.Timeout
	chanxtest.Timeout = 200 * time.Millisecond
	t.Cleanup(func() { chanxtest.Timeout = old })
}

type userKey struct{}

type InfoMessage struct {
	chanx.BaseMessage `action:"info"`
}

type InfoPayload struct {
	Room   string `json:"room"`
	Query  string `json:"query"`
	Header string `json:"header"`
	Cookie string `json:"cookie"`
	Value  string `json:"value"`
	IP     string `json:"ip"`
}

type InfoReply struct {
	chanx.BaseMessage `action:"info_reply"`
	Payload           InfoPayload `json:"payload"`
}

type Undeclared struct {
	chanx.BaseMessage `action:"undeclared"`
}

type ShoutMessage struct {
	chanx.BaseMessage `action:"shout"`
}

type Echo struct {
	chanx.BaseMessage `action:"echo"`
	Payload           string `json:"payload"`
}

type infoConsumer struct{}

func (infoConsumer) Groups(c *chanx.Conn) []string { return []string{"room." + c.Param("room")} }

func (infoConsumer) Info(ctx *chanx.Context, _ InfoMessage) (InfoReply, error) {
	r := ctx.Conn.Request
	p := InfoPayload{Room: ctx.Conn.Param("room"), Query: r.Query.Get("q"), Header: r.Header.Get("X-Test"), IP: r.ClientIP}
	if c, err := r.Cookie("session"); err == nil {
		p.Cookie = c.Value
	}
	p.Value, _ = r.Value(userKey{}).(string)
	return InfoReply{Payload: p}, nil
}

// Ping replies with an action no handler declares, for strict mode.
func (infoConsumer) Ping(ctx *chanx.Context, _ chanx.PingMessage) error {
	return ctx.Send(Undeclared{})
}

func (infoConsumer) Shout(ctx *chanx.Context, _ ShoutMessage) error {
	return ctx.Broadcast(Echo{Payload: "shout"})
}

func (infoConsumer) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{"Shout": {Replies: []chanx.Message{Echo{}}}}
}

type authConsumer struct{}

func (authConsumer) Authenticate(ctx *chanx.Context, r *chanx.Request) (any, error) {
	if r.Header.Get("Authorization") == "" {
		return nil, chanx.Unauthenticated("")
	}
	return "u", nil
}

func (authConsumer) Echo(ctx *chanx.Context, m Echo) (Echo, error) { return m, nil }

type RoomTopic struct {
	chanx.BaseTopic `pattern:"room:{name}"`
}

func (RoomTopic) Echo(ctx *chanx.Context, m Echo) (Echo, error) { return m, nil }

type hub struct{}

func (hub) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{&RoomTopic{}} }

func engine(t *testing.T, set layers.Set) *chanx.Engine {
	e := chanx.New(chanx.Config{Layers: set})
	e.Handle("/ws/info/{room}", infoConsumer{})
	e.Handle("/ws/auth", authConsumer{})
	e.Handle("/ws/hub", hub{})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = e.Shutdown(ctx)
	})
	return e
}

func TestHandshakeOptions(t *testing.T) {
	e := engine(t, nil)
	c := chanxtest.Connect(t, e, "/ws/info/lobby?q=search",
		chanxtest.Header("X-Test", "h"), chanxtest.Cookie("session", "s"),
		chanxtest.Value(userKey{}, "ann"), chanxtest.ClientIP("10.0.0.7"))
	c.Send(InfoMessage{})
	c.Expect(`{"action":"info_reply","payload":{"room":"lobby","query":"search","header":"h","cookie":"s","value":"ann","ip":"10.0.0.7"}}`)

	d := chanxtest.Connect(t, e, "/ws/info/x", chanxtest.Query("q", "opt"))
	d.Send(InfoMessage{})
	var reply InfoReply
	if err := d.Receive().Decode(&reply); err != nil || reply.Payload.Query != "opt" || reply.Payload.IP != "127.0.0.1" {
		t.Fatalf("%+v %v", reply, err)
	}
}

func TestStrictMode(t *testing.T) {
	e := engine(t, nil)
	got := failures(t, func(tb testing.TB) {
		c := chanxtest.Connect(tb, e, "/ws/info/x")
		c.Send(chanx.PingMessage{})
		c.Receive()
	})
	wantFailure(t, got, `sent "undeclared", which no handler declares`)

	c := chanxtest.Connect(t, e, "/ws/info/x", chanxtest.Lenient())
	c.Send(chanx.PingMessage{})
	if f := c.Receive(); f.Action() != "undeclared" {
		t.Fatal(f)
	}
}

func TestAuthentication(t *testing.T) {
	e := engine(t, nil)
	c := chanxtest.Connect(t, e, "/ws/auth", chanxtest.Header("Authorization", "x"))
	if !chanxtest.JSONEqual(c.Auth.String(), `{"action":"authentication","payload":{"status_code":200,"status_text":"OK","data":null}}`) {
		t.Fatal(c.Auth)
	}
	// Connect stores a failed authentication too; the test decides.
	d := chanxtest.Connect(t, e, "/ws/auth")
	if d.Auth.Payload().(map[string]any)["status_text"] != "Unauthorized" || d.WaitClosed() != 4401 {
		t.Fatal(d.Auth)
	}
	s := chanxtest.Connect(t, e, "/ws/auth", chanxtest.Header("Authorization", "x"), chanxtest.SkipAuthentication())
	if s.Auth != nil {
		t.Fatal("SkipAuthentication still read the message")
	}
	if f := s.Receive(); f.Action() != "authentication" {
		t.Fatal(f)
	}
}

func TestTopicOption(t *testing.T) {
	e := engine(t, nil)
	c := chanxtest.Connect(t, e, "/ws/hub")
	c.Send(chanx.SubscribeMessage{}, chanxtest.Topic("room:a"), chanxtest.Ref("s"))
	c.Expect(`{"action":"subscribed","payload":null,"version":1,"topic":"room:a","ref":"s"}`)
	c.Send(Echo{Payload: "x"}, chanxtest.Topic("room:a"))
	if f := c.ReceiveTopic("room:a"); f.Payload() != "x" {
		t.Fatal(f)
	}
	c.Unsubscribe("room:a")
}

// Each helper fails the test when what arrives is not what it asserts.
func TestFailures(t *testing.T) {
	shortTimeout(t)
	e := engine(t, nil)

	wantFailure(t, failures(t, func(tb testing.TB) {
		chanxtest.Connect(tb, e, "/ws/nowhere")
	}), "no route for /ws/nowhere")

	wantFailure(t, failures(t, func(tb testing.TB) {
		chanxtest.Connect(tb, e, "/ws/info/x").Receive()
	}), "receive")

	wantFailure(t, failures(t, func(tb testing.TB) {
		c := chanxtest.Connect(tb, e, "/ws/info/x")
		c.Send(InfoMessage{})
		c.Expect(`{"action":"something_else"}`)
	}), "unexpected frame")

	wantFailure(t, failures(t, func(tb testing.TB) {
		c := chanxtest.Connect(tb, e, "/ws/info/x")
		c.Send(InfoMessage{})
		c.ExpectNothing(time.Second)
	}), "expected nothing")

	wantFailure(t, failures(t, func(tb testing.TB) {
		chanxtest.Connect(tb, e, "/ws/hub").Subscribe("nope:x")
	}), "subscribe nope:x")

	wantFailure(t, failures(t, func(tb testing.TB) {
		chanxtest.Connect(tb, e, "/ws/hub").ReceiveTopic("room:a")
	}), "receive")

	wantFailure(t, failures(t, func(tb testing.TB) {
		chanxtest.Connect(tb, e, "/ws/info/x").WaitClosed()
	}), "still open")

	wantFailure(t, failures(t, func(tb testing.TB) {
		c := chanxtest.Connect(tb, e, "/ws/info/x")
		c.Close()
		c.SendText(`{}`)
	}), "send")

	wantFailure(t, failures(t, func(tb testing.TB) {
		chanxtest.Connect(tb, e, "/ws/info/x").SendJSON(func() {})
	}), "unsupported type")
}

func TestReceiveHelpers(t *testing.T) {
	e := engine(t, nil)
	c := chanxtest.Connect(t, e, "/ws/info/x")
	c.Send(InfoMessage{})
	c.Send(ShoutMessage{})
	echo, before := chanxtest.ReceiveUntilMessage[Echo](c)
	if echo.Payload != "shout" || len(before) != 1 || before[0].Action() != "info_reply" {
		t.Fatalf("%+v %v", echo, before)
	}
	c.Send(InfoMessage{})
	if frames := c.ReceiveUntil("info_reply"); len(frames) != 1 {
		t.Fatal(frames)
	}
	// Request without completion: the frames carrying its ref.
	if got := c.Request(InfoMessage{}); len(got) != 1 || got[0].Action() != "info_reply" || got[0].Ref() == "" {
		t.Fatal(got)
	}
	var generic map[string]any
	c.Send(InfoMessage{})
	if err := c.Receive().Decode(&generic); err != nil || generic["action"] != "info_reply" {
		t.Fatal(generic, err)
	}
}

func TestSpyRecordsEverySend(t *testing.T) {
	spy := chanxtest.Spy(inmemory.New())
	e := engine(t, layers.Set{"default": spy})
	c := chanxtest.Connect(t, e, "/ws/info/x")
	c.Send(ShoutMessage{})
	c.Receive()
	if g := spy.GroupMessages(); len(g) != 1 || g[0].Group != "room.x" {
		t.Fatal(g)
	}
	_ = e.Publisher().SendEvent(context.Background(), "specific.nobody", Echo{Payload: "e"})
	if ev := spy.Events(); len(ev) != 1 || ev[0].Channel != "specific.nobody" {
		t.Fatal(ev)
	}
}

func TestJSONEqual(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{`{"a":1,"b":2}`, `{"b":2,"a":1}`, true},
		{`{"a":1}`, `{"a":1.0}`, false}, // ints and floats stay distinct
		{`[1,2]`, `[2,1]`, false},
		{`not json`, `not json`, true},
		{`not json`, `{}`, false},
	} {
		if got := chanxtest.JSONEqual(c.a, c.b); got != c.want {
			t.Errorf("JSONEqual(%s, %s) = %v", c.a, c.b, got)
		}
	}
}
