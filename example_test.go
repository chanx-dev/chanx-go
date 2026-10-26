package chanx_test

import (
	"fmt"

	chanx "go.chanx.dev/v2"
)

type GreetPayload struct {
	Name string `json:"name"`
}

// GreetMessage is what a client sends: {"action": "greet", "payload": {"name": "Ann"}}.
type GreetMessage struct {
	chanx.BaseMessage `action:"greet" description:"Ask for a greeting"`
	Payload           GreetPayload `json:"payload"`
}

// GreetingMessage is the reply: {"action": "greeting", "payload": "Hello, Ann"}.
type GreetingMessage struct {
	chanx.BaseMessage `action:"greeting"`
	Payload           string `json:"payload"`
}

// GreeterConsumer serves one route. Each exported method taking a *chanx.Context
// and a message is a handler, routed by the message's action.
type GreeterConsumer struct{}

func (GreeterConsumer) HandleGreet(ctx *chanx.Context, m GreetMessage) (GreetingMessage, error) {
	return GreetingMessage{Payload: "Hello, " + m.Payload.Name}, nil
}

func (GreeterConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

// A consumer is a struct of handlers; the engine routes frames to them, validates
// payloads and answers bad ones with an error frame. Mount the engine on any
// framework: chanxhttp.Mount(mux, engine), chanxgin.Mount(r, engine)...
func Example() {
	engine := chanx.New(chanx.Config{})
	route := engine.Handle("/ws/greeter", GreeterConsumer{}, chanx.Description("Greets people"))

	for _, h := range route.Info().Consumer.Handlers {
		fmt.Println(h.Action, "->", h.Outputs[0].Action)
	}
	// Output:
	// greet -> greeting
	// ping -> pong
}
