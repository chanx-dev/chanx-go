package chanxclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/goleak"

	chanx "go.chanx.dev/v2"
	chanxclient "go.chanx.dev/v2/client"
	chanxhttp "go.chanx.dev/v2/nethttp"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type Echo struct {
	chanx.BaseMessage `action:"echo"`
	Payload           EchoPayload `json:"payload"`
}

type EchoPayload struct {
	Text  string `json:"text"`
	Times int    `json:"times"`
}

type Echoed struct {
	chanx.BaseMessage `action:"echoed"`
	Payload           EchoPayload `json:"payload"`
}

type Hello struct {
	chanx.BaseMessage `action:"hello"`
	Payload           string `json:"payload"`
}

type echoConsumer struct{}

func (echoConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

func (echoConsumer) HandleEcho(ctx *chanx.Context, m Echo) (Echoed, error) {
	return Echoed(m), nil
}

func (echoConsumer) OnConnect(ctx *chanx.Context) error {
	return ctx.Push(Hello{Payload: ctx.Conn.Param("room") + ctx.Conn.Query.Get("q") + ctx.Conn.Header.Get("X-Via")})
}

type Post struct {
	chanx.BaseMessage `action:"post"`
	Payload           string `json:"payload"`
}

type Posted struct {
	chanx.BaseMessage `action:"posted"`
	Payload           string `json:"payload"`
}

type Announced struct {
	chanx.BaseMessage `action:"announced"`
	Payload           string `json:"payload"`
}

type RoomTopic struct {
	chanx.BaseTopic `pattern:"room:{name}"`
}

func (RoomTopic) Passthrough() []chanx.Message { return []chanx.Message{Announced{}} }

func (RoomTopic) HandlePost(ctx *chanx.Context, m Post) (Posted, error) {
	return Posted(m), ctx.BroadcastTopic(Announced{Payload: "all: " + m.Payload}, chanx.Seq(7))
}

type hub struct{}

func (hub) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{&RoomTopic{}} }

func (hub) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

type locked struct{ echoConsumer }

func (locked) Authenticate(ctx *chanx.Context, r *chanx.Request) (any, error) {
	if r.Header.Get("Authorization") != "Bearer ok" {
		return nil, chanx.Unauthenticated("no")
	}
	return "user", nil
}

var (
	room   = chanxclient.Channel{Name: "room", Address: "/ws/room/{room}", Incoming: []chanx.Message{chanx.PongMessage{}, Echoed{}, Hello{}}}
	topics = chanxclient.Channel{Name: "hub", Address: "/ws/hub", Incoming: []chanx.Message{chanx.PongMessage{}}}
	secure = chanxclient.Channel{Name: "secure", Address: "/ws/secure", Incoming: []chanx.Message{chanx.PongMessage{}}}
)

func serve(t *testing.T) string {
	t.Helper()
	e := chanx.New(chanx.Config{})
	e.Handle("/ws/room/{room}", echoConsumer{})
	e.Handle("/ws/hub", hub{})
	e.Handle("/ws/secure", locked{})
	mux := http.NewServeMux()
	chanxhttp.Mount(mux, e)
	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = e.Shutdown(ctx)
		srv.Close()
	})
	return srv.URL
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

