// Package chanxtest is an in-process test client for chanx engines: no network,
// an in-memory socket.
//
//	c := chanxtest.Connect(t, engine, "/ws/chat/lobby", chanxtest.Header("Authorization", "Bearer x"))
//	c.Send(ChatMessage{Payload: ChatPayload{Message: "hi"}}, chanxtest.Ref("1"))
//	c.Expect(`{"action":"pong","payload":null,"version":1,"ref":"1"}`)
package chanxtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/internal/schema"
	"github.com/huynguyengl99/chanx-go/transport"
)

// Timeout bounds every receive.
var Timeout = 3 * time.Second

// Option configures Connect.
type Option func(*options)

type options struct {
	req    chanx.Request
	values map[any]any
	strict bool
	auth   bool
}

// Header sets a handshake header.
func Header(key, value string) Option {
	return func(o *options) { o.req.Header.Add(key, value) }
}

// Query adds a query parameter (also accepted in the path: "/ws?token=x").
func Query(key, value string) Option {
	return func(o *options) { o.req.Query.Add(key, value) }
}

// Cookie adds a cookie.
func Cookie(name, value string) Option {
	return func(o *options) { o.req.Cookies = append(o.req.Cookies, &http.Cookie{Name: name, Value: value}) }
}

// Value sets a value as framework middleware would (Request.Value).
func Value(key, v any) Option { return func(o *options) { o.values[key] = v } }

// ClientIP sets the client address.
func ClientIP(ip string) Option { return func(o *options) { o.req.ClientIP = ip } }

// Lenient turns off strict mode, which fails the test when the server sends an
// action the route never declared (handler replies, Operations Replies, Emits).
func Lenient() Option { return func(o *options) { o.strict = false } }

// SkipAuthentication leaves the authentication message to the test. By default,
// on a route that authenticates, Connect reads it into Client.Auth, whatever its
// status.
func SkipAuthentication() Option { return func(o *options) { o.auth = false } }

// Client is a connected test client.
type Client struct {
	t        testing.TB
	ws       *transport.PipeConn
	route    *chanx.Route
	declared map[string]bool
	strict   bool
	refs     atomic.Int64
	done     chan struct{}
	// Auth is the authentication message, when the route authenticates.
	Auth Frame
}

// Connect opens a connection to the route serving path and registers cleanup:
// the socket is closed and the server's handlers must finish.
func Connect(t testing.TB, e *chanx.Engine, path string, opts ...Option) *Client {
	t.Helper()
	c, err := Dial(t, e, path, opts...)
	if err != nil {
		t.Fatalf("chanxtest: connect %s: %v", path, err)
	}
	return c
}

// Dial is Connect returning a pre-upgrade rejection (an *chanx.AuthError) as an
// error instead of failing.
func Dial(t testing.TB, e *chanx.Engine, path string, opts ...Option) (*Client, error) {
	t.Helper()
	o := options{req: chanx.Request{Header: http.Header{}, Query: url.Values{}}, values: map[any]any{}, strict: true, auth: true}
	p, rawQuery, _ := strings.Cut(path, "?")
	if q, err := url.ParseQuery(rawQuery); err == nil {
		for k, vs := range q {
			for _, v := range vs {
				o.req.Query.Add(k, v)
			}
		}
	}
	for _, opt := range opts {
		opt(&o)
	}
	route, params, ok := e.Lookup(p)
	if !ok {
		return nil, fmt.Errorf("no route for %s", p)
	}
	req := o.req
	req.Path, req.Params = p, params
	if req.ClientIP == "" {
		req.ClientIP = "127.0.0.1"
	}
	values := o.values
	req.Values = func(key any) any { return values[key] }
	ready := make(chan struct{})
	var once sync.Once
	req.OnReady = func() { once.Do(func() { close(ready) }) }
	if err := e.Preflight(context.Background(), route, &req); err != nil {
		return nil, err
	}
	server, client := transport.Pipe()
	c := &Client{t: t, ws: client, route: route, declared: route.Declared(), strict: o.strict, done: make(chan struct{})}
	go func() {
		defer close(c.done)
		e.Serve(context.Background(), route, server, &req)
	}()
	// Return once the server joined the groups: a broadcast sent right after
	// Connect then reaches this client.
	select {
	case <-ready:
	case <-c.done:
	case <-time.After(Timeout):
		t.Errorf("chanxtest: %s: the connection was not set up within %s", path, Timeout)
	}
	t.Cleanup(func() {
		_ = client.Close(transport.StatusNormalClosure, "")
		select {
		case <-c.done:
		case <-time.After(Timeout):
			t.Errorf("chanxtest: %s: the server did not finish within %s", path, Timeout)
		}
	})
	if o.auth && route.Info().Consumer.TypeName != "" && authenticates(route) {
		c.Auth = c.Receive()
		if c.Auth.Action() != "authentication" {
			t.Fatalf("chanxtest: expected the authentication message, got %s", c.Auth)
		}
	}
	return c, nil
}

