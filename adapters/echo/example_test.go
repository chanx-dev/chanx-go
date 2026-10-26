package chanxecho_test

import (
	"net/http"

	"github.com/labstack/echo/v5"

	chanx "go.chanx.dev"
	chanxecho "go.chanx.dev/adapters/echo"
	"go.chanx.dev/asyncapi"
)

type ChatConsumer struct{}

func (ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

// authMiddleware runs before the upgrade, like for any echo route.
func authMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		user := c.Request().Header.Get("X-User")
		if user == "" {
			return echo.NewHTTPError(http.StatusUnauthorized, "no user")
		}
		c.Set("user", user)
		return next(c)
	}
}

func ExampleMount() {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/chat/{room}", ChatConsumer{}, chanx.UserFrom("user")) // the value authMiddleware set

	e := echo.New()
	chanxecho.Mount(e.Group("", authMiddleware), engine) // registered as /ws/chat/:room
	chanxecho.MountDocs(e, engine, "/asyncapi", asyncapi.Info{Title: "Chat"})
	_ = e.Start(":8000")
}
