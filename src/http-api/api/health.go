package api

import (
	"github.com/gofiber/fiber/v2"
)

// GetHealth implements the health check endpoint
func (s *APIServer) GetHealth(c *fiber.Ctx) error {
	response := HealthResponse{
		Status:  "healthy",
		Version: "1.0.0",
	}
	return c.JSON(response)
}
