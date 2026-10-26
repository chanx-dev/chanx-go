package chanxclient

import (
	"context"
	"reflect"
	"sync"

	chanx "go.chanx.dev"
)

// Topic is a stream multiplexed over a connection.
type Topic struct {
	conn  *Conn
	name  string
	types map[string]reflect.Type
	inbox *queue
}

// Topic returns a topic's handle; incoming are the messages it sends.
func (c *Conn) Topic(name string, incoming ...chanx.Message) *Topic {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.topics[name]; ok {
		return t
	}
	t := &Topic{conn: c, name: name, types: typesOf(incoming), inbox: newQueue()}
	c.topics[name] = t
	return t
}

func (t *Topic) Name() string { return t.name }

// Subscribe waits for the confirmation; a refusal is a *RequestError.
func (t *Topic) Subscribe(ctx context.Context) error {
	_, _, err := t.conn.request(ctx, chanx.SubscribeMessage{}, t.name)
	return err
}

func (t *Topic) Unsubscribe(ctx context.Context) error {
	_, _, err := t.conn.request(ctx, chanx.UnsubscribeMessage{}, t.name)
	return err
}

func (t *Topic) Send(ctx context.Context, m chanx.Message) error {
	return t.conn.write(ctx, m, t.name, "")
}

func (t *Topic) Request(ctx context.Context, m chanx.Message) (chanx.Message, Envelope, error) {
	return t.conn.request(ctx, m, t.name)
}

func (t *Topic) Receive(ctx context.Context) (chanx.Message, Envelope, error) {
	return t.conn.receive(ctx, t.inbox, t.types)
}

// queue is unbounded so a reply is never stuck behind unread broadcasts.
type queue struct {
	mu     sync.Mutex
	frames []frame
	ready  chan struct{}
}

func newQueue() *queue { return &queue{ready: make(chan struct{}, 1)} }

func (q *queue) push(f frame) {
	q.mu.Lock()
	q.frames = append(q.frames, f)
	q.mu.Unlock()
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

func (q *queue) pop(ctx context.Context, done <-chan struct{}) (frame, error) {
	for {
		q.mu.Lock()
		if len(q.frames) > 0 {
			f := q.frames[0]
			q.frames[0] = frame{}
			q.frames = q.frames[1:]
			q.mu.Unlock()
			return f, nil
		}
		q.mu.Unlock()
		select {
		case <-q.ready:
		case <-ctx.Done():
			return frame{}, ctx.Err()
		case <-done:
			q.mu.Lock()
			empty := len(q.frames) == 0
			q.mu.Unlock()
			if empty {
				return frame{}, ErrClosed
			}
		}
	}
}
