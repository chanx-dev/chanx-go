# Configuration

Settings live in two places: `chanx.Config`, for the whole engine, and route options,
passed to `Handle` for one route. A route option overrides the engine's setting.

```go
engine := chanx.New(chanx.Config{
	Layers:         layers.Set{"default": redisLayer},
	SendCompletion: true,
	Logger:         slog.New(slog.NewJSONHandler(os.Stdout, nil)),
})
engine.Handle("/ws/chat/{room}", &ChatConsumer{}, chanx.Name("chat"), chanx.Sequential())
```

## Engine

| `chanx.Config` | Python setting | Default | Does |
| --- | --- | --- | --- |
| `Layers` | `CHANNEL_LAYERS` | one in-memory layer | channel layers by alias |
| `Camelize` | `CAMELIZE` | off | snake_case in, camelCase out, on the wire |
| `SendCompletion` | `SEND_COMPLETION` | off | `complete`, `event_complete`, `group_complete` frames |
| `LogMessages`, `LogIgnored` | `LOG_WEBSOCKET_MESSAGE`, `LOG_IGNORED_ACTIONS` | off | log frames in and out, but these actions |
| `Logger` | | `slog.Default()` | where logs go, with `conn_id` and `action` attached |
| `OnValidationError`, `OnHandlerError`, `OnEventError` | | error frames | engine-wide error hooks; a consumer's own win |
| `RejectBeforeUpgrade` | | off | authenticate before the upgrade, answer an HTTP status |
| `MaxInFlight` | | unlimited | frames handled at once, per connection |
| `MaxFrameSize` | | 16 MiB | a larger incoming frame closes with 1009 |
| `OutboxSize` | | 256 | frames queued for a connection's writer |
| `WriteTimeout` | | 10s | how long a send waits on a full outbox before closing with 1013 |
| `DisconnectTimeout` | | 5s | how long cleanup waits for running handlers |

The last four have no Python equivalent: there, the ASGI server bounds frames and slow
clients. In Go, a client that stops reading cannot grow a server's memory: its outbox
fills, a send waits `WriteTimeout`, and the connection closes with 1013.

## Routes

| Option | Does |
| --- | --- |
| `chanx.Name("chat")` | the AsyncAPI channel name; the type name without `Consumer`, in snake case, by default |
| `chanx.Description("...")`, `chanx.Tags("a", "b")` | the channel's description and tags (Python's `@channel`) |
| `chanx.Layer("alias")` | the layer the route broadcasts on |
| `chanx.SendCompletion(bool)`, `chanx.Camelize(bool)` | override the engine's |
| `chanx.Sequential()` | one frame at a time, in arrival order |
| `chanx.MaxInFlight(n)` | override the engine's |
| `chanx.RejectBeforeUpgrade(bool)` | override the engine's |
| `chanx.UserFrom(key)` | the user from framework middleware, see [Authentication](/guide/authentication) |

## Paths

Route paths use whole-segment parameters: `/ws/chat/{room}`, `/ws/{org}/rooms/{room}`.
Each adapter turns them into its router's syntax (`/ws/chat/:room` for gin, echo and
Fiber). Wildcards such as `{rest...}` are refused at `Handle`: routers name and slice
them differently, so a consumer would see different values on each.

## Close codes

| Code | When |
| --- | --- |
| 1000 | the client or a handler closed normally |
| 1001 | the server is shutting down (`engine.Shutdown`) |
| 1009 | a frame over `MaxFrameSize` |
| 1011 | `OnConnect` failed, or the layer is unavailable |
| 1013 | the client stopped reading |
| 4401, 4403, 4000 + status | authentication failed |

Next: [Testing](/guide/testing).
