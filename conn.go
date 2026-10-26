package chanx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"go.chanx.dev/layers"
	"go.chanx.dev/transport"
)

// ErrClosed is returned when sending on a closed connection.
var ErrClosed = errors.New("chanx: connection closed")

// Conn is one WebSocket connection. A consumer value serves every connection,
// so per-connection state lives here (SetValue, ConnValue), not on the consumer.
type Conn struct {
	// ID is a short random identifier, bound to log lines as conn_id.
	ID string
	// ChannelName is the connection's layer channel.
	ChannelName string
	Params      Params
	Query       url.Values
	Header      http.Header
	// User is what authentication returned.
	User    any
	Request *Request

	engine *Engine
	route  *Route
	def    *consumerDef
	ws     transport.Conn
	layer  layers.Layer
	log    *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	outbox chan outItem
	closed atomic.Bool
	sem    chan struct{}

	mu           sync.Mutex
	groups       []string
	subs         map[string]*subscription
	values       map[any]any
	defaultTopic string
	torn         bool // teardown started: no more groups or subscriptions
}

type outItem struct {
	data  []byte
	close *closeReq
}

type closeReq struct {
	code   int
	reason string
}

type subscription struct {
	def    *topicDef
	params Params
}

func newID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newConn(e *Engine, r *Route, ws transport.Conn, req *Request, parent context.Context) *Conn {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	c := &Conn{
		ID:      newID(4),
		Params:  req.Params,
		Query:   req.Query,
		Header:  req.Header,
		Request: req,
		engine:  e,
		route:   r,
		def:     r.def,
		ws:      ws,
		layer:   r.layer,
		ctx:     ctx,
		cancel:  cancel,
		outbox:  make(chan outItem, e.cfg.OutboxSize),
		subs:    map[string]*subscription{},
	}
	if c.Query == nil {
		c.Query = url.Values{}
	}
	if c.Header == nil {
		c.Header = http.Header{}
	}
	c.log = e.log.With("conn_id", c.ID, "path", req.Path)
	if n := r.maxInFlight(); n > 0 {
		c.sem = make(chan struct{}, n)
	}
	return c
}

// Param returns a path parameter.
func (c *Conn) Param(name string) string { return c.Params[name] }

// Context is canceled when the connection closes.
func (c *Conn) Context() context.Context { return c.ctx }

// Logger returns the connection's logger.
func (c *Conn) Logger() *slog.Logger { return c.log }

// Groups returns the layer groups the connection is in, topic groups included.
func (c *Conn) Groups() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.groups)
}

// Subscriptions returns the topics the connection is subscribed to.
func (c *Conn) Subscriptions() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.subs))
	for t := range c.subs {
		out = append(out, t)
	}
	slices.Sort(out)
	return out
}

// Join adds the connection to a layer group.
func (c *Conn) Join(ctx context.Context, group string) error {
	c.mu.Lock()
	torn := c.torn
	c.mu.Unlock()
	if torn {
		return ErrClosed
	}
	if err := c.layer.GroupAdd(ctx, group, c.ChannelName); err != nil {
		return err
	}
	c.mu.Lock()
	if c.torn {
		// Teardown ran meanwhile and will not see this group: leave it here.
		c.mu.Unlock()
		_ = c.layer.GroupDiscard(context.WithoutCancel(ctx), group, c.ChannelName)
		return ErrClosed
	}
	if !slices.Contains(c.groups, group) {
		c.groups = append(c.groups, group)
	}
	c.mu.Unlock()
	return nil
}

// Leave removes the connection from a layer group.
func (c *Conn) Leave(ctx context.Context, group string) error {
	c.mu.Lock()
	c.groups = slices.DeleteFunc(c.groups, func(g string) bool { return g == group })
	c.mu.Unlock()
	return c.layer.GroupDiscard(ctx, group, c.ChannelName)
}

// SetValue stores per-connection state.
func (c *Conn) SetValue(key, v any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.values == nil {
		c.values = map[any]any{}
	}
	c.values[key] = v
}

// Value reads per-connection state.
func (c *Conn) Value(key any) any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.values[key]
}

// ConnValue reads typed per-connection state.
func ConnValue[T any](c *Conn, key any) (T, bool) {
	v, ok := c.Value(key).(T)
	return v, ok
}

