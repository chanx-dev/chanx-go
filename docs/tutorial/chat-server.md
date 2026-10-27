# Tutorial: a chat server

You will build a chat with rooms: users sign in with a token, see who comes and goes,
and post messages everyone in the room receives. Then you will test it, run it on several
processes over Redis, and generate a typed client.

It builds on the [quick start](/introduction/quick-start); reading that first helps.

## 1. The project

```bash
mkdir rooms && cd rooms
go mod init example.com/rooms
go get go.chanx.dev/v2 go.chanx.dev/layers/redis/v2
```

Everything below goes in `main.go`, and the tests in `main_test.go`.

## 2. Messages

Clients send `send`; the room receives `posted`, stamped with the author and the time.
The client never sends its own name: the server knows who it is.

<<< @/snippets/tutorial/main.go#messages

A second message tells the room when someone joins or leaves:

<<< @/snippets/tutorial/main.go#presence

## 3. The room consumer

<<< @/snippets/tutorial/main.go#consumer

`ctx.Conn.User` is what authentication returned, set in step 5. `RoomConsumer` holds a
map of users for now: a plain struct field, like any dependency, your database or user
service in real code. One consumer value serves every connection, so it holds no
per-connection state.

## 4. Presence

<<< @/snippets/tutorial/main.go#hooks

- **`OnConnect`** runs once the connection is in its room; `ExcludeSelf` keeps the
  newcomer out of their own announcement.
- **`OnDisconnect`** runs before the connection leaves its groups, so the room still
  hears the goodbye.
- **`Operations`** declares what `HandleSend` broadcasts, in its `Replies`. **`Emits`**
  declares the presence messages, which no handler sends. The AsyncAPI document lists
  both, and so do the clients generated from it.

## 5. Authentication

<<< @/snippets/tutorial/main.go#auth

`Authenticate` runs once per connection, before any handler. `auth.Bearer` reads the token
from the `Authorization` header or a `token` query parameter (browsers cannot set
headers on a WebSocket). What it returns becomes `ctx.Conn.User`.

On failure the client receives the authentication message, then the socket closes with
`4401`, so a client knows to stop reconnecting:

```json
{"action": "authentication", "payload": {"status_code": 401, "status_text": "Invalid token.", "data": null}}
```

`Credentials` documents where the token goes, as AsyncAPI security schemes.

## 6. Mount and run

<<< @/snippets/tutorial/main.go#main

```bash
go run .
npx wscat -c "ws://localhost:8000/ws/rooms/lobby?token=t-ann"
npx wscat -c "ws://localhost:8000/ws/rooms/lobby?token=t-bob"
```

Ann sees Bob arrive. Send from Bob:

```json
> {"action": "send", "payload": {"text": "hi ann"}}
< {"action": "posted", "payload": {"author": "bob", "text": "hi ann", "at": "2026-10-25T09:12:03.512Z"}}
```

## 7. Test it

`chanxtest` runs connections in memory, with no network and no ports:

<<< @/snippets/tutorial/main_test.go#test

```bash
go test ./...
```

`Connect` returns once the connection has joined its groups, so a broadcast sent right
after reaches it. By default the client is strict: a test fails if the server sends an
action nobody declared, which is how a missing `Replies` shows up. More in
[Testing](/guide/testing).

## 8. More than one process

One process keeps its groups in memory. Start Redis and point every process at it:

```bash
docker run -d -p 6379:6379 redis:7
go run . -redis redis://localhost:6379
go run . -redis redis://localhost:6379 -addr :8001
```

Connect Ann to `:8000` and Bob to `:8001`: they chat as before. The Redis layer is the
same format as Python's channels_redis, so a Django or FastAPI process on that Redis
joins the same rooms. See [Redis and Python](/layers/redis).

## 9. A typed client

The server documents itself at `/asyncapi.json`. Generate a TypeScript client from it:

```bash
npm install @chanx-js/client
npx @chanx-js/codegen http://localhost:8000/asyncapi.json -o src/generated
```

```ts
import { createClient } from '@chanx-js/client';
import { rooms } from './src/generated';

const client = createClient({ baseUrl: 'ws://localhost:8000' });
const room = client.connect(rooms, { params: { room: 'lobby' }, queryParams: { token: 't-ann' } });

room.on('posted', (m) => console.log(`${m.payload.author}: ${m.payload.text}`));
room.send({ action: 'send', payload: { text: 'hello' } });
```

Sending an action the server does not accept is a compile error. See
[Clients](/reference/clients) for Python and other options.

## Where to go next

- [Topics](/guide/topics): let one socket follow many rooms.
- [Groups and events](/guide/events): push from an HTTP handler or a worker.
- [Frameworks](/frameworks/): the same consumer on gin, echo or Fiber.