func dial(t *testing.T, url string, ch chanxclient.Channel, opts ...chanxclient.Option) *chanxclient.Conn {
	t.Helper()
	c, err := chanxclient.Dial(ctx(t), url, ch, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestSendReceiveRequest(t *testing.T) {
	c := dial(t, serve(t), room, chanxclient.Params(map[string]string{"room": "a b"}))
	m, env, err := c.Receive(ctx(t))
	if err != nil || m != (Hello{Payload: "a b"}) || env.HasRef {
		t.Fatalf("%#v %+v %v", m, env, err)
	}

	reply, env, err := c.Request(ctx(t), Echo{Payload: EchoPayload{Text: "hi", Times: 2}})
	if err != nil || reply != (Echoed{Payload: EchoPayload{Text: "hi", Times: 2}}) || env.Ref != "1" {
		t.Fatalf("%#v %+v %v", reply, env, err)
	}
	if _, _, err := c.Request(ctx(t), chanx.PingMessage{}); err != nil {
		t.Fatal(err)
	}

	if err := c.Send(ctx(t), Echo{Payload: EchoPayload{Text: "plain"}}); err != nil {
		t.Fatal(err)
	}
	if m, _, err := c.Receive(ctx(t)); err != nil || m.(Echoed).Payload.Text != "plain" {
		t.Fatalf("%#v %v", m, err)
	}
}

func TestRequestError(t *testing.T) {
	c := dial(t, serve(t), room, chanxclient.Params(map[string]string{"room": "r"}))
	c.Receive(ctx(t)) //nolint:errcheck // hello
	_, _, err := c.Request(ctx(t), Post{Payload: "not on this route"})
	var re *chanxclient.RequestError
	if !errors.As(err, &re) || len(re.Payload) == 0 {
		t.Fatalf("%v", err)
	}
}

func TestMissingParam(t *testing.T) {
	if _, err := chanxclient.Dial(ctx(t), serve(t), room); err == nil {
		t.Fatal("dialed without the room parameter")
	}
}

func TestTopics(t *testing.T) {
	c := dial(t, serve(t), topics)
	lobby := c.Topic("room:lobby", Posted{}, Announced{})
	if err := lobby.Subscribe(ctx(t)); err != nil {
		t.Fatal(err)
	}
	reply, _, err := lobby.Request(ctx(t), Post{Payload: "x"})
	if err != nil || reply != (Posted{Payload: "x"}) {
		t.Fatalf("%#v %v", reply, err)
	}
	m, env, err := lobby.Receive(ctx(t))
	if err != nil || m != (Announced{Payload: "all: x"}) || env.Seq == nil || *env.Seq != 7 || env.Topic != "room:lobby" {
		t.Fatalf("%#v %+v %v", m, env, err)
	}
	if err := lobby.Unsubscribe(ctx(t)); err != nil {
		t.Fatal(err)
	}

	var re *chanxclient.RequestError
	if err := c.Topic("nope:x").Subscribe(ctx(t)); !errors.As(err, &re) {
		t.Fatalf("unknown topic: %v", err)
	}
	if _, _, err := c.Request(ctx(t), chanx.PingMessage{}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthentication(t *testing.T) {
	url := serve(t)
	c := dial(t, url, secure, chanxclient.Header(http.Header{"Authorization": {"Bearer ok"}}))
	m, _, err := c.Receive(ctx(t))
	if a, ok := m.(chanx.AuthenticationMessage); err != nil || !ok || a.Payload.StatusCode != 200 {
		t.Fatalf("%#v %v", m, err)
	}

	d := dial(t, url, secure)
	d.Receive(ctx(t)) //nolint:errcheck // the 401 authentication message
	_, _, err = d.Receive(ctx(t))
	var ce *chanxclient.CloseError
	if !errors.As(err, &ce) || ce.Code != 4401 || !errors.Is(err, chanxclient.ErrClosed) {
		t.Fatalf("%v", err)
	}
}

func TestUndeclaredAction(t *testing.T) {
	bare := chanxclient.Channel{Address: "/ws/room/{room}"}
	c := dial(t, serve(t), bare, chanxclient.Params(map[string]string{"room": "r"}))
	m, _, err := c.Receive(ctx(t))
	if raw, ok := m.(chanxclient.RawMessage); err != nil || !ok || raw.Action != "hello" || string(raw.Payload) != `"r"` {
		t.Fatalf("%#v %v", m, err)
	}
}

func TestClosed(t *testing.T) {
	c := dial(t, serve(t), topics)
	_ = c.Close()
	if err := c.Send(ctx(t), chanx.PingMessage{}); err == nil {
		t.Fatal("sent on a closed connection")
	}
	if _, _, err := c.Receive(ctx(t)); !errors.Is(err, chanxclient.ErrClosed) {
		t.Fatalf("%v", err)
	}
}

type BadHello struct {
	chanx.BaseMessage `action:"hello"`
	Payload           int `json:"payload"`
}

func TestOptionsAndURLs(t *testing.T) {
	srv := serve(t)
	host := strings.TrimPrefix(srv, "http://")
	for _, base := range []string{srv, "ws://" + host, host + "/"} {
		c := dial(t, base, room, chanxclient.Params(map[string]string{"room": "r"}),
			chanxclient.Query(url.Values{"q": {"?"}}),
			chanxclient.DialOptions(&websocket.DialOptions{HTTPHeader: http.Header{"X-Via": {"!"}}}))
		if m, _, err := c.Receive(ctx(t)); err != nil || m != (Hello{Payload: "r?!"}) {
			t.Fatalf("%s: %#v %v", base, m, err)
		}
	}
	if _, err := chanxclient.Dial(ctx(t), "https://"+host, room, chanxclient.Params(map[string]string{"room": "r"})); err == nil {
		t.Fatal("wss to a plain server succeeded")
	}
}

func TestReplyAndDecodeErrors(t *testing.T) {
	c := dial(t, serve(t), room, chanxclient.Params(map[string]string{"room": "r"}))
	c.Receive(ctx(t)) //nolint:errcheck // hello
	if _, err := chanxclient.Reply[Echoed](c.Request(ctx(t), chanx.PingMessage{})); err == nil || !strings.Contains(err.Error(), "unexpected reply") {
		t.Fatalf("%v", err)
	}
	if _, err := chanxclient.Reply[Echoed](c.Request(ctx(t), Post{})); err == nil || !strings.Contains(err.Error(), "request failed") {
		t.Fatalf("%v", err)
	}

	bad := chanxclient.Channel{Address: "/ws/room/{room}", Incoming: []chanx.Message{BadHello{}}}
	d := dial(t, serve(t), bad, chanxclient.Params(map[string]string{"room": "r"}))
	_, _, err := d.Receive(ctx(t))
	var de *chanxclient.DecodeError
	if !errors.As(err, &de) || de.Action != "hello" || !strings.Contains(err.Error(), `cannot decode "hello"`) || de.Unwrap() == nil {
		t.Fatalf("%v", err)
	}
}

func TestTopicSend(t *testing.T) {
	c := dial(t, serve(t), topics)
	lobby := c.Topic("room:lobby", Posted{}, Announced{})
	if c.Topic("room:lobby") != lobby || lobby.Name() != "room:lobby" {
		t.Fatal("a second handle for the same topic")
	}
	if err := lobby.Subscribe(ctx(t)); err != nil {
		t.Fatal(err)
	}
	if err := lobby.Send(ctx(t), Post{Payload: "s"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []chanx.Message{Posted{Payload: "s"}, Announced{Payload: "all: s"}} {
		if m, _, err := lobby.Receive(ctx(t)); err != nil || m != want {
			t.Fatalf("%#v %v", m, err)
		}
	}
}

func TestCloseError(t *testing.T) {
	d := dial(t, serve(t), secure)
	d.Receive(ctx(t)) //nolint:errcheck // the 401 authentication message
	_, _, err := d.Receive(ctx(t))
	if err == nil || err.Error() != "chanx: connection closed with 4401" || d.Err() == nil {
		t.Fatalf("%v %v", err, d.Err())
	}
}
