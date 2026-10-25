# Authentication

A consumer with an `Authenticate` method checks each connection once, before any handler
runs. The client learns the result from an `authentication` message, and a failed
connection closes with a code that tells it not to retry.

## Authenticate

<<< @/snippets/authn/authn.go#authenticate

What `Authenticate` returns is the connection's user:

<<< @/snippets/authn/authn.go#user

### The result

On success, the first frame the client receives is:

```json
{"action": "authentication", "payload": {"status_code": 200, "status_text": "OK", "data": {"name": "ann"}}}
```

`data` is what `AuthenticationData` returns, `null` without it. On failure, the same
message with the status, then the socket closes with 4000 + the status:

| Returned error | Message | Close code |
| --- | --- | --- |
| `chanx.Unauthenticated("Invalid token.")` | `401`, `"Invalid token."` | 4401 |
| `chanx.Forbidden("This account is suspended.")` | `403`, `"This account is suspended."` | 4403 |
| `&chanx.AuthError{Status: 429, Detail: "Slow down"}` | `429`, `"Slow down"` | 4429 |
| any other error | `401`, `"Unauthorized"`, its text logged, never sent | 4401 |
| a panic | `500`, `"Internal Server Error"` | 4500 |

```json
{"action": "authentication", "payload": {"status_code": 401, "status_text": "Authentication credentials were not provided.", "data": null}}
```

A client seeing a close code from 4400 to 4599 knows retrying with the same credentials
will not help.

## Tokens

Browsers cannot set headers on a WebSocket. Package `auth` reads a bearer token from
wherever a client can put one:

| Function | Reads |
| --- | --- |
| `auth.Token(r)` | the first of the three below |
| `auth.BearerToken(r)` | `Authorization: Bearer <token>` |
| `auth.QueryToken(r, "token")` | `?token=<token>` |
| `auth.SubprotocolToken(r, "bearer")` | `Sec-WebSocket-Protocol: bearer, <token>` |

`auth.Bearer(r, verify)` reads the token and calls your `verify`: a missing token fails
with "Authentication credentials were not provided.", a `verify` error with "Invalid
token.", unless it is a `chanx.Forbidden` or another `*chanx.AuthError`.

## From framework middleware

If your framework's middleware already authenticates, take the user from it with the
`chanx.UserFrom` route option, and write no `Authenticate` at all:

<<< @/snippets/authn/authn.go#middleware

```go
engine.Handle("/ws/lobby", LobbyConsumer{}, chanx.UserFrom(UserKey{}))
mux.Handle("/ws/", RequireUser(chanxhttp.Dispatcher(engine)))
```

A missing value fails with 401. With gin and echo the key is what you passed to
`c.Set("user", u)`, with Fiber what you passed to `c.Locals`. Inside `Authenticate`,
`r.Value(key)` reads the same values.

## Rejecting before the upgrade

By default a failed connection is upgraded, told why, then closed: a browser can read
the reason. `chanx.Config{RejectBeforeUpgrade: true}`, or the `chanx.RejectBeforeUpgrade`
route option, runs `Authenticate` before the upgrade instead and answers a plain HTTP
status, 401 or 403, through the framework's error handling. Use it when unauthenticated
upgrades are a cost you do not want to pay.

## Documenting it

`Credentials` declares where a client puts its credentials. Each one becomes an AsyncAPI
security scheme, listed on the channel's operations, and query and header credentials go
in the channel's WebSocket binding:

```go
func (ChatConsumer) Credentials() []chanx.Credential {
	return []chanx.Credential{
		{Name: "token", Location: "query", Description: "Access token"},
		{Name: "sessionid", Location: "cookie"},
	}
}
```

`auth.BearerCredentials()` is the pair `auth.Token` reads: the `Authorization` header and
the `token` query parameter. See [AsyncAPI](/guide/asyncapi).

Next: [AsyncAPI](/guide/asyncapi).