// Close sends the frames already queued, then closes the socket with code and
// reason.
func (c *Conn) Close(code int, reason string) {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}
	select {
	case c.outbox <- outItem{close: &closeReq{code, reason}}:
	default: // the writer is stuck: close now
		_ = c.ws.Close(code, reason)
		c.cancel()
	}
}

// IsClosed reports whether the connection is closing or closed.
func (c *Conn) IsClosed() bool { return c.closed.Load() }

// write queues a frame for the writer. A full outbox waits up to WriteTimeout,
// then the connection is closed with 1013: a stuck client must not grow memory.
func (c *Conn) write(data []byte) error {
	if c.closed.Load() {
		return ErrClosed
	}
	select {
	case c.outbox <- outItem{data: data}:
		return nil
	default:
	}
	timer := time.NewTimer(c.engine.cfg.WriteTimeout)
	defer timer.Stop()
	select {
	case c.outbox <- outItem{data: data}:
		return nil
	case <-c.ctx.Done():
		return ErrClosed
	case <-timer.C:
		c.tooSlow()
		return ErrClosed
	}
}

func (c *Conn) tooSlow() {
	c.log.Warn("chanx: client too slow, closing", "outbox", cap(c.outbox))
	c.closed.Store(true)
	_ = c.ws.Close(transport.StatusTryAgainLater, "client too slow")
	c.cancel()
}

func (c *Conn) writer(done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-c.ctx.Done():
			return
		case it := <-c.outbox:
			if it.close != nil {
				_ = c.ws.Close(it.close.code, it.close.reason)
				c.cancel()
				return
			}
			wctx, cancel := context.WithTimeout(c.ctx, c.engine.cfg.WriteTimeout)
			err := c.ws.Write(wctx, it.data)
			timedOut := errors.Is(wctx.Err(), context.DeadlineExceeded)
			cancel()
			switch {
			case err != nil && timedOut: // the client stopped reading, as when the outbox fills
				c.tooSlow()
				return
			case err != nil:
				c.closed.Store(true)
				c.cancel()
				_ = c.ws.Close(transport.StatusInternalError, "")
				return
			}
		}
	}
}

// run is the connection's life: connect flow, read loop, teardown.
func (c *Conn) run() {
	writerDone := make(chan struct{})
	go c.writer(writerDone)
	defer func() {
		c.cancel()
		<-writerDone
		_ = c.ws.Close(transport.StatusNormalClosure, "")
	}()

	ready := func() {
		if c.Request.OnReady != nil {
			c.Request.OnReady()
		}
	}
	channel, err := c.layer.NewChannel(c.ctx, "specific.")
	if err != nil {
		ready()
		c.log.Error("chanx: layer unavailable", "err", err)
		c.closed.Store(true)
		_ = c.ws.Close(transport.StatusInternalError, "layer unavailable")
		return
	}
	c.ChannelName = channel
	c.log = c.log.With("channel", channel)

	var inflight sync.WaitGroup
	layerDone := make(chan struct{})
	started := false
	defer func() {
		c.cancel()
		waitTimeout(&inflight, c.engine.cfg.DisconnectTimeout)
		if started {
			<-layerDone
		}
		c.teardown()
	}()

	if !c.connect() {
		ready()
		// Let the writer flush the authentication message and the close frame.
		select {
		case <-writerDone:
		case <-time.After(c.engine.cfg.WriteTimeout):
		}
		return
	}
	go c.layerLoop(layerDone)
	started = true
	c.subscribeDefault()
	ready()

	sequential := c.route.opts.sequential
	for {
		typ, data, err := c.ws.Read(c.ctx)
		if err != nil {
			break
		}
		if c.closed.Load() {
			continue // closing: drain until the close completes
		}
		job := c.prepare(typ, data)
		if job == nil {
			continue
		}
		if sequential {
			job()
			continue
		}
		if c.sem != nil {
			select {
			case c.sem <- struct{}{}:
			case <-c.ctx.Done():
				continue
			}
		}
		// One goroutine per frame: a slow handler never blocks the socket.
		inflight.Go(func() {
			defer func() {
				if c.sem != nil {
					<-c.sem
				}
			}()
			job()
		})
	}
}

