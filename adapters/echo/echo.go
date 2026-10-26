// Package chanxecho mounts a chanx engine in an echo v5 router: routes, groups
// and middleware are echo's, params come from c.Param, values set with c.Set are
// read through Request.Value, the client IP is c.RealIP(), and pre-upgrade
// failures go through echo's HTTPErrorHandler.
//
//	e := echo.New()
//	chanxecho.Mount(e.Group("", authMiddleware), engine)
package chanxecho

import (
	"net/http"

	"github.com/labstack/echo/v5"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/asyncapi"
	"go.chanx.dev/v2/protocol"
	"go.chanx.dev/v2/transport"
	"go.chanx.dev/v2/transport/coderws"
)

// Router is *echo.Echo or *echo.Group.
type Router interface {
	GET(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) echo.RouteInfo
}

// Accept upgrades a request; coder/websocket by default.
type Accept func(http.ResponseWriter, *http.Request) (transport.Conn, error)

func defaultAccept(w http.ResponseWriter, r *http.Request) (transport.Conn, error) {
	return coderws.Accept(w, r, nil)
}

// Mount registers every route of the engine, "{room}" written ":room".
func Mount(r Router, e *chanx.Engine, accept ...Accept) {
	for _, route := range e.Routes() {
		r.GET(protocol.ColonPath(route.Path), Handler(e, route, accept...))
	}
}

// Handler serves one route.
func Handler(e *chanx.Engine, route *chanx.Route, accept ...Accept) echo.HandlerFunc {
	up := Accept(defaultAccept)
	if len(accept) > 0 && accept[0] != nil {
		up = accept[0]
	}
	return func(c *echo.Context) error {
		req := NewRequest(c, route)
		if err := e.Preflight(c.Request().Context(), route, req); err != nil {
			if ae, ok := err.(*chanx.AuthError); ok {
				return echo.NewHTTPError(ae.Status, ae.Detail)
			}
			return err
		}
		ws, err := up(c.Response(), c.Request())
		if err != nil {
			return nil // the library answered the handshake failure
		}
		e.Serve(c.Request().Context(), route, ws, req)
		return nil
	}
}

// NewRequest bridges an echo context.
func NewRequest(c *echo.Context, route *chanx.Route) *chanx.Request {
	params := chanx.Params{}
	for _, name := range route.ParamNames {
		params[name] = c.Param(name)
	}
	r := c.Request()
	return &chanx.Request{
		Path:     r.URL.Path,
		Params:   params,
		Query:    r.URL.Query(),
		Header:   r.Header,
		Cookies:  r.Cookies(),
		ClientIP: c.RealIP(),
		Values: func(key any) any {
			if s, ok := key.(string); ok {
				if v := c.Get(s); v != nil {
					return v
				}
			}
			return r.Context().Value(key)
		},
		NativeCtx: c,
	}
}

// MountDocs serves the AsyncAPI page at path, JSON at path+".json" and YAML at
// path+".yaml".
func MountDocs(r Router, e *chanx.Engine, path string, info asyncapi.Info) {
	r.GET(path, echo.WrapHandler(asyncapi.HTMLHandler(e, info)))
	r.GET(path+".json", echo.WrapHandler(asyncapi.JSONHandler(e, info)))
	r.GET(path+".yaml", echo.WrapHandler(asyncapi.YAMLHandler(e, info)))
}
