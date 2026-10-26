// Package coderws accepts WebSockets on net/http with github.com/coder/websocket,
// the default transport for every net/http-based framework.
package coderws

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"go.chanx.dev/transport"
)

// Options configures the upgrade.
type Options = websocket.AcceptOptions

// Accept upgrades the request. Without options only an origin matching the Host
// header is accepted (coder's default); OriginPatterns or InsecureSkipVerify
// widen it.
func Accept(w http.ResponseWriter, r *http.Request, opts *Options) (transport.Conn, error) {
	c, err := websocket.Accept(w, r, opts)
	if err != nil {
		return nil, err
	}
	return Wrap(c), nil
}

// Wrap adapts a coder connection, accepted or dialed.
func Wrap(c *websocket.Conn) transport.Conn { return &conn{c: c} }

type conn struct{ c *websocket.Conn }

func (c *conn) Read(ctx context.Context) (transport.MessageType, []byte, error) {
	typ, data, err := c.c.Read(ctx)
	if err != nil {
		return 0, nil, mapErr(err)
	}
	if typ == websocket.MessageBinary {
		return transport.Binary, data, nil
	}
	return transport.Text, data, nil
}

func (c *conn) Write(ctx context.Context, data []byte) error {
	return mapErr(c.c.Write(ctx, websocket.MessageText, data))
}

// CloseTimeout bounds the wait for the peer's close frame after ours was sent;
// a client that stopped reading must not stall a shutdown.
var CloseTimeout = time.Second

func (c *conn) SetReadLimit(n int64) { c.c.SetReadLimit(n) }

func (c *conn) Close(code int, reason string) error {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := c.c.Close(websocket.StatusCode(code), reason); err != nil {
			_ = c.c.CloseNow()
		}
	}()
	select {
	case <-done:
	case <-time.After(CloseTimeout):
		// CloseNow waits for a pending Read, which ends once the caller cancels
		// its context; do not block the caller on it.
		go func() { _ = c.c.CloseNow() }()
	}
	return nil
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if code := websocket.CloseStatus(err); code != -1 {
		var ce websocket.CloseError
		reason := ""
		if errors.As(err, &ce) {
			reason = ce.Reason
		}
		return &transport.CloseError{Code: int(code), Reason: reason}
	}
	return err
}
