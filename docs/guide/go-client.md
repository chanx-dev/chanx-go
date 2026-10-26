# Go client

`chanx-codegen` reads a chanx server's AsyncAPI document and writes a typed Go client for
it: one struct per message, one client per route, one method per request. It works
against a Go server or a Python one, since both publish the same document. It is the Go
counterpart of chanx's `chanx generate-client` and of `@chanx-js/codegen`.

Use it for Go code that talks to a chanx server: a CLI, a bot, a load test, a service
reading a Python backend. A Go client in the same codebase as a Go server can skip it and
dial with the server's own message types.

## Generate

```bash
go run go.chanx.dev/cmd/chanx-codegen@latest -o ./chatclient http://localhost:8000/asyncapi.json
```

```text
wrote chatclient/channels.go
wrote chatclient/messages.go
```

The source is a URL or a file, JSON or YAML. The package is named after the output
directory, or set it with `-package`. Keep the command in a `//go:generate` line next to
the code that uses the client, and regenerate when the server's messages change.

## Use

The server here mounts a chat consumer on `/ws/chat/{room}`: `join` is answered with a
`welcome`, `chat` is broadcast to the room.

<<< @/snippets/goclient/client_test.go#dial

`DialChat` takes the route's path parameters as arguments. `RequestJoin` sends the
message with a fresh `ref` and returns the frame that echoes it, already a
`WelcomeMessage`.

Everything else arrives through `Receive`, as a value you switch on:

<<< @/snippets/goclient/client_test.go#receive

`Send` only compiles for messages the route accepts: `chat.Send(ctx, chatclient.WelcomeMessage{})`
is a type error. A frame whose action the document does not list arrives as a
`chanxclient.RawMessage` with the raw payload. Framework frames arrive as chanx's own
types: `chanx.AuthenticationMessage`, `chanx.ErrorMessage`, `chanx.CompleteMessage`.

## What gets generated

| From the document | In Go |
| --- | --- |
| a message schema | a struct embedding `chanx.BaseMessage`, its action in the tag, and its `Payload` |
| another schema | a struct, a named type, or a string type with constants for an `enum` |
| a route, such as `chat` | `ChatChannel`, `ChatClient` and `DialChat(ctx, baseURL, room)` |
| what the route accepts | `ChatToServer`, the interface `Send` takes |
| a request with a reply | `RequestJoin(ctx, JoinMessage) (WelcomeMessage, error)`; with several replies it returns `chanx.Message` |
| a topic hosted on the route | `chat.NewsTopic(tag)`, a handle with `Subscribe`, `Send`, `Receive` and its requests |

JSON Schema types map to Go: `integer` is `int`, `number` is `float64`, a
nullable field is a pointer, an object without properties is a map. Optional fields are
`omitzero`, so a field left at its zero value is not sent and the server applies its
default. Two generated names that would clash stop the generator, which names both.

## Requests

A request returns the first frame that echoes its `ref`. Handlers that return a reply
echo it. Handlers that broadcast do not, so for those use `Send` and read the result
with `Receive`, as `chat` does above.

An error frame answering a request comes back as a `*chanxclient.RequestError`:

<<< @/snippets/goclient/client_test.go#errors

A connection the server closes with a code other than 1000 ends with a
`*chanxclient.CloseError`: 4401 and 4403 mean authentication failed. Send credentials
with `chanxclient.Header` or `chanxclient.Query`:

```go
chat, err := chatclient.DialChat(ctx, url, "lobby",
	chanxclient.Header(http.Header{"Authorization": {"Bearer " + token}}))
```

## Topics

<<< @/snippets/goclient/client_test.go#topic

A subscribed topic's frames reach its handle, not the connection's `Receive`, and
`Envelope.Seq` orders its broadcasts. A refused subscription is a `*RequestError`
carrying the `reason`: `unknown_topic`, `unauthorized`. A dedicated topic route, mounted
with `chanx.TopicConsumer`, gets its own client: dialing it subscribes.

## Without the generator

`chanxclient.Dial` takes a `chanxclient.Channel`: an address and the messages to decode.
Any struct embedding `chanx.BaseMessage` works, so a Go server's own types do:

<<< @/snippets/goclient/client_test.go#bare

## Limits

- There is no reconnect yet: when the connection ends, dial again and resubscribe.
- Received frames are decoded, not validated against the schema.
- Frames wait in memory until you call `Receive`, so read them even when you only make
  requests.

Next: [Clients](/reference/clients) lists the TypeScript and Python clients for the same
server.
