package chanx

import (
	"context"
	"errors"
	"log/slog"
	"reflect"

	"go.chanx.dev/layers/v2"
	"go.chanx.dev/v2/protocol"
)

// Publisher pushes to sockets from outside one: HTTP handlers, workers, cron
// jobs. It needs layers only, no engine and no WebSocket dependency, and over a
// shared Redis it reaches sockets held by chanx servers in other languages too.
type Publisher struct {
	layers   layers.Set
	alias    string
	explicit bool // set by Layer: overrides a topic's `layer` tag
	log      *slog.Logger
}

// NewPublisher returns a publisher on the "default" layer of set.
func NewPublisher(set layers.Set) *Publisher { return &Publisher{layers: set, log: slog.Default()} }

// Layer returns a publisher on another layer alias.
func (p *Publisher) Layer(alias string) *Publisher {
	return &Publisher{layers: p.layers, alias: alias, explicit: true, log: p.log}
}

func (p *Publisher) layer(alias string) (layers.Layer, error) {
	if alias == "" {
		alias = p.alias
	}
	return p.layers.Get(alias)
}

// BroadcastEvent sends an event to every connection in the groups; their event
// handler for its action runs.
func (p *Publisher) BroadcastEvent(ctx context.Context, event Message, groups ...string) error {
	l, err := p.layer("")
	if err != nil {
		return err
	}
	data, _, err := layerMessage(event)
	if err != nil {
		return err
	}
	var errs []error
	for _, g := range groups {
		errs = append(errs, l.GroupSend(ctx, g, layers.Message{"type": protocol.EventChannel, "event_data": data}))
	}
	return errors.Join(errs...)
}

// SendEvent sends an event to one connection by its channel name.
func (p *Publisher) SendEvent(ctx context.Context, channel string, event Message) error {
	l, err := p.layer("")
	if err != nil {
		return err
	}
	data, _, err := layerMessage(event)
	if err != nil {
		return err
	}
	return l.Send(ctx, channel, layers.Message{"type": protocol.EventChannel, "event_data": data})
}

// BroadcastMessage forwards a message unchanged to every connection in the
// groups, from outside a consumer.
func (p *Publisher) BroadcastMessage(ctx context.Context, groups []string, m Message) error {
	l, err := p.layer("")
	if err != nil {
		return err
	}
	msg, _, err := layerMessage(m)
	if err != nil {
		return err
	}
	var errs []error
	for _, g := range groups {
		errs = append(errs, l.GroupSend(ctx, g, layers.Message{
			"type": protocol.EventGroup, "message": msg, "exclude_current": false, "from_channel": "",
		}))
	}
	return errors.Join(errs...)
}

// BroadcastTopic pushes an event to everyone subscribed to topic on topic T, on
// the layer T's `layer` tag names unless the publisher was pointed at one.
func BroadcastTopic[T TopicHandler](ctx context.Context, p *Publisher, topic string, event Message, opts ...BroadcastOption) error {
	meta, err := topicMetaOf(reflect.TypeFor[T]())
	if err != nil {
		return err
	}
	alias := p.alias
	if !p.explicit && meta.layer != "" {
		alias = meta.layer
	}
	l, err := p.layer(alias)
	if err != nil {
		return err
	}
	return publishTopic(ctx, l, meta.namespace, topic, event, applyBroadcast(opts))
}

func publishTopic(ctx context.Context, l layers.Layer, namespace, topic string, m Message, o broadcastOptions) error {
	data, _, err := layerMessage(m)
	if err != nil {
		return err
	}
	group, err := groupName(namespace, topic)
	if err != nil {
		return err
	}
	var seq any
	if o.seq != nil {
		seq = *o.seq
	}
	return l.GroupSend(ctx, group, layers.Message{
		"type": protocol.EventTopic, "topic": topic, "seq": seq, "event_data": data,
	})
}
