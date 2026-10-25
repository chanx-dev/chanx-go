# Differences from Python

chanx-go matches Python chanx on the wire, and a test pins each behaviour against values
recorded from Python. These are the places it differs, each on purpose.

| Case | Python chanx | chanx-go |
| --- | --- | --- |
| A payload value of the wrong type that Pydantic's lax mode coerces (`"3"` for an int, `1` or `"true"` for a bool) | accepted, coerced | an `error` frame (`int_type`, `bool_type`...). Envelope ints (`version`, `seq`) stay lax on both |
| A text frame that is not JSON | `Invalid JSON: ` and Python's decoder message | `Invalid JSON: ` and Go's decoder message |
| `send_message_immediately` | yields after each send | not needed: each connection has a writer goroutine |
| A client that stops reading | left to the ASGI server | a bounded outbox; closed with 1013 after `WriteTimeout` |
| A huge frame | left to the ASGI server (uvicorn: 16 MiB) | `MaxFrameSize`, 16 MiB by default; closed with 1009 |
| Handler concurrency | one task per message | one goroutine per frame; `Sequential` and `MaxInFlight` to limit it |
| Redis queue layer (`RedisChannelLayer`) | available | not yet; the pub/sub layer only |
| Authentication before the upgrade | after the socket is accepted | the same by default; `RejectBeforeUpgrade` answers an HTTP 401 or 403 instead of upgrading |
| Disconnect hook | `disconnect()` runs after the groups are left | `OnDisconnect` runs before, so it can still broadcast to them |
| Groups of a mixin | a subclass's `groups` replaces its parent's | each embedded struct's `Groups` adds to the consumer's |
| Broadcasting from a topic handler without naming groups | the topic's own `groups`, empty unless set: nobody receives it | the connection's groups |
| Non-ASCII names | Unicode rules: `camelize("a_ß")` is `"aSS"`, a topic parameter may be `{nom_é}` | ASCII rules: `"aß"`, and the pattern is refused |
| AsyncAPI: two schemas with one name | prefixed with the consumer's name | prefixed with the package's name |
| AsyncAPI: channel description | the `description` option, else the class docstring | the `Description` option only: Go has no docstrings at run time |
| AsyncAPI: path parameter types | from the Django converter (`int`, `slug`...) | always `str`: Go routes have untyped parameters |
| AsyncAPI: servers and security schemes | in the order given | sorted by name, since they come from Go maps |

## Naming

| Python | Go |
| --- | --- |
| a message class `ChatMessage(BaseMessage)` | a struct embedding `chanx.BaseMessage` |
| `@ws_handler async def handle_chat` | a method `HandleChat(ctx, ChatMessage)` |
| `@event_handler` | a method taking `*chanx.EventContext` |
| `@channel(name=..., tags=...)` | `chanx.Name`, `chanx.Tags` route options |
| `authenticator_class` | an `Authenticate` method |
| `groups = [...]` | a `Groups(*Conn) []string` method |
| `self.broadcast_message(m)` | `ctx.Broadcast(m)` |
| `ChatConsumer.broadcast_event(e, groups)` | `publisher.BroadcastEvent(ctx, e, groups...)` |
| `WebsocketCommunicator` | `chanxtest.Connect` |
| `capture_broadcast_events` | `chanxtest.Spy` and `chanxtest.BroadcastEvents` |
| `CHANX` settings | `chanx.Config` |
