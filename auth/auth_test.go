package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	chanx "go.chanx.dev"
)

func TestTokens(t *testing.T) {
	r := &chanx.Request{Header: http.Header{}, Query: url.Values{}}
	if Token(r) != "" {
		t.Fatal("token from nothing")
	}
	r.Header.Set("Sec-WebSocket-Protocol", "chat, bearer, s3cret")
	if Token(r) != "s3cret" {
		t.Fatal(Token(r))
	}
	r.Query.Set("token", "q")
	if Token(r) != "q" {
		t.Fatal(Token(r))
	}
	r.Header.Set("Authorization", "Bearer h")
	if Token(r) != "h" {
		t.Fatal(Token(r))
	}
	u, err := Bearer(r, func(tok string) (string, error) { return "user:" + tok, nil })
	if err != nil || u != "user:h" {
		t.Fatal(u, err)
	}
	_, err = Bearer(r, func(string) (string, error) { return "", errors.New("expired") })
	var ae *chanx.AuthError
	if !errors.As(err, &ae) || ae.Status != 401 {
		t.Fatal(err)
	}
	_, err = Bearer(r, func(string) (string, error) { return "", fmt.Errorf("lookup: %w", chanx.Forbidden("banned")) })
	if !errors.As(err, &ae) || ae.Status != 403 || ae.Detail != "banned" {
		t.Fatalf("wrapped Forbidden: %v", err)
	}
}
