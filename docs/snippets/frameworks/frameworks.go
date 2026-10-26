// Package frameworks shows the engine mounted on each supported framework.
package frameworks

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gofiber/fiber/v3"
	"github.com/labstack/echo/v5"

	chanxecho "go.chanx.dev/adapters/echo/v2"
	chanxfiber "go.chanx.dev/adapters/fiber/v2"
	chanxgin "go.chanx.dev/adapters/gin/v2"
	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/asyncapi"
	chanxhttp "go.chanx.dev/v2/nethttp"
	"go.chanx.dev/v2/transport"
	"go.chanx.dev/v2/transport/coderws"
	"go.chanx.dev/v2/transport/gorillaws"
)

type ChatConsumer struct{}

func (ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

type userKey struct{}

func newEngine(opts ...chanx.Option) *chanx.Engine {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/chat/{room}", ChatConsumer{}, opts...)
	return engine
}

// #region nethttp
func NetHTTP(engine *chanx.Engine) *http.ServeMux {
	mux := http.NewServeMux()
	chanxhttp.Mount(mux, engine) // registers "GET /ws/chat/{room}"
	chanxhttp.MountDocs(mux, engine, "/asyncapi", asyncapi.Info{Title: "Chat"})
	return mux
}

// #endregion nethttp

// #region nethttp-options
func NetHTTPOptions(engine *chanx.Engine) *http.ServeMux {
	mux := http.NewServeMux()
	chanxhttp.Mount(mux, engine,
		chanxhttp.Prefix("/api"), // serves /api/ws/chat/{room}
		chanxhttp.AcceptOptions(&coderws.Options{OriginPatterns: []string{"app.example.com"}}),
		chanxhttp.ClientIP(func(r *http.Request) string { return r.Header.Get("X-Real-IP") }),
	)
	return mux
}

// #endregion nethttp-options

// #region gorilla
func Gorilla(engine *chanx.Engine) *http.ServeMux {
	upgrader := &gorillaws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	chanxhttp.Mount(mux, engine, chanxhttp.Upgrade(func(w http.ResponseWriter, r *http.Request) (transport.Conn, error) {
		return gorillaws.Accept(w, r, upgrader)
	}))
	return mux
}

// #endregion gorilla

// #region shutdown
func Serve(ctx context.Context, engine *chanx.Engine, mux *http.ServeMux) error {
	srv := &http.Server{Addr: ":8000", Handler: mux}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = chanxhttp.Shutdown(shutdown, srv, engine) // the server, then every socket with 1001
	}()
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// #endregion shutdown

// #region gin
func Gin(engine *chanx.Engine) *gin.Engine {
	r := gin.Default()
	auth := func(c *gin.Context) {
		c.Set("user", c.GetHeader("X-User")) // read by chanx.UserFrom("user")
	}
	chanxgin.Mount(r.Group("/", auth), engine) // "/ws/chat/:room"
	chanxgin.MountDocs(r, engine, "/asyncapi", asyncapi.Info{Title: "Chat"})
	return r
}

// #endregion gin

// #region echo
func Echo(engine *chanx.Engine) *echo.Echo {
	e := echo.New()
	auth := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("user", c.Request().Header.Get("X-User")) // read by chanx.UserFrom("user")
			return next(c)
		}
	}
	chanxecho.Mount(e.Group("", auth), engine) // "/ws/chat/:room"
	chanxecho.MountDocs(e, engine, "/asyncapi", asyncapi.Info{Title: "Chat"})
	return e
}

// #endregion echo

// #region fiber
func Fiber(engine *chanx.Engine) *fiber.App {
	app := fiber.New()
	auth := func(c fiber.Ctx) error {
		c.Locals("user", c.Get("X-User")) // copied before the upgrade, read by chanx.UserFrom("user")
		return c.Next()
	}
	chanxfiber.Mount(app.Group("/", auth), engine) // "/ws/chat/:room"
	chanxfiber.MountDocs(app, engine, "/asyncapi", asyncapi.Info{Title: "Chat"})
	return app
}

// #endregion fiber

// #region fiber-locals
func FiberTypedLocals(engine *chanx.Engine) *fiber.App {
	app := fiber.New()
	auth := func(c fiber.Ctx) error {
		c.Locals(userKey{}, c.Get("X-User"))
		return c.Next()
	}
	// A non-string Locals key must be named to be copied before the upgrade.
	chanxfiber.Mount(app.Group("/", auth), engine, chanxfiber.Locals(userKey{}))
	return app
}

// #endregion fiber-locals
