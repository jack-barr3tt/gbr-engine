package api

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
)

func (s *APIServer) GetLocations(c *fiber.Ctx) error {
	locations, err := s.Data.GetAllLocations()
	if err != nil {
		errStr := err.Error()
		return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "Failed to retrieve locations",
			Stack:   &errStr,
		})
	}

	return c.JSON(locations)
}

func (s *APIServer) GetOperators(c *fiber.Ctx) error {
	operators, err := s.Data.GetAllOperators()
	if err != nil {
		errStr := err.Error()
		return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "Failed to retrieve operators",
			Stack:   &errStr,
		})
	}

	return c.JSON(operators)
}
