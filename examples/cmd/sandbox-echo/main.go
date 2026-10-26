// sandbox-echo serves the sandbox on echo v5.
package main

import (
	"log"
	"os"

	"github.com/labstack/echo/v5"

	chanxecho "go.chanx.dev/adapters/echo"
	"go.chanx.dev/examples/sandbox"
)

func main() {
	set, err := sandbox.Layers(os.Getenv("REDIS_URL"))
	if err != nil {
		log.Fatal(err)
	}
	engine := sandbox.New(set, nil)
	e := echo.New()
	chanxecho.Mount(e, engine)
	chanxecho.MountDocs(e, engine, "/asyncapi", sandbox.Info)
	e.POST("/notify", echo.WrapHandler(sandbox.NotifyHandler(engine)))
	log.Fatal(e.Start(":8000"))
}
