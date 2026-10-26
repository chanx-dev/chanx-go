// Package redis provides Redis channel layers wire-compatible with
// channels_redis and fast-channels: same key layout, msgpack or JSON, optional
// Fernet encryption and the same client-side shard ring, so processes in any
// language can share one Redis.
package redis

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"go.chanx.dev/layers/v2"
	"go.chanx.dev/layers/v2/serializer"
)

// PubSubOptions configures the pub/sub layer.
type PubSubOptions struct {
	// Hosts are Redis URLs (redis://, rediss://, unix://). Their order defines the
	// shard ring and must match every process sharing it. Default redis://localhost:6379.
	Hosts []string
	// Clients replaces Hosts with ready clients, for Sentinel
	// (goredis.NewFailoverClient) or custom options. They are not closed by Close.
	Clients []goredis.UniversalClient
	// Prefix starts every Redis channel name. Default "asgi".
	Prefix string
	// SerializerFormat is "msgpack" (default) or "json".
	SerializerFormat string
	// SymmetricEncryptionKeys turns on Fernet: the first key encrypts, any decrypts.
	SymmetricEncryptionKeys []string
	// Serializer replaces SerializerFormat and SymmetricEncryptionKeys.
	Serializer serializer.Serializer
	// BufferSize bounds each local channel's buffer: an overflow drops the
	// message, logs it and calls OnOverflow. Default 1024.
	BufferSize int
	// SubscribeTimeout bounds the wait for Redis to acknowledge a SUBSCRIBE. Default 5s.
	SubscribeTimeout time.Duration
	// Sharded uses SSUBSCRIBE/SPUBLISH (Redis Cluster sharded pub/sub). Only
	// processes using this option read these messages.
	Sharded bool
	Logger  *slog.Logger
	// OnOverflow is called with the local channel that dropped a message.
	OnOverflow func(channel string)
	// OnReconnect is called after a shard's connection came back; go-redis has
	// resubscribed by then. Messages published while it was down are lost.
	OnReconnect func(shard int)
}

// PubSub is the Redis pub/sub channel layer: fire and forget, nothing persisted,
// group membership local to each process.
type PubSub struct {
	opts   PubSubOptions
	ser    serializer.Serializer
	shards []*shard
	log    *slog.Logger

	// membership serializes group joins and leaves with their SUBSCRIBE and
	// UNSUBSCRIBE, so a leave cannot unsubscribe a group another member just
	// joined. Not a hot path: only connects, disconnects and topic changes.
	membership sync.Mutex

	mu       sync.Mutex
	channels map[string]chan layers.Message
	groups   map[string]map[string]struct{} // Redis channel of a group -> local members
	closed   chan struct{}
	once     sync.Once
}

var _ layers.Layer = (*PubSub)(nil)

