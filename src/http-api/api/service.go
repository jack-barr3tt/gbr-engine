package api

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jack-barr3tt/gbr-engine/src/common/data"
)

const (
	MaxLimit  = 100
	MaxOffset = 1000
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

	filters.Offset = 0
	if req.Offset != nil && *req.Offset >= 0 {
		if *req.Offset > MaxOffset {
			return c.Status(http.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Bad Request",
				Message: "Offset exceeds maximum allowed value of 1000",
			})
		}
		filters.Offset = *req.Offset
	}

	filters.Limit = 50 // Default limit
	if req.Limit != nil && *req.Limit > 0 {
		if *req.Limit > MaxLimit {
			return c.Status(http.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Bad Request",
				Message: "Limit exceeds maximum allowed value of 100",
			})
		}
		filters.Limit = *req.Limit
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

	result, err := s.Data.GetServicesWithFilters(filters)
	if err != nil {
		errStr := err.Error()
		return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "Failed to retrieve services",
			Stack:   &errStr,
		})
	}

	services := result.Services
	if services == nil {
		services = []ServiceResponse{}
	}

	// Add realtime data based on the date range from filters
	realtimeDate := time.Now()
	if len(filters.PassesThrough) > 0 && filters.PassesThrough[0].TimeFrom != nil {
		realtimeDate = *filters.PassesThrough[0].TimeFrom
	}
	s.Data.AddRealtimeData(services, realtimeDate)

	// Calculate pagination info
	response := ServiceQueryResponse{
		Services: services,
		Pagination: PaginationInfo{
			Offset:       filters.Offset,
			Limit:        filters.Limit,
			Returned:     len(services),
			TotalResults: result.TotalResults,
		},
	}

	return c.JSON(response)
}

func (s *APIServer) GetService(c *fiber.Ctx, params GetServiceParams) error {
	uid := params.Uid
	id := params.Id
	dateParam := params.Date

	// Validate that at least one identifier is provided
	if uid == nil && id == nil {
		return c.Status(http.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Must provide either 'uid' or 'id' parameter",
		})
	}

	// If ID is provided, date is required
	if id != nil && dateParam == nil {
		return c.Status(http.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "When using 'id' parameter, 'date' parameter is required",
		})
	}

	// Convert date if provided
	var date *time.Time
	if dateParam != nil {
		date = &dateParam.Time
	}

	var service *ServiceResponse
	var err error

	if uid != nil {
		service, err = s.Data.GetServiceByUID(*uid, date)
	} else {
		service, err = s.Data.GetServiceByID(*id, *date)
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(http.StatusNotFound).JSON(NotFoundResponse{
				Error: "Service not found",
			})
		}

		errStr := err.Error()
		return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "Failed to retrieve service",
			Stack:   &errStr,
		})
	}

	return c.JSON(service)
}
