# Channel layers

A channel layer carries messages between processes. Every connection gets a channel,
groups are sets of channels, and a send to a group reaches every member, wherever it
runs. It is the model of Django Channels, with the same operations.

| Layer | Package | Use |
| --- | --- | --- |
| in memory | `chanx-go/layers/inmemory` | one process: development, tests, a single server |
| Redis pub/sub | `chanx-go/layers/redis` | several processes, and processes in Python or Node |

`chanx.New(chanx.Config{})` without `Layers` uses one in-memory layer. With Redis:

<<< @/snippets/layering/layering.go#redis

## Several layers

A `layers.Set` holds layers by alias. A route picks one with `chanx.Layer`, a topic with
its `layer` tag, and a publisher with `.Layer`:

<<< @/snippets/layering/layering.go#aliases

## Workers

A process with no sockets publishes to the sockets every web process holds, with the same
layers and a publisher:

<<< @/snippets/layering/layering.go#worker

## The in-memory layer

`inmemory.New` mirrors Channels' `InMemoryChannelLayer`: each channel queue holds 100
messages by default, messages expire after 60 seconds and memberships after a day, and a
send to a full channel fails with `layers.ErrChannelFull`. Messages are copied through
JSON, so a receiver sees the same value types a Redis layer would give it, and cannot
change what another receiver sees.

```go
layer := inmemory.New(inmemory.Options{
	Capacity:        500,
	ChannelCapacity: []inmemory.Capacity{{Pattern: "jobs.*", Capacity: 5000}},
})
```

## Without chanx

The layers module imports nothing from chanx. Use it alone for fan-out between replicas:
cache invalidation, a job queue's wake-ups, presence.

<<< @/snippets/layering/layering.go#standalone

A message is a `map[string]any` with a `type`. Every backend hands receivers the same
value types (`int64`, `float64`, `string`, `bool`, `map[string]any`, `[]any`), and
`layers.Int(v)` reads an integer whatever the serializer made of it.

## Your own backend

A backend implements `layers.Layer` and proves itself with the conformance suite every
built-in one passes:

```go
func TestLayer(t *testing.T) {
	layertest.Run(t, func(t *testing.T) layers.Layer { return mylayer.New() })
}
```

Next: [Redis and Python](/layers/redis).
