package api

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
)

func (s *APIServer) GetService(c *fiber.Ctx, params GetServiceParams) error {
	headcode := params.Headcode
	if headcode == "" {
		return c.Status(http.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "headcode query parameter is required",
		})
	}

	services, err := s.GetServicesByHeadcode(headcode)
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

	ctx := c.Context()
	s.AddRealtimeData(ctx, services, time.Now())

	return c.JSON(services)
}

func (s *APIServer) GetServicesAtLocation(c *fiber.Ctx, params GetServicesAtLocationParams) error {
	paramCount := 0
	var location string
	var locationType string

	if params.Name != nil {
		paramCount++
		location = *params.Name
		locationType = "name"
	}
	if params.Crs != nil {
		paramCount++
		location = *params.Crs
		locationType = "crs"
	}
	if params.Tiploc != nil {
		paramCount++
		location = *params.Tiploc
		locationType = "tiploc"
	}
	if params.Stanox != nil {
		paramCount++
		location = *params.Stanox
		locationType = "stanox"
	}

	if paramCount == 0 {
		return c.Status(http.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Must specify exactly one location parameter: name, crs, tiploc, or stanox",
		})
	}
	if paramCount > 1 {
		return c.Status(http.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Must specify exactly one location parameter, not multiple",
		})
	}

	var checkDate time.Time
	if params.Date != nil {
		checkDate = params.Date.Time
	} else {
		checkDate = time.Now().UTC().Truncate(24 * time.Hour) // Start of today
	}

	var stanox string
	var err error

	switch locationType {
	case "name":
		stanox, err = s.GetStanoxByLocationName(location)
	case "crs":
		stanox, err = GetStanoxByCRS(s.DB, location)
	case "tiploc":
		stanox, err = GetStanoxByTiploc(s.DB, location)
	case "stanox":
		stanox = location
	default:
		return c.Status(http.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid location type",
		})
	}

	if err != nil {
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

	locationName, crsCode, tiplocCodes, err := s.GetLocationDetails(stanox)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(http.StatusNotFound).JSON(NotFoundResponse{
				Error: "Location details not found",
			})
		}
		errStr := err.Error()
		return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "Failed to retrieve location details",
			Stack:   &errStr,
		})
	}

	services, err := GetScheduledServicesAtLocation(s.DB, stanox, checkDate)
	if err != nil {
		errStr := err.Error()
		return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "Failed to retrieve services at location",
			Stack:   &errStr,
		})
	}

	ctx := c.Context()
	s.AddRealtimeData(ctx, services, checkDate)

	response := LocationServicesResponse{
		Services: services,
	}

	response.Location.SearchTerm = location
	response.Location.SearchType = locationType
	response.Location.Stanox = stanox

	if locationName != "" {
		response.Location.Name = &locationName
	}
	if crsCode != "" {
		response.Location.CrsCode = &crsCode
	}
	if len(tiplocCodes) > 0 {
		response.Location.TiplocCodes = &tiplocCodes
	}

	return c.JSON(response)
}
