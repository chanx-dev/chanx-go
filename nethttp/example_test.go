package chanxhttp_test

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/asyncapi"
	chanxhttp "go.chanx.dev/v2/nethttp"
	"go.chanx.dev/v2/transport/coderws"
)

type ChatConsumer struct{}

func (ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

// Mount registers every route on a ServeMux as "GET /ws/chat/{room}"; the
// parameter reaches the consumer through r.PathValue.
func ExampleMount() {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/chat/{room}", ChatConsumer{})

	mux := http.NewServeMux()
	chanxhttp.Mount(mux, engine, chanxhttp.AcceptOptions(&coderws.Options{
		OriginPatterns: []string{"app.example.com"},
	}))
	chanxhttp.MountDocs(mux, engine, "/asyncapi", asyncapi.Info{Title: "Chat"})
	log.Fatal(http.ListenAndServe(":8000", mux))
}

// Handler serves one route, for routers Mount does not fit: chi's r.Get, go-zero,
// Kratos, Huma or Encore raw endpoints.
func ExampleHandler() {
	engine := chanx.New(chanx.Config{})
	route := engine.Handle("/ws/chat/{room}", ChatConsumer{})

	mux := http.NewServeMux()
	mux.Handle("GET /ws/chat/{room}", chanxhttp.Handler(engine, route))
}

// Dispatcher serves every route behind one prefix, matching the URL itself: for
// gorilla/mux's PathPrefix or any catch-all mount.
func ExampleDispatcher() {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/chat/{room}", ChatConsumer{})

	mux := http.NewServeMux()
	mux.Handle("/ws/", chanxhttp.Dispatcher(engine))
}

// The HTTP server does not track hijacked WebSockets: Shutdown stops both.
func ExampleShutdown() {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/chat/{room}", ChatConsumer{})
	mux := http.NewServeMux()
	chanxhttp.Mount(mux, engine)

	srv := &http.Server{Addr: ":8000", Handler: mux}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = chanxhttp.Shutdown(context.Background(), srv, engine) // closes sockets with 1001
	}()
	_ = srv.ListenAndServe()
}
