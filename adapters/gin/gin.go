// Package chanxgin mounts a chanx engine in a gin router: routes live in gin's
// router (groups, middleware, NoRoute), params come from c.Param, values set by
// middleware with c.Set are read through Request.Value, and the client IP is
// gin's c.ClientIP() with its trusted proxies.
//
//	r := gin.Default()
//	chanxgin.Mount(r.Group("/", authMiddleware), engine)
//	chanxgin.MountDocs(r, engine, "/asyncapi", sandbox.Info)
package chanxgin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/asyncapi"
	"github.com/huynguyengl99/chanx-go/protocol"
	"github.com/huynguyengl99/chanx-go/transport"
	"github.com/huynguyengl99/chanx-go/transport/coderws"
)

// Option configures the adapter.
type Option func(*config)

type config struct {
	accept func(http.ResponseWriter, *http.Request) (transport.Conn, error)
}

// AcceptOptions configures the coder/websocket upgrade.
func AcceptOptions(o *coderws.Options) Option {
	return func(c *config) {
		c.accept = func(w http.ResponseWriter, r *http.Request) (transport.Conn, error) { return coderws.Accept(w, r, o) }
	}
}

// Upgrade replaces the WebSocket library.
func Upgrade(fn func(http.ResponseWriter, *http.Request) (transport.Conn, error)) Option {
	return func(c *config) { c.accept = fn }
}

// Mount registers every route of the engine as a GET route, "{room}" written
// ":room" as gin expects.
func Mount(r gin.IRoutes, e *chanx.Engine, opts ...Option) {
	c := &config{accept: func(w http.ResponseWriter, r *http.Request) (transport.Conn, error) { return coderws.Accept(w, r, nil) }}
	for _, o := range opts {
		o(c)
	}
	for _, route := range e.Routes() {
		r.GET(protocol.ColonPath(route.Path), Handler(e, route, c.accept))
	}
}

// Handler serves one route.
func Handler(e *chanx.Engine, route *chanx.Route, accept func(http.ResponseWriter, *http.Request) (transport.Conn, error)) gin.HandlerFunc {
	if accept == nil {
		accept = func(w http.ResponseWriter, r *http.Request) (transport.Conn, error) { return coderws.Accept(w, r, nil) }
	}
	return func(c *gin.Context) {
		req := NewRequest(c, route)
		if err := e.Preflight(c.Request.Context(), route, req); err != nil {
			status, detail := http.StatusInternalServerError, err.Error()
			if ae, ok := err.(*chanx.AuthError); ok {
				status, detail = ae.Status, ae.Detail
			}
			c.AbortWithStatusJSON(status, gin.H{"detail": detail})
			return
		}
		ws, err := accept(c.Writer, c.Request)
		if err != nil {
			c.Abort()
			return
		}
		// gin's handler blocks for the socket's lifetime, so c stays valid.
		e.Serve(c.Request.Context(), route, ws, req)
	}
}

// NewRequest bridges a gin context.
func NewRequest(c *gin.Context, route *chanx.Route) *chanx.Request {
	params := chanx.Params{}
	for _, name := range route.ParamNames {
		params[name] = c.Param(name)
	}
	return &chanx.Request{
		Path:     c.Request.URL.Path,
		Params:   params,
		Query:    c.Request.URL.Query(),
		Header:   c.Request.Header,
		Cookies:  c.Request.Cookies(),
		ClientIP: c.ClientIP(),
		Values: func(key any) any {
			if s, ok := key.(string); ok {
				if v, ok := c.Get(s); ok {
					return v
				}
			}
			return c.Request.Context().Value(key)
		},
		NativeCtx: c,
	}
}

// MountDocs serves the AsyncAPI page at path, JSON at path+".json" and YAML at
// path+".yaml".
func MountDocs(r gin.IRoutes, e *chanx.Engine, path string, info asyncapi.Info) {
	r.GET(path, gin.WrapH(asyncapi.HTMLHandler(e, info)))
	r.GET(path+".json", gin.WrapH(asyncapi.JSONHandler(e, info)))
	r.GET(path+".yaml", gin.WrapH(asyncapi.YAMLHandler(e, info)))
}
