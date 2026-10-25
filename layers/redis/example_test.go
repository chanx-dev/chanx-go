package redis_test

import (
	"context"
	"log"
	"os"

	"github.com/huynguyengl99/chanx-go/layers"
	"github.com/huynguyengl99/chanx-go/layers/redis"
)

func ExampleNewPubSub() {
	layer, err := redis.NewPubSub(redis.PubSubOptions{
		Hosts: []string{"redis://localhost:6379"}, // same hosts, same order, in every process
	})
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = layer.Close() }()

	ctx := context.Background()
	ch, _ := layer.NewChannel(ctx, "specific.")
	_ = layer.GroupAdd(ctx, "room.lobby", ch) // returns once Redis acknowledged it
	_ = layer.GroupSend(ctx, "room.lobby", layers.Message{"type": "chat.message", "text": "hi"})
}

// Settings matching a Python process using channels_redis or fast-channels with
// symmetric_encryption_keys, so both decrypt each other's messages.
func ExamplePubSubOptions() {
	layer, err := redis.NewPubSub(redis.PubSubOptions{
		Hosts:                   []string{"redis://a:6379", "redis://b:6379"},
		Prefix:                  "asgi",
		SerializerFormat:        "msgpack",
		SymmetricEncryptionKeys: []string{os.Getenv("LAYER_KEY")},
	})
	if err != nil {
		log.Fatal(err)
	}
	_ = layer
}
