package api

import (
	"database/sql"
	"net/http"

	"github.com/gofiber/fiber/v2"
)

func HandleError(c *fiber.Ctx, err error) error {
	if err == sql.ErrNoRows {
		return c.Status(http.StatusNotFound).JSON(NotFoundResponse{
			Error: "Location not found",
		})
	}
	errStr := err.Error()
	return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
		Error:   "Database error",
		Message: "Failed to lookup location",
		Stack:   &errStr,
	})
}
