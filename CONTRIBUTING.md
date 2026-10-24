# Contributing

Thanks for helping. This page covers setup, the checks to run before a pull request, the
commit convention releases are built from, and how a release is cut. Code style lives in
[AGENTS.md](AGENTS.md).

## Prerequisites

| Tool | Why |
| --- | --- |
| Go 1.25 or later | every module's floor; CI also runs the latest release |
| Docker | two Redis servers for the Redis layer tests |
| [golangci-lint](https://golangci-lint.run) | `make lint` |
| [git-cliff](https://git-cliff.org) and a GPG signing key | releases only |

## Setup

```bash
git clone git@github.com:huynguyengl99/chanx-go.git
cd chanx-go
docker compose up -d   # Redis on 6393 and 6394, project "chanx-go"
make test
```

The repo has seven modules (the root, `layers`, `layers/redis`, `adapters/gin`,
`adapters/echo`, `adapters/fiber`, `examples`), tied together by `go.work`, so a change in
the root is picked up by the adapters without publishing anything.

## Checks

```bash
make test   # go test -race in every module
make lint   # golangci-lint in every module
gofmt -l .  # must print nothing
```

- The Redis layer tests use `redis://localhost:6393,redis://localhost:6394` by default,
  and skip when Redis is not reachable. Point them elsewhere with
  `CHANX_GO_REDIS=redis://host:port,redis://host2:port`.
- One module or package at a time: `cd adapters/gin && go test -race ./...`.
- To check a module on its own, as users will get it: `GOWORK=off go test ./...` inside it.

Before opening a pull request:

- Tests cover the change. Behaviour visible on the wire must match Python chanx; pin it
  with a vector recorded from Python or a test asserting Python's exact frames.
- A framework adapter passes `chanxtest/adaptertest`.
- Every `go.mod` stays on `go 1.25.0`. If a dependency's newest release needs a newer
  Go, require the newest one that does not.
- `GOWORK=off go mod tidy` leaves the touched modules unchanged.

## Commits

Commits follow [Conventional Commits](https://www.conventionalcommits.org): the release
version and the changelog are computed from them.

```text
feat(layers/redis): add sharded pub/sub
fix: echo the ref on validation errors
docs: explain topic group names
```

| Type | Release |
| --- | --- |
| `fix`, `perf` | patch |
| `feat` | minor |
| `!` after the type, or a `BREAKING CHANGE:` footer | major (minor while on 0.x) |
| `docs`, `refactor`, `test`, `build`, `ci`, `chore` | none on their own |

Scopes name the package or module: `layers`, `layers/redis`, `protocol`, `transport`,
`nethttp`, `asyncapi`, `chanxtest`, `adapters/gin`, `examples`...

## Releasing

Maintainers only. Every module is released together, at the same version.

```bash
make release-dry                 # next version and its changelog, nothing changed
make release                     # bump, CHANGELOG.md, signed commit, one tag per module
git push origin main --tags
```

`make release` refuses a dirty tree or a history with nothing to release. It bumps the
requires between this repo's modules, writes `CHANGELOG.md`, commits
`chore(release): X.Y.Z` and tags `vX.Y.Z`, `layers/vX.Y.Z`, `layers/redis/vX.Y.Z` and
`adapters/{gin,echo,fiber}/vX.Y.Z`. `scripts/release.sh X.Y.Z` forces a version.

Pushing the tags runs `release.yml`, which publishes the GitHub release and asks
proxy.golang.org to fetch the new versions. Check from a directory outside the repo:

```bash
go get github.com/huynguyengl99/chanx-go@vX.Y.Z
```

A published version can never be changed. To withdraw a bad one, release the next patch
with a `retract vX.Y.Z` line in the affected `go.mod`.
