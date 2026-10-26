package chanxclient_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"

	chanx "github.com/huynguyengl99/chanx-go"
	chanxclient "github.com/huynguyengl99/chanx-go/client"
	chanxhttp "github.com/huynguyengl99/chanx-go/nethttp"
)

type JoinMessage struct {
	chanx.BaseMessage `action:"join"`
	Payload           JoinPayload `json:"payload"`
}

type JoinPayload struct {
	Name string `json:"name"`
}

type WelcomeMessage struct {
	chanx.BaseMessage `action:"welcome"`
	Payload           string `json:"payload"`
}

type LobbyConsumer struct{}

func (LobbyConsumer) HandleJoin(ctx *chanx.Context, m JoinMessage) (WelcomeMessage, error) {
	return WelcomeMessage{Payload: "welcome to " + ctx.Conn.Param("room") + ", " + m.Payload.Name}, nil
}

// Dial takes any struct embedding chanx.BaseMessage, so a Go server's own message
// types work without generating a client.
func ExampleDial() {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/chat/{room}", LobbyConsumer{})
	mux := http.NewServeMux()
	chanxhttp.Mount(mux, engine)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")

	ctx := context.Background()
	conn, err := chanxclient.Dial(ctx, url, chanxclient.Channel{
		Address:  "/ws/chat/{room}",
		Incoming: []chanx.Message{WelcomeMessage{}},
	}, chanxclient.Params(map[string]string{"room": "lobby"}))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	welcome, err := chanxclient.Reply[WelcomeMessage](conn.Request(ctx, JoinMessage{Payload: JoinPayload{Name: "ann"}}))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(welcome.Payload)
	// Output: welcome to lobby, ann
}
