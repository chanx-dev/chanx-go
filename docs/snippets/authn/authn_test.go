package authn

import (
	"errors"
	"testing"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"
)

func newEngine(cfg chanx.Config) *chanx.Engine {
	e := chanx.New(cfg)
	e.Handle("/ws/chat", ChatConsumer{Tokens: Tokens{"t-ann": {Name: "ann"}, "t-bob": {Name: "bob", Banned: true}}})
	e.Handle("/ws/lobby", LobbyConsumer{}, chanx.UserFrom(UserKey{}))
	return e
}

// The frames and close codes the Authentication guide shows.
func TestFrames(t *testing.T) {
	e := newEngine(chanx.Config{})

	for _, opt := range []chanxtest.Option{chanxtest.Header("Authorization", "Bearer t-ann"), chanxtest.Query("token", "t-ann")} {
		ann := chanxtest.Connect(t, e, "/ws/chat", opt)
		if !chanxtest.JSONEqual(ann.Auth.String(), `{"action": "authentication", "payload": {"status_code": 200, "status_text": "OK", "data": {"name": "ann"}}}`) {
			t.Fatal(ann.Auth)
		}
		ann.SendText(`{"action": "whoami"}`)
		ann.Expect(`{"action": "you_are", "payload": "ann"}`)
	}

	for path, want := range map[string]struct {
		auth string
		code int
	}{
		"/ws/chat":             {`{"action": "authentication", "payload": {"status_code": 401, "status_text": "Authentication credentials were not provided.", "data": null}}`, 4401},
		"/ws/chat?token=nope":  {`{"action": "authentication", "payload": {"status_code": 401, "status_text": "Invalid token.", "data": null}}`, 4401},
		"/ws/chat?token=t-bob": {`{"action": "authentication", "payload": {"status_code": 403, "status_text": "This account is suspended.", "data": null}}`, 4403},
	} {
		c := chanxtest.Connect(t, e, path)
		if !chanxtest.JSONEqual(c.Auth.String(), want.auth) {
			t.Fatalf("%s: %s", path, c.Auth)
		}
		if code := c.WaitClosed(); code != want.code {
			t.Fatalf("%s: closed with %d", path, code)
		}
	}

	l := chanxtest.Connect(t, e, "/ws/lobby", chanxtest.Value(UserKey{}, User{Name: "cy"}))
	l.SendText(`{"action": "whoami"}`)
	l.Expect(`{"action": "you_are", "payload": "cy"}`)

	strict := newEngine(chanx.Config{RejectBeforeUpgrade: true})
	_, err := chanxtest.Dial(t, strict, "/ws/chat?token=t-bob")
	var ae *chanx.AuthError
	if !errors.As(err, &ae) || ae.Status != 403 {
		t.Fatalf("pre-upgrade rejection: %v", err)
	}
}
