# AsyncAPI

chanx-go builds an [AsyncAPI 3.0](https://www.asyncapi.com/docs/reference/specification/v3.0.0)
document from the consumers you mounted: every route is a channel, every handler an
operation, every message a schema. Because it comes from the code that serves traffic,
it cannot drift, and typed clients generated from it always match the server.

## Serving it

Each adapter has a `MountDocs`, serving three routes:

<<< @/snippets/docsgen/docsgen.go#mount

| Route | Serves |
| --- | --- |
| `/asyncapi` | an interactive page, the AsyncAPI React viewer |
| `/asyncapi.json` | the document as JSON, for client generators |
| `/asyncapi.yaml` | the same as YAML |

Without `MountDocs`, `asyncapi.HTMLHandler`, `JSONHandler` and `YAMLHandler` are plain
`http.Handler`s, and `asyncapi.Generate` returns the document itself.

## What goes in it

| From your code | In the document |
| --- | --- |
| a route, `/ws/chat/{room}` | a channel, with its address and path parameters |
| `chanx.Name`, `chanx.Description`, `chanx.Tags` | the channel's name, description and tags |
| a handler | a `receive` operation, named after the method in snake case |
| its return type, or `Operations` `Replies` | the operation's reply |
| an event handler, `Emits`, `Passthrough` | messages the channel sends |
| a hosted topic | a channel of its own, with an `x-topic` entry for client generators |
| a message struct | a message and its payload schema, named after the type |
| `Credentials` | security schemes, operation security, WebSocket bindings |

Request and reply messages carry a `correlationId` pointing at the frame's `ref`.

## Metadata and servers

<<< @/snippets/docsgen/docsgen.go#info

| `asyncapi.Info` field | Default |
| --- | --- |
| `Title`, `Version`, `Description` | `AsyncAPI Documentation`, `1.0.0`, none |
| `ServerURL`, `ServerProtocol` | the request's host, `ws` or `wss` from the request |
| `Servers` | replaces `ServerURL` and `ServerProtocol` with named AsyncAPI Server Objects |
| `SecuritySchemes` | added to those from `Credentials`; a name declared here wins |
| `Camelize` | camelCase names and properties, for a server with `Config.Camelize` |

## Writing it to a file

For CI, or to generate clients without running the server:

<<< @/snippets/docsgen/docsgen.go#flag

```bash
go run . -asyncapi > asyncapi.json
npx @chanx-js/codegen asyncapi.json -o src/generated
```

A test comparing the output with a committed copy catches an API change in review.

## The docs page and your security policy

The page loads `@asyncapi/react-component` 3.2.2 from unpkg, pinned with SRI hashes. To
load nothing from a CDN, serve the two files yourself:

<<< @/snippets/docsgen/docsgen.go#selfhost

A custom URL drops the pinned hash; set `DocsJSIntegrity` and `DocsCSSIntegrity` to keep
the check.

Under a Content-Security-Policy, the page needs the asset origin in `script-src` and
`style-src`, `'unsafe-inline'` for its inline script, and `'unsafe-eval'`, because the
component compiles code at runtime. Apply that policy to the docs route only, or keep
the docs out of production.

Next: [Configuration](/guide/configuration).
