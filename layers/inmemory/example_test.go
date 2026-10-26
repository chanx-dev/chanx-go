package inmemory_test

import (
	"context"
	"fmt"

	"go.chanx.dev/layers/v2"
	"go.chanx.dev/layers/v2/inmemory"
)

func ExampleNew() {
	ctx := context.Background()
	layer := inmemory.New()

	alice, _ := layer.NewChannel(ctx, "specific.")
	bob, _ := layer.NewChannel(ctx, "specific.")
	_ = layer.GroupAdd(ctx, "room.lobby", alice)
	_ = layer.GroupAdd(ctx, "room.lobby", bob)

	_ = layer.GroupSend(ctx, "room.lobby", layers.Message{"type": "chat.message", "text": "hi"})
	for _, ch := range []string{alice, bob} {
		msg, _ := layer.Receive(ctx, ch)
		fmt.Println(msg["text"])
	}
	// Output:
	// hi
	// hi
}

// Capacity bounds each channel's queue; a full channel refuses Send.
func ExampleOptions() {
	ctx := context.Background()
	layer := inmemory.New(inmemory.Options{Capacity: 1})
	ch, _ := layer.NewChannel(ctx, "specific.")
	fmt.Println(layer.Send(ctx, ch, layers.Message{"type": "a"}))
	fmt.Println(layer.Send(ctx, ch, layers.Message{"type": "b"}))
	// Output:
	// <nil>
	// layers: channel full
}
