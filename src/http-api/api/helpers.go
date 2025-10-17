package api

import (
	"context"
	"database/sql"
	"time"

	"github.com/gofiber/fiber/v2/log"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func isScheduleValidForDate(runsOn string, startDate, endDate, checkDate time.Time) bool {
	if checkDate.Before(startDate) || checkDate.After(endDate) {
		return false
	}

	if len(runsOn) != 7 {
		return false
	}

	weekday := int(checkDate.Weekday())
	if weekday == 0 {
		weekday = 7
	}

	dayIndex := weekday - 1
	return runsOn[dayIndex] == '1'
}

func (s *APIServer) GetServicesByHeadcode(headcode string) ([]ServiceResponse, error) {
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
		return nil, err
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
			return nil, err
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
			return nil, err
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
				return nil, err
			}
			locations = append(locations, location)
		}
		locationRows.Close()

		if err = locationRows.Err(); err != nil {
			return nil, err
		}

		service.Locations = locations
		services = append(services, service)
	}

	if err = scheduleRows.Err(); err != nil {
		return nil, err
	}

	return services, nil
}

func (s *APIServer) GetStanoxByLocationName(name string) (string, error) {
	rows, err := s.DB.Query(context.Background(), `
		SELECT stanox, description, tps_description FROM tiploc 
		WHERE description ILIKE $1 OR tps_description ILIKE $1
	`, "%"+name+"%")

	if err != nil {
		return "", err
	}
	defer rows.Close()

	type match struct {
		stanox      string
		description string
		lengthDiff  int
	}

	var bestMatch *match

	for rows.Next() {
		var stanox sql.NullString
		var description, tpsDescription sql.NullString

		err := rows.Scan(&stanox, &description, &tpsDescription)
		if err != nil {
			return "", err
		}

		if !stanox.Valid {
			continue
		}

		var matchedDescription string
		if description.Valid && len(description.String) > 0 {
			matchedDescription = description.String
		} else if tpsDescription.Valid && len(tpsDescription.String) > 0 {
			matchedDescription = tpsDescription.String
		} else {
			continue
		}

		lengthDiff := len(matchedDescription) - len(name)
		if lengthDiff < 0 {
			lengthDiff = -lengthDiff
		}
		if bestMatch == nil || lengthDiff < bestMatch.lengthDiff {
			bestMatch = &match{
				stanox:      stanox.String,
				description: matchedDescription,
				lengthDiff:  lengthDiff,
			}
		}
	}

	if err = rows.Err(); err != nil {
		return "", err
	}

	if bestMatch == nil {
		return "", sql.ErrNoRows
	}

	return bestMatch.stanox, nil
}

func GetStanoxByCRS(db *pgxpool.Pool, crsCode string) (string, error) {
	var stanox sql.NullString
	err := db.QueryRow(context.Background(), `
		SELECT stanox FROM tiploc 
		WHERE crs_code = $1
		LIMIT 1
	`, crsCode).Scan(&stanox)

	if err != nil {
		return "", err
	}

	if !stanox.Valid {
		return "", sql.ErrNoRows
	}

	return stanox.String, nil
}

func GetStanoxByTiploc(db *pgxpool.Pool, tiploc string) (string, error) {
	var stanox sql.NullString
	err := db.QueryRow(context.Background(), `
		SELECT stanox FROM tiploc 
		WHERE tiploc_code = $1
	`, tiploc).Scan(&stanox)

	if err != nil {
		return "", err
	}

	if !stanox.Valid {
		return "", sql.ErrNoRows
	}

	return stanox.String, nil
}

func (s *APIServer) GetLocationDetails(stanox string) (string, string, []string, error) {
	var tiplocCodes []string

	rows, err := s.DB.Query(context.Background(), `
		SELECT description, crs_code, tiploc_code FROM tiploc 
		WHERE stanox = $1
		ORDER BY tiploc_code
	`, stanox)

	if err != nil {
		return "", "", nil, err
	}
	defer rows.Close()

	var primaryName string
	var primaryCRS string

	for rows.Next() {
		var desc, crs, tiploc sql.NullString
		err := rows.Scan(&desc, &crs, &tiploc)
		if err != nil {
			return "", "", nil, err
		}

		if desc.Valid && desc.String != "" && primaryName == "" {
			primaryName = desc.String
		}

		if crs.Valid && crs.String != "" && primaryCRS == "" {
			primaryCRS = crs.String
		}

		if tiploc.Valid && tiploc.String != "" {
			tiplocCodes = append(tiplocCodes, tiploc.String)
		}
	}

	if err = rows.Err(); err != nil {
		return "", "", nil, err
	}

	if primaryName == "" && len(tiplocCodes) == 0 {
		return "", "", nil, sql.ErrNoRows
	}

	return primaryName, primaryCRS, tiplocCodes, nil
}

func GetScheduledServicesAtLocation(db *pgxpool.Pool, stanox string, date time.Time) ([]ServiceResponse, error) {
	query := `
		SELECT DISTINCT s.id, s.train_uid, s.signalling_id, s.headcode, s.train_category, 
			   s.schedule_start_date, s.schedule_end_date, s.schedule_days_runs, s.train_status, s.atoc_code
		FROM schedule s
		JOIN schedule_location sl ON s.id = sl.schedule_id
		JOIN tiploc t ON sl.tiploc_code = t.tiploc_code
		WHERE t.stanox = $1
	`

	scheduleRows, err := db.Query(context.Background(), query, stanox)
	if err != nil {
		return nil, err
	}
	defer scheduleRows.Close()

	services := make([]ServiceResponse, 0)

	for scheduleRows.Next() {
		var service ServiceResponse
		var scheduleStartDate, scheduleEndDate time.Time
		var scheduleDaysRuns string

		err := scheduleRows.Scan(
			&service.Id,
			&service.TrainUid,
			&service.SignallingId,
			&service.Headcode,
			&service.TrainCategory,
			&scheduleStartDate,
			&scheduleEndDate,
			&scheduleDaysRuns,
			&service.TrainStatus,
			&service.AtocCode,
		)

		if err != nil {
			return nil, err
		}

		if !isScheduleValidForDate(scheduleDaysRuns, scheduleStartDate, scheduleEndDate, date) {
			continue
		}

		startDate := openapi_types.Date{Time: scheduleStartDate}
		endDate := openapi_types.Date{Time: scheduleEndDate}
		service.ScheduleStartDate = &startDate
		service.ScheduleEndDate = &endDate
		service.ScheduleDaysRuns = &scheduleDaysRuns

		var locations []ScheduleLocation
		locationRows, err := db.Query(context.Background(), `
			SELECT id, location_type, tiploc_code, arrival, public_arrival, 
			       departure, public_departure, platform, location_order
			FROM schedule_location 
			WHERE schedule_id = $1 
			ORDER BY location_order
		`, service.Id)

		if err != nil {
			return nil, err
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
				return nil, err
			}
			locations = append(locations, location)
		}
		locationRows.Close()

		if err = locationRows.Err(); err != nil {
			return nil, err
		}

		service.Locations = locations
		services = append(services, service)
	}

	if err = scheduleRows.Err(); err != nil {
		return nil, err
	}

	return services, nil
}
