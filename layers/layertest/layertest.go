// Package layertest is the suite every channel layer backend passes, built-in or
// third-party:
//
//	func TestLayer(t *testing.T) {
//		layertest.Run(t, func(t *testing.T) layers.Layer { return mylayer.New() })
//	}
package layertest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"go.chanx.dev/layers/v2"
)

// Factory returns a fresh, empty layer; Run closes it.
type Factory func(t *testing.T) layers.Layer

// Timeout bounds every receive the suite waits on.
var Timeout = 3 * time.Second

// Run runs the conformance suite.
func Run(t *testing.T, factory Factory) {
	cases := []struct {
		name string
		fn   func(*testing.T, layers.Layer)
	}{
		{"NewChannel", testNewChannel},
		{"SendReceive", testSendReceive},
		{"Order", testOrder},
		{"ReceiveCanceled", testReceiveCanceled},
		{"Groups", testGroups},
		{"GroupDiscard", testGroupDiscard},
		{"DeleteChannel", testDeleteChannel},
		{"GroupSendEmpty", testGroupSendEmpty},
		{"Values", testValues},
		{"Isolation", testIsolation},
		{"InvalidNames", testInvalidNames},
		{"Concurrent", testConcurrent},
		{"Flush", testFlush},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := factory(t)
			t.Cleanup(func() { _ = l.Close() })
			c.fn(t, l)
		})
	}
}

func ctxT(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	t.Cleanup(cancel)
	return ctx
}

func newChannel(t *testing.T, l layers.Layer) string {
	t.Helper()
	ch, err := l.NewChannel(ctxT(t), "specific.")
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	return ch
}

func receive(t *testing.T, l layers.Layer, ch string) layers.Message {
	t.Helper()
	msg, err := l.Receive(ctxT(t), ch)
	if err != nil {
		t.Fatalf("Receive(%s): %v", ch, err)
	}
	return msg
}

func expectNothing(t *testing.T, l layers.Layer, ch string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if msg, err := l.Receive(ctx, ch); err == nil {
		t.Fatalf("Receive(%s) = %v, want nothing", ch, msg)
	}
}

func testNewChannel(t *testing.T, l layers.Layer) {
	a, b := newChannel(t, l), newChannel(t, l)
	if a == b {
		t.Fatalf("two channels named %q", a)
	}
	for _, ch := range []string{a, b} {
		if !strings.Contains(ch, "specific.") {
			t.Errorf("channel %q does not carry its prefix", ch)
		}
		if err := layers.ValidChannelName(ch); err != nil {
			t.Error(err)
		}
	}
}

func testSendReceive(t *testing.T, l layers.Layer) {
	ch := newChannel(t, l)
	if err := l.Send(ctxT(t), ch, layers.Message{"type": "test.message", "text": "hi"}); err != nil {
		t.Fatal(err)
	}
	msg := receive(t, l, ch)
	if msg["type"] != "test.message" || msg["text"] != "hi" {
		t.Fatalf("got %v", msg)
	}
}

func testOrder(t *testing.T, l layers.Layer) {
	ch := newChannel(t, l)
	if err := l.GroupAdd(ctxT(t), "order", ch); err != nil {
		t.Fatal(err)
	}
	for i := range 10 {
		if err := l.GroupSend(ctxT(t), "order", layers.Message{"type": "n", "n": i}); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 10 {
		got, ok := layers.Int(receive(t, l, ch)["n"])
		if !ok || got != int64(i) {
			t.Fatalf("message %d: n = %v", i, got)
		}
	}
}

func testReceiveCanceled(t *testing.T, l layers.Layer) {
	ch := newChannel(t, l)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := l.Receive(ctx, ch)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Receive after deadline: %v", err)
	}
	// The channel still works after a canceled Receive.
	if err := l.Send(ctxT(t), ch, layers.Message{"type": "after"}); err != nil {
		t.Fatal(err)
	}
	if msg := receive(t, l, ch); msg["type"] != "after" {
		t.Fatalf("got %v", msg)
	}
}

