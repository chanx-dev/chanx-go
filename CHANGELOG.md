# Changelog

## 0.2.0 (2026-10-26)

### Documentation
- Add the chanx-go logo, family colours, social card and a demo video
- Lead the README with the logo and demo, and cover the Go client
- Rewrite the README around a runnable quick start and typed clients
- Check errors in the README's Go client example
- Add Dial and Generate examples, and cover mounting and clients in doc.go

### Features
- Move the module path to go.chanx.dev

### Maintenance
- Push the root release tag on its own
- Run the humanizer and diataxis checks in the docs skill

## 0.1.0 (2026-10-26)

### Documentation
- Add readme, guide and agents guide
- Add contributing guide
- Add testable examples and package overview
- Add documentation site
- Add Go client guide

### Features
- **protocol**: Add wire protocol helpers
- Add message schema inference and validation
- **transport**: Add connection interface and websocket transports
- Add consumers, topics and engine
- **chanxtest**: Add test client and layer spy
- **nethttp**: Add net/http adapter
- **asyncapi**: Add AsyncAPI 3.0 generator
- Add auth helpers and delta tracker
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
