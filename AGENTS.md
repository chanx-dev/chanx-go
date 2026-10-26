# AGENTS.md

How to work in chanx-go.

## The repo

The Go implementation of [chanx](https://github.com/chanx-dev/chanx), a third
implementation of its protocol (after Python and chanx-js) held in line by tests against
Python.

| Path | What |
| --- | --- |
| `*.go` | package `chanx`: messages, consumer discovery, topics, engine, connection runtime, dispatch, publisher |
| `protocol/` | Wire protocol shared by every implementation: envelope, patterns, group names, casing |
| `internal/schema/` | Go types to Pydantic-shaped JSON Schema, validation, Pydantic error mapping |
| `transport/` | `transport.Conn`, the in-memory pipe, coder and gorilla upgrades |
| `nethttp/`, `adapters/*` | Framework adapters; each passes `chanxtest/adaptertest` |
| `asyncapi/` | AsyncAPI 3.0 generator, same document as Python's |
| `chanxtest/` | Test client and layer spy |
| `client/`, `codegen/`, `cmd/chanx-codegen/` | Go client (`chanxclient`), and the generator of typed clients from an AsyncAPI document |
| `layers/`, `layers/redis/` | Channel layers, own modules, never importing chanx |
| `examples/` | `sandbox_fastapi` in Go, one entry point per framework |
| `testdata/` | Golden vectors recorded from Python chanx |

## Commands

```bash
make test                      # go test -race in every module
make lint                      # golangci-lint in every module
docker compose up -d           # Redis on 6393 and 6394 (project chanx-go)
make release-dry               # each changed module's next version and changelog
make release                   # bump, changelogs, signed commit and one tag per module
```

## Rules

- **Parity is tested, not assumed.** A behaviour visible on the wire must match Python
  chanx: pin it with a vector recorded from Python or a test asserting Python's exact
  frames. When Go differs on purpose, list it in docs/reference/differences.md.
- **Go floor is 1.25** in every published module's `go.mod`; `examples` and `go.work`
  follow the latest Go. When a dependency's newest release needs a newer Go, pin the
  newest one that does not (see `layers/redis`, `adapters/fiber`).
- **Layers never import chanx.**
- **Adapters snapshot what they read** before a framework recycles its context.

## Code that speaks for itself

The code is the documentation. Reach for a better name, a smaller function or a type
before a comment.

- Comment only what the code cannot say: why a non-obvious choice was made, a constraint
  from outside (a Python behaviour, a library trap), or an algorithm a reader would
  otherwise reverse-engineer. Never restate what the line does.
- Doc comments on exported API, short; more lines only for a non-obvious contract.
- Package comments on the package's main file only; `chanx` keeps its overview in `doc.go`.
- No section dividers, commented-out code or TODOs without an owner.
- No long dashes in comments or docs. Use a colon, a comma or a new sentence.
- Name things after their Python counterpart when chanx or channels has one.
