# Testing

Package `chanxtest` runs connections in memory: no network, no ports, nothing to clean
up. A test connects, sends frames and asserts on what comes back, in the time a unit test
takes. It is the Go counterpart of chanx's `WebsocketCommunicator` and capture helpers.

## A test client

<<< @/snippets/chattest/chat_test.go#client

`Connect` returns once the connection is set up: authenticated, groups joined,
`OnConnect` run. A broadcast sent right after reaches it.

| Method | Does |
| --- | --- |
| `Send(m, chanxtest.Ref("1"), chanxtest.Topic("room:a"))` | sends a message, with envelope fields |
| `SendText(s)`, `SendJSON(v)`, `SendBinary(b)` | sends a raw frame |
| `Receive()` | the next frame; fails after `chanxtest.Timeout` (3s) |
| `Expect(json)` | the next frame equals this JSON, key order ignored |
| `ExpectNothing(d)` | no frame arrives within `d` |
| `ReceiveUntil("done")` | frames up to and including that action (`complete` when none is given) |
| `chanxtest.ReceiveUntilMessage[T](c)` | the first frame carrying T, decoded, and the frames before it |
| `Request(m)` | sends with a fresh `ref`, returns the frames answering it |
| `Subscribe(topic)`, `Unsubscribe(topic)`, `ReceiveTopic(topic)` | topic helpers that assert the confirmation |
| `Auth` | the authentication frame, on a route that authenticates |
| `Close()`, `WaitClosed()` | close, or wait for the server to, and get the code |

A `Frame` has `Action()`, `Ref()`, `Topic()`, `Payload()` and `Decode(&v)`.

## Connect options

```go
c := chanxtest.Connect(t, engine, "/ws/chat/lobby?token=t-ann",
	chanxtest.Header("Authorization", "Bearer t-ann"),
	chanxtest.Cookie("sessionid", "abc"),
	chanxtest.Value(UserKey{}, User{Name: "ann"}), // as middleware would set it
	chanxtest.ClientIP("203.0.113.7"),
)
```

`chanxtest.Dial` is `Connect` returning a rejection before the upgrade as an error, to
test `RejectBeforeUpgrade`. `chanxtest.SkipAuthentication()` leaves the authentication
frame for the test to read.

## Strict mode

By default the client fails the test when the server sends an action no handler
declares: not a reply type, not in `Operations` `Replies`, not in `Emits`. That is how a
broadcast missing from the AsyncAPI document, and so from generated clients, shows up in
a test rather than in production. `chanxtest.Lenient()` turns it off for one client.

## Errors and requests

<<< @/snippets/chattest/chat_test.go#errors

<<< @/snippets/chattest/chat_test.go#request

## Asserting on what was published

Sometimes the thing to check is what a handler published, not what a client received: an
event for other services, a broadcast to a group no test client is in. Wrap the layer in
a spy:

<<< @/snippets/chattest/chat_test.go#spy

| Spy | Python chanx |
| --- | --- |
| `chanxtest.Spy(layer)` | wraps a layer and records every send |
| `chanxtest.Spy(layer, chanxtest.Suppress())` | records without delivering |
| `chanxtest.BroadcastEvents[T](spy)` | `capture_broadcast_events`: events of type T, with their group or channel |
| `chanxtest.TopicBroadcasts[Topic, Event](spy)` | `capture_topic_broadcasts`: with the topic and `seq` |
| `spy.Events()`, `spy.GroupMessages()`, `spy.All()`, `spy.Reset()` | raw records |

## Checking a definition

`chanx.Check(&ChatConsumer{})` reports what `Handle` would panic on: wrong signatures,
duplicate actions. One line, no engine.

## Testing an adapter

A framework adapter of your own proves itself with `chanxtest/adaptertest`, over a real
TCP listener, the suite gin, echo and Fiber pass. The same goes for a channel layer and
`layers/layertest`.

Next: [Frameworks](/frameworks/).
