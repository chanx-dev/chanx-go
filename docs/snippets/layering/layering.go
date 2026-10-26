// Package layering holds the examples of the Layers guide.
package layering

import (
	"context"
	"os"

	"go.chanx.dev/layers/redis/v2"
	"go.chanx.dev/layers/v2"
	"go.chanx.dev/layers/v2/inmemory"
	chanx "go.chanx.dev/v2"
)

// #region redis
func NewEngine() (*chanx.Engine, error) {
	layer, err := redis.NewPubSub(redis.PubSubOptions{
		Hosts: []string{os.Getenv("REDIS_URL")}, // "redis://localhost:6379"
	})
	if err != nil {
		return nil, err
	}
	return chanx.New(chanx.Config{Layers: layers.Set{"default": layer}}), nil
}

// #endregion redis

// #region python
// Settings a Python process using channels_redis or fast-channels shares.
func SharedWithPython() (layers.Layer, error) {
	return redis.NewPubSub(redis.PubSubOptions{
		Hosts:                   []string{"redis://redis-a:6379", "redis://redis-b:6379"}, // same order everywhere
		Prefix:                  "asgi",
		SerializerFormat:        "msgpack",
		SymmetricEncryptionKeys: []string{os.Getenv("CHANNEL_LAYER_KEY")},
	})
}

// #endregion python

// #region aliases
// Several layers by alias: a route or a topic picks one.
func Aliased() *chanx.Engine {
	set := layers.Set{
		"default": inmemory.New(),
		"jobs":    inmemory.New(),
	}
	engine := chanx.New(chanx.Config{Layers: set})
	engine.Handle("/ws/jobs", JobsConsumer{}, chanx.Layer("jobs"))
	return engine
}

// #endregion aliases

type JobsConsumer struct{}

func (JobsConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

type JobDone struct {
	chanx.BaseMessage `action:"job_done"`
	Payload           string `json:"payload"`
}

// #region worker
// A worker process holds no sockets: a publisher on the same layers reaches the
// sockets every web process holds.
func NotifyJobDone(ctx context.Context, set layers.Set, user string) error {
	publisher := chanx.NewPublisher(set).Layer("jobs")
	return publisher.BroadcastEvent(ctx, JobDone{Payload: "report ready"}, "user."+user)
}

// #endregion worker

// #region standalone
// The layers module never imports chanx: use it alone to fan out between replicas.
func FanOut(ctx context.Context, layer layers.Layer) (layers.Message, error) {
	inbox, err := layer.NewChannel(ctx, "specific.")
	if err != nil {
		return nil, err
	}
	if err := layer.GroupAdd(ctx, "cache.invalidate", inbox); err != nil {
		return nil, err
	}
	if err := layer.GroupSend(ctx, "cache.invalidate", layers.Message{"type": "cache.invalidate", "key": "user:42"}); err != nil {
		return nil, err
	}
	return layer.Receive(ctx, inbox)
}

// #endregion standalone
