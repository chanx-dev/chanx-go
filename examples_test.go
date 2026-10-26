package chanx_test

import (
	"context"
	"fmt"

	chanx "go.chanx.dev"
	"go.chanx.dev/layers"
	"go.chanx.dev/layers/inmemory"
)

func ExampleEngine_Handle() {
	engine := chanx.New(chanx.Config{})
	route := engine.Handle("/ws/greeter/{room}", GreeterConsumer{},
		chanx.Name("greeter"),      // the AsyncAPI channel name
		chanx.Tags("greetings"),    // AsyncAPI tags
		chanx.SendCompletion(true), // a complete frame after each handled frame
		chanx.Sequential(),         // handle this route's frames one at a time
	)
	info := route.Info()
	fmt.Println(info.Name, info.Tags, info.ParamNames)
	// Output: greeter [greetings] [room]
}

// Without methods, NewConsumer and On register functions; the message types are
// inferred from their signatures.
func ExampleNewConsumer() {
	greeter := chanx.NewConsumer("greeter")
	chanx.On(greeter, func(ctx *chanx.Context, m GreetMessage) (GreetingMessage, error) {
		return GreetingMessage{Payload: "Hi " + m.Payload.Name}, nil
	})
	greeter.Groups = func(c *chanx.Conn) []string { return []string{"room." + c.Param("room")} }

	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/greeter/{room}", greeter)
	fmt.Println(chanx.Check(greeter))
	// Output: <nil>
}

type brokenConsumer struct{}

// The reply type must be a message: this handler returns a string.
func (brokenConsumer) HandleGreet(ctx *chanx.Context, m GreetMessage) (string, error) {
	return "", nil
}

// Check reports a consumer's definition mistakes without starting anything, the
// same ones Handle panics on: a one-line unit test.
func ExampleCheck() {
	fmt.Println(chanx.Check(GreeterConsumer{}))
	fmt.Println(chanx.Check(brokenConsumer{}))
	// Output:
	// <nil>
	// chanx: chanx_test.brokenConsumer.HandleGreet: want func(*chanx.Context | *chanx.EventContext, SomeMessage) error or (Reply, error), where Reply is a message type or chanx.Message
}

type RoomConsumer struct{}

// Groups puts every connection in its room's group.
func (RoomConsumer) Groups(c *chanx.Conn) []string { return []string{"room." + c.Param("room")} }

// HandleGreet answers the sender and tells everyone else in the room.
func (RoomConsumer) HandleGreet(ctx *chanx.Context, m GreetMessage) error {
	return ctx.Broadcast(GreetingMessage{Payload: m.Payload.Name + " says hi"}, chanx.ExcludeSelf())
}

// Broadcast sends to the connection's groups through the channel layer, so the
// message reaches connections held by every process sharing it.
func ExampleContext_Broadcast() {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/room/{room}", RoomConsumer{})
	fmt.Println(chanx.Check(RoomConsumer{}))
	// Output: <nil>
}

type NewsPayload struct {
	Text string `json:"text"`
}

// NewsEvent is a server to server event: it travels through the layer, and a
// consumer's event handler decides what its clients see.
type NewsEvent struct {
	chanx.BaseMessage `action:"news"`
	Payload           NewsPayload `json:"payload"`
}

// A Publisher pushes to sockets from outside one: an HTTP handler, a worker, a
// cron job. It needs the layers only, not the engine.
func ExamplePublisher_BroadcastEvent() {
	ctx := context.Background()
	set := layers.Set{"default": inmemory.New()}

	// Stand in for a connection in the "room.lobby" group.
	layer, _ := set.Get("default")
	channel, _ := layer.NewChannel(ctx, "specific.")
	_ = layer.GroupAdd(ctx, "room.lobby", channel)

	publisher := chanx.NewPublisher(set)
	_ = publisher.BroadcastEvent(ctx, NewsEvent{Payload: NewsPayload{Text: "deploy at 5"}}, "room.lobby")

	msg, _ := layer.Receive(ctx, channel)
	fmt.Println(msg["type"], msg["event_data"])
	// Output: handle_channel_event map[action:news payload:map[text:deploy at 5]]
}

type AuthedConsumer struct{}

// Authenticate runs once per connection, before any handler. An error closes the
// connection after the authentication message: 4401 for Unauthenticated, 4403 for
// Forbidden.
func (AuthedConsumer) Authenticate(ctx *chanx.Context, r *chanx.Request) (any, error) {
	switch r.Query.Get("token") {
	case "":
		return nil, chanx.Unauthenticated("Authentication credentials were not provided.")
	case "banned":
		return nil, chanx.Forbidden("Banned")
	}
	return "user:" + r.Query.Get("token"), nil // the connection's User
}

