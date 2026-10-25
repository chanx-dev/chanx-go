# net/http, chi and gorilla/mux

Package `chanxhttp` mounts the engine on anything built on `net/http`: the standard
`ServeMux`, chi, gorilla/mux, and frameworks that take a plain `http.Handler`.

## ServeMux

<<< @/snippets/frameworks/frameworks.go#nethttp

`Mount` registers `GET /ws/chat/{room}`, and the consumer reads `{room}` through
`r.PathValue`. A full server with graceful shutdown:

<<< @/snippets/frameworks/frameworks.go#shutdown

## Options

<<< @/snippets/frameworks/frameworks.go#nethttp-options

| Option | Does |
| --- | --- |
| `chanxhttp.Prefix("/api")` | prepends a path to every route |
| `chanxhttp.AcceptOptions(&coderws.Options{...})` | origins, subprotocols, compression |
| `chanxhttp.Upgrade(fn)` | another WebSocket library, see below |
| `chanxhttp.ClientIP(fn)` | how the client address is read; `RemoteAddr` by default |
| `chanxhttp.NoMethod()` | registers `/ws/...` instead of `GET /ws/...` |

::: warning Origins
coder/websocket accepts only an `Origin` matching the request's host by default. A
frontend served from another origin needs `OriginPatterns`.
:::

Behind a proxy, `ClientIP` should use the same trusted-proxy logic as the rest of your
app: `X-Forwarded-For` is whatever the client wrote unless your proxy overwrites it.

## chi

chi takes the same path syntax, without a method in the pattern:

```go
r := chi.NewRouter()
r.Use(authMiddleware)
chanxhttp.Mount(r, engine, chanxhttp.NoMethod())
```

or route by route, `r.Get("/ws/chat/{room}", chanxhttp.Handler(engine, route))`.

## gorilla/mux and prefix mounts

`Dispatcher` serves every route from one handler, matching the URL itself:

```go
r := mux.NewRouter()
r.PathPrefix("/ws/").Handler(chanxhttp.Dispatcher(engine))
```

Unknown paths get 404. It fits any router that can hand a prefix to a handler.

## Other frameworks

go-zero, Kratos, Huma and Encore raw endpoints register a plain `http.Handler`; give them
`chanxhttp.Handler(engine, route)`. Middleware values reach the consumer through the
request context: `context.WithValue` in the middleware, `chanx.UserFrom(key)` or
`r.Value(key)` in chanx.

## gorilla/websocket

The default upgrade uses coder/websocket. To use gorilla's instead, for its options or
because the team already does:

<<< @/snippets/frameworks/frameworks.go#gorilla

Next: [gin](/frameworks/gin).
