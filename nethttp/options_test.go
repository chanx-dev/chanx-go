package chanxhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	chanx "go.chanx.dev"
	chanxhttp "go.chanx.dev/nethttp"
)

type ipConsumer struct{}

type WhereMessage struct {
	chanx.BaseMessage `action:"where"`
}

type HereMessage struct {
	chanx.BaseMessage `action:"here"`
	Payload           string `json:"payload"`
}

func (ipConsumer) Where(ctx *chanx.Context, _ WhereMessage) (HereMessage, error) {
	return HereMessage{Payload: ctx.Conn.Request.ClientIP + " " + ctx.Conn.Param("room")}, nil
}

func dialAndAsk(t *testing.T, url string, opts *websocket.DialOptions) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, url, opts)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	defer func() { _ = ws.CloseNow() }()
	if err := ws.Write(ctx, websocket.MessageText, []byte(`{"action":"where"}`)); err != nil {
		t.Fatal(err)
	}
	_, data, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Prefix, NoMethod (a router without "GET " patterns), ClientIP and
// AcceptOptions; Handler mounts one route on any router.
func TestOptions(t *testing.T) {
	e := chanx.New(chanx.Config{})
	r := e.Handle("/ws/{room}", ipConsumer{})
	mux := http.NewServeMux()
	chanxhttp.Mount(mux, e, chanxhttp.Prefix("/api"), chanxhttp.NoMethod(),
		chanxhttp.ClientIP(func(r *http.Request) string { return r.Header.Get("X-Real-IP") }),
		chanxhttp.AcceptOptions(&chanxhttp.CoderOptions{OriginPatterns: []string{"app.example"}}))
	mux.Handle("/other/", http.StripPrefix("/other", chanxhttp.Handler(e, r)))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base := "ws" + strings.TrimPrefix(srv.URL, "http")

	got := dialAndAsk(t, base+"/api/ws/lobby", &websocket.DialOptions{HTTPHeader: http.Header{
		"X-Real-Ip": {"203.0.113.9"}, "Origin": {"https://app.example"},
	}})
	if !strings.Contains(got, `"203.0.113.9 lobby"`) {
		t.Fatal(got)
	}
	// The origin is checked by the accept options.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if ws, _, err := websocket.Dial(ctx, base+"/api/ws/lobby", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://evil.example"}}}); err == nil {
		_ = ws.CloseNow()
		t.Fatal("a foreign origin was accepted")
	}
	// Handler on a route the mux strips: parameters come from matching the path.
	if got := dialAndAsk(t, base+"/other/ws/side", nil); !strings.Contains(got, ` side"`) {
		t.Fatal(got)
	}

	server := &http.Server{Handler: mux}
	sctx, scancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer scancel()
	if err := chanxhttp.Shutdown(sctx, server, e); err != nil {
		t.Fatal(err)
	}
}

// A negative MaxFrameSize lifts coder/websocket's own 32 KiB default too.
func TestNoFrameLimit(t *testing.T) {
	e := chanx.New(chanx.Config{MaxFrameSize: -1})
	e.Handle("/ws", ipConsumer{})
	mux := http.NewServeMux()
	chanxhttp.Mount(mux, e)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.CloseNow() }()
	ws.SetReadLimit(-1)
	ref := strings.Repeat("x", 64<<10)
	if err := ws.Write(ctx, websocket.MessageText, []byte(`{"action":"where","ref":"`+ref+`"}`)); err != nil {
		t.Fatal(err)
	}
	if _, data, err := ws.Read(ctx); err != nil || !strings.Contains(string(data), ref) {
		t.Fatalf("64 KiB frame not served: %v", err)
	}
}
