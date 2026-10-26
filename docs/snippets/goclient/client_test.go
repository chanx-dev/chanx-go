package goclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.chanx.dev/docs/snippets/goclient"
	"go.chanx.dev/docs/snippets/goclient/chatclient"
	chanx "go.chanx.dev/v2"
	chanxclient "go.chanx.dev/v2/client"
	chanxhttp "go.chanx.dev/v2/nethttp"
)

func serve(t *testing.T) string {
	mux := http.NewServeMux()
	chanxhttp.Mount(mux, goclient.NewEngine())
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return "ws" + srv.URL[len("http"):]
}

func TestClient(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := serve(t)

	// #region dial
	chat, err := chatclient.DialChat(ctx, url, "lobby")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = chat.Close() }()

	welcome, err := chat.RequestJoin(ctx, chatclient.JoinMessage{Payload: chatclient.JoinPayload{Name: "ann"}})
	if err != nil {
		t.Fatal(err)
	}
	// welcome.Payload: {Room: "lobby", Greeter: "bot"}
	// #endregion dial
	if welcome.Payload != (chatclient.WelcomePayload{Room: "lobby", Greeter: "bot"}) {
		t.Fatalf("%+v", welcome)
	}

	// #region receive
	err = chat.Send(ctx, chatclient.ChatMessage{Payload: chatclient.ChatPayload{Message: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	msg, _, err := chat.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	switch m := msg.(type) {
	case chatclient.ChatNotification:
		t.Log(m.Payload.Message) // hi
	case chanxclient.RawMessage:
		t.Fatalf("an action the document does not list: %s", m.Action)
	}
	// #endregion receive
	if msg != (chatclient.ChatNotification{Payload: chatclient.ChatPayload{Message: "hi"}}) {
		t.Fatalf("%#v", msg)
	}

	// #region topic
	news := chat.NewsTopic("go")
	if err := news.Subscribe(ctx); err != nil {
		t.Fatal(err)
	}
	// #endregion topic
	if err := news.Unsubscribe(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	chat, err := chatclient.DialChat(ctx, serve(t), "lobby")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = chat.Close() }()

	// #region errors
	_, err = chat.RequestJoin(ctx, chatclient.JoinMessage{}) // the handler refuses an empty name
	var refused *chanxclient.RequestError
	if errors.As(err, &refused) {
		t.Log(string(refused.Payload)) // {"detail":"Failed to process message for join"}
	}
	// #endregion errors
	if refused == nil || string(refused.Payload) != `{"detail":"Failed to process message for join"}` {
		t.Fatalf("%v", err)
	}

	_, _, err = chat.Conn().Request(ctx, chanx.PingMessage{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWithoutGenerator(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := serve(t)

	// #region bare
	conn, err := chanxclient.Dial(ctx, url, chanxclient.Channel{
		Address:  "/ws/chat/{room}",
		Incoming: []chanx.Message{goclient.ChatNotification{}, goclient.WelcomeMessage{}},
	}, chanxclient.Params(map[string]string{"room": "lobby"}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	reply, _, err := conn.Request(ctx, goclient.JoinMessage{Payload: goclient.JoinPayload{Name: "ann"}})
	// reply: goclient.WelcomeMessage{Payload: {Room: "lobby", Greeter: "bot"}}
	// #endregion bare
	if err != nil || reply != (goclient.WelcomeMessage{Payload: goclient.WelcomePayload{Room: "lobby", Greeter: "bot"}}) {
		t.Fatalf("%#v %v", reply, err)
	}
}
