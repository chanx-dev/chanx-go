package transport

import (
	"context"
	"sync"
	"sync/atomic"
)

type frame struct {
	typ  MessageType
	data []byte
}

// pipeEnd is one side of an in-memory connection.
type pipeEnd struct {
	in    chan frame
	peer  *pipeEnd
	state *pipeState
	limit atomic.Int64
}

type pipeState struct {
	mu     sync.Mutex
	closed chan struct{}
	err    *CloseError
}

// Pipe returns two connected in-memory Conns, server and client, for tests: no
// network, no goroutines of its own. Frames are buffered up to 1024 per side.
// Client can also write binary frames with WriteType.
func Pipe() (server, client *PipeConn) {
	st := &pipeState{closed: make(chan struct{})}
	a := &pipeEnd{in: make(chan frame, 1024), state: st}
	b := &pipeEnd{in: make(chan frame, 1024), state: st}
	a.peer, b.peer = b, a
	return &PipeConn{a}, &PipeConn{b}
}

// PipeConn is a Conn backed by memory.
type PipeConn struct{ end *pipeEnd }

var _ Conn = (*PipeConn)(nil)

// SetReadLimit closes the connection with 1009 when a larger frame arrives.
func (c *PipeConn) SetReadLimit(n int64) { c.end.limit.Store(n) }

func (c *PipeConn) Read(ctx context.Context) (MessageType, []byte, error) {
	typ, data, err := c.read(ctx)
	if err == nil {
		if limit := c.end.limit.Load(); limit > 0 && int64(len(data)) > limit {
			_ = c.Close(StatusMessageTooBig, "message too big")
			return 0, nil, c.end.state.err
		}
	}
	return typ, data, err
}

func (c *PipeConn) read(ctx context.Context) (MessageType, []byte, error) {
	// Frames already sent are still delivered after a close, as on a socket.
	select {
	case f := <-c.end.in:
		return f.typ, f.data, nil
	default:
	}
	select {
	case f := <-c.end.in:
		return f.typ, f.data, nil
	case <-c.end.state.closed:
		select {
		case f := <-c.end.in:
			return f.typ, f.data, nil
		default:
		}
		return 0, nil, c.end.state.err
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	}
}

func (c *PipeConn) Write(ctx context.Context, data []byte) error {
	return c.WriteType(ctx, Text, data)
}

// WriteType sends a frame of the given type.
func (c *PipeConn) WriteType(ctx context.Context, typ MessageType, data []byte) error {
	select {
	case <-c.end.state.closed:
		return c.end.state.err
	default:
	}
	select {
	case c.end.peer.in <- frame{typ, append([]byte(nil), data...)}:
		return nil
	case <-c.end.state.closed:
		return c.end.state.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *PipeConn) Close(code int, reason string) error {
	st := c.end.state
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.err == nil {
		st.err = &CloseError{Code: code, Reason: reason}
		close(st.closed)
	}
	return nil
}

// Closed returns the close code and reason once the connection is closed.
func (c *PipeConn) Closed() (*CloseError, bool) {
	select {
	case <-c.end.state.closed:
		return c.end.state.err, true
	default:
		return nil, false
	}
}

// Done is closed when either side closes.
func (c *PipeConn) Done() <-chan struct{} { return c.end.state.closed }
