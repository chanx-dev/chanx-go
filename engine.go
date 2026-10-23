// Package chanx is the Go implementation of the chanx WebSocket protocol:
// consumers and topics written as structs with handler methods, structured
// validation errors, groups and events over a channel layer shared with other
// chanx servers, AsyncAPI generation, and a test client.
//
// The engine knows nothing about HTTP; adapters (chanx-go/nethttp, adapters/gin,
// adapters/echo, adapters/fiber) mount its routes in a framework's router.
package chanx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/huynguyengl99/chanx-go/layers"
	"github.com/huynguyengl99/chanx-go/layers/inmemory"
	"github.com/huynguyengl99/chanx-go/protocol"
	"github.com/huynguyengl99/chanx-go/transport"
)

// Config holds the engine's defaults.
type Config struct {
	// Layers are the channel layers by alias. Nil: one in-memory layer under
	// "default".
	Layers layers.Set
	// Camelize converts incoming keys to snake_case and outgoing keys to
	// camelCase.
	Camelize bool
	// SendCompletion sends complete, event_complete and group_complete frames.
	SendCompletion bool
	Logger         *slog.Logger
	// LogMessages logs every frame received and sent (log_websocket_message),
	// except LogIgnored actions and completion frames.
	LogMessages bool
	LogIgnored  []string
	// MaxInFlight caps concurrently handled frames per connection; 0 is
	// unlimited.
	MaxInFlight int
	// OutboxSize bounds frames queued for a connection's writer. Default 256.
	OutboxSize int
	// WriteTimeout is how long a send waits on a full outbox before the
	// connection closes with 1013. Default 10s.
	WriteTimeout time.Duration
	// DisconnectTimeout bounds the wait for in-flight handlers and the cleanup
	// hooks when a connection ends. Default 5s.
	DisconnectTimeout time.Duration
	// MaxFrameSize caps an incoming frame; a larger one closes the connection
	// with 1009 before it is buffered. Default 16 MiB; negative means no limit.
	MaxFrameSize int64
	// RejectBeforeUpgrade runs authentication before the upgrade and answers a
	// failure with an HTTP status instead of an authentication message and close.
	RejectBeforeUpgrade bool
	// Error hooks; a consumer's own OnValidationError, OnHandlerError and
	// OnEventError methods take precedence.
	OnValidationError func(*Context, []Issue)
	OnHandlerError    func(*Context, string, error)
	OnEventError      func(*EventContext, string, error)
}

// Engine holds the mounted routes and serves connections handed over by an
// adapter.
type Engine struct {
	cfg    Config
	layers layers.Set
	log    *slog.Logger
	ignore map[string]bool

	mu       sync.Mutex
	routes   []*Route
	conns    map[*Conn]struct{}
	closing  bool
	active   sync.WaitGroup
	ownLayer bool
}

// New returns an engine.
func New(cfg Config) *Engine {
	e := &Engine{cfg: cfg, layers: cfg.Layers, log: cfg.Logger, conns: map[*Conn]struct{}{}, ignore: map[string]bool{}}
	if e.layers == nil {
		e.layers = layers.Set{layers.DefaultAlias: inmemory.New()}
		e.ownLayer = true
	}
	if e.log == nil {
		e.log = slog.Default()
	}
	if e.cfg.OutboxSize <= 0 {
		e.cfg.OutboxSize = 256
	}
	if e.cfg.WriteTimeout <= 0 {
		e.cfg.WriteTimeout = 10 * time.Second
	}
	if e.cfg.MaxFrameSize == 0 {
		e.cfg.MaxFrameSize = 16 << 20
	}
	if e.cfg.DisconnectTimeout <= 0 {
		e.cfg.DisconnectTimeout = 5 * time.Second
	}
	for _, a := range cfg.LogIgnored {
		e.ignore[a] = true
	}
	return e
}

// Logger returns the engine's logger.
func (e *Engine) Logger() *slog.Logger { return e.log }

// Layers returns the engine's layers.
func (e *Engine) Layers() layers.Set { return e.layers }

// Publisher returns a publisher on the engine's layers.
func (e *Engine) Publisher() *Publisher { return &Publisher{layers: e.layers, log: e.log} }

// Option configures a route.
type Option func(*routeOptions)

type routeOptions struct {
	name, description   string
	tags                []string
	layer               string
	sendCompletion      *bool
	camelize            *bool
	sequential          bool
	maxInFlight         int
	rejectBeforeUpgrade *bool
	userFrom            func(*Context, *Request) (any, error)
}

// Name sets the AsyncAPI channel name (snake case of the type name without
// "Consumer" by default).
func Name(name string) Option { return func(o *routeOptions) { o.name = name } }

// Description sets the channel description.
func Description(d string) Option { return func(o *routeOptions) { o.description = d } }

// Tags sets the channel tags.
func Tags(tags ...string) Option { return func(o *routeOptions) { o.tags = tags } }

