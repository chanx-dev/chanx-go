// Package chanxfiber mounts a chanx engine in a Fiber v3 router (fasthttp). Routes,
// groups and middleware are Fiber's; pre-upgrade failures go through its
// ErrorHandler. Fiber recycles its context once the upgrade returns, so params,
// query, headers, cookies, client IP and string-keyed Locals (string and []byte
// values cloned) are copied before upgrading, and Request.Native returns nil.
//
//	app := fiber.New()
//	chanxfiber.Mount(app.Group("/", authMiddleware), engine)
package chanxfiber

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	fws "github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/asyncapi"
	"go.chanx.dev/v2/protocol"
	"go.chanx.dev/v2/transport"
)

// Option configures the adapter.
type Option func(*config)

type config struct {
	locals []any
	ws     fws.Config
}

// Locals names non-string Locals keys to copy before the upgrade; string keys
// are copied anyway.
func Locals(keys ...any) Option { return func(c *config) { c.locals = append(c.locals, keys...) } }

// WebSocket configures the upgrade (origins, subprotocols, buffers).
func WebSocket(cfg fws.Config) Option { return func(c *config) { c.ws = cfg } }

// Mount registers every route of the engine, "{room}" written ":room".
func Mount(r fiber.Router, e *chanx.Engine, opts ...Option) {
	for _, route := range e.Routes() {
		r.Get(protocol.ColonPath(route.Path), Handler(e, route, opts...))
	}
}

// Handler serves one route.
func Handler(e *chanx.Engine, route *chanx.Route, opts ...Option) fiber.Handler {
	c := &config{}
	for _, o := range opts {
		o(c)
	}
	return func(ctx fiber.Ctx) error {
		if !fws.IsWebSocketUpgrade(ctx) {
			return fiber.ErrUpgradeRequired
		}
		req := snapshot(ctx, route, c.locals)
		if err := e.Preflight(ctx.Context(), route, req); err != nil {
			if ae, ok := err.(*chanx.AuthError); ok {
				return fiber.NewError(ae.Status, ae.Detail)
			}
			return err
		}
		return fws.New(func(conn *fws.Conn) {
			e.Serve(context.Background(), route, Wrap(conn.Conn), req)
		}, c.ws)(ctx)
	}
}

// snapshot copies what chanx reads, since Fiber reuses ctx and its strings.
func snapshot(ctx fiber.Ctx, route *chanx.Route, keys []any) *chanx.Request {
	params := chanx.Params{}
	for _, name := range route.ParamNames {
		params[name] = strings.Clone(ctx.Params(name))
	}
	query := url.Values{}
	for k, v := range ctx.RequestCtx().QueryArgs().All() { // Queries() keeps one value per key
		query.Add(string(k), string(v))
	}
	header := http.Header{}
	for k, vs := range ctx.GetReqHeaders() {
		for _, v := range vs {
			header.Add(strings.Clone(k), strings.Clone(v))
		}
	}
	values := map[any]any{}
	ctx.RequestCtx().VisitUserValues(func(k []byte, v any) { values[string(k)] = cloneValue(v) })
	for _, k := range keys {
		if v := ctx.Locals(k); v != nil {
			values[k] = cloneValue(v)
		}
	}
	return &chanx.Request{
		Path:     strings.Clone(ctx.Path()),
		Params:   params,
		Query:    query,
		Header:   header,
		Cookies:  (&http.Request{Header: header}).Cookies(),
		ClientIP: strings.Clone(ctx.IP()),
		Values:   func(key any) any { return values[key] },
	}
}

// cloneValue copies a string or byte slice Local: one read with c.Get or c.Body
// in a middleware points into a buffer fasthttp reuses for the next request.
func cloneValue(v any) any {
	switch x := v.(type) {
	case string:
		return strings.Clone(x)
	case []byte:
		return bytes.Clone(x)
	}
	return v
}

// Wrap adapts a fasthttp/websocket connection.
func Wrap(c *websocket.Conn) transport.Conn { return &conn{c: c} }

type conn struct {
	c    *websocket.Conn
	once sync.Once
}

func (f *conn) Read(ctx context.Context) (transport.MessageType, []byte, error) {
	stop := context.AfterFunc(ctx, func() { _ = f.c.NetConn().Close() })
	defer stop()
	typ, data, err := f.c.ReadMessage()
	if err != nil {
		if ce, ok := err.(*websocket.CloseError); ok {
			return 0, nil, &transport.CloseError{Code: ce.Code, Reason: ce.Text}
		}
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		return 0, nil, err
	}
	if typ == websocket.BinaryMessage {
		return transport.Binary, data, nil
	}
	return transport.Text, data, nil
}

func (f *conn) Write(ctx context.Context, data []byte) error {
	if d, ok := ctx.Deadline(); ok {
		_ = f.c.SetWriteDeadline(d)
	} else {
		_ = f.c.SetWriteDeadline(time.Time{})
	}
	return f.c.WriteMessage(websocket.TextMessage, data)
}

func (f *conn) SetReadLimit(n int64) { f.c.SetReadLimit(n) }

func (f *conn) Close(code int, reason string) error {
	f.once.Do(func() {
		_ = f.c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
		_ = f.c.NetConn().Close()
	})
	return nil
}

// MountDocs serves the AsyncAPI page at path, JSON at path+".json" and YAML at
// path+".yaml".
func MountDocs(r fiber.Router, e *chanx.Engine, path string, info asyncapi.Info) {
	doc := func(render func(*chanx.Engine, asyncapi.Info) ([]byte, error), contentType string) fiber.Handler {
		return func(c fiber.Ctx) error {
			i := info
			if i.ServerURL == "" {
				i.ServerURL = c.Host()
			}
			data, err := render(e, i)
			if err != nil {
				return err
			}
			c.Set("Content-Type", contentType)
			return c.Send(data)
		}
	}
	r.Get(path+".json", doc(asyncapi.JSON, "application/json"))
	r.Get(path+".yaml", doc(asyncapi.YAML, "application/x-yaml"))
	r.Get(path, func(c fiber.Ctx) error {
		rec := &recorder{header: http.Header{}}
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		req.Host = c.Host()
		asyncapi.HTMLHandler(e, info).ServeHTTP(rec, req)
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.Status(rec.status()).Send(rec.body)
	})
}

type recorder struct {
	header http.Header
	code   int
	body   []byte
}

func (r *recorder) Header() http.Header         { return r.header }
func (r *recorder) Write(b []byte) (int, error) { r.body = append(r.body, b...); return len(b), nil }
func (r *recorder) WriteHeader(code int)        { r.code = code }
func (r *recorder) status() int {
	if r.code == 0 {
		return http.StatusOK
	}
	return r.code
}
