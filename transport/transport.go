// Package transport is the seam between chanx and a WebSocket library: a
// three-method Conn, implemented once per library, so swapping the library never
// touches consumers.
package transport

import (
	"context"
	"errors"
	"fmt"
)

// MessageType is the WebSocket frame type.
type MessageType int

const (
	Text   MessageType = 1
	Binary MessageType = 2
)

// Close codes chanx uses.
const (
	StatusNormalClosure   = 1000
	StatusGoingAway       = 1001
	StatusPolicyViolation = 1008
	StatusMessageTooBig   = 1009
	StatusInternalError   = 1011
	StatusTryAgainLater   = 1013
)

// Conn is an accepted WebSocket. Read is called from one goroutine and Write from
// one other; Close may be called from anywhere, more than once.
type Conn interface {
	// Read returns the next data frame. When the peer closes, the error is a
	// *CloseError.
	Read(ctx context.Context) (MessageType, []byte, error)
	// Write sends a text frame.
	Write(ctx context.Context, data []byte) error
	// Close sends a close frame with code and reason and releases the connection.
	Close(code int, reason string) error
}

// ReadLimiter is implemented by a Conn whose incoming frame size can be capped.
// A larger frame closes the connection with 1009; a negative n means no limit.
type ReadLimiter interface {
	SetReadLimit(n int64)
}

// CloseError is returned by Read once the connection is closed by either side.
type CloseError struct {
	Code   int
	Reason string
}

func (e *CloseError) Error() string {
	return fmt.Sprintf("websocket closed: %d %s", e.Code, e.Reason)
}

// CloseCode returns the close code in err, or -1.
func CloseCode(err error) int {
	var ce *CloseError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return -1
}
