// Package inmemory is a channel layer held in process memory, the counterpart of
// channels' InMemoryChannelLayer: bounded per-channel queues, message and group
// expiry, ChannelFull on Send, and full members skipped by GroupSend.
//
// Messages are copied through a JSON round trip on Send, so receivers see the
// same value types a Redis layer would give them (int64, float64, map[string]any)
// and a receiver mutating a map cannot affect another.
package inmemory

import (
	"context"
	"math/rand/v2"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.chanx.dev/layers"
	"go.chanx.dev/layers/serializer"
)

// Capacity sets the capacity of channels whose name matches a glob pattern
// ("*" and "?"), the first match winning.
type Capacity struct {
	Pattern  string
	Capacity int
}

// Options mirrors InMemoryChannelLayer's constructor.
type Options struct {
	Capacity        int           // default 100
	Expiry          time.Duration // default 60s
	GroupExpiry     time.Duration // default 24h
	ChannelCapacity []Capacity
}

type entry struct {
	expires time.Time
	msg     layers.Message
}

type queue struct {
	items    []entry
	capacity int
	wake     chan struct{} // capacity 1: a pending message or a closed layer
	local    bool          // created by NewChannel, kept until DeleteChannel
}

// Layer is the in-memory channel layer.
type Layer struct {
	opts       Options
	capacities []compiledCapacity

	mu       sync.Mutex
	channels map[string]*queue
	groups   map[string]map[string]time.Time // group -> channel -> joined at
	closed   chan struct{}
	once     sync.Once
}

type compiledCapacity struct {
	re       *regexp.Regexp
	capacity int
}

var _ layers.Layer = (*Layer)(nil)

// New returns an in-memory layer.
func New(opts ...Options) *Layer {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.Capacity <= 0 {
		o.Capacity = 100
	}
	if o.Expiry <= 0 {
		o.Expiry = 60 * time.Second
	}
	if o.GroupExpiry <= 0 {
		o.GroupExpiry = 24 * time.Hour
	}
	l := &Layer{
		opts:     o,
		channels: map[string]*queue{},
		groups:   map[string]map[string]time.Time{},
		closed:   make(chan struct{}),
	}
	for _, c := range o.ChannelCapacity {
		l.capacities = append(l.capacities, compiledCapacity{re: globRegexp(c.Pattern), capacity: c.Capacity})
	}
	return l
}

func globRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func (l *Layer) capacityOf(channel string) int {
	for _, c := range l.capacities {
		if c.re.MatchString(channel) {
			return c.capacity
		}
	}
	return l.opts.Capacity
}

// queueLocked returns the channel's queue, creating it: a message can wait for a
// receiver that has not asked yet.
func (l *Layer) queueLocked(channel string) *queue {
	q, ok := l.channels[channel]
	if !ok {
		q = &queue{capacity: l.capacityOf(channel), wake: make(chan struct{}, 1)}
		l.channels[channel] = q
	}
	return q
}

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// NewChannel returns "{prefix}.inmemory!{12 letters}".
func (l *Layer) NewChannel(_ context.Context, prefix string) (string, error) {
	if prefix == "" {
		prefix = "specific."
	}
	b := make([]byte, 12)
	for i := range b {
		b[i] = letters[rand.IntN(len(letters))]
	}
	name := prefix + ".inmemory!" + string(b)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.isClosed() {
		return "", layers.ErrClosed
	}
	l.queueLocked(name).local = true
	return name, nil
}

func (l *Layer) isClosed() bool {
	select {
	case <-l.closed:
		return true
	default:
		return false
	}
}

func (l *Layer) Send(_ context.Context, channel string, msg layers.Message) error {
	if err := layers.ValidChannelName(channel); err != nil {
		return err
	}
	copied, err := roundTrip(msg)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.isClosed() {
		return layers.ErrClosed
	}
	return l.sendLocked(channel, copied)
}

func (l *Layer) sendLocked(channel string, msg layers.Message) error {
	q := l.queueLocked(channel)
	if len(q.items) >= q.capacity {
		return layers.ErrChannelFull
	}
	q.items = append(q.items, entry{expires: time.Now().Add(l.opts.Expiry), msg: msg})
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return nil
}

