# Fiber

For Fiber v3, on fasthttp.

```bash
go get go.chanx.dev/adapters/fiber/v2
```

<<< @/snippets/frameworks/frameworks.go#fiber

```go
engine.Handle("/ws/chat/{room}", ChatConsumer{}, chanx.UserFrom("user"))
```

- **Routes, groups and middleware are Fiber's.** `{room}` is registered as `:room`.
- **Middleware runs before the upgrade**, and an error it returns goes through Fiber's
  `ErrorHandler`.
- **The upgrade** uses Fiber's own WebSocket package; `chanxfiber.WebSocket(cfg)` sets
  its origins, subprotocols and buffers.
- **Docs**: `chanxfiber.MountDocs` serves `/asyncapi`, `.json` and `.yaml`.

## Fiber recycles its context

Once the upgrade returns, Fiber reuses the request's context and its strings for the
next request. The adapter copies what chanx reads before that happens: path parameters,
query, headers, cookies, the client IP, and every `Locals` value under a string key. A
value under another key type must be named:

<<< @/snippets/frameworks/frameworks.go#fiber-locals

`Request.Native()` returns `nil` on Fiber, since the context it would return is no longer
yours.

Next: [Channel layers](/layers/).
