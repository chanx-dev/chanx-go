# Messages and validation

A message is a Go struct. Its tag names the action, and its `Payload` field is the body.
chanx-go infers a JSON Schema from the struct, checks every incoming frame against it, and
puts the same schema in the AsyncAPI document. What is documented is what is enforced.

## Declaring a message

<<< @/snippets/messages/messages.go#basic

| Tag on `BaseMessage` | Meaning |
| --- | --- |
| `action` | the action, required: frames are routed by it |
| `description` | the message's description, and its operation's, in AsyncAPI |
| `title` | the AsyncAPI name; the Go type name by default |

The type name is the message's name in AsyncAPI and in generated clients, as Python uses
the class name: name it `PostMessage`, not `post`.

## Fields

Payload fields follow `encoding/json`, plus a `default` tag:

| Python (Pydantic) | Go |
| --- | --- |
| `body: str` (required) | `` Body string `json:"body"` `` |
| `author: str = "anonymous"` | `` Author string `json:"author" default:"anonymous"` `` |
| `tags: list[str] = []` | `` Tags []string `json:"tags,omitempty"` `` |
| `reply_to: str \| None = None` | `` Reply *string `json:"reply_to,omitempty"` `` |
| `data: Any = None` | `` Data any `json:"data,omitempty"` `` |
| no payload | no `Payload` field: the payload is `null` |

A field is required unless it has `omitempty`, `omitzero` or a `default`. A default is
filled before your handler runs:

```json
> {"action": "post", "payload": {"body": "hello"}}
< {"action": "post", "payload": {"body": "hello", "author": "anonymous"}}
```

## What a client gets for a bad frame

The frame never reaches the handler. The client receives an `error` frame listing every
problem, in Pydantic's format, with `loc` starting at the action:

```json
> {"action": "post", "payload": {"body": 42}}
< {"action": "error", "payload": [{"type": "string_type", "loc": ["post", "payload", "body"], "msg": "Input should be a valid string"}]}
```

An action nobody handles:

```json
> {"action": "dance"}
< {"action": "error", "payload": [{"type": "union_tag_invalid", "loc": [], "msg": "Input tag 'dance' found using 'action' does not match any of the expected tags: 'mood', 'post', 'signup', 'typing'"}]}
```

A text frame that is not JSON gets a `json_invalid` error, and a binary frame too; the
connection stays open.

::: tip Strict types
Payloads are validated strictly: `"3"` is not an int and `1` is not a bool. Pydantic's
lax mode would coerce them; chanx-go answers `int_type` or `bool_type`. Envelope fields
(`version`, `seq`) stay lax on both. See [Differences from Python](/reference/differences).
:::

## Enums

A named string type with a `JSONSchema` method that sets `Enum`, Pydantic's `StrEnum`:

<<< @/snippets/messages/messages.go#enum

```json
> {"action": "mood", "payload": {"mood": "angry"}}
< {"action": "error", "payload": [{"type": "enum", "loc": ["mood", "payload", "mood"], "msg": "Input should be 'happy' or 'sad'"}]}
```

## Constraints and custom checks

Two methods on the payload type cover what tags cannot say:

<<< @/snippets/messages/messages.go#constraints

- **`JSONSchema(*jsonschema.Schema)`** edits the inferred schema: lengths, ranges,
  patterns, formats. It is Pydantic's `Field(min_length=3)`, and it shows in AsyncAPI.
  The schema type is [google/jsonschema-go](https://pkg.go.dev/github.com/google/jsonschema-go/jsonschema)'s.
- **`Validate() error`** runs after the schema passed, with the decoded value: Pydantic's
  `@field_validator`. Its error becomes a `value_error`.

```json
> {"action": "signup", "payload": {"username": "al", "password": "x"}}
< {"action": "error", "payload": [{"type": "string_too_short", "loc": ["signup", "payload", "username"], "msg": "String should have at least 3 characters"}]}

> {"action": "signup", "payload": {"username": "ann", "password": "ann123"}}
< {"action": "error", "payload": [{"type": "value_error", "loc": ["signup", "payload"], "msg": "Value error, password must not contain the username"}]}
```

## Built-in messages

| Type | Action | Use |
| --- | --- | --- |
| `chanx.PingMessage`, `chanx.PongMessage` | `ping`, `pong` | a health check: handle ping, reply pong |
| `chanx.ErrorMessage` | `error` | validation and handler errors |
| `chanx.AuthenticationMessage` | `authentication` | the result of `Authenticate` |
| `chanx.CompleteMessage` and others | `complete`... | completion frames, with `SendCompletion` |
| `chanx.SubscribeMessage` and others | `subscribe`... | topic subscriptions |

## Outside a frame

A value decoded elsewhere (an HTTP body, a queue job) does not get its defaults by
itself. Call `chanx.ApplyDefaults(&v)` before decoding into it.

Next: [Consumers](/guide/consumers).