func testGroups(t *testing.T, l layers.Layer) {
	a, b, c := newChannel(t, l), newChannel(t, l), newChannel(t, l)
	for _, ch := range []string{a, b} {
		if err := l.GroupAdd(ctxT(t), "room.lobby", ch); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.GroupAdd(ctxT(t), "room.other", c); err != nil {
		t.Fatal(err)
	}
	if err := l.GroupSend(ctxT(t), "room.lobby", layers.Message{"type": "chat", "text": "hi"}); err != nil {
		t.Fatal(err)
	}
	for _, ch := range []string{a, b} {
		if msg := receive(t, l, ch); msg["text"] != "hi" {
			t.Fatalf("%s got %v", ch, msg)
		}
	}
	expectNothing(t, l, c)
}

func testGroupDiscard(t *testing.T, l layers.Layer) {
	a, b := newChannel(t, l), newChannel(t, l)
	for _, ch := range []string{a, b} {
		if err := l.GroupAdd(ctxT(t), "g", ch); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.GroupDiscard(ctxT(t), "g", a); err != nil {
		t.Fatal(err)
	}
	// Discarding twice, or from a group never joined, is not an error.
	if err := l.GroupDiscard(ctxT(t), "g", a); err != nil {
		t.Fatal(err)
	}
	if err := l.GroupDiscard(ctxT(t), "never", a); err != nil {
		t.Fatal(err)
	}
	if err := l.GroupSend(ctxT(t), "g", layers.Message{"type": "x"}); err != nil {
		t.Fatal(err)
	}
	receive(t, l, b)
	expectNothing(t, l, a)
}

func testDeleteChannel(t *testing.T, l layers.Layer) {
	a, b := newChannel(t, l), newChannel(t, l)
	for _, g := range []string{"g1", "g2"} {
		for _, ch := range []string{a, b} {
			if err := l.GroupAdd(ctxT(t), g, ch); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := l.DeleteChannel(ctxT(t), a); err != nil {
		t.Fatal(err)
	}
	for _, g := range []string{"g1", "g2"} {
		if err := l.GroupSend(ctxT(t), g, layers.Message{"type": g}); err != nil {
			t.Fatal(err)
		}
		if msg := receive(t, l, b); msg["type"] != g {
			t.Fatalf("got %v", msg)
		}
	}
}

func testGroupSendEmpty(t *testing.T, l layers.Layer) {
	if err := l.GroupSend(ctxT(t), "nobody.here", layers.Message{"type": "x"}); err != nil {
		t.Fatal(err)
	}
}

// testValues pins the value types receivers see, the same for every backend, so
// code never asserts a type one backend cannot produce.
func testValues(t *testing.T, l layers.Layer) {
	ch := newChannel(t, l)
	sent := layers.Message{
		"type":    "values",
		"int":     7,
		"big":     int64(1) << 40,
		"neg":     -3,
		"float":   2.0,
		"frac":    1.5,
		"str":     "héllo ✓",
		"bool":    true,
		"none":    nil,
		"list":    []any{1, "a", []any{2.5}},
		"nested":  map[string]any{"seq": 3, "inner": map[string]any{"ok": false}},
		"empty":   map[string]any{},
		"strings": []string{"x", "y"},
	}
	if err := l.Send(ctxT(t), ch, sent); err != nil {
		t.Fatal(err)
	}
	got := receive(t, l, ch)
	want := map[string]any{
		"type": "values", "int": int64(7), "big": int64(1) << 40, "neg": int64(-3),
		"float": 2.0, "frac": 1.5, "str": "héllo ✓", "bool": true, "none": nil,
		"list":    []any{int64(1), "a", []any{2.5}},
		"nested":  map[string]any{"seq": int64(3), "inner": map[string]any{"ok": false}},
		"empty":   map[string]any{},
		"strings": []any{"x", "y"},
	}
	if diff := compare("", got, want); diff != "" {
		t.Fatal(diff)
	}
}

func compare(path string, got, want any) string {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: got %T %v, want a map", path, got, got)
		}
		if len(g) != len(w) {
			return fmt.Sprintf("%s: got %d keys %v, want %d", path, len(g), g, len(w))
		}
		for k, v := range w {
			if d := compare(path+"."+k, g[k], v); d != "" {
				return d
			}
		}
		return ""
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return fmt.Sprintf("%s: got %T %v, want %v", path, got, got, want)
		}
		for i := range w {
			if d := compare(fmt.Sprintf("%s[%d]", path, i), g[i], w[i]); d != "" {
				return d
			}
		}
		return ""
	}
	if got != want {
		return fmt.Sprintf("%s: got %T %v, want %T %v", path, got, got, want, want)
	}
	return ""
}

func testIsolation(t *testing.T, l layers.Layer) {
	a, b := newChannel(t, l), newChannel(t, l)
	for _, ch := range []string{a, b} {
		if err := l.GroupAdd(ctxT(t), "iso", ch); err != nil {
			t.Fatal(err)
		}
	}
	sent := layers.Message{"type": "x", "data": map[string]any{"n": 1}}
	if err := l.GroupSend(ctxT(t), "iso", sent); err != nil {
		t.Fatal(err)
	}
	sent["data"].(map[string]any)["n"] = 99 // the sender reusing its map
	ma := receive(t, l, a)
	ma["data"].(map[string]any)["n"] = 42 // a receiver mutating its copy
	mb := receive(t, l, b)
	if n, _ := layers.Int(mb["data"].(map[string]any)["n"]); n != 1 {
		t.Fatalf("receivers share state: n = %v", n)
	}
}

func testInvalidNames(t *testing.T, l layers.Layer) {
	ch := newChannel(t, l)
	for _, g := range []string{"has space", "émoji", strings.Repeat("g", 100), ""} {
		if err := l.GroupAdd(ctxT(t), g, ch); !errors.Is(err, layers.ErrInvalidName) {
			t.Errorf("GroupAdd(%q) = %v, want ErrInvalidName", g, err)
		}
		if err := l.GroupSend(ctxT(t), g, layers.Message{"type": "x"}); !errors.Is(err, layers.ErrInvalidName) {
			t.Errorf("GroupSend(%q) = %v, want ErrInvalidName", g, err)
		}
	}
	if err := l.Send(ctxT(t), "bad name", layers.Message{"type": "x"}); !errors.Is(err, layers.ErrInvalidName) {
		t.Errorf("Send to a bad name = %v, want ErrInvalidName", err)
	}
}

func testConcurrent(t *testing.T, l layers.Layer) {
	const members, messages = 5, 20
	chans := make([]string, members)
	for i := range chans {
		chans[i] = newChannel(t, l)
		if err := l.GroupAdd(ctxT(t), "busy", chans[i]); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for i := range messages {
		wg.Go(func() {
			if err := l.GroupSend(ctxT(t), "busy", layers.Message{"type": "n", "n": i}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	for _, ch := range chans {
		seen := map[int64]bool{}
		for range messages {
			n, _ := layers.Int(receive(t, l, ch)["n"])
			seen[n] = true
		}
		if len(seen) != messages {
			t.Fatalf("%s saw %d distinct messages, want %d", ch, len(seen), messages)
		}
	}
}

// testFlush: groups are gone and pending messages dropped; channels stay usable.
func testFlush(t *testing.T, l layers.Layer) {
	ch := newChannel(t, l)
	if err := l.GroupAdd(ctxT(t), "flushed", ch); err != nil {
		t.Fatal(err)
	}
	if err := l.Send(ctxT(t), ch, layers.Message{"type": "pending"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond) // let a networked layer deliver it first
	if err := l.Flush(ctxT(t)); err != nil {
		t.Fatal(err)
	}
	if err := l.GroupSend(ctxT(t), "flushed", layers.Message{"type": "after"}); err != nil {
		t.Fatal(err)
	}
	expectNothing(t, l, ch)
	if err := l.Send(ctxT(t), ch, layers.Message{"type": "direct"}); err != nil {
		t.Fatal(err)
	}
	if msg := receive(t, l, ch); msg["type"] != "direct" {
		t.Fatalf("got %v", msg)
	}
}
