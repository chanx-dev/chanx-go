// sandbox serves the Go port of sandbox_fastapi on net/http.
//
//	go run ./cmd/sandbox                     # in-memory layers, :8000
//	go run ./cmd/sandbox -redis redis://localhost:6393
//	go run ./cmd/sandbox -asyncapi > asyncapi.json
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.chanx.dev/asyncapi"
	"go.chanx.dev/examples/sandbox"
	chanxhttp "go.chanx.dev/nethttp"
)

func main() {
	addr := flag.String("addr", ":8000", "listen address")
	redisURL := flag.String("redis", os.Getenv("REDIS_URL"), "Redis URL(s); empty: in-memory layers")
	doc := flag.Bool("asyncapi", false, "print the AsyncAPI document and exit")
	flag.Parse()

	set, err := sandbox.Layers(*redisURL)
	if err != nil {
		log.Fatal(err)
	}
	engine := sandbox.New(set, nil)
	if *doc {
		if err := asyncapi.Write(os.Stdout, engine, sandbox.Info); err != nil {
			log.Fatal(err)
		}
		return
	}

	mux := http.NewServeMux()
	chanxhttp.Mount(mux, engine)
	chanxhttp.MountDocs(mux, engine, "/asyncapi", sandbox.Info)
	mux.Handle("POST /notify", sandbox.NotifyHandler(engine))

	srv := &http.Server{Addr: *addr, Handler: mux}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		// http.Server.Shutdown does not track hijacked WebSockets: shut the engine down too.
		_ = chanxhttp.Shutdown(context.Background(), srv, engine)
	}()
	log.Printf("sandbox on http://localhost%s (docs at /asyncapi)", *addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
