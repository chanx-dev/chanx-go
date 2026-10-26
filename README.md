<h1>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/chanx-dev/chanx-go/main/docs/public/brand/chanx-go-wordmark-dark.svg">
    <img alt="chanx-go" src="https://raw.githubusercontent.com/chanx-dev/chanx-go/main/docs/public/brand/chanx-go-wordmark.svg" height="72">
  </picture>
</h1>

[![Go Reference](https://pkg.go.dev/badge/go.chanx.dev/v2.svg)](https://pkg.go.dev/go.chanx.dev/v2)
[![CI](https://github.com/chanx-dev/chanx-go/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/chanx-dev/chanx-go/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**Structured WebSockets for Go.** You write a struct per message and a method per
handler. chanx-go routes each frame to its handler, validates it first, broadcasts to
groups across processes, and documents the API as AsyncAPI, from which typed Go,
TypeScript and Python clients are generated. It runs on net/http, chi, gorilla/mux, gin,
echo and Fiber.

**[Documentation](https://go.chanx.dev/)** ·
[Quick start](https://go.chanx.dev/introduction/quick-start) ·
[Tutorial](https://go.chanx.dev/tutorial/chat-server) ·
[Why chanx-go?](https://go.chanx.dev/introduction/why) ·
[API reference](https://pkg.go.dev/go.chanx.dev/v2)

![A typed Go handler, the AsyncAPI docs chanx-go generates from it, generating a Go client, and the compiler catching a typo](https://raw.githubusercontent.com/chanx-dev/chanx-go/main/docs/public/chanx-go-demo.gif)

## Why chanx-go

gorilla/websocket and coder/websocket stop at the socket. After the upgrade, every team
writes the same loop:

```go
for {
	var msg struct {
		Action  string          `json:"action"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := conn.ReadJSON(&msg); err != nil {
		return
	}
	switch msg.Action {
	case "chat":
		var p ChatPayload
		if json.Unmarshal(msg.Payload, &p) != nil || p.Text == "" {
			conn.WriteJSON(map[string]string{"error": "bad payload"})
			continue
		}
		hub.Broadcast(room, p) // your own hub, and one process only
	case "ping":
		// ...
	}
	// no schema, no docs, and the frontend guesses the message shapes
}
```

With chanx-go, each message is a struct and each handler is a typed method:

```go
func (ChatConsumer) HandleChat(ctx *chanx.Context, m ChatMessage) error {
	return ctx.Broadcast(ChatNotification{Payload: m.Payload})
}
```

From that, you get:

- **Routing by type.** A frame's `action` picks the handler whose message type matches.
  No `switch`, no `json.RawMessage`.
- **Validation.** A JSON Schema is inferred from each struct. A bad frame gets an error
  frame listing every problem, and your handler never sees it.
- **Groups and events across processes.** Broadcast to rooms, push from HTTP handlers and
  workers, over an in-memory layer or Redis.
- **Topics.** A client subscribes to many streams, such as `ticker:ACME`, over one socket.
- **AsyncAPI 3.0.** A document and a docs page built from the handlers that serve
  traffic, so they cannot drift apart.
- **Typed clients.** Generate a Go client with `chanx-codegen`, or TypeScript and Python
  clients with the chanx tools.
- **Testing.** `chanxtest` runs connections in memory, with no network and no ports.

## Install

```bash
go get go.chanx.dev/v2
```

Go 1.25 or later. The gin, echo and Fiber adapters and the Redis layer are separate
modules, so you only download what you use.

## Quick start

A chat server with rooms, in one file. Save it as `main.go`:

```go
package main

import (
	"log"
	"net/http"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/asyncapi"
	chanxhttp "go.chanx.dev/v2/nethttp"
)

type ChatPayload struct {
	Text string `json:"text"`
}

// ChatMessage is what a client sends.
type ChatMessage struct {
	chanx.BaseMessage `action:"chat" description:"Send a message to the room"`
	Payload           ChatPayload `json:"payload"`
}

// ChatNotification is what everyone in the room receives.
type ChatNotification struct {
	chanx.BaseMessage `action:"chat_notification"`
	Payload           ChatPayload `json:"payload"`
}

type ChatConsumer struct{}

// Groups puts each connection in its room, from the {room} path parameter.
func (ChatConsumer) Groups(c *chanx.Conn) []string { return []string{"room." + c.Param("room")} }

// Emits declares what the consumer sends besides replies.
func (ChatConsumer) Emits() []chanx.Message { return []chanx.Message{ChatNotification{}} }

func (ChatConsumer) HandleChat(ctx *chanx.Context, m ChatMessage) error {
	return ctx.Broadcast(ChatNotification{Payload: m.Payload})
}

func (ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

func main() {
	engine := chanx.New(chanx.Config{}) // an in-memory layer; Redis for several processes
	engine.Handle("/ws/chat/{room}", ChatConsumer{}, chanx.Name("chat"))

	mux := http.NewServeMux()
	chanxhttp.Mount(mux, engine)
	chanxhttp.MountDocs(mux, engine, "/asyncapi", asyncapi.Info{Title: "Chat"})
	log.Fatal(http.ListenAndServe(":8000", mux))
}
```

Run it with `go run .`, open two connections with `npx wscat -c ws://localhost:8000/ws/chat/lobby`,
and send from one:

```json
{"action": "chat", "payload": {"text": "hi"}}
```

Both receive:

```json
{"action": "chat_notification", "payload": {"text": "hi"}}
```

A request with a `ref` gets a reply carrying the same `ref`:

```json
> {"action": "ping", "ref": "1"}
< {"action": "pong", "payload": null, "version": 1, "ref": "1"}
```

Now misspell the field. `HandleChat` never runs, and the sender gets an error in the same
format as Python chanx:

```json
> {"action": "chat", "payload": {"txt": "hi"}}
< {"action": "error", "payload": [{"type": "missing", "loc": ["chat", "payload", "text"], "msg": "Field required"}]}
```

The interactive docs for this API are at <http://localhost:8000/asyncapi>.

## Typed clients

Clients are generated from the server's AsyncAPI document, so they match the server.

**Go**, with `chanx-codegen`:

```bash
go run go.chanx.dev/v2/cmd/chanx-codegen@latest -o ./chatclient http://localhost:8000/asyncapi.json
```

```go
chat, err := chatclient.DialChat(ctx, "ws://localhost:8000", "lobby")
if err != nil {
	log.Fatal(err)
}
defer chat.Close()

err = chat.Send(ctx, chatclient.ChatMessage{Payload: chatclient.ChatPayload{Text: "hi from Go"}})
if err != nil {
	log.Fatal(err)
}

msg, _, err := chat.Receive(ctx)
if err != nil {
	log.Fatal(err)
}
if n, ok := msg.(chatclient.ChatNotification); ok {
	fmt.Println(n.Payload.Text) // hi from Go
}
```

`Send` only accepts messages the route declares, and `ChatPayload{Txt: "hi"}` is a
compile error. The generator also reads Python chanx servers' documents.

**TypeScript**, with [chanx-js](https://js.chanx.dev/), for the
browser, React, Vue, Svelte, Solid and Node:

```bash
npx @chanx-js/codegen http://localhost:8000/asyncapi.json -o src/generated
```

**Python**, with [chanx](https://github.com/chanx-dev/chanx):

```bash
chanx generate-client --schema http://localhost:8000/asyncapi.json --output ./chat_client
```

## Frameworks

A consumer is written once and mounted on any of these. Routes go into your router, and
your middleware runs before the upgrade.

| Framework | Import | Mount |
| --- | --- | --- |
| net/http, chi, gorilla/mux | `chanx-go/nethttp` | `chanxhttp.Mount(mux, engine)` |
| gin | `chanx-go/adapters/gin` | `chanxgin.Mount(r.Group("/"), engine)` |
| echo v5 | `chanx-go/adapters/echo` | `chanxecho.Mount(e.Group(""), engine)` |
| Fiber v3 | `chanx-go/adapters/fiber` | `chanxfiber.Mount(app.Group("/"), engine)` |

Any router that takes an `http.Handler` (go-zero, Kratos, Huma) works through
`chanxhttp.Handler`. See [Frameworks](https://go.chanx.dev/frameworks/).

## With Python and Node

chanx-go is the third implementation of the [chanx](https://github.com/chanx-dev/chanx)
protocol, after Python and [chanx-js](https://js.chanx.dev/). A Go
server sends the same frames and publishes the same AsyncAPI document, so existing chanx
clients connect to it unchanged.

The Redis layer is wire-compatible with channels_redis, fast-channels and
`@chanx-js/layers`. A Go worker can push to sockets held by a Django server, and the
other way round.

Parity is checked by tests against values recorded from Python chanx: validation errors,
group names, key casing, the Redis encodings and the full AsyncAPI document of
`sandbox_fastapi`, which [`examples/`](examples) reproduces in Go. The few deliberate
differences, such as strict type checks on payloads, are listed in
[Differences from Python](https://go.chanx.dev/reference/differences).

## Modules

| Module | What |
| --- | --- |
| `go.chanx.dev/v2` | The framework: messages, consumers, topics, engine, publisher, `nethttp`, `asyncapi`, `chanxtest`, `auth`, the Go client and `chanx-codegen` |
| [`.../layers`](layers) | Channel layer interface, in-memory layer and conformance suite. Standard library only, usable without chanx |
| [`.../layers/redis`](layers/redis) | Redis pub/sub layer, shared with Python and Node |
| [`.../adapters/gin`](adapters/gin), [`echo`](adapters/echo), [`fiber`](adapters/fiber) | Framework adapters |
| [`.../examples`](examples) | Python's `sandbox_fastapi` in Go, one entry point per framework |

Every package is listed on [pkg.go.dev](https://pkg.go.dev/go.chanx.dev/v2).

## Learn more

- [Tutorial: a chat server](https://go.chanx.dev/tutorial/chat-server):
  authentication, presence, tests, Redis and a typed client, step by step
- Guides: [messages and validation](https://go.chanx.dev/guide/messages),
  [consumers](https://go.chanx.dev/guide/consumers),
  [groups and events](https://go.chanx.dev/guide/events),
  [topics](https://go.chanx.dev/guide/topics),
  [authentication](https://go.chanx.dev/guide/authentication),
  [testing](https://go.chanx.dev/guide/testing)
- [Channel layers and Redis](https://go.chanx.dev/layers/)
- [Changelog](CHANGELOG.md)

## Ecosystem

- [chanx](https://github.com/chanx-dev/chanx): the original, for Django Channels and
  FastAPI.
- [chanx-js](https://js.chanx.dev/): TypeScript clients for the
  browser and Node, and a TypeScript server on the same protocol.
- [chanx-kit](https://kit.chanx.dev/): WebSocket components such as
  notifications, chat and presence, copied into your project in one command.

## Contributing

Issues and pull requests are welcome. Setup, the checks a pull request needs and the
commit convention are in [CONTRIBUTING.md](CONTRIBUTING.md); code conventions are in
[AGENTS.md](AGENTS.md).

## License

MIT
