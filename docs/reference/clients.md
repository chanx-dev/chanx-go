# Clients

chanx-go is a server. Clients come from the chanx ecosystem, and work with a Go server as
they do with a Python or Node one: same frames, same AsyncAPI document.

## TypeScript and JavaScript

[chanx-js](https://huynguyengl99.github.io/chanx-js/) has a client for the browser and
Node, with React, Vue, Svelte and Solid bindings, and a generator that turns the server's
AsyncAPI document into typed channels:

```bash
npm install @chanx-js/client
npx @chanx-js/codegen http://localhost:8000/asyncapi.json -o src/generated
```

```ts
import { createClient } from '@chanx-js/client';
import { chat } from './src/generated';

const client = createClient({ baseUrl: 'ws://localhost:8000' });
const room = client.connect(chat, { params: { room: 'lobby' } });

room.on('chat_notification', (m) => console.log(m.payload.text));
const pong = await room.request({ action: 'ping' }); // resolved by its ref
room.send({ action: 'chat', payload: { text: 'hi' } }); // checked against the schema
```

The generated names come from the document: the channel is the route's `chanx.Name`, the
message types are your Go type names. Regenerate when the server's messages change; a
CI step comparing `go run . -asyncapi` with a committed copy tells you when.

## Python

chanx generates a typed Python client from the same document:

```bash
pip install "chanx[cli]"
chanx generate-client --schema http://localhost:8000/asyncapi.json --output ./chat_client
```

## Anything else

Any WebSocket client works: the protocol is JSON frames with an `action`, a `payload` and
an optional `ref`. A client in another language reads `/asyncapi.json` for the messages
and their schemas, and handles three framework frames:

| Frame | Means |
| --- | --- |
| `authentication` | the result of authentication, first on a route that authenticates |
| `error` | a list of validation issues, or `{"detail": ...}`, or a topic refusal with a `reason` |
| `subscribed`, `unsubscribed` | topic confirmations |

Close codes from 4400 to 4599 mean authentication failed: do not reconnect with the same
credentials.
