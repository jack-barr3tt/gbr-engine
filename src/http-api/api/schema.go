package api

import (
	"github.com/gofiber/fiber/v2"
)

// GetSchema returns the OpenAPI schema JSON
func (a *APIServer) GetSchema(c *fiber.Ctx) error {
	schema, err := rawSpec()
	if err != nil {
		a.Logger.Errorw("failed to decode openapi schema", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to retrieve schema",
			Message: err.Error(),
		})
	}

	c.Set("Content-Type", "application/json")
	return c.Send(schema)
}
