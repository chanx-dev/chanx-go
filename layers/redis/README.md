# chanx-go Redis layer

A Redis pub/sub channel layer for Go, wire-compatible with `RedisPubSubChannelLayer` from
Python's [channels_redis](https://github.com/django/channels_redis) and
[fast-channels](https://github.com/huynguyengl99/fast-channels), and with
[`@chanx-js/layers`](https://js.chanx.dev/server/layers): Go,
Python and Node processes can share one Redis and meet in the same groups.

```bash
go get go.chanx.dev/layers/redis
```

```go
layer, err := redis.NewPubSub(redis.PubSubOptions{
	Hosts: []string{"redis://localhost:6379"},
})
if err != nil {
	log.Fatal(err)
}
defer layer.Close()

ch, _ := layer.NewChannel(ctx, "specific.")
_ = layer.GroupAdd(ctx, "room.lobby", ch) // returns once Redis acknowledged the subscription
_ = layer.GroupSend(ctx, "room.lobby", layers.Message{"type": "chat.message", "text": "hi"})
```

## Sharing Redis with Python

Use the same settings on both sides:

| Go | Python | Default |
| --- | --- | --- |
| `Hosts` (same order) | `hosts` | `redis://localhost:6379` |
| `Prefix` | `prefix` | `asgi` |
| `SerializerFormat` | `serializer_format` | `msgpack` |
| `SymmetricEncryptionKeys` | `symmetric_encryption_keys` | none (unencrypted) |

The host order defines the shard ring, so every process must list the hosts in the same
order. The ring, msgpack and JSON encodings and Fernet tokens are tested byte for byte
against vectors recorded from Python.

## Options

- `Clients`: ready go-redis clients instead of `Hosts`, for Sentinel
  (`goredis.NewFailoverClient`) or custom settings.
- `Sharded`: Redis Cluster sharded pub/sub (`SSUBSCRIBE`/`SPUBLISH`).
- `BufferSize` and `OnOverflow`: each local channel's buffer, and what to do when a slow
  reader overflows it.
- `OnReconnect`: called when a shard's connection comes back. Pub/sub keeps nothing, so
  messages published while it was down are lost.

There is no Redis queue layer (`RedisChannelLayer`) yet.

MIT licensed.
