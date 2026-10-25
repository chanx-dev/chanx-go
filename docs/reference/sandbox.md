# The sandbox example

[`examples/`](https://github.com/huynguyengl99/chanx-go/tree/main/examples) holds Python
chanx's `sandbox_fastapi` written with chanx-go: the same consumers, messages and routes,
mounted on net/http, gin, echo or Fiber. A test compares its AsyncAPI document with the one
Python generates, key for key.

```bash
git clone https://github.com/huynguyengl99/chanx-go
cd chanx-go/examples
go run ./cmd/sandbox            # or sandbox-gin, sandbox-echo, sandbox-fiber
```

Open <http://localhost:8000/asyncapi> for the full API. It covers request and reply,
groups, mixins, passthrough events, enums and defaults, several layer aliases, a
background worker answering through the layer, topics on a hub and on their own route,
and pushing to sockets from a plain HTTP endpoint.

With Redis, two processes share their groups:

```bash
docker compose up -d   # from the repo root
go run ./cmd/sandbox -redis redis://localhost:6393
go run ./cmd/sandbox -redis redis://localhost:6393 -addr :8001
```

The [examples README](https://github.com/huynguyengl99/chanx-go/tree/main/examples#readme)
lists every route.
