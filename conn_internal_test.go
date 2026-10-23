package chanx

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/huynguyengl99/chanx-go/transport"
)

// stuckConn accepts no write until it is closed, like a client that stopped
// reading with a full TCP window.
type stuckConn struct {
	once   sync.Once
	closed chan struct{}
	code   chan int
}

func newStuckConn() *stuckConn {
	return &stuckConn{closed: make(chan struct{}), code: make(chan int, 1)}
}

func (s *stuckConn) Read(ctx context.Context) (transport.MessageType, []byte, error) {
	select {
	case <-s.closed:
		return 0, nil, &transport.CloseError{Code: transport.StatusNormalClosure}
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	}
}

func (s *stuckConn) Write(context.Context, []byte) error {
	<-s.closed
	return &transport.CloseError{Code: transport.StatusNormalClosure}
}

func (s *stuckConn) Close(code int, _ string) error {
	s.once.Do(func() {
		s.code <- code
		close(s.closed)
	})
	return nil
}

type Flood struct {
	BaseMessage `action:"flood"`
}

type flooder struct{}

func (flooder) OnConnect(ctx *Context) error {
	for range 10 {
		if err := ctx.Push(Flood{}); err != nil {
			return nil // closed for being too slow: expected
		}
	}
	return nil
}

// deadlineConn is a stuckConn whose Write gives up at the context's deadline,
// as coder/websocket and gorilla do.
type deadlineConn struct{ *stuckConn }

func (d deadlineConn) Write(ctx context.Context, _ []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-d.closed:
		return &transport.CloseError{Code: transport.StatusNormalClosure}
	}
}

// A client that stopped reading is closed with 1013, whether the outbox or the
// writer's own write times out first.
func TestSlowClientClosedWith1013(t *testing.T) {
	for name, wrap := range map[string]func(*stuckConn) transport.Conn{
		"outbox":       func(s *stuckConn) transport.Conn { return s },
		"write timing": func(s *stuckConn) transport.Conn { return deadlineConn{s} },
	} {
		t.Run(name, func(t *testing.T) {
			e := New(Config{OutboxSize: 1, WriteTimeout: 50 * time.Millisecond})
			r := e.Handle("/ws", flooder{})
			ws := newStuckConn()
			done := make(chan struct{})
			go func() {
				defer close(done)
				e.Serve(context.Background(), r, wrap(ws), &Request{Path: "/ws"})
			}()
			select {
			case code := <-ws.code:
				if code != transport.StatusTryAgainLater {
					t.Fatalf("close code %d, want 1013", code)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("a client that stopped reading was never closed")
			}
			<-done
		})
	}
}

type closer struct{}

func (closer) OnConnect(ctx *Context) error {
	_ = ctx.Push(Flood{}) // the writer takes it and blocks
	_ = ctx.Push(Flood{}) // fills the outbox (size 1)
	ctx.Close(4001, "now")
	return nil
}

// Close with a full outbox closes the socket at once instead of queueing.
func TestCloseWithFullOutbox(t *testing.T) {
	e := New(Config{OutboxSize: 1, WriteTimeout: time.Minute})
	r := e.Handle("/ws", closer{})
	ws := newStuckConn()
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.Serve(context.Background(), r, ws, &Request{Path: "/ws"})
	}()
	select {
	case code := <-ws.code:
		if code != 4001 {
			t.Fatalf("close code %d, want 4001", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close waited behind a stuck writer")
	}
	<-done
}
