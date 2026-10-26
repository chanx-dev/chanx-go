# echo

For echo v5.

```bash
go get go.chanx.dev/adapters/echo/v2
```

<<< @/snippets/frameworks/frameworks.go#echo

```go
engine.Handle("/ws/chat/{room}", ChatConsumer{}, chanx.UserFrom("user"))
```

- **Routes, groups and middleware are echo's.** `{room}` is registered as `:room`.
- **Middleware runs before the upgrade.** A value it stores with `c.Set("user", u)` is
  the connection's user through `chanx.UserFrom("user")`. An error it returns goes
  through echo's `HTTPErrorHandler`, and no socket is opened.
- **The client IP** is `c.RealIP()`, so echo's `IPExtractor` applies.
- **Docs**: `chanxecho.MountDocs` serves `/asyncapi`, `.json` and `.yaml`.

`chanxecho.Mount` and `chanxecho.Handler` take an optional `Accept` function replacing
the default coder/websocket upgrade. Shut down with `engine.Shutdown(ctx)` as well as
echo's.

Next: [Fiber](/frameworks/fiber).