func waitTimeout(wg *sync.WaitGroup, d time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
	}
}

// connect authenticates and reports it, closes on failure, joins the groups,
// then runs OnConnect. A panicking hook closes the connection with 1011.
func (c *Conn) connect() (ok bool) {
	defer func() {
		if p := recover(); p != nil {
			c.log.Error("chanx: connect panicked", "panic", panicText(p))
			c.Close(transport.StatusInternalError, "")
			ok = false
		}
	}()
	base := c.baseContext()
	if auth := c.route.authenticator(); auth != nil {
		var user any
		var err error
		if c.Request.preAuthed {
			user = c.Request.preUser
		} else {
			user, err = callAuth(auth, base, c.Request)
		}
		status := AuthenticationPayload{StatusCode: http.StatusOK, StatusText: "OK"}
		if err != nil {
			status.StatusCode, status.StatusText = authStatus(err)
			if !errors.As(err, new(*AuthError)) {
				c.log.Info("chanx: authentication failed", "err", err)
			}
		} else if c.def.authData != nil {
			status.Data = c.def.authData(base, user)
		}
		_ = base.Push(AuthenticationMessage{Payload: status})
		if err != nil {
			c.Close(authCloseCode(status.StatusCode), "")
			return false
		}
		c.User = user
	}
	seen := map[string]bool{}
	for _, g := range c.def.groups {
		for _, group := range g(c) {
			if seen[group] {
				continue
			}
			seen[group] = true
			if err := c.Join(c.ctx, group); err != nil {
				c.log.Error("chanx: cannot join group", "group", group, "err", err)
				c.Close(transport.StatusInternalError, "")
				return false
			}
		}
	}
	if c.def.onConnect != nil {
		if err := safeCall(func() error { return c.def.onConnect(base) }); err != nil {
			c.log.Error("chanx: OnConnect failed", "err", err)
			c.Close(transport.StatusInternalError, "")
			return false
		}
	}
	return true
}

func safeCall(fn func() error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = errors.New("panic: " + panicText(p))
		}
	}()
	return fn()
}

func (c *Conn) baseContext() *Context {
	return &Context{Context: c.ctx, Conn: c, logger: c.log}
}

// subscribeDefault subscribes a dedicated topic route from its URL parameters.
func (c *Conn) subscribeDefault() {
	td := c.route.defaultTopic
	if td == nil || c.closed.Load() {
		return
	}
	topic, err := td.pattern.Format(c.Params)
	if err != nil {
		c.log.Error("chanx: default topic", "err", err)
		c.Close(transport.StatusInternalError, "")
		return
	}
	c.mu.Lock()
	c.defaultTopic = topic
	c.mu.Unlock()
	c.subscribe(c.baseContext(), topic)
}

// teardown leaves every topic and group and deletes the layer channel, after
// OnDisconnect, which can still broadcast to the connection's groups.
func (c *Conn) teardown() {
	ctx, cancel := context.WithTimeout(context.Background(), c.engine.cfg.DisconnectTimeout)
	defer cancel()
	c.closed.Store(true)
	c.mu.Lock()
	c.torn = true
	subs := c.subs
	c.subs = map[string]*subscription{}
	c.mu.Unlock()
	for topic, sub := range subs {
		tc := &Context{Context: ctx, Conn: c, Topic: topic, Params: sub.params, topic: sub.def, logger: c.log}
		c.runUnsubscribeHook(tc, sub.def)
	}
	if c.def.onDisconnect != nil && c.ChannelName != "" {
		func() {
			defer c.recoverLog("OnDisconnect")
			c.def.onDisconnect(&Context{Context: ctx, Conn: c, logger: c.log})
		}()
	}
	if c.ChannelName == "" {
		return
	}
	for _, g := range c.Groups() {
		_ = c.layer.GroupDiscard(ctx, g, c.ChannelName)
	}
	_ = c.layer.DeleteChannel(ctx, c.ChannelName)
	c.log.Debug("chanx: disconnected")
}

func (c *Conn) recoverLog(where string) {
	if p := recover(); p != nil {
		c.log.Error("chanx: "+where+" panicked", "panic", panicText(p))
	}
}
