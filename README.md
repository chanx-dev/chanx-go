# chanx-go

The Go implementation of [chanx](https://github.com/huynguyengl99/chanx): structured
WebSockets with typed messages, topics, groups, events and AsyncAPI, on the same wire
protocol as Python chanx. A Go backend serves the same frames, emits the same AsyncAPI
document and shares a Redis channel layer with Python (and Node) processes, so
`@chanx-js/client`, `@chanx-js/codegen` and chanx's Python client generator work against
it unchanged.

**[Documentation](https://huynguyengl99.github.io/chanx-go/)** ·
[Quick start](https://huynguyengl99.github.io/chanx-go/introduction/quick-start) ·
[Why chanx-go?](https://huynguyengl99.github.io/chanx-go/introduction/why) ·
[API](https://pkg.go.dev/github.com/huynguyengl99/chanx-go)

```go
type ChatPayload struct {
	Message string `json:"message"`
}

type ChatMessage struct {
	chanx.BaseMessage `action:"chat" description:"Send a chat message"`
	Payload           ChatPayload `json:"payload"`
}

type ChatNotification struct {
	chanx.BaseMessage `action:"chat_notification"`
	Payload           ChatPayload `json:"payload"`
}

type ChatConsumer struct{}

func (ChatConsumer) Groups(c *chanx.Conn) []string { return []string{"room." + c.Param("room")} }

func (ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

// Emits declares what HandleChat broadcasts, so the AsyncAPI document lists it.
func (ChatConsumer) Emits() []chanx.Message { return []chanx.Message{ChatNotification{}} }

func (ChatConsumer) HandleChat(ctx *chanx.Context, m ChatMessage) error {
	return ctx.Broadcast(ChatNotification{Payload: m.Payload}, chanx.ExcludeSelf())
}

func main() {
	engine := chanx.New(chanx.Config{}) // in-memory layer; redis.NewPubSub for several processes
	engine.Handle("/ws/chat/{room}", ChatConsumer{}, chanx.Tags("chat"))

	mux := http.NewServeMux()
	chanxhttp.Mount(mux, engine)
	chanxhttp.MountDocs(mux, engine, "/asyncapi", asyncapi.Info{Title: "Chat"})
	log.Fatal(http.ListenAndServe(":8000", mux))
}
```

```text
> {"action":"ping","ref":"1"}
< {"action":"pong","payload":null,"version":1,"ref":"1"}
> {"action":"chat","payload":{}}
< {"action":"error","payload":[{"type":"missing","loc":["chat","payload","message"],"msg":"Field required"}]}
```

## Modules

| Module | Package | What |
| --- | --- | --- |
| `github.com/huynguyengl99/chanx-go` | `chanx` | Messages, consumers, topics, engine, publisher, auth |
| | `protocol` | Envelope, actions, topic patterns, `SafeGroupName`, pyhumps casing |
| | `nethttp` (`chanxhttp`) | net/http: ServeMux, chi, gorilla/mux and any `http.Handler` router |
| | `asyncapi` | AsyncAPI 3.0 document, JSON/YAML/HTML handlers |
| | `chanxtest`, `chanxtest/adaptertest` | In-process test client, layer spy, adapter suite |
| | `transport/coderws`, `transport/gorillaws` | WebSocket libraries behind a 3-method `transport.Conn` |
| | `auth`, `stream` | Bearer token helpers, `DeltaTracker` |
| [`.../layers`](layers) | `layers` | Channel layer interface, in-memory layer, serializers, conformance suite (stdlib only) |
| [`.../layers/redis`](layers/redis) | `redis` | Redis pub/sub layer, wire-compatible with channels_redis and fast-channels |
| [`.../adapters/gin`](adapters/gin) | `chanxgin` | gin |
| [`.../adapters/echo`](adapters/echo) | `chanxecho` | echo v5 |
| [`.../adapters/fiber`](adapters/fiber) | `chanxfiber` | Fiber v3 (fasthttp) |
| [`.../examples`](examples) | | Python's `sandbox_fastapi` in Go, one entry point per framework |

Go 1.25 or later. The layers never import chanx: use them alone for fan-out between
replicas, or to join a Python Channels deployment.

## Parity with Python

Wire behaviour is pinned by tests against fixtures recorded from Python chanx, not by
reading its code: `safe_group_name` (with the blake2s digest), topic patterns, pyhumps
casing, Pydantic's validation errors, the Redis shard ring, msgpack and JSON encodings,
Fernet tokens (byte for byte), and the AsyncAPI document of `sandbox_fastapi`, which the
Go port in `examples/sandbox` reproduces exactly.

Known differences, by decision (see [Differences from Python](https://huynguyengl99.github.io/chanx-go/reference/differences)):
payloads are validated strictly (Pydantic coerces `"3"` into an int, chanx-go answers
`int_type`); the JSON decoder's error text after `Invalid JSON: ` is Go's.

## Development

```bash
docker compose up -d   # two Redis for the layer tests
make test              # go test -race in every module
make lint
```

Setup, the checks a pull request needs, the commit convention and releasing are in
[CONTRIBUTING.md](CONTRIBUTING.md); code conventions in [AGENTS.md](AGENTS.md).

## License

MIT