func (l *Layer) Receive(ctx context.Context, channel string) (layers.Message, error) {
	if err := layers.ValidChannelName(channel); err != nil {
		return nil, err
	}
	for {
		l.mu.Lock()
		if l.isClosed() {
			l.mu.Unlock()
			return nil, layers.ErrClosed
		}
		l.cleanExpiredLocked()
		q := l.queueLocked(channel)
		if len(q.items) > 0 {
			e := q.items[0]
			q.items[0] = entry{}
			q.items = q.items[1:]
			if len(q.items) > 0 {
				select { // let the next receiver in
				case q.wake <- struct{}{}:
				default:
				}
			}
			l.mu.Unlock()
			// Copied again, so two receives of one channel never share a map.
			return roundTrip(e.msg)
		}
		wake := q.wake
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-l.closed:
			return nil, layers.ErrClosed
		case <-wake:
		}
	}
}

// DeleteChannel drops the channel's queue and its group memberships.
func (l *Layer) DeleteChannel(_ context.Context, channel string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.channels, channel)
	l.removeFromGroupsLocked(channel)
	return nil
}

func (l *Layer) removeFromGroupsLocked(channel string) {
	for name, members := range l.groups {
		delete(members, channel)
		if len(members) == 0 {
			delete(l.groups, name)
		}
	}
}

// cleanExpiredLocked drops expired messages and memberships. A channel with an
// expired message is taken out of every group: nobody is reading it.
func (l *Layer) cleanExpiredLocked() {
	now := time.Now()
	for name, q := range l.channels {
		expired := false
		for len(q.items) > 0 && q.items[0].expires.Before(now) {
			q.items = q.items[1:]
			expired = true
		}
		if expired {
			l.removeFromGroupsLocked(name)
			if len(q.items) == 0 && !q.local {
				delete(l.channels, name)
			}
		}
	}
	cutoff := now.Add(-l.opts.GroupExpiry)
	for name, members := range l.groups {
		for ch, joined := range members {
			if joined.Before(cutoff) {
				delete(members, ch)
			}
		}
		if len(members) == 0 {
			delete(l.groups, name)
		}
	}
}

func (l *Layer) GroupAdd(_ context.Context, group, channel string) error {
	if err := layers.ValidGroupName(group); err != nil {
		return err
	}
	if err := layers.ValidChannelName(channel); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.groups[group] == nil {
		l.groups[group] = map[string]time.Time{}
	}
	l.groups[group][channel] = time.Now()
	return nil
}

func (l *Layer) GroupDiscard(_ context.Context, group, channel string) error {
	if err := layers.ValidGroupName(group); err != nil {
		return err
	}
	if err := layers.ValidChannelName(channel); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if members := l.groups[group]; members != nil {
		delete(members, channel)
		if len(members) == 0 {
			delete(l.groups, group)
		}
	}
	return nil
}

func (l *Layer) GroupSend(_ context.Context, group string, msg layers.Message) error {
	if err := layers.ValidGroupName(group); err != nil {
		return err
	}
	copied, err := roundTrip(msg)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.isClosed() {
		return layers.ErrClosed
	}
	l.cleanExpiredLocked()
	for channel := range l.groups[group] {
		// Each member gets its own copy; a full member is skipped.
		own, err := roundTrip(copied)
		if err != nil {
			return err
		}
		_ = l.sendLocked(channel, own)
	}
	return nil
}

// Flush drops every channel and group. Channels created by NewChannel stay
// usable, empty.
func (l *Layer) Flush(_ context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for name, q := range l.channels {
		if q.local {
			q.items = nil
		} else {
			delete(l.channels, name)
		}
	}
	l.groups = map[string]map[string]time.Time{}
	return nil
}

// Close wakes every pending Receive with ErrClosed.
func (l *Layer) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func roundTrip(msg layers.Message) (layers.Message, error) {
	data, err := serializer.JSON.Marshal(msg)
	if err != nil {
		return nil, err
	}
	v, err := serializer.JSON.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	out, _ := layers.Normalize(v).(map[string]any)
	return out, nil
}
