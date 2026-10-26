package chanxfiber_test

import (
	"github.com/gofiber/fiber/v3"

	chanx "go.chanx.dev"
	chanxfiber "go.chanx.dev/adapters/fiber"
	"go.chanx.dev/asyncapi"
)

type ChatConsumer struct{}

func (ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

// authMiddleware runs before the upgrade, like for any Fiber route.
func authMiddleware(c fiber.Ctx) error {
	user := c.Get("X-User")
	if user == "" {
		return fiber.ErrUnauthorized
	}
	c.Locals("user", user) // string keys are copied before Fiber recycles c
	return c.Next()
}

func ExampleMount() {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/chat/{room}", ChatConsumer{}, chanx.UserFrom("user")) // the value authMiddleware set

	app := fiber.New()
	chanxfiber.Mount(app.Group("/", authMiddleware), engine) // registered as /ws/chat/:room
	chanxfiber.MountDocs(app, engine, "/asyncapi", asyncapi.Info{Title: "Chat"})
	_ = app.Listen(":8000")
}
