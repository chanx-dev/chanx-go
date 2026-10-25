# Topics

A topic is a stream a client subscribes to by name, such as `ticker:ACME` or
`room:lobby`. One socket can follow any number of them. Groups are decided by the server
when a client connects; topics are chosen by the client, while connected.

## Declaring a topic

A topic is a struct embedding `chanx.BaseTopic`, with a pattern. It has its own handlers
and hooks, like a small consumer:

<<< @/snippets/topics/topics.go#topic

| Tag on `BaseTopic` | Meaning |
| --- | --- |
| `pattern` | the topic names it serves; `{symbol}` is a parameter, stopping at `:` |
| `name` | the group namespace; the type name by default |
| `description` | its description in AsyncAPI |
| `layer` | the layer alias it publishes on |

| Hook | Runs |
| --- | --- |
| `Authorize(*Context, Params) (bool, error)` | before a subscription: false refuses it |
| `OnSubscribe(*Context) error` | after it: push the current state |
| `OnUnsubscribe(*Context) error` | on unsubscribe, and on disconnect |
| `Passthrough() []Message` | events published to the topic, forwarded to subscribers |

## Hosting topics

A consumer lists the topics it hosts, and a topic can also have a route of its own:

<<< @/snippets/topics/topics.go#hub

## The conversation

Subscribe, with a `ref`; the confirmation carries it, then `OnSubscribe` pushes the state:

```json
> {"action": "subscribe", "topic": "ticker:ACME", "ref": "s1"}
< {"action": "subscribed", "payload": null, "version": 1, "topic": "ticker:ACME", "ref": "s1"}
< {"action": "price_update", "payload": {"symbol": "ACME", "price": 100.0}, "version": 1, "topic": "ticker:ACME"}
```

A frame for a topic carries its `topic`, and so does every reply:

```json
> {"action": "quote", "topic": "ticker:ACME", "ref": "q1", "payload": {"price": 101.5}}
< {"action": "price_update", "payload": {"symbol": "ACME", "price": 101.5}, "version": 1, "topic": "ticker:ACME", "ref": "q1"}
```

Refusals are error frames with a `reason`:

```json
< {"action": "error", "payload": {"reason": "unauthorized", "detail": "Not allowed to subscribe to 'ticker:SECRET'"}, "version": 1, "topic": "ticker:SECRET"}
< {"action": "error", "payload": {"reason": "not_subscribed", "detail": "Not subscribed to 'ticker:OTHER'"}, "version": 1, "topic": "ticker:OTHER"}
< {"action": "error", "payload": {"reason": "unknown_topic", "detail": "Unknown topic 'weather:paris'"}, "version": 1, "topic": "weather:paris"}
```

A second subscribe to the same topic confirms again without rerunning `OnSubscribe`.
Unsubscribe the same way:

```json
> {"action": "unsubscribe", "topic": "ticker:ACME", "ref": "u1"}
< {"action": "unsubscribed", "payload": null, "version": 1, "topic": "ticker:ACME", "ref": "u1"}
```

On a dedicated route (`/ws/ticker/ACME`), the topic comes from the URL: the client is
subscribed on connect, and its frames need no `topic` field.

## Publishing to a topic

Inside a topic handler, `ctx.BroadcastTopic(m)` reaches every subscriber. From anywhere
else:

<<< @/snippets/topics/topics.go#publish

```json
< {"action": "price_update", "payload": {"symbol": "ACME", "price": 102.25}, "version": 1, "topic": "ticker:ACME", "seq": 7}
```

`seq` is yours to assign: a client that sees 7 after 5 knows it missed one. Leave it
unset on a topic with several writers.

::: warning Declare what a topic forwards
An event published to a topic reaches subscribers only through an event handler on the
topic or its `Passthrough`. Without either, it is dropped and logged as "No event
handlers registered".
:::

## Group names

A topic's subscribers are a layer group: `TickerTopic.ticker-ACME`, the type name and the
topic. Python chanx names a topic class's group the same way, so a Go server and a Python
server hosting a topic of the same name meet on a shared Redis without configuration.
`chanx.TopicGroup[T](topic)` returns it, and `chanx.FormatTopic[T](params)` fills the
pattern.

Next: [Authentication](/guide/authentication).
