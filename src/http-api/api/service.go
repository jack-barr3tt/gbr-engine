package api

import (
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jack-barr3tt/gbr-engine/src/common/data"
)

func (s *APIServer) QueryServices(c *fiber.Ctx) error {
	var req ServiceQueryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	filters := data.ServiceFilters{}

	if req.Headcode != nil {
		filters.Headcode = req.Headcode
	}

	if req.OperatorCode != nil {
		filters.OperatorCode = req.OperatorCode
	}

	if req.PassesThrough != nil && len(*req.PassesThrough) > 0 {
		filters.PassesThrough = make([]data.LocationFilter, 0, len(*req.PassesThrough))
		for _, loc := range *req.PassesThrough {
			stanox, err := s.StanoxFromLocationFilter(*loc.LocationFilter)
			if err != nil {
				return HandleError(c, err)
			}
			if stanox == "" {
				return c.Status(http.StatusBadRequest).JSON(ErrorResponse{
					Error:   "Bad Request",
					Message: "Must specify one of: stanox, crs, tiploc, or name for location filter",
				})
			}

			locFilter := data.LocationFilter{
				Stanox: stanox,
			}
			if loc.TimeFrom != nil {
				locFilter.TimeFrom = loc.TimeFrom
			}
			if loc.TimeTo != nil {
				locFilter.TimeTo = loc.TimeTo
			}
			filters.PassesThrough = append(filters.PassesThrough, locFilter)
		}
	}

	services, err := s.Data.GetServicesWithFilters(filters)
	if err != nil {
		errStr := err.Error()
		return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "Failed to retrieve services",
			Stack:   &errStr,
		})
	}

	if services == nil {
		services = []ServiceResponse{}
	}

	// Add realtime data based on the date range from filters
	realtimeDate := time.Now()
	if len(filters.PassesThrough) > 0 && filters.PassesThrough[0].TimeFrom != nil {
		realtimeDate = *filters.PassesThrough[0].TimeFrom
	}
	s.Data.AddRealtimeData(services, realtimeDate)

	return c.JSON(services)
}
