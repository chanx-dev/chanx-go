package chanx

import (
	"errors"
	"fmt"
	"net/http"
)

// AuthError is an authentication failure with the status reported to the
// client: in the authentication message, or as the HTTP status when the route
// rejects before the upgrade.
type AuthError struct {
	Status int
	Detail string
}

func (e *AuthError) Error() string { return fmt.Sprintf("chanx: %d %s", e.Status, e.Detail) }

// Unauthenticated fails authentication with 401 and detail as status_text.
func Unauthenticated(detail string) error {
	if detail == "" {
		detail = http.StatusText(http.StatusUnauthorized)
	}
	return &AuthError{Status: http.StatusUnauthorized, Detail: detail}
}

// Forbidden fails authentication with 403 and detail as status_text.
func Forbidden(detail string) error {
	if detail == "" {
		detail = http.StatusText(http.StatusForbidden)
	}
	return &AuthError{Status: http.StatusForbidden, Detail: detail}
}

// authStatus maps an Authenticate error to the reported status. An error that
// is not an *AuthError is a 401 "Unauthorized": its text could leak internals.
func authStatus(err error) (int, string) {
	var ae *AuthError
	if errors.As(err, &ae) {
		return ae.Status, ae.Detail
	}
	return http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized)
}

// authCloseCode closes a failed authentication with 4000 + a 4xx or 5xx status
// (4401, 4403, 4500), else 4401, so a client can stop reconnecting.
func authCloseCode(status int) int {
	if status >= 400 && status < 600 {
		return 4000 + status
	}
	return 4401
}

// Credential is where Authenticate reads a credential from, documented in
// AsyncAPI as a security scheme. Any one of a consumer's credentials is enough.
type Credential struct {
	Name        string
	Location    string // "query", "header" or "cookie"
	Description string
}

// ValueKey names a value framework middleware stored on the request.
type ValueKey any

// UserFrom authenticates a route from a value its framework middleware set
// (gin's c.Set("user", u), Fiber's c.Locals, a context value), for consumers
// without an Authenticate method: a missing value fails with 401.
//
//	engine.Handle("/ws/chat", &ChatConsumer{}, chanx.UserFrom("user"))
func UserFrom(key ValueKey) Option {
	return func(o *routeOptions) {
		o.userFrom = func(_ *Context, r *Request) (any, error) {
			if u := r.Value(key); u != nil {
				return u, nil
			}
			return nil, Unauthenticated("")
		}
	}
}
