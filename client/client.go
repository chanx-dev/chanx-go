// Package chanxclient is a WebSocket client for chanx servers, Go or Python.
// chanx-codegen generates typed clients on top of it.
package chanxclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"

	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/protocol"
)

// Channel is a route's address and the messages it sends.
type Channel struct {
	Name     string
	Address  string
	Incoming []chanx.Message
}

// Envelope is a received frame's envelope, and the frame itself.
type Envelope struct {
	Ref    string
	HasRef bool
	Topic  string
	Seq    *int64
	Raw    json.RawMessage
}

// RawMessage is a frame with an action the channel does not declare.
type RawMessage struct {
	chanx.BaseMessage
	Action  string
	Payload json.RawMessage
}

// RequestError is a request answered by an error frame.
type RequestError struct {
	Payload json.RawMessage
}

func (e *RequestError) Error() string { return "chanx: request failed: " + string(e.Payload) }

// DecodeError is a frame that does not fit its message type.
type DecodeError struct {
	Action string
	Frame  json.RawMessage
	Err    error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("chanx: cannot decode %q frame: %v", e.Action, e.Err)
}

func (e *DecodeError) Unwrap() error { return e.Err }

var ErrClosed = errors.New("chanx: connection closed")

// Option configures Dial.
type Option func(*config)

type config struct {
	header http.Header
	query  url.Values
	params map[string]string
	dial   *websocket.DialOptions
}

// Header adds handshake headers.
func Header(h http.Header) Option {
	return func(c *config) {
		for k, vs := range h {
			for _, v := range vs {
				c.header.Add(k, v)
			}
		}
	}
}

// Query adds URL query parameters.
func Query(v url.Values) Option {
	return func(c *config) {
		for k, vs := range v {
			for _, s := range vs {
				c.query.Add(k, s)
			}
		}
	}
}

// Params fills the address's {param} placeholders.
func Params(p map[string]string) Option {
	return func(c *config) { maps.Copy(c.params, p) }
}

// DialOptions sets coder/websocket's dial options.
func DialOptions(o *websocket.DialOptions) Option { return func(c *config) { c.dial = o } }

// Conn is a connection to one channel, safe for concurrent use.
type Conn struct {
	ws       *websocket.Conn
	channel  Channel
	types    map[string]reflect.Type
	nextRef  atomic.Uint64
	mu       sync.Mutex
	pending  map[string]chan frame
	topics   map[string]*Topic
	inbox    *queue
	readDone chan struct{}
	readErr  error
}

type frame struct {
	action string
	env    Envelope
}

// Dial connects to a channel. baseURL is ws, wss, http, https or a bare host.
func Dial(ctx context.Context, baseURL string, ch Channel, opts ...Option) (*Conn, error) {
	cfg := &config{header: http.Header{}, query: url.Values{}, params: map[string]string{}}
	for _, o := range opts {
		o(cfg)
	}
	u, err := channelURL(baseURL, ch.Address, cfg)
	if err != nil {
		return nil, err
	}
	dial := websocket.DialOptions{}
	if cfg.dial != nil {
		dial = *cfg.dial
	}
	if len(cfg.header) > 0 {
		h := dial.HTTPHeader.Clone()
		if h == nil {
			h = http.Header{}
		}
		for k, vs := range cfg.header {
			h[k] = append(h[k], vs...)
		}
		dial.HTTPHeader = h
	}
	ws, _, err := websocket.Dial(ctx, u, &dial)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(-1)
	c := &Conn{
		ws: ws, channel: ch, types: typesOf(ch.Incoming),
		pending: map[string]chan frame{}, topics: map[string]*Topic{},
		inbox: newQueue(), readDone: make(chan struct{}),
	}
	go c.read()
	return c, nil
}