// NewPubSub returns a pub/sub layer. It does not connect until first used.
func NewPubSub(opts PubSubOptions) (*PubSub, error) {
	if opts.Prefix == "" {
		opts.Prefix = "asgi"
	}
	if opts.BufferSize <= 0 {
		opts.BufferSize = 1024
	}
	if opts.SubscribeTimeout <= 0 {
		opts.SubscribeTimeout = 5 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	ser := opts.Serializer
	if ser == nil {
		var format serializer.Format
		switch opts.SerializerFormat {
		case "", "msgpack":
			format = MsgPack
		case "json":
			format = serializer.JSON
		default:
			return nil, fmt.Errorf("redis layer: unknown serializer format %q", opts.SerializerFormat)
		}
		ser = serializer.New(serializer.Options{Format: format, SymmetricEncryptionKeys: opts.SymmetricEncryptionKeys})
	}
	l := &PubSub{
		opts:     opts,
		ser:      ser,
		log:      opts.Logger,
		channels: map[string]chan layers.Message{},
		groups:   map[string]map[string]struct{}{},
		closed:   make(chan struct{}),
	}
	if len(opts.Clients) > 0 {
		for i, c := range opts.Clients {
			l.shards = append(l.shards, newShard(l, i, c, false))
		}
		return l, nil
	}
	hosts := opts.Hosts
	if len(hosts) == 0 {
		hosts = []string{"redis://localhost:6379"}
	}
	for i, host := range hosts {
		o, err := goredis.ParseURL(host)
		if err != nil {
			return nil, fmt.Errorf("redis layer: host %d: %w", i, err)
		}
		l.shards = append(l.shards, newShard(l, i, goredis.NewClient(o), true))
	}
	return l, nil
}

func (l *PubSub) shardFor(name string) *shard { return l.shards[consistentHash(name, len(l.shards))] }

func (l *PubSub) groupChannel(group string) string { return l.opts.Prefix + "__group__" + group }

func (l *PubSub) isClosed() bool {
	select {
	case <-l.closed:
		return true
	default:
		return false
	}
}

// NewChannel returns "{Prefix}{prefix}{32 hex}" and returns once Redis has
// acknowledged the subscription.
func (l *PubSub) NewChannel(ctx context.Context, prefix string) (string, error) {
	if l.isClosed() {
		return "", layers.ErrClosed
	}
	name := l.opts.Prefix + prefix + layers.RandomSuffix()
	l.mu.Lock()
	l.channels[name] = make(chan layers.Message, l.opts.BufferSize)
	l.mu.Unlock()
	if err := l.shardFor(name).subscribe(ctx, name); err != nil {
		l.mu.Lock()
		delete(l.channels, name)
		l.mu.Unlock()
		return "", err
	}
	return name, nil
}

func (l *PubSub) Receive(ctx context.Context, channel string) (layers.Message, error) {
	l.mu.Lock()
	ch, ok := l.channels[channel]
	l.mu.Unlock()
	if !ok {
		if l.isClosed() {
			return nil, layers.ErrClosed
		}
		return nil, fmt.Errorf("%w: %s", layers.ErrNotLocal, channel)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-l.closed:
		return nil, layers.ErrClosed
	case msg := <-ch:
		return msg, nil
	}
}

func (l *PubSub) Send(ctx context.Context, channel string, msg layers.Message) error {
	if err := layers.ValidChannelName(channel); err != nil {
		return err
	}
	data, err := l.ser.Serialize(msg)
	if err != nil {
		return err
	}
	return l.shardFor(channel).publish(ctx, channel, data)
}

// GroupAdd subscribes the group's Redis channel on its first local member and
// returns once Redis acknowledged it, so "join, then publish" never loses the
// message.
func (l *PubSub) GroupAdd(ctx context.Context, group, channel string) error {
	if err := layers.ValidGroupName(group); err != nil {
		return err
	}
	gch := l.groupChannel(group)
	l.membership.Lock()
	defer l.membership.Unlock()
	l.mu.Lock()
	if _, ok := l.channels[channel]; !ok {
		l.mu.Unlock()
		return fmt.Errorf("%w: GroupAdd(%q, %q)", layers.ErrNotLocal, group, channel)
	}
	if l.groups[gch] == nil {
		l.groups[gch] = map[string]struct{}{}
	}
	_, member := l.groups[gch][channel]
	l.groups[gch][channel] = struct{}{}
	l.mu.Unlock()
	err := l.shardFor(gch).subscribe(ctx, gch)
	if err != nil && !member {
		l.mu.Lock()
		delete(l.groups[gch], channel)
		empty := len(l.groups[gch]) == 0
		if empty {
			delete(l.groups, gch)
		}
		l.mu.Unlock()
		if empty {
			_ = l.shardFor(gch).unsubscribe(context.WithoutCancel(ctx), gch)
		}
	}
	return err
}

// GroupDiscard unsubscribes the group's Redis channel after its last local member.
func (l *PubSub) GroupDiscard(ctx context.Context, group, channel string) error {
	if err := layers.ValidGroupName(group); err != nil {
		return err
	}
	gch := l.groupChannel(group)
	l.membership.Lock()
	defer l.membership.Unlock()
	l.mu.Lock()
	members := l.groups[gch]
	if _, ok := members[channel]; !ok {
		l.mu.Unlock()
		return nil
	}
	delete(members, channel)
	empty := len(members) == 0
	if empty {
		delete(l.groups, gch)
	}
	l.mu.Unlock()
	if empty {
		return l.shardFor(gch).unsubscribe(ctx, gch)
	}
	return nil
}

func (l *PubSub) GroupSend(ctx context.Context, group string, msg layers.Message) error {
	if err := layers.ValidGroupName(group); err != nil {
		return err
	}
	data, err := l.ser.Serialize(msg)
	if err != nil {
		return err
	}
	gch := l.groupChannel(group)
	return l.shardFor(gch).publish(ctx, gch, data)
}

// DeleteChannel stops receiving on a channel and leaves every group it joined.
func (l *PubSub) DeleteChannel(ctx context.Context, channel string) error {
	l.membership.Lock()
	defer l.membership.Unlock()
	l.mu.Lock()
	_, ok := l.channels[channel]
	delete(l.channels, channel)
	var emptied []string
	for gch, members := range l.groups {
		if _, in := members[channel]; in {
			delete(members, channel)
			if len(members) == 0 {
				delete(l.groups, gch)
				emptied = append(emptied, gch)
			}
		}
	}
	l.mu.Unlock()
	var errs []error
	if ok {
		errs = append(errs, l.shardFor(channel).unsubscribe(ctx, channel))
	}
	for _, gch := range emptied {
		errs = append(errs, l.shardFor(gch).unsubscribe(ctx, gch))
	}
	return errors.Join(errs...)
}

// Flush leaves every group and drops buffered messages. Channels stay usable.
func (l *PubSub) Flush(ctx context.Context) error {
	l.membership.Lock()
	defer l.membership.Unlock()
	l.mu.Lock()
	groups := l.groups
	l.groups = map[string]map[string]struct{}{}
	for _, ch := range l.channels {
	drain:
		for {
			select {
			case <-ch:
			default:
				break drain
			}
		}
	}
	l.mu.Unlock()
	var errs []error
	for gch := range groups {
		errs = append(errs, l.shardFor(gch).unsubscribe(ctx, gch))
	}
	return errors.Join(errs...)
}

// Close stops every receive loop, wakes pending Receive calls with ErrClosed and
// closes the clients the layer created.
func (l *PubSub) Close() error {
	var errs []error
	l.once.Do(func() {
		close(l.closed)
		for _, s := range l.shards {
			errs = append(errs, s.close())
		}
	})
	return errors.Join(errs...)
}

// deliver hands a Redis message to the local channel or to every local member of
// the group. Each member decodes its own copy.
func (l *PubSub) deliver(name string, data []byte) {
	l.mu.Lock()
	type target struct {
		name string
		ch   chan layers.Message
	}
	var targets []target
	if ch, ok := l.channels[name]; ok {
		targets = append(targets, target{name, ch})
	} else {
		for member := range l.groups[name] {
			if ch, ok := l.channels[member]; ok {
				targets = append(targets, target{member, ch})
			}
		}
	}
	l.mu.Unlock()
	for _, t := range targets {
		msg, err := l.ser.Deserialize(data)
		if err != nil {
			l.log.Error("redis layer: cannot decode message", "channel", name, "err", err)
			return
		}
		select {
		case t.ch <- msg:
		default:
			l.log.Warn("redis layer: channel buffer full, message dropped", "channel", t.name, "buffer", l.opts.BufferSize)
			if l.opts.OnOverflow != nil {
				l.opts.OnOverflow(t.name)
			}
		}
	}
}

// shard owns one connection's subscriptions and its receive loop. go-redis'
// PubSub.Channel() drops messages for slow readers after a timeout, so the loop
// reads Receive itself and the layer decides what an overflow means.
type shard struct {
	index  int
	layer  *PubSub
	client goredis.UniversalClient
	owned  bool

	mu   sync.Mutex
	ps   *goredis.PubSub
	subs map[string]*subscription
	stop context.CancelFunc
	done chan struct{}
}

type subscription struct {
	acked   bool
	waiters []chan struct{}
}

func newShard(l *PubSub, index int, c goredis.UniversalClient, owned bool) *shard {
	return &shard{index: index, layer: l, client: c, owned: owned, subs: map[string]*subscription{}}
}

func (s *shard) publish(ctx context.Context, channel string, data []byte) error {
	if s.layer.opts.Sharded {
		return s.client.SPublish(ctx, channel, data).Err()
	}
	return s.client.Publish(ctx, channel, data).Err()
}

// subscribe returns once Redis acknowledged the subscription.
func (s *shard) subscribe(ctx context.Context, channel string) error {
	s.mu.Lock()
	if s.layer.isClosed() { // checked under s.mu: close cannot run between this and the loop's start
		s.mu.Unlock()
		return layers.ErrClosed
	}
	sub := s.subs[channel]
	if sub != nil && sub.acked {
		s.mu.Unlock()
		return nil
	}
	ack := make(chan struct{})
	if sub == nil {
		sub = &subscription{}
		s.subs[channel] = sub
		if err := s.sendSubscribe(ctx, channel); err != nil {
			delete(s.subs, channel)
			s.mu.Unlock()
			return err
		}
	}
	sub.waiters = append(sub.waiters, ack)
	s.mu.Unlock()

	timer := time.NewTimer(s.layer.opts.SubscribeTimeout)
	defer timer.Stop()
	select {
	case <-ack:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.layer.closed:
		return layers.ErrClosed
	case <-timer.C:
		return fmt.Errorf("redis layer: no SUBSCRIBE acknowledgement for %s within %s", channel, s.layer.opts.SubscribeTimeout)
	}
}

// sendSubscribe starts the receive loop with the first subscription. Callers
// hold s.mu.
func (s *shard) sendSubscribe(ctx context.Context, channel string) error {
	if s.ps == nil {
		if s.layer.opts.Sharded {
			s.ps = s.client.SSubscribe(ctx, channel)
		} else {
			s.ps = s.client.Subscribe(ctx, channel)
		}
		loopCtx, cancel := context.WithCancel(context.Background())
		s.stop, s.done = cancel, make(chan struct{})
		go s.loop(loopCtx, s.ps)
		return nil
	}
	if s.layer.opts.Sharded {
		return s.ps.SSubscribe(ctx, channel)
	}
	return s.ps.Subscribe(ctx, channel)
}

func (s *shard) unsubscribe(ctx context.Context, channel string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub := s.subs[channel]
	if sub == nil {
		return nil
	}
	delete(s.subs, channel)
	for _, w := range sub.waiters {
		close(w)
	}
	if s.ps == nil {
		return nil
	}
	if s.layer.opts.Sharded {
		return s.ps.SUnsubscribe(ctx, channel)
	}
	return s.ps.Unsubscribe(ctx, channel)
}

func (s *shard) loop(ctx context.Context, ps *goredis.PubSub) {
	defer close(s.done)
	failing := false
	backoff := 50 * time.Millisecond
	for {
		msg, err := ps.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if !failing {
				s.layer.log.Warn("redis layer: connection lost, reconnecting", "shard", s.index, "err", err)
			}
			failing = true
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 2*time.Second)
			continue
		}
		if failing {
			failing, backoff = false, 50*time.Millisecond
			s.layer.log.Info("redis layer: reconnected", "shard", s.index)
			if s.layer.opts.OnReconnect != nil {
				s.layer.opts.OnReconnect(s.index)
			}
		}
		switch m := msg.(type) {
		case *goredis.Subscription:
			if m.Kind == "subscribe" || m.Kind == "ssubscribe" {
				s.mu.Lock()
				if sub := s.subs[m.Channel]; sub != nil {
					sub.acked = true
					for _, w := range sub.waiters {
						close(w)
					}
					sub.waiters = nil
				}
				s.mu.Unlock()
			}
		case *goredis.Message:
			s.layer.deliver(m.Channel, []byte(m.Payload))
		}
	}
}

func (s *shard) close() error {
	s.mu.Lock()
	ps, stop, done := s.ps, s.stop, s.done
	s.ps = nil
	s.mu.Unlock()
	var errs []error
	if stop != nil {
		stop()
	}
	if ps != nil {
		errs = append(errs, ps.Close())
	}
	if done != nil {
		<-done
	}
	if s.owned {
		errs = append(errs, s.client.Close())
	}
	return errors.Join(errs...)
}
