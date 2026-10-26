package chanxgin_test

import (
	"net/http"

	"github.com/gin-gonic/gin"

	chanx "go.chanx.dev"
	chanxgin "go.chanx.dev/adapters/gin"
	"go.chanx.dev/asyncapi"
)

type ChatConsumer struct{}

func (ChatConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

// authMiddleware runs before the upgrade, like for any gin route.
func authMiddleware(c *gin.Context) {
	user := c.GetHeader("X-User")
	if user == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Set("user", user)
}

func ExampleMount() {
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws/chat/{room}", ChatConsumer{}, chanx.UserFrom("user")) // the value authMiddleware set

	r := gin.Default()
	chanxgin.Mount(r.Group("/", authMiddleware), engine) // registered as /ws/chat/:room
	chanxgin.MountDocs(r, engine, "/asyncapi", asyncapi.Info{Title: "Chat"})
	_ = r.Run(":8000")
}
