package api

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

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
		s.Logger.Errorw("failed to query services by headcode", "error", err, "headcode", headcode)
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

		locations, err := fetchStops(s.DB, service.Id)
		if err != nil {
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

		if !utils.IsScheduleValidForDate(scheduleDaysRuns, scheduleStartDate, scheduleEndDate, date) {
			continue
		}

		startDate := openapi_types.Date{Time: scheduleStartDate}
		endDate := openapi_types.Date{Time: scheduleEndDate}
		service.ScheduleStartDate = &startDate
		service.ScheduleEndDate = &endDate
		service.ScheduleDaysRuns = &scheduleDaysRuns

		locations, err := fetchStops(db, service.Id)
		if err != nil {
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

func (s *APIServer) AddRealtimeData(ctx context.Context, service *ServiceResponse, date time.Time) {
	if service == nil || service.TrainUid == "" {
		return
	}

	runDate := utils.FormatRunDate(date)
	trainUid := strings.TrimSpace(service.TrainUid)

	journey, err := utils.LoadTrainJourney(ctx, s.DB, s.Redis, trainUid, runDate)
	if err != nil {
		s.Logger.Debugw("no realtime data for train", "train_uid", trainUid, "error", err)
		return
	}

	stanoxToStop := make(map[string]types.Stop)
	for _, stop := range journey.Stops {
		stanoxToStop[stop.Stanox] = stop
	}

	for i := range service.Locations {
		location := &service.Locations[i]

		stanox, err := utils.GetStanoxByTiplocCached(ctx, s.DB, s.Redis, location.TiplocCode, 24*time.Hour)
		if err != nil {
			continue
		}

		stop, found := stanoxToStop[stanox]
		if !found {
			continue
		}

		if stop.ActualArr != "" {
			formattedTime := utils.FormatActualTime(stop.ActualArr)
			service.Locations[i].ActualArrival = &formattedTime

			if location.Arrival != nil && *location.Arrival != "" {
				lateness := utils.CalculateLateness(*location.Arrival, stop.ActualArr)
				service.Locations[i].ArrivalLateness = &lateness
			}
		}

		if stop.ActualDep != "" {
			formattedTime := utils.FormatActualTime(stop.ActualDep)
			service.Locations[i].ActualDeparture = &formattedTime

			if location.Departure != nil && *location.Departure != "" {
				lateness := utils.CalculateLateness(*location.Departure, stop.ActualDep)
				service.Locations[i].DepartureLateness = &lateness
			}
		}
	}
}

func fetchStops(db *pgxpool.Pool, scheduleID int) ([]ScheduleLocation, error) {
	rows, err := db.Query(context.Background(), `
		SELECT id, location_type, tiploc_code,
			   arrival::text, public_arrival::text,
			   departure::text, public_departure::text,
			   platform, location_order
		FROM schedule_location 
		WHERE schedule_id = $1 
		ORDER BY location_order
	`, scheduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locations []ScheduleLocation
	for rows.Next() {
		var location ScheduleLocation
		if err := rows.Scan(
			&location.Id,
			&location.LocationType,
			&location.TiplocCode,
			&location.Arrival,
			&location.PublicArrival,
			&location.Departure,
			&location.PublicDeparture,
			&location.Platform,
			&location.LocationOrder,
		); err != nil {
			return nil, err
		}
		locations = append(locations, location)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return locations, nil
}
