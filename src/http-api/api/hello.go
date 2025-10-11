package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
)



func (s *APIServer) GetHello(c *fiber.Ctx) error {
	response := HelloResponse{
		Message:   "Hello, World!",
		Timestamp: time.Now(),
	}
	return c.JSON(response)
}

// GetHealth implements the health check endpoint
func (s *APIServer) GetHealth(c *fiber.Ctx) error {
	response := HealthResponse{
		Status:  "healthy",
		Version: "1.0.0",
	}
	return c.JSON(response)
}
