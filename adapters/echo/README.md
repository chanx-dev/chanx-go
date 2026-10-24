# chanx-go for echo

Mounts a [chanx-go](https://github.com/huynguyengl99/chanx-go) engine in an
[echo](https://github.com/labstack/echo) v5 router. Routes, groups and middleware are
echo's; params come from `c.Param`, values your middleware sets with `c.Set` are read by
the consumer, the client IP is `c.RealIP()`, and failures before the upgrade go through
echo's `HTTPErrorHandler`.

```bash
go get github.com/huynguyengl99/chanx-go/adapters/echo
```

```go
engine := chanx.New(chanx.Config{})
engine.Handle("/ws/chat/{room}", &ChatConsumer{}, chanx.UserFrom("user"))

e := echo.New()
chanxecho.Mount(e.Group("", authMiddleware), engine)   // "{room}" becomes ":room"
chanxecho.MountDocs(e, engine, "/asyncapi", asyncapi.Info{Title: "Chat"})
e.Start(":8000")
```

`authMiddleware` runs before the upgrade: a value it stores with `c.Set("user", u)` is
the connection's user through `chanx.UserFrom("user")`, or `r.Value("user")` inside
`Authenticate`.

- `chanxecho.Handler(engine, route)` registers one route by hand.
- Both take an optional `Accept` function replacing the default coder/websocket
  upgrade, for example one calling `gorillaws.Accept`.
- `MountDocs` serves the AsyncAPI page at the path, with `.json` and `.yaml` next to it.

Shut down with `engine.Shutdown(ctx)` as well as echo's own shutdown: WebSockets are
hijacked connections the HTTP server no longer tracks.

MIT licensed.
