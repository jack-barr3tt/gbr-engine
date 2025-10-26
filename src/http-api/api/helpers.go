package api

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"time"

	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s *APIServer) GetServicesByHeadcode(headcode string) ([]ServiceResponse, error) {
	var services []ServiceResponse

	scheduleRows, err := s.DB.Query(context.Background(), `
		SELECT s.id, s.train_uid, s.signalling_id, s.headcode, s.train_category, 
		       s.schedule_start_date, s.schedule_end_date, s.schedule_days_runs, 
		       s.train_status, s.atoc_code, t.name
		FROM schedule s
		LEFT JOIN reference_toc t ON s.atoc_code = t.code
		WHERE s.signalling_id = $1 
		ORDER BY s.id
	`, headcode)

	if err != nil {
		s.Logger.Errorw("failed to query services by headcode", "error", err, "headcode", headcode)
		return nil, err
	}
	defer scheduleRows.Close()

	for scheduleRows.Next() {
		var service ServiceResponse
		var scheduleStartDate, scheduleEndDate time.Time
		var atocCode sql.NullString
		var tocName sql.NullString

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
			&atocCode,
			&tocName,
		)

		if err != nil {
			return nil, err
		}

		startDate := openapi_types.Date{Time: scheduleStartDate}
		endDate := openapi_types.Date{Time: scheduleEndDate}
		service.ScheduleStartDate = &startDate
		service.ScheduleEndDate = &endDate

		// Populate operator if available
		if atocCode.Valid && tocName.Valid {
			service.Operator = &Operator{
				Code: atocCode.String,
				Name: tocName.String,
			}
		}

		locationMap, err := fetchStops(s.DB, service.Id)
		if err != nil {
			return nil, err
		}
		service.Locations = locationMap[service.Id]
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
			   s.schedule_start_date, s.schedule_end_date, s.schedule_days_runs, s.train_status, s.atoc_code, rt.name
		FROM schedule s
		JOIN schedule_location sl ON s.id = sl.schedule_id
		JOIN tiploc t ON sl.tiploc_code = t.tiploc_code
		LEFT JOIN reference_toc rt ON s.atoc_code = rt.code
		WHERE t.stanox = $1
	`

	scheduleRows, err := db.Query(context.Background(), query, stanox)
	if err != nil {
		return nil, err
	}
	defer scheduleRows.Close()

	services := make([]ServiceResponse, 0)
	scheduleIDs := make([]int, 0)

	for scheduleRows.Next() {
		var service ServiceResponse
		var scheduleStartDate, scheduleEndDate time.Time
		var scheduleDaysRuns string
		var atocCode sql.NullString
		var tocName sql.NullString

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
			&atocCode,
			&tocName,
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

		// Populate operator if available
		if atocCode.Valid && tocName.Valid {
			service.Operator = &Operator{
				Code: atocCode.String,
				Name: tocName.String,
			}
		}

		scheduleIDs = append(scheduleIDs, service.Id)
		services = append(services, service)
	}

	if err = scheduleRows.Err(); err != nil {
		return nil, err
	}

	if len(scheduleIDs) > 0 {
		allStops, err := fetchStops(db, scheduleIDs...)
		if err != nil {
			return nil, err
		}

		// Assign stops to each service
		for i := range services {
			services[i].Locations = allStops[services[i].Id]
		}
	}

	return services, nil
}

func (s *APIServer) AddRealtimeData(ctx context.Context, services []ServiceResponse, date time.Time) {
	if len(services) == 0 {
		return
	}

	runDate := utils.FormatRunDate(date)

	trainUIDs := make(map[string]bool)
	for i := range services {
		if services[i].TrainUid != "" {
			trainUIDs[strings.TrimSpace(services[i].TrainUid)] = true
		}
	}

	journeys := make(map[string]types.TrainJourney)
	journeyMutex := &sync.Mutex{}
	journeyWg := &sync.WaitGroup{}
	semaphore := make(chan struct{}, 50)

	for trainUID := range trainUIDs {
		journeyWg.Add(1)
		go func(uid string) {
			defer journeyWg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			journey, err := utils.LoadTrainJourney(ctx, s.DB, s.Redis, uid, runDate)
			if err == nil {
				journeyMutex.Lock()
				journeys[uid] = journey
				journeyMutex.Unlock()
			}
		}(trainUID)
	}
	journeyWg.Wait()

	tiplocs := make(map[string]bool)
	for i := range services {
		for j := range services[i].Locations {
			tiplocs[services[i].Locations[j].Location.Tiploc] = true
		}
	}

	tiplocToStanox := make(map[string]string)
	stanoxMutex := &sync.Mutex{}
	stanoxWg := &sync.WaitGroup{}

	stanoxSemaphore := make(chan struct{}, 100)

	for tiploc := range tiplocs {
		stanoxWg.Add(1)
		go func(t string) {
			defer stanoxWg.Done()
			stanoxSemaphore <- struct{}{}
			defer func() { <-stanoxSemaphore }()

			stanox, err := utils.GetStanoxByTiplocCached(ctx, s.DB, s.Redis, t, 24*time.Hour)
			if err == nil {
				stanoxMutex.Lock()
				tiplocToStanox[t] = stanox
				stanoxMutex.Unlock()
			}
		}(tiploc)
	}
	stanoxWg.Wait()

	for i := range services {
		trainUid := strings.TrimSpace(services[i].TrainUid)
		journey, hasJourney := journeys[trainUid]
		if !hasJourney {
			continue
		}

		stanoxToStop := make(map[string]types.Stop)
		for _, stop := range journey.Stops {
			stanoxToStop[stop.Stanox] = stop
		}

		for j := range services[i].Locations {
			location := &services[i].Locations[j]

			stanox, hasStanox := tiplocToStanox[location.Location.Tiploc]
			if !hasStanox {
				continue
			}

			stop, found := stanoxToStop[stanox]
			if !found {
				continue
			}

			if stop.ActualArr != "" {
				formattedTime := utils.FormatActualTime(stop.ActualArr)
				services[i].Locations[j].ActualArrival = &formattedTime

				if location.Arrival != nil && *location.Arrival != "" {
					lateness := utils.CalculateLateness(*location.Arrival, stop.ActualArr)
					services[i].Locations[j].ArrivalLateness = &lateness
				}
			}

			if stop.ActualDep != "" {
				formattedTime := utils.FormatActualTime(stop.ActualDep)
				services[i].Locations[j].ActualDeparture = &formattedTime

				if location.Departure != nil && *location.Departure != "" {
					lateness := utils.CalculateLateness(*location.Departure, stop.ActualDep)
					services[i].Locations[j].DepartureLateness = &lateness
				}
			}
		}
	}
}

func fetchStops(db *pgxpool.Pool, scheduleIDs ...int) (map[int][]ScheduleLocation, error) {
	if len(scheduleIDs) == 0 {
		return make(map[int][]ScheduleLocation), nil
	}

	rows, err := db.Query(context.Background(), `
		SELECT sl.schedule_id, sl.id, sl.location_type, sl.tiploc_code,
			   sl.arrival::text, sl.public_arrival::text,
			   sl.departure::text, sl.public_departure::text,
			   sl.platform, sl.location_order,
			   t.stanox, t.crs_code, t.description
		FROM schedule_location sl
		LEFT JOIN tiploc t ON sl.tiploc_code = t.tiploc_code
		WHERE sl.schedule_id = ANY($1)
		ORDER BY sl.schedule_id, sl.location_order
	`, scheduleIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	locationsBySchedule := make(map[int][]ScheduleLocation)
	for rows.Next() {
		var scheduleID int
		var location ScheduleLocation
		var tiplocCode string
		var stanox, crsCode, fullName sql.NullString

		if err := rows.Scan(
			&scheduleID,
			&location.Id,
			&location.LocationType,
			&tiplocCode,
			&location.Arrival,
			&location.PublicArrival,
			&location.Departure,
			&location.PublicDeparture,
			&location.Platform,
			&location.LocationOrder,
			&stanox,
			&crsCode,
			&fullName,
		); err != nil {
			return nil, err
		}

		// Populate the Location object
		location.Location.Tiploc = tiplocCode
		if stanox.Valid {
			location.Location.Stanox = &stanox.String
		}
		if crsCode.Valid {
			location.Location.Crs = &crsCode.String
		}
		if fullName.Valid {
			location.Location.FullName = &fullName.String
		}

		locationsBySchedule[scheduleID] = append(locationsBySchedule[scheduleID], location)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return locationsBySchedule, nil
}
