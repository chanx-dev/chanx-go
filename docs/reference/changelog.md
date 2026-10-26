# Changelog

Each module has its own version and changelog, as each chanx-js package does:

| Module | Changelog |
| --- | --- |
| `go.chanx.dev/v2` | [CHANGELOG.md](https://github.com/chanx-dev/chanx-go/blob/main/CHANGELOG.md) |
| `go.chanx.dev/layers/v2` | [layers/CHANGELOG.md](https://github.com/chanx-dev/chanx-go/blob/main/layers/CHANGELOG.md) |
| `go.chanx.dev/layers/redis/v2` | [layers/redis/CHANGELOG.md](https://github.com/chanx-dev/chanx-go/blob/main/layers/redis/CHANGELOG.md) |
| `go.chanx.dev/adapters/gin/v2` | [adapters/gin/CHANGELOG.md](https://github.com/chanx-dev/chanx-go/blob/main/adapters/gin/CHANGELOG.md) |
| `go.chanx.dev/adapters/echo/v2` | [adapters/echo/CHANGELOG.md](https://github.com/chanx-dev/chanx-go/blob/main/adapters/echo/CHANGELOG.md) |
| `go.chanx.dev/adapters/fiber/v2` | [adapters/fiber/CHANGELOG.md](https://github.com/chanx-dev/chanx-go/blob/main/adapters/fiber/CHANGELOG.md) |

Every release is also on
[GitHub releases](https://github.com/chanx-dev/chanx-go/releases), tagged `v2.1.0` for the
root module and `layers/v2.1.0`, `adapters/gin/v2.1.0` for the others.

A module releases only when it changes, so versions drift apart. Each adapter requires the
core it was tested with, and Go picks the newest version any module asks for, so
`@latest` of each is always a working set:

```bash
go get go.chanx.dev/v2@latest go.chanx.dev/adapters/gin/v2@latest
```

## Versions and the chanx contract

The major version is the chanx contract, shared by every implementation: chanx 2.x,
chanx-js 2.x and chanx-go 2.x speak the same wire protocol and work with each other. A new
contract bumps all three to the next major together.

Within a major, a minor adds features and a patch fixes bugs; neither breaks the Go API.
Go puts the major in the import path, so contract 2 is `go.chanx.dev/v2` and the next one
will be `go.chanx.dev/v3`.
