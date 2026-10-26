package docsgen

import (
	"testing"

	chanx "go.chanx.dev"
	"go.chanx.dev/asyncapi"
)

type ChatConsumer struct{}

func (ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

func TestInfo(t *testing.T) {
	e := chanx.New(chanx.Config{})
	e.Handle("/ws/chat", ChatConsumer{})
	doc, err := asyncapi.Generate(e, Info)
	if err != nil {
		t.Fatal(err)
	}
	servers, _ := doc.Get("servers")
	if keys := servers.(*asyncapi.Map).Keys(); len(keys) != 2 || keys[0] != "production" {
		t.Fatal(keys)
	}
}
