// Package gorillaws accepts WebSockets on net/http with github.com/gorilla/websocket,
// for teams already on gorilla.
package gorillaws

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"go.chanx.dev/transport"
)

// Upgrader is gorilla's upgrader. Its CheckOrigin defaults to same-origin.
type Upgrader = websocket.Upgrader

// Accept upgrades the request; a nil upgrader uses gorilla's defaults.
func Accept(w http.ResponseWriter, r *http.Request, u *Upgrader) (transport.Conn, error) {
	if u == nil {
		u = &websocket.Upgrader{}
	}
	c, err := u.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	return Wrap(c), nil
}

// Wrap adapts a gorilla connection.
func Wrap(c *websocket.Conn) transport.Conn {
	return &conn{c: c}
}

type conn struct {
	c    *websocket.Conn
	once sync.Once
}

func (g *conn) Read(ctx context.Context) (transport.MessageType, []byte, error) {
	// gorilla has no context; a canceled context closes the connection.
	stop := context.AfterFunc(ctx, func() { _ = g.c.Close() })
	defer stop()
	typ, data, err := g.c.ReadMessage()
	if err != nil {
		if ce, ok := err.(*websocket.CloseError); ok {
			return 0, nil, &transport.CloseError{Code: ce.Code, Reason: ce.Text}
		}
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		return 0, nil, err
	}
	if typ == websocket.BinaryMessage {
		return transport.Binary, data, nil
	}
	return transport.Text, data, nil
}

func (g *conn) Write(ctx context.Context, data []byte) error {
	if d, ok := ctx.Deadline(); ok {
		_ = g.c.SetWriteDeadline(d)
	} else {
		_ = g.c.SetWriteDeadline(time.Time{})
	}
	return g.c.WriteMessage(websocket.TextMessage, data)
}

func (g *conn) SetReadLimit(n int64) { g.c.SetReadLimit(n) }

func (g *conn) Close(code int, reason string) error {
	g.once.Do(func() {
		msg := websocket.FormatCloseMessage(code, reason)
		_ = g.c.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
		_ = g.c.Close()
	})
	return nil
}
