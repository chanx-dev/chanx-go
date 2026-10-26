package frameworks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"

	chanx "go.chanx.dev"
)

// Each mount serves the route over a real socket; gin and echo hand the
// middleware's user to chanx.UserFrom.
func TestMounts(t *testing.T) {
	withUser := chanx.UserFrom("user")
	for name, tc := range map[string]struct {
		handler      http.Handler
		authenticate bool
	}{
		"nethttp": {NetHTTP(newEngine()), false},
		"gorilla": {Gorilla(newEngine()), false},
		"gin":     {Gin(newEngine(withUser)), true},
		"echo":    {Echo(newEngine(withUser)), true},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			ctx := context.Background()
			url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/chat/lobby"
			ws, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{"X-User": {"ann"}}})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = ws.CloseNow() }()
			read := func(want string) {
				t.Helper()
				if _, data, err := ws.Read(ctx); err != nil || !strings.Contains(string(data), want) {
					t.Fatalf("got %s %v, want %s", data, err, want)
				}
			}
			if tc.authenticate {
				read(`"status_code":200`)
			}
			_ = ws.Write(ctx, websocket.MessageText, []byte(`{"action":"ping"}`))
			read(`"pong"`)
		})
	}
}
