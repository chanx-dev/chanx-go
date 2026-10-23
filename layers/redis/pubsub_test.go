package redis

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/huynguyengl99/chanx-go/layers"
	"github.com/huynguyengl99/chanx-go/layers/internal/vectors"
	"github.com/huynguyengl99/chanx-go/layers/layertest"
)

// redisHosts returns the Redis servers to test against (CHANX_GO_REDIS, the
// docker-compose pair by default), skipping when they are not reachable.
func redisHosts(t *testing.T) []string {
	t.Helper()
	env := os.Getenv("CHANX_GO_REDIS")
	if env == "" {
		env = "redis://localhost:6393,redis://localhost:6394"
	}
	hosts := strings.Split(env, ",")
	for _, h := range hosts {
		o, err := goredis.ParseURL(h)
		if err != nil {
			t.Fatal(err)
		}
		c := goredis.NewClient(o)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err = c.Ping(ctx).Err()
		cancel()
		_ = c.Close()
		if err != nil {
			t.Skipf("Redis at %s not reachable (docker compose up -d): %v", h, err)
		}
	}
	return hosts
}

func TestConsistentHashVectors(t *testing.T) {
	var cases []struct {
		Name  string
		Ring  int
		Index int
	}
	if err := vectors.Load("testdata/consistent_hash.json", &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if got := consistentHash(c.Name, c.Ring); got != c.Index {
			t.Errorf("consistentHash(%q, %d) = %d, want %d", c.Name, c.Ring, got, c.Index)
		}
	}
}

func TestMsgPackVectors(t *testing.T) {
	var cases []struct {
		MsgPack string `json:"msgpack"`
		Value   any
	}
	if err := vectors.Load("testdata/msgpack.json", &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		data, _ := base64.StdEncoding.DecodeString(c.MsgPack)
		v, err := MsgPack.Unmarshal(data)
		if err != nil {
			t.Fatal(err)
		}
		want := vectors.Value(c.Value)
		if d := vectors.Diff("", layers.Normalize(v), want); d != "" {
			t.Errorf("decoding Python msgpack: %s", d)
		}
		out, err := MsgPack.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		again, _ := MsgPack.Unmarshal(out)
		if d := vectors.Diff("", layers.Normalize(again), want); d != "" {
			t.Errorf("round trip: %s", d)
		}
	}
}