func authenticates(r *chanx.Route) bool { return r.Authenticates() }

// FrameOption sets envelope fields on a sent message.
type FrameOption func(map[string]any)

// Ref sets the frame's ref.
func Ref(ref string) FrameOption { return func(f map[string]any) { f["ref"] = ref } }

// Topic addresses the frame to a topic.
func Topic(topic string) FrameOption { return func(f map[string]any) { f["topic"] = topic } }

// Frame is a received frame, decoded with json.Number numbers.
type Frame map[string]any

// Action returns the frame's action.
func (f Frame) Action() string { s, _ := f["action"].(string); return s }

// Ref returns the frame's ref.
func (f Frame) Ref() string { s, _ := f["ref"].(string); return s }

// Topic returns the frame's topic.
func (f Frame) Topic() string { s, _ := f["topic"].(string); return s }

// Payload returns the frame's payload.
func (f Frame) Payload() any { return f["payload"] }

// Decode decodes the frame into a message (or any struct).
func (f Frame) Decode(v any) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if _, ok := v.(chanx.Message); ok {
		var body struct {
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.Unmarshal(data, &body)
		if pf, ok := payloadField(reflect.ValueOf(v).Elem()); ok && len(body.Payload) > 0 {
			return json.Unmarshal(body.Payload, pf.Addr().Interface())
		}
		return nil
	}
	return json.Unmarshal(data, v)
}

func (f Frame) String() string {
	data, _ := json.Marshal(f)
	return string(data)
}

func frameOf(m chanx.Message, opts []FrameOption) (map[string]any, error) {
	info, err := chanx.Info(m)
	if err != nil {
		return nil, err
	}
	frame := map[string]any{"action": info.Action}
	if info.PayloadType != nil {
		v := reflect.ValueOf(m)
		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}
		if pf, ok := payloadField(v); ok {
			frame["payload"] = pf.Interface()
		}
	}
	for _, o := range opts {
		o(frame)
	}
	return frame, nil
}

// payloadField finds the field the message's JSON calls "payload", whatever its
// Go name.
func payloadField(v reflect.Value) (reflect.Value, bool) {
	for _, f := range schema.Fields(v.Type()) {
		if f.Name == "payload" {
			return v.FieldByIndex(f.Index), true
		}
	}
	return reflect.Value{}, false
}

// Send sends a message, with envelope options.
func (c *Client) Send(m chanx.Message, opts ...FrameOption) {
	c.t.Helper()
	frame, err := frameOf(m, opts)
	if err != nil {
		c.t.Fatal(err)
	}
	c.SendJSON(frame)
}

// SendJSON sends any value as a JSON text frame.
func (c *Client) SendJSON(v any) {
	c.t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		c.t.Fatal(err)
	}
	c.SendText(string(data))
}

// SendText sends a raw text frame.
func (c *Client) SendText(s string) {
	c.t.Helper()
	if err := c.ws.Write(context.Background(), []byte(s)); err != nil {
		c.t.Fatalf("chanxtest: send: %v", err)
	}
}

// SendBinary sends a binary frame.
func (c *Client) SendBinary(b []byte) {
	c.t.Helper()
	if err := c.ws.WriteType(context.Background(), transport.Binary, b); err != nil {
		c.t.Fatalf("chanxtest: send: %v", err)
	}
}

// Receive returns the next frame, failing the test after Timeout.
func (c *Client) Receive() Frame {
	c.t.Helper()
	f, err := c.receive(Timeout)
	if err != nil {
		c.t.Fatalf("chanxtest: receive: %v", err)
	}
	return f
}

func (c *Client) receive(d time.Duration) (Frame, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	_, data, err := c.ws.Read(ctx)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var f Frame
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("frame is not a JSON object: %s", data)
	}
	if c.strict && !c.declared[f.Action()] {
		c.t.Errorf("chanxtest: route %s sent %q, which no handler declares (add it to Operations Replies or Emits, or connect with chanxtest.Lenient())", c.route.Path, f.Action())
	}
	return f, nil
}

// ReceiveUntil collects frames until one with any of the given actions
// ("complete" when none is given), which is included last.
func (c *Client) ReceiveUntil(stop ...string) []Frame {
	c.t.Helper()
	if len(stop) == 0 {
		stop = []string{"complete"}
	}
	var out []Frame
	for {
		f := c.Receive()
		out = append(out, f)
		for _, s := range stop {
			if f.Action() == s {
				return out
			}
		}
	}
}

