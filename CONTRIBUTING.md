# Contributing

Thanks for helping. This page covers setup, the checks to run before a pull request, the
commit convention releases are built from, and how a release is cut. Code style lives in
[AGENTS.md](AGENTS.md).

## Prerequisites

| Tool | Why |
| --- | --- |
| Go 1.26 | the workspace, `examples` and `docs`; with Go 1.25, `go` fetches 1.26 itself. Published modules support Go 1.25 |
| Docker | two Redis servers for the Redis layer tests |
| [golangci-lint](https://golangci-lint.run) | `make lint` |
| Node 22 and pnpm | the documentation site only |
| [git-cliff](https://git-cliff.org) and a GPG signing key | releases only |

## Setup

```bash
git clone git@github.com:huynguyengl99/chanx-go.git
cd chanx-go
docker compose up -d   # Redis on 6393 and 6394, project "chanx-go"
make test
```

The repo has eight modules (the root, `layers`, `layers/redis`, `adapters/gin`,
`adapters/echo`, `adapters/fiber`, `examples`, `docs`), tied together by `go.work`, so a change in
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
- Every published module's `go.mod` stays on `go 1.25.0`: CI tests each one alone on Go
  1.25, with `GOWORK=off`. If a dependency's newest release needs a newer Go, require
  the newest one that does not.
- `GOWORK=off go mod tidy` leaves the touched modules unchanged.

## Documentation

| Where | What |
| --- | --- |
| doc comments and `Example*` functions | the API reference on pkg.go.dev |
| `README.md` in each published module | its landing page on GitHub and pkg.go.dev |
| `docs/` | the guide site, VitePress, at huynguyengl99.github.io/chanx-go |

```bash
make docs-dev   # the site at http://localhost:5173/chanx-go/, reloading on save
make docs       # build it; dead links fail
```

Code on the site comes from `docs/snippets/`, a Go module tested like the others: each
snippet package has a test pinning the frames its page shows. Include a region with
`<<< @/snippets/<dir>/<file>.go#<region>`. The voice and the rules are in
[.claude/skills/docs/SKILL.md](.claude/skills/docs/SKILL.md).

The site deploys when a release is tagged, or by hand from the Docs workflow. Pages must
be enabled once in the repository settings, with GitHub Actions as the source.

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
git push origin main layers/vX.Y.Z layers/redis/vX.Y.Z adapters/{gin,echo,fiber}/vX.Y.Z
git push origin vX.Y.Z           # alone: it runs release.yml
```

`make release` refuses a dirty tree or a history with nothing to release. It bumps the
requires between this repo's modules, writes `CHANGELOG.md`, commits
`chore(release): X.Y.Z` and tags `vX.Y.Z`, `layers/vX.Y.Z`, `layers/redis/vX.Y.Z` and
`adapters/{gin,echo,fiber}/vX.Y.Z`. `scripts/release.sh X.Y.Z` forces a version.

The root tag goes last and on its own because GitHub starts no workflow for a push of
more than three tags. It runs `release.yml`, which publishes the GitHub release, asks
proxy.golang.org to fetch the new versions and deploys the docs. Check from a directory outside the repo:

```bash
go get github.com/huynguyengl99/chanx-go@vX.Y.Z
```

A published version can never be changed. To withdraw a bad one, release the next patch
with a `retract vX.Y.Z` line in the affected `go.mod`.
