---
layout: home

hero:
  name: chanx-go
  text: Structured WebSockets for Go.
  tagline: Write a struct of handlers. chanx-go routes each frame to the right one, validates it first, broadcasts across processes and documents it all as AsyncAPI, on the same protocol as Python chanx.
  actions:
    - theme: brand
      text: Quick start
      link: /introduction/quick-start
    - theme: alt
      text: What is chanx-go?
      link: /introduction/
    - theme: alt
      text: GitHub
      link: https://github.com/huynguyengl99/chanx-go

features:
  - title: Handlers, not a switch
    details: 'Each action gets a typed method: HandleChat(ctx, ChatMessage). No json.RawMessage, no switch on msg.Type.'
  - title: Validated before your code runs
    details: "Payloads are checked against a JSON Schema inferred from the struct. A bad frame gets an error frame in Pydantic's format; your handler never sees it."
  - title: Groups and topics across processes
    details: 'Broadcast to rooms, push from HTTP handlers and workers, let clients subscribe to many streams over one socket. Over Redis, every process is reached.'
  - title: AsyncAPI from the code
    details: 'The document is built from the handlers that serve traffic, so it cannot drift. Typed clients for TypeScript and Python are generated from it.'
  - title: Your framework
    details: 'net/http, chi, gorilla/mux, gin, echo and Fiber. Routes live in your router and your middleware runs before the upgrade.'
  - title: One protocol with Python and Node
    details: 'Same frames, same AsyncAPI, same Redis layer as chanx and chanx-js: their clients work unchanged, and Go and Python processes share groups.'
---

## A consumer in Go

<<< @/snippets/quickstart/main.go#consumer

Mount it, and two clients in `/ws/chat/lobby` talk:

```json
> {"action": "chat", "payload": {"text": "hi"}}
< {"action": "chat_notification", "payload": {"text": "hi"}}
```

A frame that does not match the message never reaches `HandleChat`:

```json
> {"action": "chat", "payload": {"txt": "hi"}}
< {"action": "error", "payload": [{"type": "missing", "loc": ["chat", "payload", "text"], "msg": "Field required"}]}
```

[Build it in five minutes](/introduction/quick-start), or read
[what chanx-go is](/introduction/) first.
