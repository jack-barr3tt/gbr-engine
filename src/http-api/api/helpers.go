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

func (s *APIServer) StanoxFromLocationFilter(loc LocationFilter) (string, error) {
	if loc.Stanox != nil {
		return *loc.Stanox, nil
	} else if loc.Crs != nil {
		return s.Data.GetStanoxByCRS(*loc.Crs)
	} else if loc.Tiploc != nil {
		return s.Data.GetStanoxByTiploc(*loc.Tiploc)
	} else if loc.Name != nil {
		return s.Data.GetStanoxByLocationName(*loc.Name)
	} else {
		return "", nil
	}
}
