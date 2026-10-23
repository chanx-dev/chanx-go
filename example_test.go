package chanx_test

import (
	"context"
	"fmt"

	chanx "github.com/huynguyengl99/chanx-go"
)

type GreetPayload struct {
	Name string `json:"name"`
}

type GreetMessage struct {
	chanx.BaseMessage `action:"greet" description:"Ask for a greeting"`
	Payload           GreetPayload `json:"payload"`
}

type GreetingMessage struct {
	chanx.BaseMessage `action:"greeting"`
	Payload           string `json:"payload"`
}

// GreeterConsumer answers greet with greeting; the reply echoes the frame's ref.
type GreeterConsumer struct{}

func (GreeterConsumer) HandleGreet(ctx *chanx.Context, m GreetMessage) (GreetingMessage, error) {
	return GreetingMessage{Payload: "Hello, " + m.Payload.Name}, nil
}

func Example() {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/greeter", GreeterConsumer{}, chanx.Description("Greets people"))
	// Mount it: chanxhttp.Mount(mux, engine), chanxgin.Mount(r, engine)...
	fmt.Println(engine.Routes()[0].Path)
	// Output: /ws/greeter
}

func ExampleOn() {
	hub := chanx.NewConsumer("hub")
	chanx.On(hub, func(ctx *chanx.Context, m GreetMessage) (GreetingMessage, error) {
		return GreetingMessage{Payload: "Hi " + m.Payload.Name}, nil
	})
	fmt.Println(chanx.Check(hub))
	// Output: <nil>
}

type LobbyTopic struct {
	chanx.BaseTopic `pattern:"lobby:{floor}"`
}

func ExampleBroadcastTopic() {
	engine := chanx.New(chanx.Config{})
	topic, _ := chanx.FormatTopic[LobbyTopic](chanx.Params{"floor": "3"})
	err := chanx.BroadcastTopic[LobbyTopic](context.Background(), engine.Publisher(), topic,
		GreetingMessage{Payload: "doors open"}, chanx.Seq(1))
	group, _ := chanx.TopicGroup[LobbyTopic](topic)
	fmt.Println(topic, group, err)
	// Output: lobby:3 LobbyTopic.lobby-3 <nil>
}
