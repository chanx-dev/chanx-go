package chanxfiber_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	fws "github.com/gofiber/contrib/v3/websocket"

	"github.com/gofiber/fiber/v3"

	chanx "github.com/huynguyengl99/chanx-go"
	chanxfiber "github.com/huynguyengl99/chanx-go/adapters/fiber"
	"github.com/huynguyengl99/chanx-go/asyncapi"
	"github.com/huynguyengl99/chanx-go/chanxtest/adaptertest"
)

func TestAdapter(t *testing.T) {
	adaptertest.Run(t, func(t *testing.T, e *chanx.Engine) *adaptertest.Server {
		app := fiber.New()
		auth := func(c fiber.Ctx) error {
			if c.Get("X-Reject") != "" {
				return fiber.NewError(fiber.StatusUnauthorized, "rejected")
			}
			if u := c.Get("X-User"); u != "" {
				c.Locals("user", u)
			}
			return c.Next()
		}
		chanxfiber.Mount(app.Group("/", auth), e)
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go func() { _ = app.Listener(ln, fiber.ListenConfig{DisableStartupMessage: true}) }()
		return &adaptertest.Server{
			URL:      "http://" + ln.Addr().String(),
			Shutdown: func(ctx context.Context) error { return app.ShutdownWithContext(ctx) },
		}
	})
}

func TestMountDocs(t *testing.T) {
	e := chanx.New(chanx.Config{})
	app := fiber.New()
	chanxfiber.MountDocs(app, e, "/asyncapi", asyncapi.Info{Title: "Docs"})
	for _, p := range []string{"/asyncapi", "/asyncapi.json", "/asyncapi.yaml"} {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, p, nil))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Docs") {
			t.Errorf("%s: %d", p, resp.StatusCode)
		}
	}
}

// Non-string Locals keys named with the Locals option are copied before the
// upgrade, as string keys are.
type ctxKey struct{}

func TestLocalsOption(t *testing.T) {
	e := chanx.New(chanx.Config{})
	defer func() { _ = e.Shutdown(context.Background()) }()
	e.Handle("/ws", keyed{}, chanx.UserFrom(ctxKey{}))
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error { c.Locals(ctxKey{}, "kim"); return c.Next() })
	chanxfiber.Mount(app, e, chanxfiber.Locals(ctxKey{}), chanxfiber.WebSocket(fws.Config{}))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = app.Listener(ln, fiber.ListenConfig{DisableStartupMessage: true}) }()
	defer func() { _ = app.Shutdown() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws://"+ln.Addr().String()+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.CloseNow() }()
	_, data, err := ws.Read(ctx)
	if err != nil || !strings.Contains(string(data), `"status_code":200`) {
		t.Fatalf("%s %v", data, err)
	}
}

type keyed struct{}

func (keyed) Ping(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}