func channelURL(base, address string, cfg *config) (string, error) {
	switch {
	case strings.HasPrefix(base, "http://"):
		base = "ws://" + strings.TrimPrefix(base, "http://")
	case strings.HasPrefix(base, "https://"):
		base = "wss://" + strings.TrimPrefix(base, "https://")
	case !strings.HasPrefix(base, "ws://") && !strings.HasPrefix(base, "wss://"):
		base = "ws://" + base
	}
	path := address
	for _, name := range protocol.PathParams(address) {
		v, ok := cfg.params[name]
		if !ok {
			return "", fmt.Errorf("chanx: %s needs the path parameter %q", address, name)
		}
		path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(v))
	}
	u, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil {
		return "", err
	}
	if len(cfg.query) > 0 {
		q := u.Query()
		for k, vs := range cfg.query {
			q[k] = append(q[k], vs...)
		}
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

var builtins = []chanx.Message{
	chanx.ErrorMessage{}, chanx.AuthenticationMessage{}, chanx.CompleteMessage{},
	chanx.EventCompleteMessage{}, chanx.GroupCompleteMessage{},
	chanx.SubscribedMessage{}, chanx.UnsubscribedMessage{},
}

func typesOf(msgs []chanx.Message) map[string]reflect.Type {
	types := map[string]reflect.Type{}
	for _, m := range append(append([]chanx.Message(nil), builtins...), msgs...) {
		if info, err := chanx.Info(m); err == nil {
			types[info.Action] = info.Type
		}
	}
	return types
}

func (c *Conn) Send(ctx context.Context, m chanx.Message) error {
	return c.write(ctx, m, "", "")
}

// Request returns the first frame echoing the message's ref; an error frame is a
// *RequestError.
func (c *Conn) Request(ctx context.Context, m chanx.Message) (chanx.Message, Envelope, error) {
	return c.request(ctx, m, "")
}

// Reply narrows a Request's result to the reply type R.
func Reply[R chanx.Message](m chanx.Message, _ Envelope, err error) (R, error) {
	var zero R
	if err != nil {
		return zero, err
	}
	r, ok := m.(R)
	if !ok {
		return zero, fmt.Errorf("chanx: unexpected reply %T, want %T", m, zero)
	}
	return r, nil
}

// Receive returns the next frame not taken by a request or a topic.
func (c *Conn) Receive(ctx context.Context) (chanx.Message, Envelope, error) {
	return c.receive(ctx, c.inbox, c.types)
}

func (c *Conn) Close() error {
	err := c.ws.Close(websocket.StatusNormalClosure, "")
	<-c.readDone
	return err
}

// Err returns why the connection ended, nil while open.
func (c *Conn) Err() error {
	select {
	case <-c.readDone:
		return c.readErr
	default:
		return nil
	}
}

func (c *Conn) request(ctx context.Context, m chanx.Message, topic string) (chanx.Message, Envelope, error) {
	ref := strconv.FormatUint(c.nextRef.Add(1), 10)
	reply := make(chan frame, 1)
	c.mu.Lock()
	c.pending[ref] = reply
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, ref)
		c.mu.Unlock()
	}()
	if err := c.write(ctx, m, topic, ref); err != nil {
		return nil, Envelope{}, err
	}
	types := c.types
	if topic != "" {
		if t := c.topic(topic); t != nil {
			types = t.types
		}
	}
	select {
	case f := <-reply:
		if f.action == protocol.ActionError {
			var e struct {
				Payload json.RawMessage `json:"payload"`
			}
			_ = json.Unmarshal(f.env.Raw, &e)
			return nil, f.env, &RequestError{Payload: e.Payload}
		}
		msg, err := decode(f, types)
		return msg, f.env, err
	case <-ctx.Done():
		return nil, Envelope{}, ctx.Err()
	case <-c.readDone:
		return nil, Envelope{}, c.closedErr()
	}
}

func (c *Conn) write(ctx context.Context, m chanx.Message, topic, ref string) error {
	data, err := encode(m, topic, ref)
	if err != nil {
		return err
	}
	if err := c.ws.Write(ctx, websocket.MessageText, data); err != nil {
		if c.Err() != nil {
			return c.closedErr()
		}
		return err
	}
	return nil
}

