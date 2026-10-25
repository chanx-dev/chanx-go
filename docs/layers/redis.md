# Redis and Python

`layers/redis` is a Redis pub/sub channel layer, wire-compatible with
`RedisPubSubChannelLayer` from Python's channels_redis and fast-channels, and with
`@chanx-js/layers`. Go, Python and Node processes on one Redis share channels and groups:
a Go worker can push to sockets a Django server holds, and a FastAPI process can
broadcast to a room some of whose members are connected to Go.

```bash
go get github.com/huynguyengl99/chanx-go/layers/redis
```

<<< @/snippets/layering/layering.go#redis

## Sharing Redis with Python

Use the same settings on both sides:

<<< @/snippets/layering/layering.go#python

| Go | Python | Default |
| --- | --- | --- |
| `Hosts`, in the same order | `hosts` | `redis://localhost:6379` |
| `Prefix` | `prefix` | `asgi` |
| `SerializerFormat` | `serializer_format` | `msgpack` |
| `SymmetricEncryptionKeys` | `symmetric_encryption_keys` | none |

```python
# settings.py, the same layer on the Django side
CHANNEL_LAYERS = {
    "default": {
        "BACKEND": "channels_redis.pubsub.RedisPubSubChannelLayer",
        "CONFIG": {
            "hosts": ["redis://redis-a:6379", "redis://redis-b:6379"],
            "prefix": "asgi",
            "symmetric_encryption_keys": [os.environ["CHANNEL_LAYER_KEY"]],
        },
    },
}
```

What has to match, and is tested to:

- **The shard ring.** With several hosts, a channel's shard is a CRC-32 of its name over
  4096 slots, as in channels_redis. Every process must list the hosts in the same order.
- **The encoding.** msgpack (default) or JSON, with ints and floats kept apart, so `2.0`
  stays a float in Python.
- **Encryption.** Fernet, keyed as channels_redis keys it; tokens are tested byte for
  byte against Python's.
- **Group names.** `protocol.SafeGroupName` and topic groups compute names as chanx does.
- **The messages chanx puts on the layer**: `handle_channel_event`,
  `handle_group_message`, `handle_topic_event`, with the same fields.

## Options

| Option | Does |
| --- | --- |
| `Clients` | ready go-redis clients instead of `Hosts`: Sentinel (`goredis.NewFailoverClient`) or custom settings. Not closed by `Close` |
| `Sharded` | Redis Cluster sharded pub/sub, `SSUBSCRIBE` and `SPUBLISH`; only processes with this option read these messages |
| `BufferSize` | each local channel's buffer, 1024 by default; an overflow drops the message |
| `OnOverflow` | called with the channel that dropped a message, for a metric |
| `OnReconnect` | called when a shard's connection comes back |
| `SubscribeTimeout` | how long a subscribe waits for Redis's acknowledgement, 5s |
| `Serializer` | replaces `SerializerFormat` and the keys |

`GroupAdd` and `NewChannel` return once Redis has acknowledged the subscription, so
"join, then publish" never loses the message.

## What pub/sub does not do

Pub/sub keeps nothing. A message published while a process is disconnected from Redis is
lost to it, and a slow reader past `BufferSize` loses messages. That suits real-time
traffic, where a late update is replaced by the next. For delivery that must survive a
restart, keep the state in your database and send what changed when a client
reconnects.

The queue-based `RedisChannelLayer` of channels_redis has no Go version yet.

Next: [Clients](/reference/clients).
