package chanxtest_test

import (
	"testing"

	"go.chanx.dev/layers/v2"
	"go.chanx.dev/layers/v2/inmemory"
	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"
)

type JoinedPayload struct {
	Name string `json:"name"`
}

// UserJoined is an event other processes react to, published when someone connects.
type UserJoined struct {
	chanx.BaseMessage `action:"user_joined"`
	Payload           JoinedPayload `json:"payload"`
}

type LobbyConsumer struct{}

func (LobbyConsumer) Groups(*chanx.Conn) []string { return []string{"lobby"} }

func (LobbyConsumer) OnConnect(ctx *chanx.Context) error {
	name := ctx.Conn.Request.Query.Get("name")
	return ctx.Publisher().BroadcastEvent(ctx, UserJoined{Payload: JoinedPayload{Name: name}}, "lobby")
}

func (LobbyConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

// testJoinAnnouncement asserts on what was published rather than on what a second
// client received: wrap the layer in a spy, then read the events back typed.
func testJoinAnnouncement(t *testing.T) {
	spy := chanxtest.Spy(inmemory.New(), chanxtest.Suppress()) // record, do not deliver
	engine := chanx.New(chanx.Config{Layers: layers.Set{"default": spy}})
	engine.Handle("/ws/lobby", LobbyConsumer{})

	chanxtest.Connect(t, engine, "/ws/lobby?name=ann")

	joined := chanxtest.BroadcastEvents[UserJoined](spy)
	if len(joined) != 1 || joined[0].Event.Payload.Name != "ann" || joined[0].Group != "lobby" {
		t.Fatalf("published %+v", joined)
	}
}

func ExampleSpy() {
	_ = testJoinAnnouncement
}
