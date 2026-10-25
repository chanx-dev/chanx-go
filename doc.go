// Package chanx builds structured WebSocket servers: typed messages routed to
// handlers by action, validated before they run, groups and topics across
// processes over a channel layer, and an AsyncAPI document generated from the
// handlers. It speaks the protocol of Python [chanx], so its clients, client
// generators and Redis layer work unchanged with a Go server.
//
// # Messages
//
// A message is a struct embedding [BaseMessage], with its action in the tag and an
// optional Payload field. The payload's JSON Schema is inferred from the struct, and
// a frame that does not match it gets an error frame instead of reaching a handler.
//
//	type ChatMessage struct {
//		chanx.BaseMessage `action:"chat" description:"Send a chat message"`
//		Payload           ChatPayload `json:"payload"`
//	}
//
// # Consumers
//
// A consumer serves one route. Its exported methods taking a [*Context] and a
// message are handlers; the return type is the reply, echoing the frame's ref.
// Optional hooks are matched by name: Authenticate, Groups, OnConnect,
// OnDisconnect, Emits, Topics and others. One consumer value serves every
// connection; per-connection state lives on [Conn].
//
//	func (c *ChatConsumer) HandleChat(ctx *chanx.Context, m ChatMessage) error {
//		return ctx.Broadcast(ChatNotification{Payload: m.Payload})
//	}
//
// [NewConsumer] and [On] build the same from functions.
//
// # Engine and frameworks
//
// [New] returns an [Engine], and [Engine.Handle] mounts consumers on path patterns
// such as "/ws/chat/{room}". The engine knows nothing about HTTP: an adapter
// registers its routes in a framework's router, where the framework's middleware
// runs before the upgrade. Adapters exist for net/http (package nethttp, also chi
// and gorilla/mux), gin, echo and Fiber.
//
// # Groups, events and topics
//
// [Context.Broadcast] sends to groups through the channel layer, reaching sockets
// held by every process sharing it, Python ones included. A [Publisher] does the
// same from outside a socket: an HTTP handler, a worker. Events are server to
// server messages handled by methods taking an [*EventContext]. Topics, structs
// embedding [BaseTopic], are streams a client subscribes to by name over one
// socket.
//
// # Testing
//
// Package chanxtest runs a connection in memory with
// [github.com/huynguyengl99/chanx-go/chanxtest.Connect] and records what was
// published with [github.com/huynguyengl99/chanx-go/chanxtest.Spy].
//
// [chanx]: https://github.com/huynguyengl99/chanx
package chanx
