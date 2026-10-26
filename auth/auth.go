// Package auth extracts bearer tokens from a WebSocket handshake, so the common
// JWT setup is one line with your own verify function:
//
//	func (c *ChatConsumer) Authenticate(ctx *chanx.Context, r *chanx.Request) (any, error) {
//		return auth.Bearer(r, c.verify)
//	}
package auth

import (
	"errors"
	"strings"

	chanx "go.chanx.dev/v2"
)

// BearerToken returns the token of an "Authorization: Bearer <token>" header.
func BearerToken(r *chanx.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// QueryToken returns a token passed as a query parameter (browsers cannot set
// headers on a WebSocket).
func QueryToken(r *chanx.Request, name string) string { return r.Query.Get(name) }

// SubprotocolToken returns a token offered as the Sec-WebSocket-Protocol entry
// after the marker ("bearer", "<token>"), the other way browsers can send one.
func SubprotocolToken(r *chanx.Request, marker string) string {
	protos := r.Subprotocols()
	for i, p := range protos {
		if strings.EqualFold(p, marker) && i+1 < len(protos) {
			return protos[i+1]
		}
	}
	return ""
}

// Token returns the first token found in the Authorization header, the query
// parameter "token", or after the "bearer" subprotocol.
func Token(r *chanx.Request) string {
	if t := BearerToken(r); t != "" {
		return t
	}
	if t := QueryToken(r, "token"); t != "" {
		return t
	}
	return SubprotocolToken(r, "bearer")
}

// Bearer verifies the request's token: a missing token or a verify error fails
// with 401 (verify may return chanx.Forbidden for a 403).
func Bearer[U any](r *chanx.Request, verify func(token string) (U, error)) (any, error) {
	tok := Token(r)
	if tok == "" {
		return nil, chanx.Unauthenticated("Authentication credentials were not provided.")
	}
	u, err := verify(tok)
	if err != nil {
		if errors.As(err, new(*chanx.AuthError)) {
			return nil, err
		}
		return nil, chanx.Unauthenticated("Invalid token.")
	}
	return u, nil
}

// BearerCredentials documents where Token looks, for a consumer's Credentials
// hook: the Authorization header and the "token" query parameter. The
// subprotocol has no AsyncAPI security scheme.
func BearerCredentials() []chanx.Credential {
	return []chanx.Credential{
		{Name: "Authorization", Location: "header", Description: "Bearer token"},
		{Name: "token", Location: "query", Description: "Bearer token, for browsers"},
	}
}