// ReceiveUntilMessage receives frames until one carrying message T, and returns
// it decoded along with the frames received before it.
//
//	pong, before := chanxtest.ReceiveUntilMessage[chanx.PongMessage](c)
func ReceiveUntilMessage[T chanx.Message](c *Client) (T, []Frame) {
	c.t.Helper()
	var before []Frame
	for {
		f := c.Receive()
		if m, ok, err := decodeAs[T](f); ok {
			if err != nil {
				c.t.Fatalf("chanxtest: decoding %s: %v", f, err)
			}
			return m, before
		}
		before = append(before, f)
	}
}

// decodeAs decodes a frame or layer message into T when its action is T's.
func decodeAs[T chanx.Message](data map[string]any) (T, bool, error) {
	var m T
	if action, _ := data["action"].(string); action != chanx.ActionOf(m) {
		return m, false, nil
	}
	payload, ok := data["payload"]
	if !ok || payload == nil {
		return m, true, nil
	}
	pf, ok := payloadField(reflect.ValueOf(&m).Elem())
	if !ok {
		return m, true, nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return m, true, err
	}
	return m, true, json.Unmarshal(raw, pf.Addr().Interface())
}

// Request sends m with a fresh ref and returns the frames carrying that ref,
// until the complete frame (needs SendCompletion) or until nothing arrived for
// 200ms. Frames without the ref are skipped.
func (c *Client) Request(m chanx.Message, opts ...FrameOption) []Frame {
	c.t.Helper()
	ref := fmt.Sprintf("chanxtest-%d", c.refs.Add(1))
	c.Send(m, append(opts, Ref(ref))...)
	var out []Frame
	for {
		f, err := c.receive(200 * time.Millisecond)
		if err != nil {
			return out
		}
		if f.Ref() != ref {
			continue
		}
		if f.Action() == "complete" {
			return out
		}
		out = append(out, f)
	}
}

// Expect receives a frame and compares it to want (JSON, key order ignored).
func (c *Client) Expect(want string) Frame {
	c.t.Helper()
	f := c.Receive()
	if !JSONEqual(f.String(), want) {
		c.t.Fatalf("chanxtest: unexpected frame\n got: %s\nwant: %s", f, want)
	}
	return f
}

// ExpectNothing fails when a frame arrives within d.
func (c *Client) ExpectNothing(d time.Duration) {
	c.t.Helper()
	if f, err := c.receive(d); err == nil {
		c.t.Fatalf("chanxtest: expected nothing, got %s", f)
	}
}

// Subscribe subscribes to a topic and asserts the confirmation.
func (c *Client) Subscribe(topic string) Frame {
	c.t.Helper()
	c.SendJSON(map[string]any{"action": "subscribe", "topic": topic})
	f := c.Receive()
	if f.Action() != "subscribed" || f.Topic() != topic {
		c.t.Fatalf("chanxtest: subscribe %s: got %s", topic, f)
	}
	return f
}

// Unsubscribe unsubscribes from a topic and asserts the confirmation.
func (c *Client) Unsubscribe(topic string) {
	c.t.Helper()
	c.SendJSON(map[string]any{"action": "unsubscribe", "topic": topic})
	if f := c.Receive(); f.Action() != "unsubscribed" || f.Topic() != topic {
		c.t.Fatalf("chanxtest: unsubscribe %s: got %s", topic, f)
	}
}

// ReceiveTopic receives the next frame and asserts it belongs to topic.
func (c *Client) ReceiveTopic(topic string) Frame {
	c.t.Helper()
	f := c.Receive()
	if f.Topic() != topic {
		c.t.Fatalf("chanxtest: expected a frame on %s, got %s", topic, f)
	}
	return f
}

// Close closes the client side.
func (c *Client) Close() { _ = c.ws.Close(transport.StatusNormalClosure, "") }

// WaitClosed waits for the server to close and returns the close code.
func (c *Client) WaitClosed() int {
	c.t.Helper()
	select {
	case <-c.ws.Done():
		ce, _ := c.ws.Closed()
		return ce.Code
	case <-time.After(Timeout):
		c.t.Fatalf("chanxtest: connection still open after %s", Timeout)
		return -1
	}
}

// JSONEqual compares two JSON documents ignoring key order and number formats
// (1 and 1.0 differ: ints and floats stay distinct).
func JSONEqual(a, b string) bool {
	norm := func(s string) string {
		dec := json.NewDecoder(strings.NewReader(s))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			return "invalid:" + s
		}
		out, _ := json.Marshal(v)
		return string(out)
	}
	return norm(a) == norm(b)
}
