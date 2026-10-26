// Package adaptertest is the suite every framework adapter passes, over a real
// TCP listener. An adapter's test builds its framework app around the engine
// Run hands it, with a middleware that behaves as follows:
//
//   - a request with an "X-Reject" header is answered 401 before any upgrade;
//   - an "X-User" header is stored as the request value "user" (c.Set, Locals,
//     context value: whatever the framework uses).
//
// and returns the base URL of the running server.
package adaptertest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	chanx "go.chanx.dev/v2"
)

// Server is a running app.
type Server struct {
	// URL is the base URL, "http://127.0.0.1:port".
	URL string
	// Shutdown stops the framework's server; the suite calls engine.Shutdown
	// itself first.
	Shutdown func(ctx context.Context) error
}

// Factory builds and starts the app.
type Factory func(t *testing.T, e *chanx.Engine) *Server

// InfoMessage asks the server to describe the connection.
type InfoMessage struct {
	chanx.BaseMessage `action:"info"`
}

// ConnInfo is what the server saw of the handshake.
type ConnInfo struct {
	Params   map[string]string `json:"params"`
	Query    string            `json:"query"`
	Tags     []string          `json:"tags"`
	Header   string            `json:"header"`
	Cookie   string            `json:"cookie"`
	ClientIP string            `json:"client_ip"`
	User     any               `json:"user"`
	Value    any               `json:"value"`
}

// InfoReply carries ConnInfo.
type InfoReply struct {
	chanx.BaseMessage `action:"info_reply"`
	Payload           ConnInfo `json:"payload"`
}

// ShoutPayload is broadcast to the room.
type ShoutPayload struct {
	Text string `json:"text"`
}

// ShoutMessage broadcasts to the connection's room.
type ShoutMessage struct {
	chanx.BaseMessage `action:"shout"`
	Payload           ShoutPayload `json:"payload"`
}

// HeardMessage is what the room receives.
type HeardMessage struct {
	chanx.BaseMessage `action:"heard"`
	Payload           ShoutPayload `json:"payload"`
}

type echoConsumer struct{}

func (echoConsumer) Groups(c *chanx.Conn) []string { return []string{"room." + c.Param("room")} }

func (echoConsumer) Ping(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

func (echoConsumer) Info(ctx *chanx.Context, _ InfoMessage) (InfoReply, error) {
	return InfoReply{Payload: describe(ctx)}, nil
}

func (echoConsumer) Shout(ctx *chanx.Context, m ShoutMessage) error {
	return ctx.Broadcast(HeardMessage{Payload: m.Payload})
}

func (echoConsumer) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{"Shout": {Replies: []chanx.Message{HeardMessage{}}}}
}

func describe(ctx *chanx.Context) ConnInfo {
	r := ctx.Conn.Request
	info := ConnInfo{Params: ctx.Conn.Params, Query: ctx.Conn.Query.Get("q"), Tags: ctx.Conn.Query["tag"], Header: ctx.Conn.Header.Get("X-Test"), ClientIP: r.ClientIP, User: ctx.Conn.User, Value: r.Value("user")}
	if c, err := r.Cookie("session"); err == nil {
		info.Cookie = c.Value
	}
	return info
}

type userConsumer struct{}

func (userConsumer) Info(ctx *chanx.Context, _ InfoMessage) (InfoReply, error) {
	return InfoReply{Payload: describe(ctx)}, nil
}

type strictConsumer struct{}

func (strictConsumer) Authenticate(ctx *chanx.Context, r *chanx.Request) (any, error) {
	if r.Header.Get("X-Token") == "ok" {
		return "token-user", nil
	}
	return nil, chanx.Forbidden("Bad token")
}

func (strictConsumer) Info(ctx *chanx.Context, _ InfoMessage) (InfoReply, error) {
	return InfoReply{Payload: describe(ctx)}, nil
}

// RoomTopic is the suite's topic.
type RoomTopic struct {
	chanx.BaseTopic `pattern:"room:{name}"`
}

// Passthrough forwards HeardMessage events.
func (RoomTopic) Passthrough() []chanx.Message { return []chanx.Message{HeardMessage{}} }

type hub struct{}

func (hub) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{&RoomTopic{}} }

// NewEngine builds the engine the suite runs against.
func NewEngine() *chanx.Engine {
	e := chanx.New(chanx.Config{})
	e.Handle("/ws/echo/{room}", echoConsumer{})
	e.Handle("/ws/user", userConsumer{}, chanx.UserFrom("user"))
	e.Handle("/ws/strict", strictConsumer{}, chanx.RejectBeforeUpgrade(true))
	e.Handle("/ws/topics", hub{})
	return e
}

