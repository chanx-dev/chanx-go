// sandbox-gin serves the sandbox on gin.
package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"

	chanxgin "github.com/huynguyengl99/chanx-go/adapters/gin"
	"github.com/huynguyengl99/chanx-go/examples/sandbox"
)

func main() {
	set, err := sandbox.Layers(os.Getenv("REDIS_URL"))
	if err != nil {
		log.Fatal(err)
	}
	engine := sandbox.New(set, nil)
	r := gin.Default()
	chanxgin.Mount(r, engine) // or r.Group("/", authMiddleware): middleware runs before the upgrade
	chanxgin.MountDocs(r, engine, "/asyncapi", sandbox.Info)
	r.POST("/notify", gin.WrapH(sandbox.NotifyHandler(engine)))
	log.Fatal(r.Run(":8000"))
}
