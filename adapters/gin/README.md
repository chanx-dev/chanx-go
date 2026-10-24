# chanx-go for gin

Mounts a [chanx-go](https://github.com/huynguyengl99/chanx-go) engine in a
[gin](https://github.com/gin-gonic/gin) router. Routes live in gin's router, so its
groups, middleware and `NoRoute` apply; params come from `c.Param`, values your
middleware sets with `c.Set` are read by the consumer, and the client IP is
`c.ClientIP()` with gin's trusted proxies.

```bash
go get github.com/huynguyengl99/chanx-go/adapters/gin
```

```go
engine := chanx.New(chanx.Config{})
engine.Handle("/ws/chat/{room}", &ChatConsumer{}, chanx.UserFrom("user"))

r := gin.Default()
chanxgin.Mount(r.Group("/", authMiddleware), engine)   // "{room}" becomes ":room"
chanxgin.MountDocs(r, engine, "/asyncapi", asyncapi.Info{Title: "Chat"})
r.Run(":8000")
```

`authMiddleware` runs before the upgrade: a value it stores with `c.Set("user", u)` is
the connection's user through `chanx.UserFrom("user")`, or `r.Value("user")` inside
`Authenticate`.

- `chanxgin.Handler(engine, route, nil)` registers one route by hand.
- `chanxgin.AcceptOptions(&coderws.Options{...})` sets origins, subprotocols and
  compression for the default coder/websocket upgrade; `chanxgin.Upgrade(fn)` swaps the
  library, for example with a function calling `gorillaws.Accept`.
- `MountDocs` serves the AsyncAPI page at the path, with `.json` and `.yaml` next to it.

Shut down with `engine.Shutdown(ctx)` as well as the server's own shutdown: WebSockets
are hijacked connections the HTTP server no longer tracks.

MIT licensed.
