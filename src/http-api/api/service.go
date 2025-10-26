package api

import (
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jack-barr3tt/gbr-engine/src/common/data"
)

func (s *APIServer) GetService(c *fiber.Ctx, params GetServiceParams) error {
	headcode := params.Headcode
	if headcode == "" {
		return c.Status(http.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "headcode query parameter is required",
		})
	}

	filters := data.ServiceFilters{
		Headcode: &headcode,
	}

	services, err := s.Data.GetServicesWithFilters(filters)
	if err != nil {
		errStr := err.Error()
		return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "Failed to retrieve services by headcode",
			Stack:   &errStr,
		})
	}

	// Check if any services were found
	if len(services) == 0 {
		return c.Status(http.StatusNotFound).JSON(NotFoundResponse{
			Error: "No services found",
		})
	}

	s.Data.AddRealtimeData(services, time.Now())

	return c.JSON(services)
}

func (s *APIServer) GetServicesAtLocation(c *fiber.Ctx, params GetServicesAtLocationParams) error {
	// Resolve location to stanox
	var stanox string
	var err error

	if params.Name != nil {
		stanox, err = s.Data.GetStanoxByLocationName(*params.Name)
	}
	if params.Crs != nil {
		stanox, err = s.Data.GetStanoxByCRS(*params.Crs)
	}
	if params.Tiploc != nil {
		stanox, err = s.Data.GetStanoxByTiploc(*params.Tiploc)
	}
	if params.Stanox != nil {
		stanox = *params.Stanox
	}
	if err != nil {
		return HandleError(c, err)
	}

	// Determine check date
	var checkDate time.Time
	if params.Date != nil {
		checkDate = params.Date.Time
	} else {
		checkDate = time.Now().UTC().Truncate(24 * time.Hour)
	}

	// Get location details
	locationDetails, err := s.Data.GetLocationDetails(stanox)
	if err != nil {
		return HandleError(c, err)
	}

	// Create time range for the entire day
	dayStart := checkDate.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24*time.Hour - time.Second)

	// Get services with filters (includes day-of-week filtering)
	services, err := s.Data.GetServicesWithFilters(data.ServiceFilters{
		PassesThrough: []data.LocationFilter{{
			Stanox:   stanox,
			TimeFrom: &dayStart,
			TimeTo:   &dayEnd,
		}},
	})
	if err != nil {
		return HandleError(c, err)
	}

	// Add realtime data
	s.Data.AddRealtimeData(services, checkDate)

	// Build response
	response := LocationServicesResponse{
		Services: services,
	}

	response.Location = *locationDetails

	return c.JSON(response)
}
