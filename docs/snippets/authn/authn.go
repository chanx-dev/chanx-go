// Package authn holds the examples of the Authentication guide.
package authn

import (
	"context"
	"errors"
	"net/http"

	chanx "go.chanx.dev"
	"go.chanx.dev/auth"
)

// User is what authentication returns, kept on the connection.
type User struct {
	Name   string
	Banned bool
}

// Tokens stands in for your token store or JWT verification.
type Tokens map[string]User

var errUnknownToken = errors.New("unknown token")

// #region authenticate
type ChatConsumer struct {
	Tokens Tokens
}

// verify maps a token to a user. A chanx.Forbidden error is reported as 403;
// any other error as 401 "Invalid token.".
func (c ChatConsumer) verify(token string) (User, error) {
	user, ok := c.Tokens[token]
	switch {
	case !ok:
		return User{}, errUnknownToken
	case user.Banned:
		return User{}, chanx.Forbidden("This account is suspended.")
	}
	return user, nil
}

// Authenticate runs once per connection, before any handler. auth.Bearer reads the
// token from the Authorization header, the "token" query parameter or the
// "bearer" subprotocol.
func (c ChatConsumer) Authenticate(ctx *chanx.Context, r *chanx.Request) (any, error) {
	return auth.Bearer(r, c.verify)
}

// AuthenticationData fills the data of the authentication message.
func (ChatConsumer) AuthenticationData(ctx *chanx.Context, user any) any {
	return map[string]string{"name": user.(User).Name}
}

// Credentials documents where Authenticate looks, as AsyncAPI security schemes.
func (ChatConsumer) Credentials() []chanx.Credential { return auth.BearerCredentials() }

// #endregion authenticate

type WhoAmI struct {
	chanx.BaseMessage `action:"whoami"`
}

type YouAre struct {
	chanx.BaseMessage `action:"you_are"`
	Payload           string `json:"payload"`
}

// #region user
// HandleWhoami reads the user Authenticate returned.
func (ChatConsumer) HandleWhoami(ctx *chanx.Context, _ WhoAmI) (YouAre, error) {
	return YouAre{Payload: ctx.Conn.User.(User).Name}, nil
}

// #endregion user

// #region middleware
// UserKey is the context key the middleware stores the user under.
type UserKey struct{}

// RequireUser is ordinary net/http middleware: it runs before the upgrade, and
// what it stores reaches the consumer.
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.Header.Get("X-User") // in real life: a session or a verified token
		if name == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), UserKey{}, User{Name: name})))
	})
}

// LobbyConsumer has no Authenticate: the route takes the user from the middleware
// with chanx.UserFrom(UserKey{}).
type LobbyConsumer struct{}

func (LobbyConsumer) HandleWhoami(ctx *chanx.Context, _ WhoAmI) (YouAre, error) {
	return YouAre{Payload: ctx.Conn.User.(User).Name}, nil
}

// #endregion middleware
