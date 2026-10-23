// Package layers is a channel layer for Go: named channels, groups and fan-out
// across processes, with the operations of Django Channels' layers.
//
// It never imports chanx, so it can be used on its own: fan-out between replicas,
// or a worker pushing to the web tier. The Redis backends in layers/redis are
// wire-compatible with channels_redis and fast-channels, so Go services can join
// a Django Channels or FastAPI deployment.
package layers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
)

// Message is what travels through a layer: a JSON-like map.
type Message = map[string]any

// Layer is the channel layer interface every backend implements.
type Layer interface {
	// NewChannel creates a channel this process receives on. The name starts
	// with prefix, which conventionally ends in "." ("specific.").
	NewChannel(ctx context.Context, prefix string) (string, error)
	// Receive blocks until a message arrives on a channel created by NewChannel.
	// Canceling ctx only stops this call.
	Receive(ctx context.Context, channel string) (Message, error)
	// DeleteChannel stops receiving on a channel and leaves every group it joined.
	DeleteChannel(ctx context.Context, channel string) error
	Send(ctx context.Context, channel string, msg Message) error
	GroupAdd(ctx context.Context, group, channel string) error
	GroupDiscard(ctx context.Context, group, channel string) error
	GroupSend(ctx context.Context, group string, msg Message) error
	// Flush drops every channel and group the layer holds (tests).
	Flush(ctx context.Context) error
	Close() error
}

var (
	ErrChannelFull = errors.New("layers: channel full")
	ErrInvalidName = errors.New("layers: invalid channel or group name")
	ErrNotLocal    = errors.New("layers: channel was not created by this layer")
	ErrClosed      = errors.New("layers: layer closed")
)

// Set holds named layers. "default" is the one used when nothing else is named.
type Set map[string]Layer

// DefaultAlias is the layer used when none is named.
const DefaultAlias = "default"

// Get returns the layer under alias, "default" when alias is empty.
func (s Set) Get(alias string) (Layer, error) {
	if alias == "" {
		alias = DefaultAlias
	}
	l, ok := s[alias]
	if !ok {
		return nil, fmt.Errorf("layers: no layer registered under %q", alias)
	}
	return l, nil
}

// Close closes every layer in the set.
func (s Set) Close() error {
	var errs []error
	for _, l := range s {
		errs = append(errs, l.Close())
	}
	return errors.Join(errs...)
}

// MaxNameLength is the exclusive upper bound on channel and group names.
const MaxNameLength = 100

var (
	groupNameRe   = regexp.MustCompile(`^[a-zA-Z\d\-_.]+$`)
	channelNameRe = regexp.MustCompile(`^[a-zA-Z\d\-_.]+(!([a-zA-Z\d\-_.]*))?$`)
)

// ValidGroupName reports whether name is a group name every backend accepts.
func ValidGroupName(name string) error {
	if len(name) >= MaxNameLength || !groupNameRe.MatchString(name) {
		return fmt.Errorf("%w: group %q must be ASCII letters, digits, '-', '_' or '.', shorter than %d", ErrInvalidName, name, MaxNameLength)
	}
	return nil
}

// ValidChannelName reports whether name is a channel name every backend accepts.
// A "!" marks a process-local channel in the queue layer.
func ValidChannelName(name string) error {
	if len(name) >= MaxNameLength || !channelNameRe.MatchString(name) {
		return fmt.Errorf("%w: channel %q must be ASCII letters, digits, '-', '_', '.' or a single '!', shorter than %d", ErrInvalidName, name, MaxNameLength)
	}
	return nil
}

// RandomSuffix returns 32 random hex characters for channel names.
func RandomSuffix() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Int reads an integer from a layer message whatever the serializer made of it:
// any int or uint type, json.Number, or a float with no fractional part.
func Int(v any) (int64, bool) {
	if n, ok := v.(json.Number); ok {
		i, err := n.Int64()
		return i, err == nil
	}
	rv := reflect.ValueOf(v)
	switch {
	case rv.CanInt():
		return rv.Int(), true
	case rv.CanUint():
		u := rv.Uint()
		return int64(u), u <= math.MaxInt64
	case rv.CanFloat():
		f := rv.Float()
		return int64(f), f == math.Trunc(f) && math.Abs(f) < 1<<63
	}
	return 0, false
}

// Normalize turns decoder-specific values into plain Go ones, recursively:
// json.Number into int64 (or float64 when it has a fraction), every int type
// into int64 (a uint64 above MaxInt64 stays), float32 into float64, and
// map[any]any into map[string]any. Serializers call it on what they decode, so
// every backend hands receivers the same types.
func Normalize(v any) any {
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		f, _ := x.Float64()
		return f
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = Normalize(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[fmt.Sprint(k)] = Normalize(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = Normalize(val)
		}
		return out
	}
	rv := reflect.ValueOf(v)
	switch {
	case rv.CanInt():
		return rv.Int()
	case rv.CanUint():
		// msgpack packs positive ints unsigned; they are still ints.
		if u := rv.Uint(); u <= math.MaxInt64 {
			return int64(u)
		}
		return rv.Uint()
	case rv.CanFloat():
		return rv.Float()
	}
	return v
}