// Run runs the suite.
func Run(t *testing.T, factory Factory) {
	e := NewEngine()
	srv := factory(t, e)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = e.Shutdown(ctx)
		if srv.Shutdown != nil {
			_ = srv.Shutdown(ctx)
		}
	})
	base := "ws" + strings.TrimPrefix(srv.URL, "http")

	t.Run("PingPong", func(t *testing.T) {
		c := dial(t, base+"/ws/echo/lobby", nil)
		c.send(t, `{"action":"ping","ref":"1"}`)
		c.expect(t, `{"action":"pong","payload":null,"version":1,"ref":"1"}`)
	})

	t.Run("Request", func(t *testing.T) {
		h := http.Header{"X-Test": {"hello"}, "Cookie": {"session=abc"}, "X-User": {"ann"}}
		c := dial(t, base+"/ws/echo/my-room?q=search&tag=a&tag=b", h)
		c.send(t, `{"action":"info"}`)
		var info struct {
			Payload ConnInfo `json:"payload"`
		}
		c.decode(t, &info)
		p := info.Payload
		if p.Params["room"] != "my-room" || p.Query != "search" || !slices.Equal(p.Tags, []string{"a", "b"}) || p.Header != "hello" || p.Cookie != "abc" || p.Value != "ann" || p.ClientIP == "" {
			t.Fatalf("handshake not bridged: %+v", p)
		}
	})

	t.Run("MiddlewareUser", func(t *testing.T) {
		c := dial(t, base+"/ws/user", http.Header{"X-User": {"bob"}})
		c.expect(t, `{"action":"authentication","payload":{"status_code":200,"status_text":"OK","data":null}}`)
		c.send(t, `{"action":"info"}`)
		var info struct {
			Payload ConnInfo `json:"payload"`
		}
		c.decode(t, &info)
		if info.Payload.User != "bob" {
			t.Fatalf("user: %+v", info.Payload)
		}
		anon := dial(t, base+"/ws/user", nil)
		anon.expect(t, `{"action":"authentication","payload":{"status_code":401,"status_text":"Unauthorized","data":null}}`)
		if code := anon.closeCode(t); code != 4401 {
			t.Fatalf("close code %d", code)
		}
	})

	t.Run("MiddlewareRejects", func(t *testing.T) {
		status := dialStatus(t, base+"/ws/echo/x", http.Header{"X-Reject": {"1"}})
		if status != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401 from the middleware", status)
		}
	})

	t.Run("RejectBeforeUpgrade", func(t *testing.T) {
		if status := dialStatus(t, base+"/ws/strict", nil); status != http.StatusForbidden {
			t.Fatalf("status %d, want 403", status)
		}
		c := dial(t, base+"/ws/strict", http.Header{"X-Token": {"ok"}})
		c.expect(t, `{"action":"authentication","payload":{"status_code":200,"status_text":"OK","data":null}}`)
	})

	t.Run("NotFound", func(t *testing.T) {
		if status := dialStatus(t, base+"/ws/nope", nil); status != http.StatusNotFound {
			t.Fatalf("status %d, want 404", status)
		}
	})

	t.Run("Groups", func(t *testing.T) {
		a := dial(t, base+"/ws/echo/g", nil)
		b := dial(t, base+"/ws/echo/g", nil)
		other := dial(t, base+"/ws/echo/h", nil)
		a.send(t, `{"action":"shout","payload":{"text":"hey"}}`)
		a.expect(t, `{"action":"heard","payload":{"text":"hey"}}`)
		b.expect(t, `{"action":"heard","payload":{"text":"hey"}}`)
		other.expectNothing(t)
	})

	t.Run("Topics", func(t *testing.T) {
		a := dial(t, base+"/ws/topics", nil)
		a.send(t, `{"action":"subscribe","topic":"room:x","ref":"s"}`)
		a.expect(t, `{"action":"subscribed","payload":null,"version":1,"topic":"room:x","ref":"s"}`)
		err := chanx.BroadcastTopic[RoomTopic](context.Background(), e.Publisher(), "room:x", HeardMessage{Payload: ShoutPayload{Text: "t"}}, chanx.Seq(3))
		if err != nil {
			t.Fatal(err)
		}
		a.expect(t, `{"action":"heard","payload":{"text":"t"},"version":1,"topic":"room:x","seq":3}`)
	})

	t.Run("Shutdown", func(t *testing.T) {
		c := dial(t, base+"/ws/echo/s", nil)
		c.send(t, `{"action":"ping","ref":"x"}`)
		c.expect(t, `{"action":"pong","payload":null,"version":1,"ref":"x"}`)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := e.Shutdown(ctx); err != nil {
			t.Fatal(err)
		}
		if code := c.closeCode(t); code != 1001 {
			t.Fatalf("close code %d, want 1001", code)
		}
	})
}

type client struct{ ws *websocket.Conn }

func dial(t *testing.T, url string, h http.Header) *client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: h})
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial %s: %v (status %d)", url, err, status)
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	return &client{ws: ws}
}

func dialStatus(t *testing.T, url string, h http.Header) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: h})
	if err == nil {
		_ = ws.CloseNow()
		t.Fatalf("dial %s succeeded, want a refusal", url)
	}
	if resp == nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	return resp.StatusCode
}

func (c *client) send(t *testing.T, s string) {
	t.Helper()
	if err := c.ws.Write(context.Background(), websocket.MessageText, []byte(s)); err != nil {
		t.Fatal(err)
	}
}

func (c *client) read(t *testing.T, d time.Duration) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	_, data, err := c.ws.Read(ctx)
	return data, err
}

func (c *client) expect(t *testing.T, want string) {
	t.Helper()
	data, err := c.read(t, 5*time.Second)
	if err != nil {
		t.Fatalf("waiting for %s: %v", want, err)
	}
	if !jsonEqual(string(data), want) {
		t.Fatalf("\n got %s\nwant %s", data, want)
	}
}

func (c *client) decode(t *testing.T, v any) {
	t.Helper()
	data, err := c.read(t, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", data, err)
	}
}

func (c *client) expectNothing(t *testing.T) {
	t.Helper()
	if data, err := c.read(t, 200*time.Millisecond); err == nil {
		t.Fatalf("unexpected frame %s", data)
	}
}

func (c *client) closeCode(t *testing.T) int {
	t.Helper()
	for {
		_, err := c.read(t, 5*time.Second)
		if err == nil {
			continue
		}
		var ce websocket.CloseError
		if errors.As(err, &ce) {
			return int(ce.Code)
		}
		t.Fatalf("expected a close, got %v", err)
	}
}

func jsonEqual(a, b string) bool {
	var x, y any
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	xa, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return string(xa) == string(yb)
}

// Describe formats a ConnInfo, for adapter test failures.
func Describe(i ConnInfo) string { return fmt.Sprintf("%+v", i) }
