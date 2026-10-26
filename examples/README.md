# chanx-go examples

The sandbox is Python chanx's
[`sandbox_fastapi`](https://github.com/chanx-dev/chanx/tree/main/sandbox_fastapi)
written with chanx-go: the same consumers, messages and routes, and the same AsyncAPI
document, which a test compares with the one Python generates. It runs on four
frameworks with one line of difference.

```bash
cd examples
go run ./cmd/sandbox                    # net/http, in-memory layers
go run ./cmd/sandbox-gin                # or sandbox-echo, sandbox-fiber
go run ./cmd/sandbox -asyncapi > asyncapi.json
```

Every server listens on <http://localhost:8000>; the interactive docs are at
<http://localhost:8000/asyncapi>.

To fan out across processes, start Redis from the repo root (`docker compose up -d`) and
run two servers on it:

```bash
go run ./cmd/sandbox -redis redis://localhost:6393
go run ./cmd/sandbox -redis redis://localhost:6393 -addr :8001
```

A chat sent to one reaches clients connected to the other. Every entry point also reads
`REDIS_URL`; `SEND_COMPLETION=1` turns on completion frames.

## Try it

Connect with any WebSocket client, such as `npx wscat -c ws://localhost:8000/ws/chat`,
and send:

```json
{"action": "chat", "payload": {"message": "hi"}}
```

Every client on `/ws/chat` receives:

```json
{"action": "chat_notification", "payload": {"message": "💬 hi"}}
```

A malformed message (`{"action": "chat", "payload": {}}`) gets a validation error
instead, and `{"action": "ping", "ref": "1"}` gets a pong carrying the same `ref`.

## Routes

| Route | Consumer | Shows |
| --- | --- | --- |
| `/ws/system` | `SystemMessageConsumer` | request/reply on one socket, no layer |
| `/ws/chat` | `ChatConsumer` | a group, mixins, passthrough events, join and leave broadcasts |
| `/ws/room/{room_name}` | `RoomChatConsumer` | a group per path parameter, enums and defaults |
| `/ws/reliable` | `ReliableChatConsumer` | broadcasting on a second layer alias |
| `/ws/notifications` | `NotificationConsumer` | a group, plus server events turned into notifications |
| `/ws/analytics` | `AnalyticsConsumer` | events on a dedicated layer |
| `/ws/background_jobs` | `BackgroundJobConsumer` | a worker answering through the layer with an event |
| `/ws/topics` | `TopicHubConsumer` | several topics (`room:<name>`, `presence:<user>`) over one socket |
| `/ws/topics/room/{room_name}` | `RoomTopic` | the same topic on its own route, no envelope needed |
| `POST /notify` | `NotifyHandler` | pushing to sockets from plain HTTP, `{"message": "..."}` |
| `/asyncapi`, `.json`, `.yaml` | | the AsyncAPI page and document |

## Code

| File | What |
| --- | --- |
| [`sandbox/messages.go`](sandbox/messages.go) | the messages, one struct per action |
| [`sandbox/consumers.go`](sandbox/consumers.go) | the consumers, mixins and topics |
| [`sandbox/sandbox.go`](sandbox/sandbox.go) | the layers and the routes |
| [`sandbox/http.go`](sandbox/http.go) | the HTTP endpoint publishing to sockets |
| [`cmd/`](cmd) | one `main.go` per framework |
| [`sandbox/sandbox_test.go`](sandbox/sandbox_test.go) | the consumers tested with `chanxtest` |
| [`sandbox/asyncapi_test.go`](sandbox/asyncapi_test.go) | the parity test against Python's document |
| [`sandboxclient/`](sandboxclient) | the Go client `chanx-codegen` generates from Python's document, driven against the Go sandbox by its test |
