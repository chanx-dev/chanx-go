# Changelog
## 0.1.0 (2026-10-26)

### Documentation
- Add readme, guide and agents guide
- Add contributing guide
- Add testable examples and package overview
- Add documentation site
- Add Go client guide

### Features
- **layers**: Add channel layer interface, in-memory layer and serializers
- **layers/redis**: Add Redis pub/sub layer
- **protocol**: Add wire protocol helpers
- Add message schema inference and validation
- **transport**: Add connection interface and websocket transports
- Add consumers, topics and engine
- **chanxtest**: Add test client and layer spy
- **nethttp**: Add net/http adapter
- **asyncapi**: Add AsyncAPI 3.0 generator
- Add auth helpers and delta tracker
- **adapters/gin**: Add gin adapter
- **adapters/echo**: Add echo v5 adapter
- **adapters/fiber**: Add fiber v3 adapter
- **examples**: Add sandbox example
- **client**: Add Go client for chanx servers
- **codegen**: Generate typed Go clients from AsyncAPI documents

### Maintenance
- Initialize chanx-go project
- Add workflows and release script
- Compute release version from commit history
- Test each module alone on the latest go patch
- **examples**: Require go 1.26 for patched dependencies
- Run govulncheck on the latest go only
- Add docs writing skill
- **examples**: Drive a generated client against the sandbox