func newLayer(t *testing.T, opts PubSubOptions) *PubSub {
	t.Helper()
	if opts.Hosts == nil {
		opts.Hosts = redisHosts(t)
	}
	if opts.Prefix == "" {
		opts.Prefix = fmt.Sprintf("test%d", time.Now().UnixNano())
	}
	l, err := NewPubSub(opts)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestSuite(t *testing.T) {
	for _, c := range []struct {
		name string
		opts func([]string) PubSubOptions
	}{
		{"msgpack/one shard", func(h []string) PubSubOptions { return PubSubOptions{Hosts: h[:1]} }},
		{"msgpack/two shards", func(h []string) PubSubOptions { return PubSubOptions{Hosts: h} }},
		{"json/fernet", func(h []string) PubSubOptions {
			return PubSubOptions{Hosts: h, SerializerFormat: "json", SymmetricEncryptionKeys: []string{"k1", "k0"}}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			hosts := redisHosts(t)
			layertest.Run(t, func(t *testing.T) layers.Layer { return newLayer(t, c.opts(hosts)) })
		})
	}
}

// Two layers sharing Redis act as two processes.
func TestAcrossProcesses(t *testing.T) {
	hosts := redisHosts(t)
	prefix := fmt.Sprintf("x%d", time.Now().UnixNano())
	a := newLayer(t, PubSubOptions{Hosts: hosts, Prefix: prefix})
	b := newLayer(t, PubSubOptions{Hosts: hosts, Prefix: prefix})
	defer func() { _ = a.Close() }()
	defer func() { _ = b.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cb, _ := b.NewChannel(ctx, "specific.")
	if err := b.GroupAdd(ctx, "room", cb); err != nil {
		t.Fatal(err)
	}
	if err := a.GroupSend(ctx, "room", layers.Message{"type": "hi", "n": 1}); err != nil {
		t.Fatal(err)
	}
	if msg, err := b.Receive(ctx, cb); err != nil || msg["type"] != "hi" {
		t.Fatalf("%v %v", msg, err)
	}
	if err := a.Send(ctx, cb, layers.Message{"type": "direct"}); err != nil {
		t.Fatal(err)
	}
	if msg, err := b.Receive(ctx, cb); err != nil || msg["type"] != "direct" {
		t.Fatalf("%v %v", msg, err)
	}
	// A key mismatch is a decode error on the receiver, not a crash.
	c := newLayer(t, PubSubOptions{Hosts: hosts, Prefix: prefix, SymmetricEncryptionKeys: []string{"other"}})
	defer func() { _ = c.Close() }()
	if err := c.GroupSend(ctx, "room", layers.Message{"type": "secret"}); err != nil {
		t.Fatal(err)
	}
	short, cancel2 := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel2()
	if msg, err := b.Receive(short, cb); err == nil {
		t.Fatalf("undecodable message delivered: %v", msg)
	}
}

func TestOverflow(t *testing.T) {
	var dropped atomic.Int32
	l := newLayer(t, PubSubOptions{BufferSize: 2, OnOverflow: func(string) { dropped.Add(1) }})
	defer func() { _ = l.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ch, _ := l.NewChannel(ctx, "specific.")
	for i := range 5 {
		if err := l.Send(ctx, ch, layers.Message{"type": "n", "n": i}); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for dropped.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if dropped.Load() != 3 {
		t.Fatalf("dropped %d, want 3", dropped.Load())
	}
}

func TestNotLocal(t *testing.T) {
	l := newLayer(t, PubSubOptions{})
	defer func() { _ = l.Close() }()
	ctx := context.Background()
	if _, err := l.Receive(ctx, "asgispecific.nope"); !errors.Is(err, layers.ErrNotLocal) {
		t.Fatalf("Receive on a foreign channel: %v", err)
	}
	if err := l.GroupAdd(ctx, "g", "asgispecific.nope"); !errors.Is(err, layers.ErrNotLocal) {
		t.Fatalf("GroupAdd of a foreign channel: %v", err)
	}
	_ = l.Close()
	if _, err := l.NewChannel(ctx, "specific."); !errors.Is(err, layers.ErrClosed) {
		t.Fatalf("NewChannel after Close: %v", err)
	}
}

// A GroupAdd after Close fails without starting a new receive loop, and a failed
// GroupAdd leaves no member behind.
func TestGroupAddFailures(t *testing.T) {
	ctx := context.Background()
	l := newLayer(t, PubSubOptions{})
	ch, err := l.NewChannel(ctx, "specific.")
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := l.GroupAdd(canceled, "g", ch); err == nil {
		t.Fatal("GroupAdd with a canceled context succeeded")
	}
	if len(l.groups) != 0 {
		t.Fatalf("failed GroupAdd left members: %v", l.groups)
	}
	_ = l.Close()
	if err := l.GroupAdd(ctx, "g", ch); !errors.Is(err, layers.ErrClosed) {
		t.Fatalf("GroupAdd after Close: %v", err)
	}
	for _, s := range l.shards {
		if s.ps != nil {
			t.Fatalf("shard %d started a receive loop after Close", s.index)
		}
	}
	if len(l.groups) != 0 {
		t.Fatalf("GroupAdd after Close left members: %v", l.groups)
	}
}

func TestChannelName(t *testing.T) {
	l, _ := NewPubSub(PubSubOptions{Hosts: []string{"redis://localhost:1"}})
	if got := l.groupChannel("room.lobby"); got != "asgi__group__room.lobby" {
		t.Fatal(got)
	}
	if _, err := NewPubSub(PubSubOptions{SerializerFormat: "xml"}); err == nil {
		t.Fatal("unknown format accepted")
	}
}

// The last member leaving while another joins must not leave the newcomer
// subscribed in the layer but unsubscribed in Redis.
func TestJoinWhileLastMemberLeaves(t *testing.T) {
	l := newLayer(t, PubSubOptions{})
	defer func() { _ = l.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	var lost atomic.Int32
	for i := range 200 {
		wg.Go(func() {
			group := fmt.Sprintf("race%d", i)
			a, _ := l.NewChannel(ctx, "specific.")
			b, _ := l.NewChannel(ctx, "specific.")
			if err := l.GroupAdd(ctx, group, a); err != nil {
				t.Error(err)
				return
			}
			start := make(chan struct{})
			var both sync.WaitGroup
			both.Go(func() { <-start; _ = l.GroupDiscard(ctx, group, a) })
			both.Go(func() { <-start; _ = l.GroupAdd(ctx, group, b) })
			close(start)
			both.Wait()
			_ = l.GroupSend(ctx, group, layers.Message{"type": "x"})
			rctx, rcancel := context.WithTimeout(ctx, 2*time.Second)
			defer rcancel()
			if _, err := l.Receive(rctx, b); err != nil {
				lost.Add(1)
			}
		})
	}
	wg.Wait()
	if n := lost.Load(); n > 0 {
		t.Fatalf("%d of 200 members that joined while the last one left got nothing", n)
	}
}

// When Redis drops the subscription connection, the layer reconnects,
// resubscribes, reports it, and delivers again.
func TestReconnect(t *testing.T) {
	hosts := redisHosts(t)
	var reconnects atomic.Int32
	l := newLayer(t, PubSubOptions{Hosts: hosts[:1], OnReconnect: func(int) { reconnects.Add(1) }})
	defer func() { _ = l.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ch, err := l.NewChannel(ctx, "specific.")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.GroupAdd(ctx, "survivors", ch); err != nil {
		t.Fatal(err)
	}

	o, _ := goredis.ParseURL(hosts[0])
	admin := goredis.NewClient(o)
	defer func() { _ = admin.Close() }()
	if err := admin.ClientKillByFilter(ctx, "TYPE", "pubsub").Err(); err != nil {
		t.Fatal(err)
	}
	for reconnects.Load() == 0 {
		if ctx.Err() != nil {
			t.Fatal("no reconnect after the subscription connection was killed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Resubscribed: group and direct messages arrive again.
	for _, send := range []func() error{
		func() error { return l.GroupSend(ctx, "survivors", layers.Message{"type": "after"}) },
		func() error { return l.Send(ctx, ch, layers.Message{"type": "after"}) },
	} {
		received := false
		for !received && ctx.Err() == nil {
			if err := send(); err != nil {
				t.Fatal(err)
			}
			rctx, rcancel := context.WithTimeout(ctx, 300*time.Millisecond)
			msg, err := l.Receive(rctx, ch)
			rcancel()
			received = err == nil && msg["type"] == "after"
		}
		if !received {
			t.Fatal("nothing delivered after reconnecting")
		}
	}
}
