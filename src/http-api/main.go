//go:generate oapi-codegen -config server.cfg.yaml ../../spec/openapi.yaml

package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/jack-barr3tt/gbr-engine/src/http-api/api"
)

func main() {
	app := fiber.New()

	app.Use(logger.New())
	app.Use(cors.New())

	server, err := api.NewServer()
	if err != nil {
		log.Fatal(err)
		return
	}

	api.RegisterHandlers(app, server)

	log.Fatal(app.Listen(":3000"))
}
