# Groups and events

A group is a name connections join, such as `room.lobby` or `user.ann`. A broadcast to a
group goes through the channel layer, so it reaches every member on every process that
shares the layer. Events are the server's side of this: messages between servers, that
each connection turns into what its client should see.

## Groups

`Groups` returns the groups a connection joins on connect; it leaves them on disconnect:

```go
func (ChatConsumer) Groups(c *chanx.Conn) []string {
	return []string{"room." + c.Param("room"), "user." + c.User.(User).ID}
}
```

From a handler, `ctx.Broadcast(m)` sends to the connection's own groups. Join and leave
at run time with `ctx.Conn.Join(ctx, group)` and `ctx.Conn.Leave(ctx, group)`.

A broadcast arrives exactly as a reply would, without a `ref`:

```json
{"action": "chat_notification", "payload": {"message": "hi"}}
```

::: warning Group names
Layers accept letters, digits, `.`, `-` and `_`, under 100 characters. Build names from
user input with `protocol.SafeGroupName("room", name)`, which every chanx implementation
computes the same way.
:::

## Events

A broadcast sends a message to clients as it is. An event is different: it is delivered to
each connection's event handler, which decides what the client receives, if anything.

<<< @/snippets/events/events.go#event

The handler takes a `*chanx.EventContext`; its return value goes to the client. A handler
returning only an `error` sends nothing, for an event that only updates state. On a
[consumer built from functions](/guide/consumers#without-methods), register them with
`chanx.OnEvent` and `chanx.HandleEvent`.

Events forwarded unchanged need no handler, only a declaration:

<<< @/snippets/events/events.go#passthrough

## From outside a socket

A `chanx.Publisher` pushes to groups from anywhere: an HTTP handler, a worker, a cron
job. It needs the layers, not the engine, so a worker process holding no sockets can use
it too.

<<< @/snippets/events/events.go#publisher

```bash
curl -X POST "localhost:8000/orders/A17/shipped?user=ann"
```

Every socket Ann has open receives:

```json
{"action": "notification", "payload": {"text": "Order A17 shipped"}}
```

| Publisher method | Does |
| --- | --- |
| `BroadcastEvent(ctx, event, groups...)` | an event to every connection in the groups |
| `SendEvent(ctx, channel, event)` | an event to one connection, by its `ctx.Conn.ChannelName` |
| `BroadcastMessage(ctx, groups, m)` | a message to clients unchanged, no event handler |
| `chanx.BroadcastTopic[T](ctx, p, topic, event)` | an event to a [topic](/guide/topics)'s subscribers |

Get one with `engine.Publisher()`, `ctx.Publisher()` inside a handler, or
`chanx.NewPublisher(layers)` in a process without an engine. `.Layer("alias")` picks
another layer.

Python processes on the same Redis receive these events too, and Go connections receive
theirs: the layer messages are the same. See [Redis and Python](/layers/redis).

## Completion frames

With `SendCompletion` on, the server confirms each processed frame, event and group
message, for clients that wait until a request is done:

```json
{"action": "complete", "payload": null, "version": 1, "ref": "7"}
{"action": "event_complete", "payload": null}
{"action": "group_complete", "payload": null}
```

Turn it on for every route with `chanx.Config{SendCompletion: true}`, or one with the
`chanx.SendCompletion(true)` route option.

Next: [Topics](/guide/topics).
