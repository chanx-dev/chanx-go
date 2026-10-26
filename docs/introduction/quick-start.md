# Quick start

A chat server with rooms, in one file. You need Go 1.25 or later, and any WebSocket
client to try it: `npx wscat` is used below.

## 1. Set up

```bash
mkdir chat && cd chat
go mod init example.com/chat
go get go.chanx.dev/v2
```

## 2. Declare the messages

Create `main.go`. A message is a struct embedding `chanx.BaseMessage`, with its action in
the tag and its body in a `Payload` field:

<<< @/snippets/quickstart/main.go#messages

## 3. Write the consumer

A consumer serves one route. Each exported method taking a `*chanx.Context` and a message
is a handler, picked by the message's action:

<<< @/snippets/quickstart/main.go#consumer

What each part does:

- **`Groups`** puts each connection in its room. `{room}` is a path parameter, read with
  `c.Param("room")`.
- **`HandleChat`** runs for every `chat` frame, after its payload was validated, and
  broadcasts to the room: the sender included, everyone connected to the same room.
- **`HandlePing`** returns a reply. A reply goes back to the sender and carries the `ref`
  of the frame it answers.
- **`Emits`** declares what the consumer broadcasts, so the AsyncAPI document lists it.

## 4. Mount and run

<<< @/snippets/quickstart/main.go#main

```bash
go run .
```

`chanx.New` gives the engine an in-memory channel layer, enough for one process.
`chanxhttp.Mount` registers the route on the `ServeMux`; gin, echo and Fiber have an
adapter each, see [Frameworks](/frameworks/).

## 5. Talk to it

Open two terminals:

```bash
npx wscat -c ws://localhost:8000/ws/chat/lobby
```

Send from one:

```json
{"action": "chat", "payload": {"text": "hi"}}
```

Both receive:

```json
{"action": "chat_notification", "payload": {"text": "hi"}}
```

Ask with a `ref`, and the reply carries it back:

```json
> {"action": "ping", "ref": "1"}
< {"action": "pong", "payload": null, "version": 1, "ref": "1"}
```

## 6. Try breaking it

Misspell the field:

```json
> {"action": "chat", "payload": {"txt": "hi"}}
< {"action": "error", "payload": [{"type": "missing", "loc": ["chat", "payload", "text"], "msg": "Field required"}]}
```

`HandleChat` never ran. The error lists every problem with its location, in the same
format as Python chanx, so a client handles errors once for every server.

Now open <http://localhost:8000/asyncapi>: the API you wrote, documented from the
code, with `chat`, `chat_notification` and `ping` and their schemas.

## Next

- [Tutorial: a chat server](/tutorial/chat-server): authentication, presence, tests and
  Redis, step by step.
- [Messages and validation](/guide/messages): optional fields, defaults, enums, custom
  checks.
- [Clients](/reference/clients): a typed TypeScript client generated from
  `/asyncapi.json`.
