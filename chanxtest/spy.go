package chanxtest

import (
	"context"
	"sync"

	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/layers"
	"github.com/huynguyengl99/chanx-go/protocol"
)

// SpyLayer wraps a layer and records what is sent through it, to assert on
// what was published rather than on what a second client received:
//
//	spy := chanxtest.Spy(inmemory.New())
//	engine := chanx.New(chanx.Config{Layers: layers.Set{"default": spy}})
//	... code under test publishes through engine.Publisher() or a handler ...
//	joined := chanxtest.BroadcastEvents[UserJoinedEvent](spy)
type SpyLayer struct {
	layers.Layer
	suppress bool
	mu       sync.Mutex
	sends    []Sent
}

// SpyOption configures a SpyLayer.
type SpyOption func(*SpyLayer)

// Suppress records sends without delivering them.
func Suppress() SpyOption { return func(s *SpyLayer) { s.suppress = true } }

// Sent is one recorded Send or GroupSend.
type Sent struct {
	Group   string // set for GroupSend
	Channel string // set for Send
	Message layers.Message
}

// Spy wraps l.
func Spy(l layers.Layer, opts ...SpyOption) *SpyLayer {
	s := &SpyLayer{Layer: l}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *SpyLayer) record(x Sent) {
	s.mu.Lock()
	s.sends = append(s.sends, x)
	s.mu.Unlock()
}

func (s *SpyLayer) Send(ctx context.Context, channel string, msg layers.Message) error {
	s.record(Sent{Channel: channel, Message: msg})
	if s.suppress {
		return nil
	}
	return s.Layer.Send(ctx, channel, msg)
}

func (s *SpyLayer) GroupSend(ctx context.Context, group string, msg layers.Message) error {
	s.record(Sent{Group: group, Message: msg})
	if s.suppress {
		return nil
	}
	return s.Layer.GroupSend(ctx, group, msg)
}

// All returns everything recorded.
func (s *SpyLayer) All() []Sent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Sent(nil), s.sends...)
}

// Reset forgets what was recorded.
func (s *SpyLayer) Reset() {
	s.mu.Lock()
	s.sends = nil
	s.mu.Unlock()
}

func (s *SpyLayer) ofType(typ string) []Sent {
	var out []Sent
	for _, x := range s.All() {
		if x.Message["type"] == typ {
			out = append(out, x)
		}
	}
	return out
}

// Events returns the channel events sent (broadcast_event and send_event).
func (s *SpyLayer) Events() []Sent { return s.ofType(protocol.EventChannel) }

// GroupMessages returns the group messages sent (Broadcast).
func (s *SpyLayer) GroupMessages() []Sent { return s.ofType(protocol.EventGroup) }

// Captured is one recorded event of type T, with where it was sent: a group
// (BroadcastEvent sends one per group) or a channel (SendEvent).
type Captured[T chanx.Message] struct {
	Event   T
	Group   string
	Channel string
}

// BroadcastEvents returns the events of type T sent with BroadcastEvent or
// SendEvent, in order.
func BroadcastEvents[T chanx.Message](s *SpyLayer) []Captured[T] {
	var out []Captured[T]
	for _, x := range s.ofType(protocol.EventChannel) {
		data, _ := x.Message["event_data"].(map[string]any)
		if e, ok, err := decodeAs[T](data); ok && err == nil {
			out = append(out, Captured[T]{Event: e, Group: x.Group, Channel: x.Channel})
		}
	}
	return out
}

// TopicBroadcast is one recorded topic event.
type TopicBroadcast[E chanx.Message] struct {
	Topic string
	Seq   *int64
	Event E
}

// TopicBroadcasts returns the events of type E broadcast on topics of type T.
func TopicBroadcasts[T chanx.TopicHandler, E chanx.Message](s *SpyLayer) []TopicBroadcast[E] {
	var out []TopicBroadcast[E]
	for _, x := range s.ofType(protocol.EventTopic) {
		topic, _ := x.Message["topic"].(string)
		group, err := chanx.TopicGroup[T](topic)
		if err != nil || group != x.Group {
			continue
		}
		data, _ := x.Message["event_data"].(map[string]any)
		e, ok, err := decodeAs[E](data)
		if !ok || err != nil {
			continue
		}
		b := TopicBroadcast[E]{Topic: topic, Event: e}
		if n, ok := layers.Int(x.Message["seq"]); ok {
			b.Seq = &n
		}
		out = append(out, b)
	}
	return out
}
