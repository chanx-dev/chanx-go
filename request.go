package chanx

import (
	"net/http"
	"net/url"
	"strings"
)

// Request is the handshake request as the framework saw it, built by an adapter
// before the upgrade. Fiber recycles its context, so its adapter snapshots what
// is read here and Native returns nil once upgraded.
type Request struct {
	Path     string
	Params   Params
	Query    url.Values
	Header   http.Header
	Cookies  []*http.Cookie
	ClientIP string
	// Values reads the framework's per-request store (gin and echo c.Get, Fiber
	// c.Locals, net/http context values).
	Values func(key any) any
	// NativeCtx is the framework's own context (*http.Request, *gin.Context...).
	NativeCtx any
	// OnReady, when set, is called once the connection is set up (authenticated,
	// groups joined, OnConnect run, a dedicated topic subscribed) or has failed
	// to: from then on, broadcasts to its groups reach it.
	OnReady func()

	preUser   any
	preAuthed bool
}

// Param returns a path parameter.
func (r *Request) Param(name string) string { return r.Params[name] }

// Value reads a value set by framework middleware, nil when absent.
func (r *Request) Value(key any) any {
	if r.Values == nil {
		return nil
	}
	return r.Values(key)
}

// Cookie returns the named cookie.
func (r *Request) Cookie(name string) (*http.Cookie, error) {
	for _, c := range r.Cookies {
		if c.Name == name {
			return c, nil
		}
	}
	return nil, http.ErrNoCookie
}

// Native returns the framework's context, for escape hatches.
func (r *Request) Native() any { return r.NativeCtx }

// Subprotocols lists the Sec-WebSocket-Protocol values the client offered.
func (r *Request) Subprotocols() []string {
	var out []string
	for _, h := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, p := range strings.Split(h, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
