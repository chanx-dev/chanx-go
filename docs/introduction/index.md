# What is chanx-go?

chanx-go is a WebSocket framework for Go servers. You describe your messages as structs
and write a handler method per message. chanx-go does the rest of what a real-time API
needs: routing frames to handlers, validating them, replying, broadcasting to groups
across processes, topics, authentication, AsyncAPI documentation and testing.

It is the Go implementation of [chanx](https://github.com/chanx-dev/chanx), the
Python framework on Django Channels and FastAPI, and speaks the same protocol as chanx
and [chanx-js](https://js.chanx.dev/). A client written for one
works with all three.

## How a frame travels

<div class="flow">
  <div class="flow-step"><strong>Your framework</strong><span>net/http, gin, echo or Fiber matches the route and runs its middleware</span></div>
  <div class="flow-arrow">→</div>
  <div class="flow-step"><strong>Adapter</strong><span>upgrades the request and hands the socket over</span></div>
  <div class="flow-arrow">→</div>
  <div class="flow-step"><strong>Engine</strong><span>authenticates, validates each frame, finds its handler</span></div>
  <div class="flow-arrow">→</div>
  <div class="flow-step"><strong>Your consumer</strong><span>a struct of handler methods</span></div>
</div>

- **Messages** are structs embedding `chanx.BaseMessage`, with the action in a tag.
  Their JSON Schema is inferred, and it both validates frames and documents them.
- **A consumer** is a struct whose methods are handlers: `HandleChat(ctx, ChatMessage)`.
  Optional methods named `Groups`, `OnConnect`, `Authenticate` and so on are hooks.
- **The engine** holds the consumers and runs every connection. It knows nothing about
  HTTP: an adapter mounts it on your framework, in a line.
- **A channel layer** carries broadcasts between processes: in memory for one process,
  Redis for many.

## The envelope

Every frame is a JSON object with an `action`, a `payload`, and optional envelope
fields:

```json
{"action": "chat", "payload": {"text": "hi"}, "ref": "7"}
```

| Field | Meaning |
| --- | --- |
| `action` | which message this is, and so which handler runs |
| `payload` | the message body, validated against the message's schema |
| `ref` | set by a client on a request; the reply carries the same `ref` |
| `topic` | addresses a frame to a topic the client subscribed to |
| `version`, `seq` | stamped by the server on replies and topic events |

The full protocol is described in the
[chanx documentation](https://python.chanx.dev/en/latest/protocol.html).

## What it is not

- **Not tied to Go clients.** A chanx-go server serves any chanx client:
  `@chanx-js/client` in the browser and Node, generated Python clients, and Go clients
  from `chanx-codegen`. See [Clients](/reference/clients).
- **Not a pub/sub server.** If you need a standalone message broker with presence and
  history, look at [Centrifugo](https://centrifugal.dev). chanx-go is the application
  server: your handlers, your rules.

Next: [why chanx-go](/introduction/why), or straight to the
[quick start](/introduction/quick-start).
