# Changelog

Each module has its own version and changelog, as each chanx-js package does:

| Module | Changelog |
| --- | --- |
| `go.chanx.dev` | [CHANGELOG.md](https://github.com/chanx-dev/chanx-go/blob/main/CHANGELOG.md) |
| `go.chanx.dev/layers` | [layers/CHANGELOG.md](https://github.com/chanx-dev/chanx-go/blob/main/layers/CHANGELOG.md) |
| `go.chanx.dev/layers/redis` | [layers/redis/CHANGELOG.md](https://github.com/chanx-dev/chanx-go/blob/main/layers/redis/CHANGELOG.md) |
| `go.chanx.dev/adapters/gin` | [adapters/gin/CHANGELOG.md](https://github.com/chanx-dev/chanx-go/blob/main/adapters/gin/CHANGELOG.md) |
| `go.chanx.dev/adapters/echo` | [adapters/echo/CHANGELOG.md](https://github.com/chanx-dev/chanx-go/blob/main/adapters/echo/CHANGELOG.md) |
| `go.chanx.dev/adapters/fiber` | [adapters/fiber/CHANGELOG.md](https://github.com/chanx-dev/chanx-go/blob/main/adapters/fiber/CHANGELOG.md) |

Every release is also on
[GitHub releases](https://github.com/chanx-dev/chanx-go/releases), tagged `v0.3.0` for the
root module and `layers/v0.3.0`, `adapters/gin/v0.3.0` for the others.

A module releases only when it changes, so versions drift apart. Each adapter requires the
core it was tested with, and Go picks the newest version any module asks for, so
`@latest` of each is always a working set:

```bash
go get go.chanx.dev@latest go.chanx.dev/adapters/gin@latest
```

While a module is on 0.x, a minor version may change its API; a patch never does.
