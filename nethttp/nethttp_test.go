package chanxhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/asyncapi"
	"github.com/huynguyengl99/chanx-go/chanxtest/adaptertest"
	chanxhttp "github.com/huynguyengl99/chanx-go/nethttp"
	"github.com/huynguyengl99/chanx-go/transport"
	"github.com/huynguyengl99/chanx-go/transport/gorillaws"
)

// middleware is the suite's contract: X-Reject answers 401, X-User sets "user".
func middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Reject") != "" {
			http.Error(w, "rejected", http.StatusUnauthorized)
			return
		}
		if u := r.Header.Get("X-User"); u != "" {
			r = r.WithContext(context.WithValue(r.Context(), "user", u)) //nolint:staticcheck // the suite reads the key "user"
		}
		next.ServeHTTP(w, r)
	})
}

func serve(t *testing.T, mux *http.ServeMux) *adaptertest.Server {
	srv := httptest.NewServer(middleware(mux))
	t.Cleanup(srv.Close)
	return &adaptertest.Server{URL: srv.URL}
}

func TestServeMux(t *testing.T) {
	adaptertest.Run(t, func(t *testing.T, e *chanx.Engine) *adaptertest.Server {
		mux := http.NewServeMux()
		chanxhttp.Mount(mux, e)
		return serve(t, mux)
	})
}

func TestGorillaTransport(t *testing.T) {
	adaptertest.Run(t, func(t *testing.T, e *chanx.Engine) *adaptertest.Server {
		mux := http.NewServeMux()
		chanxhttp.Mount(mux, e, chanxhttp.Upgrade(func(w http.ResponseWriter, r *http.Request) (transport.Conn, error) {
			return gorillaws.Accept(w, r, nil)
		}))
		return serve(t, mux)
	})
}

// A catch-all mount: the engine matches the path, as for routers that do not
// set path values (gorilla/mux) or a prefix handed to one handler.
func TestDispatcher(t *testing.T) {
	adaptertest.Run(t, func(t *testing.T, e *chanx.Engine) *adaptertest.Server {
		mux := http.NewServeMux()
		mux.Handle("/ws/", chanxhttp.Dispatcher(e))
		return serve(t, mux)
	})
}

func TestMountDocs(t *testing.T) {
	e := chanx.New(chanx.Config{})
	defer func() { _ = e.Shutdown(context.Background()) }()
	mux := http.NewServeMux()
	chanxhttp.MountDocs(mux, e, "/asyncapi", asyncapi.Info{Title: "Docs"})
	for _, p := range []string{"/asyncapi", "/asyncapi.json", "/asyncapi.yaml"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Docs") {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
}
