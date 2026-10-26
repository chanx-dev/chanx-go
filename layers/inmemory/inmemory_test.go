package inmemory_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"go.chanx.dev/layers"
	"go.chanx.dev/layers/inmemory"
	"go.chanx.dev/layers/layertest"
)

func TestSuite(t *testing.T) {
	layertest.Run(t, func(t *testing.T) layers.Layer { return inmemory.New() })
}

func TestCapacity(t *testing.T) {
	ctx := context.Background()
	l := inmemory.New(inmemory.Options{
		Capacity:        2,
		ChannelCapacity: []inmemory.Capacity{{Pattern: "big.*", Capacity: 3}},
	})
	for _, c := range []struct {
		channel string
		fits    int
	}{{"small", 2}, {"big.one", 3}} {
		for range c.fits {
			if err := l.Send(ctx, c.channel, layers.Message{"type": "x"}); err != nil {
				t.Fatal(err)
			}
		}
		if err := l.Send(ctx, c.channel, layers.Message{"type": "x"}); !errors.Is(err, layers.ErrChannelFull) {
			t.Fatalf("%s: send past capacity = %v", c.channel, err)
		}
	}
	// GroupSend skips a full member instead of failing.
	if err := l.GroupAdd(ctx, "g", "small"); err != nil {
		t.Fatal(err)
	}
	if err := l.GroupSend(ctx, "g", layers.Message{"type": "x"}); err != nil {
		t.Fatalf("GroupSend to a full member: %v", err)
	}
}

func TestExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		l := inmemory.New(inmemory.Options{Expiry: time.Minute, GroupExpiry: time.Hour})
		ch, _ := l.NewChannel(ctx, "specific.")
		other, _ := l.NewChannel(ctx, "specific.")
		_ = l.GroupAdd(ctx, "g", ch)
		_ = l.GroupAdd(ctx, "g", other)
		_ = l.Send(ctx, ch, layers.Message{"type": "old"})
		time.Sleep(2 * time.Minute)

		// The expired message is gone, and its channel left every group.
		_ = l.GroupSend(ctx, "g", layers.Message{"type": "new"})
		rctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if msg, err := l.Receive(rctx, ch); err == nil {
			t.Fatalf("expired channel received %v", msg)
		}
		if msg, err := l.Receive(ctx, other); err != nil || msg["type"] != "new" {
			t.Fatalf("other: %v %v", msg, err)
		}

		// Memberships older than GroupExpiry are dropped.
		time.Sleep(2 * time.Hour)
		_ = l.GroupSend(ctx, "g", layers.Message{"type": "late"})
		rctx2, cancel2 := context.WithTimeout(ctx, time.Second)
		defer cancel2()
		if msg, err := l.Receive(rctx2, other); err == nil {
			t.Fatalf("expired membership received %v", msg)
		}
	})
}

func TestClose(t *testing.T) {
	l := inmemory.New()
	ch, _ := l.NewChannel(context.Background(), "specific.")
	done := make(chan error)
	go func() {
		_, err := l.Receive(context.Background(), ch)
		done <- err
	}()
	_ = l.Close()
	if err := <-done; !errors.Is(err, layers.ErrClosed) {
		t.Fatalf("Receive after Close: %v", err)
	}
}

func TestChannelName(t *testing.T) {
	ch, _ := inmemory.New().NewChannel(context.Background(), "specific.")
	if len(ch) != len("specific..inmemory!")+12 {
		t.Fatalf("unexpected channel name %q", ch)
	}
}
