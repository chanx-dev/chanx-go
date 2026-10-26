package chanx

import (
	"context"
	"errors"
	"log/slog"

	"go.chanx.dev/layers/v2"
	"go.chanx.dev/v2/protocol"
)

// Context is created per frame or event. As a context.Context it is canceled when
// the socket closes. It carries the frame's envelope: replies echo its ref and,
// inside a topic, carry the topic.
type Context struct {
	context.Context
	Conn *Conn
	// Ref is the frame's correlation id, echoed on replies; HasRef tells an empty
	// ref from none.
	Ref    string
	HasRef bool
	// Topic, Params and Seq are set inside a topic.
	Topic  string
	Params Params
	Seq    *int64
	// Action is the action being handled.
	Action string

	topic   *topicDef
	inTopic bool // a frame addressed to topic "" is still inside a topic
	logger  *slog.Logger
}

// EventContext marks a handler as server to server: it handles layer events.
// It never carries a ref.
type EventContext struct{ *Context }

// Logger returns a logger with conn_id, message_id and action bound.
func (c *Context) Logger() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}
	if c.Conn != nil {
		return c.Conn.log
	}
	return slog.Default()
}

// Publisher returns a publisher on the engine's layers, defaulting to this
// route's layer; a topic's `layer` tag still picks its own.
func (c *Context) Publisher() *Publisher {
	return &Publisher{layers: c.Conn.engine.layers, alias: c.Conn.route.alias, log: c.Conn.engine.log}
}

// Send replies on this connection, echoing the frame's ref (and inside a topic,
// stamping the topic and the event's seq).
func (c *Context) Send(m Message) error { return c.write(m, true) }

// Push sends without a ref: the server initiated it.
func (c *Context) Push(m Message) error { return c.write(m, false) }

func (c *Context) write(m Message, withRef bool) error {
	if c.Conn == nil || c.Conn.ws == nil {
		return ErrClosed
	}
	info, payload, err := payloadOf(m)
	if err != nil {
		return err
	}
	return c.Conn.send(info.Action, payload, c.stamp(withRef))
}

func (c *Context) stamp(withRef bool) stamp {
	st := stamp{topic: c.Topic, inTopic: c.inTopic || c.Topic != "", seq: c.Seq, camel: c.Conn.route.camelize()}
	if withRef && c.HasRef {
		ref := c.Ref
		st.ref = &ref
	}
	return st
}

func (conn *Conn) send(action string, payload any, st stamp) error {
	data, err := encodeFrame(action, payload, st)
	if err != nil {
		return err
	}
	if conn.engine.cfg.LogMessages && !conn.engine.ignore[action] && !protocol.IsCompleteAction(action) {
		conn.log.Info("Websocket sent", "sent_action", action)
	}
	return conn.write(data)
}

// BroadcastOption tunes Broadcast, BroadcastTopic and the Publisher.
type BroadcastOption func(*broadcastOptions)

type broadcastOptions struct {
	groups      []string
	hasGroups   bool
	excludeSelf bool
	seq         *int64
}

// ToGroups broadcasts to these groups instead of the connection's.
func ToGroups(groups ...string) BroadcastOption {
	return func(o *broadcastOptions) { o.groups, o.hasGroups = groups, true }
}

// ExcludeSelf skips the sending connection.
func ExcludeSelf() BroadcastOption { return func(o *broadcastOptions) { o.excludeSelf = true } }

// Seq stamps a topic broadcast with a caller-assigned sequence number, for gap
// detection. Leave it unset on a topic with several writers.
func Seq(n int64) BroadcastOption { return func(o *broadcastOptions) { o.seq = &n } }

func applyBroadcast(opts []BroadcastOption) broadcastOptions {
	var o broadcastOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// Broadcast sends a message to every connection in the groups, the
// connection's own groups by default.
func (c *Context) Broadcast(m Message, opts ...BroadcastOption) error {
	o := applyBroadcast(opts)
	groups := o.groups
	if !o.hasGroups {
		groups = c.Conn.Groups()
	}
	msg, _, err := layerMessage(m)
	if err != nil {
		return err
	}
	var errs []error
	for _, g := range groups {
		errs = append(errs, c.Conn.layer.GroupSend(c, g, layers.Message{
			"type":            protocol.EventGroup,
			"message":         msg,
			"exclude_current": o.excludeSelf,
			"from_channel":    c.Conn.ChannelName,
		}))
	}
	return errors.Join(errs...)
}

// BroadcastTopic publishes an event to everyone subscribed to the current topic,
// this connection included.
func (c *Context) BroadcastTopic(m Message, opts ...BroadcastOption) error {
	if c.topic == nil || c.Topic == "" {
		return errors.New("chanx: BroadcastTopic outside a topic")
	}
	return publishTopic(c, c.Conn.layer, c.topic.namespace, c.Topic, m, applyBroadcast(opts))
}

// Close closes the connection after the frames already sent.
func (c *Context) Close(code int, reason string) { c.Conn.Close(code, reason) }
