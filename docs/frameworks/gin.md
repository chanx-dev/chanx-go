# gin

```bash
go get go.chanx.dev/adapters/gin/v2
```

<<< @/snippets/frameworks/frameworks.go#gin

```go
engine.Handle("/ws/chat/{room}", ChatConsumer{}, chanx.UserFrom("user"))
```

- **Routes live in gin's router**, so its groups, middleware and `NoRoute` apply.
  `{room}` is registered as `:room`.
- **Middleware runs before the upgrade.** A value it stores with `c.Set("user", u)` is
  the connection's user through `chanx.UserFrom("user")`, or `r.Value("user")` in
  `Authenticate`. A middleware that aborts answers with an HTTP status, and no socket is
  opened.
- **The client IP** is `c.ClientIP()`, with gin's trusted proxies.
- **Docs**: `chanxgin.MountDocs` serves `/asyncapi`, `.json` and `.yaml`.

| Option | Does |
| --- | --- |
| `chanxgin.AcceptOptions(&coderws.Options{...})` | origins, subprotocols, compression |
| `chanxgin.Upgrade(fn)` | another WebSocket library, such as a function calling `gorillaws.Accept` |

`chanxgin.Handler(engine, route, nil)` registers one route by hand. Shut down with
`engine.Shutdown(ctx)` as well as your `http.Server`'s.

Next: [echo](/frameworks/echo).