// Layer picks the channel layer alias for the route; "default" when unset.
func Layer(alias string) Option { return func(o *routeOptions) { o.layer = alias } }

// SendCompletion overrides Config.SendCompletion for the route.
func SendCompletion(on bool) Option { return func(o *routeOptions) { o.sendCompletion = &on } }

// Camelize overrides Config.Camelize for the route.
func Camelize(on bool) Option { return func(o *routeOptions) { o.camelize = &on } }

// Sequential handles the route's frames one at a time, in order (Go only).
func Sequential() Option { return func(o *routeOptions) { o.sequential = true } }

// MaxInFlight overrides Config.MaxInFlight for the route.
func MaxInFlight(n int) Option { return func(o *routeOptions) { o.maxInFlight = n } }

// RejectBeforeUpgrade overrides Config.RejectBeforeUpgrade for the route.
func RejectBeforeUpgrade(on bool) Option {
	return func(o *routeOptions) { o.rejectBeforeUpgrade = &on }
}

// Route is a mounted consumer.
type Route struct {
	// Path is the pattern as given, "/ws/chat/{room}": net/http's syntax and
	// AsyncAPI's channel address.
	Path string
	// ParamNames lists the path parameters in order.
	ParamNames []string

	engine       *Engine
	def          *consumerDef
	opts         routeOptions
	alias        string
	layer        layers.Layer
	defaultTopic *topicDef
	matcher      *regexp.Regexp // also matches under a router's prefix
	exact        *regexp.Regexp
}

// Handle mounts a consumer on a path pattern such as "/ws/chat/{room}". The
// consumer is a struct with handler methods, a *Consumer, or a TopicConsumer.
// Definition mistakes (bad signatures, duplicate actions, unknown layer) panic
// here, at startup; chanx.Check reports the same in a test.
func (e *Engine) Handle(path string, consumer any, opts ...Option) *Route {
	r, err := e.handle(path, consumer, opts)
	if err != nil {
		panic(err)
	}
	return r
}

func (e *Engine) handle(path string, consumer any, opts []Option) (*Route, error) {
	if err := protocol.ValidatePath(path); err != nil {
		return nil, err
	}
	def, err := buildAny(consumer)
	if err != nil {
		return nil, err
	}
	r := &Route{
		Path: path, ParamNames: protocol.PathParams(path), engine: e, def: def,
		matcher: protocol.PathRegexp(path, true), exact: protocol.PathRegexp(path, false),
	}
	for _, o := range opts {
		o(&r.opts)
	}
	if _, ok := consumer.(*topicConsumer); ok {
		r.defaultTopic = def.topics[0]
		for _, name := range r.defaultTopic.pattern.Params() {
			if !slices.Contains(r.ParamNames, name) {
				return nil, fmt.Errorf("chanx: topic pattern %q needs URL parameter %q that route %q does not provide", r.defaultTopic.pattern, name, path)
			}
		}
		if r.opts.layer == "" {
			r.opts.layer = r.defaultTopic.layer
		}
	}
	alias := r.opts.layer
	if alias == "" {
		alias = layers.DefaultAlias
	}
	r.alias = alias
	if r.layer, err = e.layers.Get(alias); err != nil {
		return nil, fmt.Errorf("chanx: route %q: %w", path, err)
	}
	for _, td := range def.topics {
		if td.layer != "" && td.layer != alias {
			return nil, fmt.Errorf("chanx: route %q uses layer %q but topic %s declares layer %q; subscribers and broadcasts would not meet", path, alias, td.typeName, td.layer)
		}
	}
	e.mu.Lock()
	for _, existing := range e.routes {
		if existing.Path == path {
			e.mu.Unlock()
			return nil, fmt.Errorf("chanx: route %q mounted twice", path)
		}
	}
	e.routes = append(e.routes, r)
	e.mu.Unlock()
	e.log.Debug("chanx: mounted", "path", path, "consumer", def.typeName, "handlers", def.describe())
	return r, nil
}

// Routes returns the mounted routes in order.
func (e *Engine) Routes() []*Route {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.routes)
}

// Match returns the route parameters when urlPath matches the route's pattern
// (also when the router mounted it under a prefix).
func (r *Route) Match(urlPath string) (Params, bool) { return match(r.matcher, urlPath) }

func match(re *regexp.Regexp, urlPath string) (Params, bool) {
	m := re.FindStringSubmatch(urlPath)
	if m == nil {
		return nil, false
	}
	p := Params{}
	for i, name := range re.SubexpNames() {
		if name != "" {
			p[name] = m[i]
		}
	}
	return p, true
}

// Lookup finds the route serving urlPath exactly, for adapters without a router.
// The most specific route wins, as in http.ServeMux: fewer parameters first,
// then the longer pattern.
func (e *Engine) Lookup(urlPath string) (*Route, Params, bool) {
	var best *Route
	var bestParams Params
	for _, r := range e.Routes() {
		p, ok := match(r.exact, urlPath)
		if !ok {
			continue
		}
		if best == nil || len(r.ParamNames) < len(best.ParamNames) ||
			(len(r.ParamNames) == len(best.ParamNames) && len(r.Path) > len(best.Path)) {
			best, bestParams = r, p
		}
	}
	return best, bestParams, best != nil
}

