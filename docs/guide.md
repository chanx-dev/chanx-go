# chanx-go guide

The protocol, envelope and client side are the ones described in the
[chanx documentation](https://chanx.readthedocs.io). This guide covers what is specific to
Go. API reference: pkg.go.dev.

## Messages

A message is a struct embedding `chanx.BaseMessage`, with its action in the tag and an
optional `Payload` field. The AsyncAPI names are the Go type names, as Python uses class
names.

```go
type PostPayload struct {
	Body   string `json:"body"`
	Author string `json:"author" default:"anonymous"` // optional, filled when absent
	Tags   []string `json:"tags,omitempty"`        // optional
}

type PostMessage struct {
	chanx.BaseMessage `action:"post" description:"Post something to a room."`
	Payload           PostPayload `json:"payload"`
}

type TypingMessage struct {
	chanx.BaseMessage `action:"typing"` // no Payload field: payload is null
}
```

| Python | Go |
| --- | --- |
| field without default (required) | field without `omitempty`/`omitzero`/`default` |
| `x: str = "a"` | `` X string `json:"x" default:"a"` `` |
| `x: str \| None = None` | `` X *string `json:"x,omitempty"` `` |
| `x: Any = None` | `` X any `json:"x,omitempty"` `` |
| `StrEnum` | a named string type with a `JSONSchema(*jsonschema.Schema)` method setting `Enum` |
| `Field(min_length=3)` | `JSONSchema(*jsonschema.Schema)` on the payload type |
| `@field_validator` | `Validate() error` on the payload type |

Values decoded outside chanx (an HTTP body, a queue job) get their defaults with
`chanx.ApplyDefaults(&v)` before decoding.

## Consumers

A consumer is a struct; exported methods taking `*chanx.Context` (client to server) or
`*chanx.EventContext` (server to server) and a message are handlers. The action comes from
the message type, the reply from the return type.

```go
type ChatConsumer struct {
	Moderation          // a mixin: its handler methods are promoted
	Rooms *RoomService  // dependencies are plain fields
}

func (c *ChatConsumer) HandleChat(ctx *chanx.Context, m ChatMessage) error              // no reply
func (c *ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error)
func (c *ChatConsumer) HandleNotify(ctx *chanx.EventContext, e SystemNotify) (ChatNotification, error)
```

One value serves every connection, like an `http.Handler`: keep per-connection state on
`ctx.Conn` (`SetValue`, `chanx.ConnValue[T]`).

Optional hooks, matched by name and signature (a wrong signature panics at `Handle`):

| Hook | Python |
| --- | --- |
| `Authenticate(*Context, *Request) (any, error)` | `authenticator_class` |
| `AuthenticationData(*Context, any) any` | the `data` of the authentication message (null by default) |
| `Credentials() []Credential` | the authenticator's `credentials`: AsyncAPI security schemes |
| `Groups(*Conn) []string` | `groups` (embedded mixins' groups are merged) |
| `OnConnect(*Context) error` | `post_authentication` |
| `OnDisconnect(*Context)` | `websocket_disconnect`; groups are still joined, so it can broadcast |
| `Passthrough() []Message` | `passthrough_events` (merged across embedded structs) |
| `Emits() []Message` | outputs of handlers returning `chanx.Message` |
| `Topics() []TopicHandler` | `topics` |
| `Operations() map[string]Operation` | `summary`, `description`, `tags`, `output_type` of `@ws_handler`, by method name |
| `OnValidationError`, `OnHandlerError`, `OnEventError` | `handle_validation_error`... |

A failed `Authenticate` sends the authentication message, then closes with 4000 + its
status: 4401 for `chanx.Unauthenticated` or any plain error, 4403 for `chanx.Forbidden`,
4500 when `Authenticate` panics. A client can stop reconnecting on these.

Operation names in AsyncAPI are the method names in snake case: name methods
`HandlePing` to get Python's `handle_ping`.

`chanx.Check(&ChatConsumer{})` in a unit test reports wrong signatures and duplicate
actions without starting anything.

Without methods, `chanx.NewConsumer` plus `chanx.On`, `chanx.Handle`, `chanx.OnEvent` and
`chanx.HandleEvent` register functions with their types inferred.

## Topics

```go
type RoomTopic struct {
	chanx.BaseTopic `pattern:"room:{room_name}" description:"A chat room"`
}

func (RoomTopic) Authorize(ctx *chanx.Context, p chanx.Params) (bool, error) { ... }
func (RoomTopic) OnSubscribe(ctx *chanx.Context) error                      { ... } // no ref
func (RoomTopic) OnUnsubscribe(ctx *chanx.Context) error                    { ... } // also on disconnect
func (RoomTopic) HandlePost(ctx *chanx.Context, m PostMessage) error {
	return ctx.BroadcastTopic(RoomEvent{Payload: m.Payload})
}

engine.Handle("/ws/hub", &HubConsumer{})                                     // Topics() returns &RoomTopic{}
engine.Handle("/ws/room/{room_name}", chanx.TopicConsumer(&RoomTopic{}))      // as_consumer()
chanx.BroadcastTopic[RoomTopic](ctx, publisher, "room:lobby", RoomEvent{...}, chanx.Seq(7))
```

The group namespace is the type name, as Python's class name, so Go and Python processes
serving the same topic over one Redis meet without configuration. Tag keys: `pattern`,
`name` (namespace), `description`, `layer`.

## Engine, routes and frameworks

`chanx.New(chanx.Config{...})` takes Python's settings by their Go names (`Camelize`,
`SendCompletion`, `LogMessages`, `LogIgnored`) plus Go-only ones (`MaxInFlight`,
`OutboxSize`, `WriteTimeout`, `MaxFrameSize`, `RejectBeforeUpgrade`). Route options:
`chanx.Name/Description/Tags` (Python's `@channel`), `chanx.Layer(alias)`,
`chanx.SendCompletion`, `chanx.Camelize`, `chanx.Sequential`, `chanx.UserFrom(key)`.

Route paths use whole-segment parameters, `/ws/chat/{room}`. Wildcards such as
`{rest...}` are refused at `Handle`: each router names and slices them differently.

`MaxFrameSize` (16 MiB by default) caps an incoming frame; a larger one closes the
connection with 1009 before it is buffered.

| Framework | Mount |
| --- | --- |
| net/http `ServeMux` | `chanxhttp.Mount(mux, engine)` |
| chi | `chanxhttp.Mount(r, engine, chanxhttp.NoMethod())` or `r.Get(path, chanxhttp.Handler(engine, route))` |
| gorilla/mux, a prefix mount | `r.PathPrefix("/ws/").Handler(chanxhttp.Dispatcher(engine))` |
| go-zero, Kratos, Encore raw, Huma | register `chanxhttp.Handler(engine, route)` as a plain `http.Handler` |
| gin | `chanxgin.Mount(r.Group("/", authMiddleware), engine)` |
| echo v5 | `chanxecho.Mount(e.Group("", authMiddleware), engine)` |
| Fiber v3 | `chanxfiber.Mount(app.Group("/", authMiddleware), engine)` |

Middleware runs before the upgrade; a value it stores (`c.Set("user", u)`, `c.Locals`, a
context value) is read with `r.Value("user")` in `Authenticate`, or with no
`Authenticate` at all through the `chanx.UserFrom("user")` route option.

Shut down with `engine.Shutdown(ctx)` (closes sockets with 1001) as well as the server's
own shutdown: hijacked connections are not tracked by `http.Server.Shutdown`.

## AsyncAPI

```go
func (c *ChatConsumer) Credentials() []chanx.Credential { return auth.BearerCredentials() }

chanxhttp.MountDocs(mux, engine, "/asyncapi", asyncapi.Info{
	Title:           "Chat",
	Servers:         map[string]any{"production": map[string]any{"host": "api.example.com", "protocol": "wss"}},
	SecuritySchemes: map[string]any{"oauth": map[string]any{"type": "oauth2"}},
})
```

Each credential becomes an `httpApiKey` security scheme named `<location>_<name>`
(`header_authorization`), listed on the channel's operations; query and header
credentials also go in the channel's `ws` binding. Topics hosted by a consumer carry
its credentials. A scheme in `SecuritySchemes` replaces a derived one of the same name;
`Servers` replaces `ServerURL` and `ServerProtocol`.

The docs page loads a pinned `@asyncapi/react-component` from unpkg with SRI hashes.
`DocsJSURL` and `DocsCSSURL` point it at your own files (a custom URL has no hash
unless you set `DocsJSIntegrity`/`DocsCSSIntegrity`). Under a Content-Security-Policy,
the page needs the asset origin in `script-src` and `style-src`, `'unsafe-inline'` for
its inline script, and `'unsafe-eval'` because the component compiles code at runtime:
apply that policy to the docs route only.

## Layers

```go
layer, err := redis.NewPubSub(redis.PubSubOptions{
	Hosts:                   []string{"redis://a:6379", "redis://b:6379"}, // same order as Python's hosts
	Prefix:                  "asgi",
	SerializerFormat:        "msgpack",
	SymmetricEncryptionKeys: []string{os.Getenv("LAYER_KEY")},
})
engine := chanx.New(chanx.Config{Layers: layers.Set{"default": layer}})
```

A worker, cron job or HTTP handler pushes to sockets with a `chanx.Publisher` built from
layers alone (`chanx.NewPublisher(set)` or `engine.Publisher()`), including sockets held
by Python servers on the same Redis.

## Testing

```go
c := chanxtest.Connect(t, engine, "/ws/chat/lobby", chanxtest.Header("Authorization", "Bearer x"))
c.SendText(`{"action":"ping","ref":"1"}`)
c.Expect(`{"action":"pong","payload":null,"version":1,"ref":"1"}`)
c.Subscribe("room:lobby")
frames := c.Request(ChatMessage{...})                               // with a ref, until complete
frames = c.ReceiveUntil("chat_notification")                         // up to and including that action
note, before := chanxtest.ReceiveUntilMessage[ChatNotification](c)  // typed
```

`Connect` returns once the connection is set up (groups joined, `OnConnect` run), so
a broadcast sent right after reaches it. Strict mode (default) fails a test when the
server sends an action no handler declares.

To assert on what was published rather than on what a client received, the
counterpart of `capture_broadcast_events` and `capture_topic_broadcasts`, wrap the
layer in a spy:

```go
spy := chanxtest.Spy(inmemory.New())             // chanxtest.Suppress() records without delivering
engine := chanx.New(chanx.Config{Layers: layers.Set{"default": spy}})
// ... code under test publishes ...
joined := chanxtest.BroadcastEvents[UserJoinedEvent](spy)       // []Captured: Event, Group or Channel
posts := chanxtest.TopicBroadcasts[RoomTopic, RoomEvent](spy)   // Topic, Seq, Event
```

## Differences from Python

Each is a deliberate decision.

| Case | Python chanx | chanx-go |
| --- | --- | --- |
| A payload value of the wrong type that Pydantic's lax mode coerces (`"3"` for an int, `1` for a bool, `"true"`) | accepted, coerced | `error` frame (`int_type`, `bool_type`...). Envelope ints (`version`, `seq`) stay lax on both sides |
| Text frame that is not JSON | `Invalid JSON: ` + Python's decoder message | `Invalid JSON: ` + Go's decoder message |
| `send_message_immediately` | yields after each send | not needed: a dedicated writer goroutine |
| A slow client | left to the ASGI server | bounded outbox; closed with 1013 after `WriteTimeout` |
| A huge frame | left to the ASGI server (uvicorn: 16 MiB) | `MaxFrameSize`, 16 MiB by default; closed with 1009 |
| Redis queue layer (`RedisChannelLayer`) | available | not yet (pub/sub layer only) |
