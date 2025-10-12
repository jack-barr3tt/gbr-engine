package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s *APIServer) GetService(c *fiber.Ctx, params GetServiceParams) error {
	headcode := params.Headcode
	if headcode == "" {
		return c.Status(http.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "headcode query parameter is required",
		})
	}

	var services []ServiceResponse

	scheduleRows, err := s.DB.Query(context.Background(), `
		SELECT id, train_uid, signalling_id, headcode, train_category, 
		       schedule_start_date, schedule_end_date, schedule_days_runs, 
		       train_status, atoc_code
		FROM schedule 
		WHERE signalling_id = $1 
		ORDER BY id
	`, headcode)

	if err != nil {
		log.Error(err)
		errStr := err.Error()
		return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "Failed to query schedule table",
			Stack:   &errStr,
		})
	}
	defer scheduleRows.Close()

	for scheduleRows.Next() {
		var service ServiceResponse
		var scheduleStartDate, scheduleEndDate time.Time

		err := scheduleRows.Scan(
			&service.Id,
			&service.TrainUid,
			&service.SignallingId,
			&service.Headcode,
			&service.TrainCategory,
			&scheduleStartDate,
			&scheduleEndDate,
			&service.ScheduleDaysRuns,
			&service.TrainStatus,
			&service.AtocCode,
		)

		if err != nil {
			errStr := err.Error()
			return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Database error",
				Message: "Error scanning schedule data",
				Stack:   &errStr,
			})
		}

		startDate := openapi_types.Date{Time: scheduleStartDate}
		endDate := openapi_types.Date{Time: scheduleEndDate}
		service.ScheduleStartDate = &startDate
		service.ScheduleEndDate = &endDate

		var locations []ScheduleLocation
		locationRows, err := s.DB.Query(context.Background(), `
			SELECT id, location_type, tiploc_code, arrival, public_arrival, 
			       departure, public_departure, platform, location_order
			FROM schedule_location 
			WHERE schedule_id = $1 
			ORDER BY location_order
		`, service.Id)

		if err != nil {
			errStr := err.Error()
			return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Database error",
				Message: "Failed to query schedule_location table",
				Stack:   &errStr,
			})
		}

		for locationRows.Next() {
			var location ScheduleLocation
			err := locationRows.Scan(
				&location.Id,
				&location.LocationType,
				&location.TiplocCode,
				&location.Arrival,
				&location.PublicArrival,
				&location.Departure,
				&location.PublicDeparture,
				&location.Platform,
				&location.LocationOrder,
			)
			if err != nil {
				locationRows.Close()
				errStr := err.Error()
				return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
					Error:   "Database error",
					Message: "Error scanning location data",
					Stack:   &errStr,
				})
			}
			locations = append(locations, location)
		}
		locationRows.Close()

		if err = locationRows.Err(); err != nil {
			errStr := err.Error()
			return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Database error",
				Message: "Error iterating over locations",
				Stack:   &errStr,
			})
		}

		service.Locations = locations
		services = append(services, service)
	}

	if err = scheduleRows.Err(); err != nil {
		errStr := err.Error()
		return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "Error iterating over schedules",
			Stack:   &errStr,
		})
	}

	// Check if any services were found
	if len(services) == 0 {
		return c.Status(http.StatusNotFound).JSON(NotFoundResponse{
			Error: "No services found",
		})
	}

	return c.JSON(services)
}