// Engine returns the engine the route belongs to.
func (r *Route) Engine() *Engine { return r.engine }

func (r *Route) sendCompletion() bool {
	if r.opts.sendCompletion != nil {
		return *r.opts.sendCompletion
	}
	return r.engine.cfg.SendCompletion
}

func (r *Route) camelize() bool {
	if r.opts.camelize != nil {
		return *r.opts.camelize
	}
	return r.engine.cfg.Camelize
}

func (r *Route) maxInFlight() int {
	if r.opts.maxInFlight > 0 {
		return r.opts.maxInFlight
	}
	return r.engine.cfg.MaxInFlight
}

// RejectsBeforeUpgrade reports whether adapters must call Preflight.
func (r *Route) RejectsBeforeUpgrade() bool {
	if r.opts.rejectBeforeUpgrade != nil {
		return *r.opts.rejectBeforeUpgrade
	}
	return r.engine.cfg.RejectBeforeUpgrade
}

func (r *Route) authenticator() func(*Context, *Request) (any, error) {
	if r.def.authenticate != nil {
		return r.def.authenticate
	}
	return r.opts.userFrom
}

// Preflight runs authentication before the upgrade when the route rejects
// before upgrading. A failure is an *AuthError carrying the HTTP status; the
// adapter answers through the framework's error path. On success the user is
// kept on req for Serve.
func (e *Engine) Preflight(ctx context.Context, r *Route, req *Request) error {
	auth := r.authenticator()
	if !r.RejectsBeforeUpgrade() || auth == nil {
		return nil
	}
	// No socket yet: everything but sending works, as after the upgrade.
	conn := &Conn{
		Params: req.Params, Query: req.Query, Header: req.Header, Request: req,
		engine: e, route: r, def: r.def, layer: r.layer, ctx: ctx, cancel: func() {},
		log: e.log.With("path", req.Path), subs: map[string]*subscription{},
	}
	conn.closed.Store(true)
	c := &Context{Context: ctx, Conn: conn, logger: conn.log}
	user, err := callAuth(auth, c, req)
	if err != nil {
		status, detail := authStatus(err)
		if !errors.As(err, new(*AuthError)) {
			e.log.Info("chanx: authentication failed", "path", req.Path, "err", err)
		}
		return &AuthError{Status: status, Detail: detail}
	}
	req.preUser, req.preAuthed = user, true
	return nil
}

func callAuth(auth func(*Context, *Request) (any, error), c *Context, req *Request) (user any, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("chanx: Authenticate panicked: %v: %w", p,
				&AuthError{Status: http.StatusInternalServerError, Detail: http.StatusText(http.StatusInternalServerError)})
		}
	}()
	return auth(c, req)
}

// Serve runs an accepted connection until it closes. Adapters call it after the
// upgrade, from the handler goroutine, which it blocks.
func (e *Engine) Serve(ctx context.Context, r *Route, ws transport.Conn, req *Request) {
	if req == nil {
		req = &Request{}
	}
	if req.Params == nil {
		req.Params = Params{}
	}
	if l, ok := ws.(transport.ReadLimiter); ok {
		l.SetReadLimit(max(e.cfg.MaxFrameSize, -1)) // also when negative: coder/websocket defaults to 32 KiB
	}
	c := newConn(e, r, ws, req, ctx)
	// Registered under the same lock as the closing check, so Shutdown either
	// refuses this connection or sees it.
	e.mu.Lock()
	if e.closing {
		e.mu.Unlock()
		c.cancel()
		_ = ws.Close(transport.StatusGoingAway, "server shutting down")
		return
	}
	e.active.Add(1)
	e.conns[c] = struct{}{}
	e.mu.Unlock()
	defer e.active.Done()
	defer func() {
		e.mu.Lock()
		delete(e.conns, c)
		e.mu.Unlock()
	}()
	c.run()
}

// Shutdown closes every connection with 1001 and waits for their handlers and
// cleanup, or for ctx. New connections are refused. Hijacked connections are not
// tracked by http.Server.Shutdown, so call this too.
func (e *Engine) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	e.closing = true
	conns := make([]*Conn, 0, len(e.conns))
	for c := range e.conns {
		conns = append(conns, c)
	}
	e.mu.Unlock()
	for _, c := range conns {
		c.Close(transport.StatusGoingAway, "server shutting down")
	}
	done := make(chan struct{})
	go func() {
		e.active.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	if e.ownLayer {
		return e.layers.Close()
	}
	return nil
}

// Authenticates reports whether connecting runs authentication (and sends the
// authentication message).
func (r *Route) Authenticates() bool { return r.authenticator() != nil }
