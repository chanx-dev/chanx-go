// sandbox-fiber serves the sandbox on Fiber v3.
package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"

	chanxfiber "go.chanx.dev/adapters/fiber"
	"go.chanx.dev/examples/sandbox"
)

func main() {
	set, err := sandbox.Layers(os.Getenv("REDIS_URL"))
	if err != nil {
		log.Fatal(err)
	}
	engine := sandbox.New(set, nil)
	app := fiber.New()
	chanxfiber.Mount(app, engine)
	chanxfiber.MountDocs(app, engine, "/asyncapi", sandbox.Info)
	app.Post("/notify", adaptor.HTTPHandler(sandbox.NotifyHandler(engine)))
	log.Fatal(app.Listen(":8000"))
}
