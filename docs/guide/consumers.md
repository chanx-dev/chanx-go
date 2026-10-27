# Consumers

A consumer serves one route. It is a struct: its exported methods taking a
`*chanx.Context` and a message are handlers, and methods with reserved names are hooks.
You mount it with `engine.Handle`.

## A complete consumer

<<< @/snippets/consumers/consumers.go#consumer

```go
engine.Handle("/ws/chat/{room}", &ChatConsumer{Rooms: rooms})
```

## Handlers

A handler is any exported method with one of these shapes:

```go
func (c *ChatConsumer) HandleChat(ctx *chanx.Context, m ChatMessage) error                  // no reply
func (c *ChatConsumer) HandlePing(ctx *chanx.Context, m chanx.PingMessage) (chanx.PongMessage, error) // replies
func (c *ChatConsumer) HandleShipped(ctx *chanx.EventContext, e OrderShipped) (Notification, error) // an event
```

- **The message type picks the action.** Two handlers for one action panic at `Handle`.
- **A returned message is the reply.** It goes to the sender, with the frame's `ref`
  and `"version": 1`:

  ```json
  > {"action": "ping", "ref": "42"}
  < {"action": "pong", "payload": null, "version": 1, "ref": "42"}
  ```

- **A returned error** is logged and the client gets a generic error frame, without the
  error's text:

  ```json
  < {"action": "error", "payload": {"detail": "Failed to process message for report"}, "version": 1, "ref": "r1"}
  ```

  A panic is recovered and answered the same way.
- **The method name is the AsyncAPI operation**, in snake case: name it `HandlePing` to
  get Python's `handle_ping`.
- **Each frame runs in its own goroutine**, so a slow handler never blocks the socket.
  `chanx.Sequential()` on the route handles frames one at a time, in order, and
  `chanx.MaxInFlight(n)` caps how many run at once.

`*chanx.Context` is also a `context.Context`, canceled when the socket closes: pass it to
your database calls.

## Sending

| Call | Goes to |
| --- | --- |
| `return reply, nil` | the sender, with its `ref` |
| `ctx.Send(m)` | the sender, with its `ref`: for several replies to one frame |
| `ctx.Push(m)` | the sender, without a `ref`: server-initiated |
| `ctx.Broadcast(m)` | the connection's groups, through the layer |
| `ctx.Broadcast(m, chanx.ExcludeSelf())` | the same, minus the sender |
| `ctx.Broadcast(m, chanx.ToGroups("a", "b"))` | other groups |

## Hooks

Matched by name and signature; a wrong signature panics at `Handle`.

| Hook | Python chanx | Runs |
| --- | --- | --- |
| `Authenticate(*Context, *Request) (any, error)` | `authenticator_class` | once, before anything; see [Authentication](/guide/authentication) |
| `AuthenticationData(*Context, any) any` | the authenticator's `data` | fills the authentication message |
| `Credentials() []Credential` | the authenticator's `credentials` | documents the auth in AsyncAPI |
| `Groups(*Conn) []string` | `groups` | on connect: the groups to join |
| `OnConnect(*Context) error` | `post_authentication` | once set up; an error closes with 1011 |
| `OnDisconnect(*Context)` | `websocket_disconnect` | on close, groups still joined |
| `Emits() []Message` | none | declares what the consumer sends outside any handler; see below |
| `Operations() map[string]Operation` | `@ws_handler(summary=..., output_type=...)` | documents handlers, by method name |
| `Passthrough() []Message` | `passthrough_events` | events forwarded to the client unchanged |
| `Topics() []TopicHandler` | `topics` | the [topics](/guide/topics) it hosts |
| `OnValidationError(*Context, []Issue)` | `handle_validation_error` | replaces the validation error frame |
| `OnHandlerError(*Context, string, error)` | handler error handling | replaces the generic error frame |
| `OnEventError(*EventContext, string, error)` | event error handling | an event handler failed |

`Operations` sets `Summary`, `Description`, `Tags`, `Name` and `Replies` per handler.

## Declaring what a handler sends

The AsyncAPI document and the clients generated from it know what a handler sends from
its return type. When the return type does not say, list the messages in `Replies`, like
Python's `output_type`:

| The handler | Declare it with |
| --- | --- |
| returns one message type | nothing: the return type says it |
| returns one of several, as `chanx.Message` (Python's `-> A \| B`) | `Replies`, required: `Handle` panics without it |
| broadcasts, returning only `error` | `Replies` |

A lobby that answers `join` with `joined`, or with `room_full` when the room has no seat
left:

<<< @/snippets/consumers/consumers.go#replies

Either reply echoes the request's `ref`:

```json
{"action": "join", "payload": {"room": "general"}, "ref": "1"}
{"action": "joined", "payload": {"room": "general"}, "version": 1, "ref": "1"}

{"action": "join", "payload": {"room": "vip"}, "ref": "2"}
{"action": "room_full", "payload": {"room": "vip"}, "version": 1, "ref": "2"}
```

With `chanx.On`, `Replies` is an option:

<<< @/snippets/consumers/consumers.go#replies_function

The AsyncAPI document lists `join` with both replies, and a generated Go client gets
`RequestJoin(ctx, JoinMessage) (chanx.Message, error)`, whose result you switch on.

`Emits` is for the rare message no handler sends: a presence broadcast from `OnConnect`,
a push from a worker through a `Publisher`. Python has no counterpart. Reach for
`Replies` first.

## One value, every connection

The consumer value you pass to `Handle` serves every connection, like an `http.Handler`.
Keep dependencies on it (a database, a service), never per-connection state. That lives
on the connection:

```go
n, _ := chanx.ConnValue[int](ctx.Conn, visitsKey{})
ctx.Conn.SetValue(visitsKey{}, n+1)
```

`ctx.Conn` also holds the `User`, the path `Params`, the handshake `Request` and the
layer `ChannelName`.

## Mixins

Embed a struct, and its handlers are promoted:

<<< @/snippets/consumers/consumers.go#mixin

The embedding consumer's `Groups` and `Passthrough` are merged with the mixin's.

## Without methods

`chanx.NewConsumer` builds the same from functions, for closures and dynamic setups.
Hooks are fields; `chanx.On` registers a handler with a reply, `chanx.Handle` one without,
`chanx.OnEvent` and `chanx.HandleEvent` the event versions:

<<< @/snippets/consumers/consumers.go#function

## Catching mistakes early

`Handle` panics at startup on a definition mistake: a handler with a wrong signature, two
handlers for one action, an unknown layer alias. `chanx.Check` reports the same as an
error, for a one-line unit test:

```go
func TestConsumer(t *testing.T) {
	if err := chanx.Check(&ChatConsumer{}); err != nil {
		t.Fatal(err)
	}
}
```

Next: [Groups and events](/guide/events).
