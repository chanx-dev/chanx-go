package auth_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	chanx "go.chanx.dev"
	"go.chanx.dev/auth"
)

type ChatConsumer struct{}

func (ChatConsumer) verify(token string) (string, error) {
	if token != "s3cret" {
		return "", errors.New("unknown token")
	}
	return "ann", nil
}

// Authenticate accepts a bearer token from the Authorization header, the "token"
// query parameter or the "bearer" subprotocol; a missing or rejected one fails
// with 401.
func (c ChatConsumer) Authenticate(ctx *chanx.Context, r *chanx.Request) (any, error) {
	return auth.Bearer(r, c.verify)
}

// Credentials documents where Authenticate looks, as AsyncAPI security schemes.
func (ChatConsumer) Credentials() []chanx.Credential { return auth.BearerCredentials() }

func (ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

func ExampleBearer() {
	fmt.Println(chanx.Check(ChatConsumer{}))
	// Output: <nil>
}

func ExampleToken() {
	r := &chanx.Request{Header: http.Header{}, Query: url.Values{"token": {"from-query"}}}
	fmt.Println(auth.Token(r))
	r.Header.Set("Authorization", "Bearer from-header")
	fmt.Println(auth.Token(r))
	// Output:
	// from-query
	// from-header
}