func (c *Conn) receive(ctx context.Context, q *queue, types map[string]reflect.Type) (chanx.Message, Envelope, error) {
	f, err := q.pop(ctx, c.readDone)
	if err != nil {
		if errors.Is(err, ErrClosed) {
			return nil, Envelope{}, c.closedErr()
		}
		return nil, Envelope{}, err
	}
	msg, err := decode(f, types)
	return msg, f.env, err
}

func (c *Conn) closedErr() error {
	if c.readErr != nil && websocket.CloseStatus(c.readErr) == -1 {
		return fmt.Errorf("%w: %v", ErrClosed, c.readErr)
	}
	if code := websocket.CloseStatus(c.readErr); code != -1 && code != websocket.StatusNormalClosure {
		return &CloseError{Code: int(code), Err: c.readErr}
	}
	return ErrClosed
}

// CloseError is a close with a code other than 1000, such as 4401.
type CloseError struct {
	Code int
	Err  error
}

func (e *CloseError) Error() string { return fmt.Sprintf("chanx: connection closed with %d", e.Code) }

func (e *CloseError) Unwrap() error { return ErrClosed }

func (c *Conn) topic(name string) *Topic {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.topics[name]
}

func (c *Conn) read() {
	defer close(c.readDone)
	for {
		_, data, err := c.ws.Read(context.Background())
		if err != nil {
			c.readErr = err
			return
		}
		f, ok := parseFrame(data)
		if !ok {
			continue
		}
		c.mu.Lock()
		reply, pending := c.pending[f.env.Ref]
		if pending && f.env.HasRef {
			delete(c.pending, f.env.Ref)
		}
		t := c.topics[f.env.Topic]
		c.mu.Unlock()
		switch {
		case pending && f.env.HasRef:
			reply <- f
		case t != nil && f.env.Topic != "":
			t.inbox.push(f)
		default:
			c.inbox.push(f)
		}
	}
}

func parseFrame(data []byte) (frame, bool) {
	var head struct {
		Action string  `json:"action"`
		Ref    *string `json:"ref"`
		Topic  string  `json:"topic"`
		Seq    *int64  `json:"seq"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return frame{}, false
	}
	f := frame{action: head.Action, env: Envelope{Topic: head.Topic, Seq: head.Seq, Raw: json.RawMessage(data)}}
	if head.Ref != nil {
		f.env.Ref, f.env.HasRef = *head.Ref, true
	}
	return f, true
}

// encode keeps the server's key order: action, payload, version, topic, ref.
func encode(m chanx.Message, topic, ref string) ([]byte, error) {
	info, err := chanx.Info(m)
	if err != nil {
		return nil, err
	}
	payload := json.RawMessage("null")
	if info.PayloadType != nil {
		raw, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		var fields struct {
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, err
		}
		if fields.Payload != nil {
			payload = fields.Payload
		}
	}
	var b bytes.Buffer
	b.WriteString(`{"action":`)
	a, _ := json.Marshal(info.Action)
	b.Write(a)
	b.WriteString(`,"payload":`)
	b.Write(payload)
	if topic != "" || ref != "" {
		b.WriteString(`,"version":`)
		b.WriteString(strconv.Itoa(protocol.Version))
	}
	if topic != "" {
		t, _ := json.Marshal(topic)
		b.WriteString(`,"topic":`)
		b.Write(t)
	}
	if ref != "" {
		r, _ := json.Marshal(ref)
		b.WriteString(`,"ref":`)
		b.Write(r)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func decode(f frame, types map[string]reflect.Type) (chanx.Message, error) {
	t, ok := types[f.action]
	if !ok {
		var raw struct {
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.Unmarshal(f.env.Raw, &raw)
		return RawMessage{Action: f.action, Payload: raw.Payload}, nil
	}
	v := reflect.New(t)
	if err := json.Unmarshal(f.env.Raw, v.Interface()); err != nil {
		return nil, &DecodeError{Action: f.action, Frame: f.env.Raw, Err: err}
	}
	return v.Elem().Interface().(chanx.Message), nil
}
