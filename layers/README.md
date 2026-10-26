# chanx-go layers

Channel layers for Go: named channels and groups, so a message reaches every process
subscribed to a group, not only the one that sent it. The operations are Django
Channels' (`Send`, `GroupAdd`, `GroupSend`...), and the module imports nothing from
chanx: use it alone for fan-out between replicas, or under
[chanx-go](https://github.com/chanx-dev/chanx-go).

```bash
go get go.chanx.dev/layers
```

```go
layer := inmemory.New()

ch, _ := layer.NewChannel(ctx, "specific.")
_ = layer.GroupAdd(ctx, "room.lobby", ch)
_ = layer.GroupSend(ctx, "room.lobby", layers.Message{"type": "chat.message", "text": "hi"})

msg, _ := layer.Receive(ctx, ch) // map[type:chat.message text:hi]
```

| Package | What |
| --- | --- |
| `layers` | the `Layer` interface, `Set` of layers by alias, name checks |
| `layers/inmemory` | one process, like Channels' `InMemoryChannelLayer`: capacity, expiry, `ErrChannelFull` |
| `layers/serializer` | JSON, Fernet encryption keyed as channels_redis keys it |
| `layers/layertest` | the conformance suite every backend passes, yours included |
| [`layers/redis`](redis) | Redis pub/sub, wire-compatible with Python (its own module) |

Every backend hands receivers the same value types (`int64`, `float64`,
`map[string]any`, `[]any`), so code reading a message works the same on any of them.

A third-party backend proves itself with one test:

```go
func TestLayer(t *testing.T) {
	layertest.Run(t, func(t *testing.T) layers.Layer { return mylayer.New() })
}
```

MIT licensed.
