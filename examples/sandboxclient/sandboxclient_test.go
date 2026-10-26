package sandboxclient_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	chanx "github.com/huynguyengl99/chanx-go"
	chanxclient "github.com/huynguyengl99/chanx-go/client"
	"github.com/huynguyengl99/chanx-go/codegen"
	"github.com/huynguyengl99/chanx-go/examples/sandbox"
	"github.com/huynguyengl99/chanx-go/examples/sandboxclient"
	chanxhttp "github.com/huynguyengl99/chanx-go/nethttp"
)

// The committed client is what chanx-codegen writes from Python's document.
func TestGeneratedFromPython(t *testing.T) {
	doc, err := os.ReadFile("../sandbox/testdata/python_asyncapi.json")
	if err != nil {
		t.Fatal(err)
	}
	files, err := codegen.Generate(doc, codegen.Options{Package: "sandboxclient"})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range files {
		got, err := os.ReadFile(name)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s is stale: run go generate ./sandboxclient", name)
		}
	}
}

func serve(t *testing.T) string {
	t.Helper()
	set, _ := sandbox.Layers("")
	e := sandbox.New(set, nil)
	mux := http.NewServeMux()
	chanxhttp.Mount(mux, e)
	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = e.Shutdown(ctx)
		srv.Close()
	})
	return srv.URL
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type receiver interface {
	Receive(context.Context) (chanx.Message, chanxclient.Envelope, error)
}

func receive(t *testing.T, r receiver) (chanx.Message, chanxclient.Envelope) {
	t.Helper()
	m, env, err := r.Receive(ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	return m, env
}

func TestSystem(t *testing.T) {
	c, err := sandboxclient.DialSystem(ctx(t), serve(t))
	check(t, err)
	defer func() { _ = c.Close() }()
	if m, _ := receive(t, c); m != (sandboxclient.SystemEchoMessage{Payload: sandboxclient.MessagePayload{Message: "🔧 System: Connection established!"}}) {
		t.Fatalf("%#v", m)
	}
	echo, err := c.RequestUserMessage(ctx(t), sandboxclient.UserMessage{Payload: sandboxclient.MessagePayload{Message: "hi"}})
	check(t, err)
	if echo.Payload.Message != "🔧 System Echo: hi" {
		t.Fatalf("%#v", echo)
	}
	_, err = c.RequestPing(ctx(t), sandboxclient.PingMessage{})
	check(t, err)
}

func TestChat(t *testing.T) {
	c, err := sandboxclient.DialChat(ctx(t), serve(t))
	check(t, err)
	defer func() { _ = c.Close() }()
	receive(t, c) // joined
	extra, err := c.RequestExtraRequest(ctx(t), sandboxclient.ExtraRequestMessage{Payload: "x"})
	check(t, err)
	if extra.Payload != "x any extra thing" {
		t.Fatalf("%#v", extra)
	}
	check(t, c.Send(ctx(t), sandboxclient.ChatMessage{Payload: sandboxclient.ChatPayload{Message: "hello"}}))
	if m, _ := receive(t, c); m != (sandboxclient.ChatNotificationMessage{Payload: sandboxclient.ChatPayload{Message: "💬 hello"}}) {
		t.Fatalf("%#v", m)
	}
}

func TestRoomChat(t *testing.T) {
	c, err := sandboxclient.DialRoomChat(ctx(t), serve(t), "lobby")
	check(t, err)
	defer func() { _ = c.Close() }()
	check(t, c.Send(ctx(t), sandboxclient.RoomChatMessage{Payload: sandboxclient.RoomMessagePayload{Message: "yo"}}))
	m, _ := receive(t, c)
	n, ok := m.(sandboxclient.RoomNotificationMessage)
	if !ok || n.Payload.Message != "💬 yo" || *n.Payload.RoomName != "lobby" || n.Payload.Status != sandboxclient.MessageStatusPending {
		t.Fatalf("%#v", m)
	}
}

func TestBackgroundJobs(t *testing.T) {
	c, err := sandboxclient.DialBackgroundJobs(ctx(t), serve(t))
	check(t, err)
	defer func() { _ = c.Close() }()
	receive(t, c) // connected
	status, err := c.RequestJob(ctx(t), sandboxclient.JobMessage{Payload: sandboxclient.JobPayload{Type: "translate", Content: "hola"}})
	check(t, err)
	if status.Payload["status"] != "queuing" {
		t.Fatalf("%#v", status)
	}
	for _, want := range []string{"queued", "result"} {
		m, _ := receive(t, c)
		if s, ok := m.(sandboxclient.JobStatusMessage); !ok || s.Payload["status"] != want {
			t.Fatalf("want %s: %#v", want, m)
		}
	}
}

func TestTopics(t *testing.T) {
	url := serve(t)
	hub, err := sandboxclient.DialTopicHub(ctx(t), url)
	check(t, err)
	defer func() { _ = hub.Close() }()
	room := hub.RoomTopic("lobby")
	check(t, room.Subscribe(ctx(t)))
	posted, err := room.RequestEcho(ctx(t), sandboxclient.EchoMessage{Payload: sandboxclient.PostPayload{Body: "e"}})
	check(t, err)
	if posted.Payload != (sandboxclient.PostPayload{Body: "e", Author: "anonymous"}) {
		t.Fatalf("server default not applied: %#v", posted)
	}
	presence := hub.PresenceTopic("ann")
	check(t, presence.Subscribe(ctx(t)))

	dedicated, err := sandboxclient.DialRoomTopic(ctx(t), url, "lobby")
	check(t, err)
	defer func() { _ = dedicated.Close() }()
	if m, env := receive(t, dedicated); m != (chanx.SubscribedMessage{}) || env.Topic != "room:lobby" {
		t.Fatalf("%#v %+v", m, env)
	}
	check(t, dedicated.Send(ctx(t), sandboxclient.PostMessage{Payload: sandboxclient.PostPayload{Body: "b"}}))
	want := sandboxclient.RoomEvent{Payload: sandboxclient.PostPayload{Body: "b", Author: "anonymous"}}
	for _, r := range []receiver{room, dedicated} {
		if m, env := receive(t, r); m != want || env.Topic != "room:lobby" {
			t.Fatalf("%#v %+v", m, env)
		}
	}
	check(t, presence.Unsubscribe(ctx(t)))
}
