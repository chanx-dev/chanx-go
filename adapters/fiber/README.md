# chanx-go for Fiber

Mounts a [chanx-go](https://github.com/chanx-dev/chanx-go) engine in a
[Fiber](https://github.com/gofiber/fiber) v3 router, on fasthttp. Routes, groups and
middleware are Fiber's, and failures before the upgrade go through its `ErrorHandler`.

```bash
go get go.chanx.dev/adapters/fiber
```

```go
engine := chanx.New(chanx.Config{})
engine.Handle("/ws/chat/{room}", &ChatConsumer{}, chanx.UserFrom("user"))

app := fiber.New()
chanxfiber.Mount(app.Group("/", authMiddleware), engine)   // "{room}" becomes ":room"
chanxfiber.MountDocs(app, engine, "/asyncapi", asyncapi.Info{Title: "Chat"})
app.Listen(":8000")
```

`authMiddleware` runs before the upgrade: a value it stores with `c.Locals("user", u)` is
the connection's user through `chanx.UserFrom("user")`, or `r.Value("user")` inside
`Authenticate`.

Fiber recycles its context once the upgrade returns, so the adapter copies what chanx
reads first: params, query, headers, cookies, the client IP and every string-keyed
`Locals` value. Name other keys with `chanxfiber.Locals(ctxKey{})`. `Request.Native`
is nil afterwards.

- `chanxfiber.Handler(engine, route)` registers one route by hand.
- `chanxfiber.WebSocket(websocket.Config{...})` sets origins, subprotocols and buffers.
- `MountDocs` serves the AsyncAPI page at the path, with `.json` and `.yaml` next to it.

MIT licensed.