func (AuthedConsumer) HandleGreet(ctx *chanx.Context, m GreetMessage) (GreetingMessage, error) {
	return GreetingMessage{Payload: fmt.Sprint("Hello, ", ctx.Conn.User)}, nil
}

func ExampleUnauthenticated() {
	engine := chanx.New(chanx.Config{})
	route := engine.Handle("/ws/greeter", AuthedConsumer{})
	fmt.Println(route.Authenticates())
	// Output: true
}

// UserFrom takes the user from framework middleware that ran before the upgrade
// (gin's c.Set, Fiber's c.Locals, a context value), for consumers without an
// Authenticate method.
func ExampleUserFrom() {
	engine := chanx.New(chanx.Config{})
	route := engine.Handle("/ws/greeter", GreeterConsumer{}, chanx.UserFrom("user"))
	fmt.Println(route.Authenticates())
	// Output: true
}

type visits struct{}

type CounterConsumer struct{}

// One consumer value serves every connection, so per-connection state lives on
// the connection.
func (CounterConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	n, _ := chanx.ConnValue[int](ctx.Conn, visits{})
	ctx.Conn.SetValue(visits{}, n+1)
	return chanx.PongMessage{}, nil
}

func ExampleConnValue() {
	fmt.Println(chanx.Check(CounterConsumer{}))
	// Output: <nil>
}

type ChatRoomTopic struct {
	chanx.BaseTopic `pattern:"room:{room_name}" description:"A chat room"`
}

// Authorize decides who may subscribe.
func (ChatRoomTopic) Authorize(ctx *chanx.Context, p chanx.Params) (bool, error) {
	return p["room_name"] != "staff", nil
}

// HandleGreet runs for frames sent to the topic, and broadcasts to its subscribers.
func (ChatRoomTopic) HandleGreet(ctx *chanx.Context, m GreetMessage) error {
	return ctx.BroadcastTopic(GreetingMessage{Payload: m.Payload.Name + " joined " + ctx.Topic})
}

type HubConsumer struct{}

// Topics lets clients subscribe to many rooms over one socket.
func (HubConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{ChatRoomTopic{}} }

// A topic is a stream a client subscribes to by name ("room:lobby"). A consumer
// hosts topics, or a topic gets a route of its own with TopicConsumer.
func ExampleTopicConsumer() {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/hub", HubConsumer{})                                     // {"action": "subscribe", "topic": "room:lobby"}
	engine.Handle("/ws/room/{room_name}", chanx.TopicConsumer(ChatRoomTopic{})) // subscribed on connect

	for _, r := range engine.Routes() {
		info := r.Info()
		if info.Standalone != nil {
			fmt.Println(info.Path, "serves", info.Standalone.Pattern)
		} else {
			fmt.Println(info.Path, "hosts", info.Topics[0].Pattern)
		}
	}
	// Output:
	// /ws/hub hosts room:{room_name}
	// /ws/room/{room_name} serves room:{room_name}
}

func ExampleFormatTopic() {
	topic, _ := chanx.FormatTopic[ChatRoomTopic](chanx.Params{"room_name": "lobby"})
	group, _ := chanx.TopicGroup[ChatRoomTopic](topic)
	fmt.Println(topic, group)
	// Output: room:lobby ChatRoomTopic.room-lobby
}

type LobbyTopic struct {
	chanx.BaseTopic `pattern:"lobby:{floor}"`
}

// BroadcastTopic publishes to a topic's subscribers from outside a socket; Seq
// stamps the frame for gap detection.
func ExampleBroadcastTopic() {
	engine := chanx.New(chanx.Config{})
	topic, _ := chanx.FormatTopic[LobbyTopic](chanx.Params{"floor": "3"})
	err := chanx.BroadcastTopic[LobbyTopic](context.Background(), engine.Publisher(), topic,
		GreetingMessage{Payload: "doors open"}, chanx.Seq(1))
	fmt.Println(err)
	// Output: <nil>
}

type ProfilePayload struct {
	Name     string `json:"name"`
	Language string `json:"language" default:"en"`
}

// ApplyDefaults fills `default` tags, for values decoded outside chanx (an HTTP
// body, a queue job) that should match what a frame would get.
func ExampleApplyDefaults() {
	var p ProfilePayload
	chanx.ApplyDefaults(&p)
	fmt.Printf("%+v\n", p)
	// Output: {Name: Language:en}
}
