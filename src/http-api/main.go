//go:generate oapi-codegen -config server.cfg.yaml ../../spec/openapi.yaml

package main

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
	"github.com/jack-barr3tt/gbr-engine/src/http-api/api"
)

func main() {
	utils.InitLogger()
	defer utils.SyncLogger()
	log := utils.GetLogger()

	app := fiber.New()

	app.Use(func(c *fiber.Ctx) error {
		path := c.Path()
		method := c.Method()

		if path != "/health" {
			log.Infow("request", "method", method, "path", path, "status", c.Response().StatusCode())
		}

		return c.Next()
	})

	app.Use(cors.New())

	server, err := api.NewServer()
	if err != nil {
		log.Fatalw("failed to start http api server", "error", err)
		return
	}

	api.RegisterHandlers(app, server)

	if err := app.Listen(":3000"); err != nil {
		log.Fatalw("fiber listen failed", "error", err)
	}
}
