# Frameworks

The engine knows nothing about HTTP. An adapter mounts it on your framework: the routes
go into your router, your middleware runs before the upgrade, and path parameters, values
your middleware stored, the client IP and errors all go through the framework's own
mechanisms. A consumer is written once and mounted anywhere.

| Framework | Package | Mount |
| --- | --- | --- |
| net/http `ServeMux` | `chanx-go/nethttp` | `chanxhttp.Mount(mux, engine)` |
| chi | `chanx-go/nethttp` | `chanxhttp.Mount(r, engine, chanxhttp.NoMethod())` |
| gorilla/mux, a prefix mount | `chanx-go/nethttp` | `r.PathPrefix("/ws/").Handler(chanxhttp.Dispatcher(engine))` |
| go-zero, Kratos, Huma, Encore raw | `chanx-go/nethttp` | `chanxhttp.Handler(engine, route)`, a plain `http.Handler` |
| gin | `chanx-go/adapters/gin` | `chanxgin.Mount(r.Group("/", auth), engine)` |
| echo v5 | `chanx-go/adapters/echo` | `chanxecho.Mount(e.Group("", auth), engine)` |
| Fiber v3 | `chanx-go/adapters/fiber` | `chanxfiber.Mount(app.Group("/", auth), engine)` |

gin, echo and Fiber adapters are separate modules, so a net/http user never downloads
them:

```bash
go get github.com/huynguyengl99/chanx-go/adapters/gin
```

## What every adapter does

1. **Registers the routes** in the framework's router, `{room}` written in its syntax.
2. **Reads the request**: path parameters, query, headers, cookies, the client IP, and
   the values middleware stored, into a `chanx.Request` your `Authenticate` reads.
3. **Upgrades** the connection, with coder/websocket by default, or the framework's own
   library (Fiber).
4. **Hands the socket to the engine**, which runs it until it closes.

Every adapter passes the same suite, `chanxtest/adaptertest`, over a real TCP listener:
request and reply, the user set by middleware and a middleware rejection, a rejection
before the upgrade, unknown paths, groups, topics and shutdown.

## Shutting down

The HTTP server does not track upgraded connections, so its own `Shutdown` leaves them
open. Call `engine.Shutdown(ctx)` too: it closes every socket with 1001 and waits for
their handlers. `chanxhttp.Shutdown(ctx, srv, engine)` does both for net/http.

## Writing an adapter

An adapter is about a hundred lines: build a `chanx.Request`, call
`engine.Preflight` when `route.RejectsBeforeUpgrade()`, upgrade, wrap the socket as a
`transport.Conn` (three methods: `Read`, `Write`, `Close`), and call `engine.Serve`. Read
`adapters/gin/gin.go` for the shape, and run `adaptertest.Run` to prove it.

Next: [net/http, chi and gorilla/mux](/frameworks/nethttp).
