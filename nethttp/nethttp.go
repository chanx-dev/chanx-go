// Package chanxhttp mounts a chanx engine on net/http: http.ServeMux, and through
// plain http.Handlers chi, gorilla/mux, go-zero, Kratos, Encore raw endpoints and
// Huma's routers.
//
//	mux := http.NewServeMux()
//	chanxhttp.Mount(mux, engine)
//	chanxhttp.MountDocs(mux, engine, "/asyncapi", asyncapi.Info{Title: "My API"})
package chanxhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/transport"
	"go.chanx.dev/v2/transport/coderws"
)

// Router is what Mount registers routes on; *http.ServeMux and chi.Router
// satisfy it.
type Router interface {
	Handle(pattern string, h http.Handler)
}

// Option configures the adapter.
type Option func(*config)

type config struct {
	prefix   string
	method   bool
	accept   func(http.ResponseWriter, *http.Request) (transport.Conn, error)
	clientIP func(*http.Request) string
}

// Prefix is prepended to every route path when mounting.
func Prefix(p string) Option { return func(c *config) { c.prefix = p } }

// AcceptOptions configures the coder/websocket upgrade (origins, subprotocols,
// compression).
func AcceptOptions(o *coderws.Options) Option {
	return func(c *config) {
		c.accept = func(w http.ResponseWriter, r *http.Request) (transport.Conn, error) { return coderws.Accept(w, r, o) }
	}
}

// Upgrade replaces the WebSocket library, e.g. gorillaws.Accept.
func Upgrade(fn func(http.ResponseWriter, *http.Request) (transport.Conn, error)) Option {
	return func(c *config) { c.accept = fn }
}

// ClientIP sets how the client address is read; the default is RemoteAddr.
// Behind a proxy, use the same trusted-proxy logic as the rest of the app.
func ClientIP(fn func(*http.Request) string) Option { return func(c *config) { c.clientIP = fn } }

// NoMethod registers bare paths ("/ws/chat/{room}") instead of "GET /ws/...",
// for routers that do not take a method in the pattern (chi's Handle does not).
func NoMethod() Option { return func(c *config) { c.method = false } }

func newConfig(opts []Option) *config {
	c := &config{method: true}
	for _, o := range opts {
		o(c)
	}
	if c.accept == nil {
		c.accept = func(w http.ResponseWriter, r *http.Request) (transport.Conn, error) { return coderws.Accept(w, r, nil) }
	}
	if c.clientIP == nil {
		c.clientIP = func(r *http.Request) string {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				return r.RemoteAddr
			}
			return host
		}
	}
	return c
}

// Mount registers every route of the engine on the router.
func Mount(mux Router, e *chanx.Engine, opts ...Option) {
	c := newConfig(opts)
	for _, r := range e.Routes() {
		pattern := c.prefix + r.Path
		if c.method {
			pattern = "GET " + pattern
		}
		mux.Handle(pattern, handler(e, r, c))
	}
}

// Handler serves one route, for routers Mount does not fit. Path parameters
// come from r.PathValue, or from matching the route pattern against the URL.
func Handler(e *chanx.Engine, r *chanx.Route, opts ...Option) http.Handler {
	return handler(e, r, newConfig(opts))
}

// Dispatcher serves every route from one handler, matching the URL path against
// the route patterns: for a catch-all mount ("/ws/") or routers that set no
// path values. Unknown paths get 404.
func Dispatcher(e *chanx.Engine, opts ...Option) http.Handler {
	c := newConfig(opts)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, params, ok := e.Lookup(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		serveRoute(e, route, c, w, r, params)
	})
}

func handler(e *chanx.Engine, route *chanx.Route, c *config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveRoute(e, route, c, w, r, nil)
	})
}

func serveRoute(e *chanx.Engine, route *chanx.Route, c *config, w http.ResponseWriter, r *http.Request, params chanx.Params) {
	req := NewRequest(r, route)
	if params != nil {
		req.Params = params
	}
	req.ClientIP = c.clientIP(r)
	if err := e.Preflight(r.Context(), route, req); err != nil {
		writeError(w, err)
		return
	}
	ws, err := c.accept(w, r)
	if err != nil {
		return // the library answered the handshake failure
	}
	e.Serve(r.Context(), route, ws, req)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	detail := http.StatusText(status)
	var ae *chanx.AuthError
	if errors.As(err, &ae) {
		status, detail = ae.Status, ae.Detail
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": detail})
}

// NewRequest bridges an *http.Request; context values are read with
// Request.Value.
func NewRequest(r *http.Request, route *chanx.Route) *chanx.Request {
	params := chanx.Params{}
	missing := false
	for _, name := range route.ParamNames {
		v := r.PathValue(name)
		if v == "" {
			missing = true
		}
		params[name] = v
	}
	if missing {
		if p, ok := route.Match(r.URL.Path); ok {
			params = p
		}
	}
	ctx := r.Context()
	return &chanx.Request{
		Path:      r.URL.Path,
		Params:    params,
		Query:     r.URL.Query(),
		Header:    r.Header,
		Cookies:   r.Cookies(),
		ClientIP:  r.RemoteAddr,
		Values:    func(key any) any { return ctx.Value(key) },
		NativeCtx: r,
	}
}

// Shutdown is http.Server.Shutdown plus the engine's: hijacked WebSockets are not
// tracked by the server, so both are needed.
func Shutdown(ctx context.Context, srv *http.Server, e *chanx.Engine) error {
	return errors.Join(e.Shutdown(ctx), srv.Shutdown(ctx))
}

// CoderOptions are coder/websocket's accept options.
type CoderOptions = coderws.Options
