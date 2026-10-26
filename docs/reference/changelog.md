# Changelog

Every release is listed on
[GitHub releases](https://github.com/chanx-dev/chanx-go/releases), with its changes
grouped by type, and in
[CHANGELOG.md](https://github.com/chanx-dev/chanx-go/blob/main/CHANGELOG.md).

Every module of the repo is released together, at the same version: `chanx-go`,
`layers`, `layers/redis` and the three adapters. Require the same version of each:

```bash
go get go.chanx.dev@latest go.chanx.dev/adapters/gin@latest
```

While chanx-go is on 0.x, a minor version may change the API; a patch never does.
