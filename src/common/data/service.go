package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	api_types "github.com/jack-barr3tt/gbr-engine/src/common/api-types"
	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type ServiceFilters struct {
	Headcode      *string
	OperatorCode  *string
	PassesThrough []LocationFilter
	Offset        int
	Limit         int
}

type LocationFilter struct {
	Stanox   string
	TimeFrom *time.Time
	TimeTo   *time.Time
}

type ServiceQueryResult struct {
	Services     []api_types.ServiceResponse
	TotalResults int
}

func (dc *DataClient) buildServiceFilter(filters ServiceFilters) (string, []interface{}) {
	conditions := []string{}
	args := []interface{}{}
	argIndex := 1

	if filters.Headcode != nil {
		conditions = append(conditions, fmt.Sprintf("s.signalling_id = $%d", argIndex))
		args = append(args, *filters.Headcode)
		argIndex++
	}

	if filters.OperatorCode != nil {
		conditions = append(conditions, fmt.Sprintf("s.atoc_code = $%d", argIndex))
		args = append(args, *filters.OperatorCode)
		argIndex++
	}

	dateSet := make(map[time.Time]bool)
	for _, locFilter := range filters.PassesThrough {
		if locFilter.TimeFrom != nil {
			dateSet[locFilter.TimeFrom.Truncate(24*time.Hour)] = true
		}
	}

	if len(dateSet) > 0 {
		var minDate, maxDate time.Time
		for d := range dateSet {
			if minDate.IsZero() || d.Before(minDate) {
				minDate = d
			}
			if maxDate.IsZero() || d.After(maxDate) {
				maxDate = d
			}
		}
		conditions = append(conditions, fmt.Sprintf("s.schedule_start_date <= $%d", argIndex))
		args = append(args, maxDate.Format("2006-01-02"))
		argIndex++
		conditions = append(conditions, fmt.Sprintf("s.schedule_end_date >= $%d", argIndex))
		args = append(args, minDate.Format("2006-01-02"))
		argIndex++
	}

	for _, locFilter := range filters.PassesThrough {
		existsClause := fmt.Sprintf("EXISTS (SELECT 1 FROM schedule_location sl WHERE sl.schedule_id = s.id AND sl.tiploc_code IN (SELECT tiploc_code FROM tiploc WHERE stanox = $%d))", argIndex)
		args = append(args, locFilter.Stanox)
		argIndex++

		if locFilter.TimeFrom != nil && locFilter.TimeTo != nil {
			timeFrom := locFilter.TimeFrom.Format("15:04:05")
			timeTo := locFilter.TimeTo.Format("15:04:05")
			existsClause = fmt.Sprintf("EXISTS (SELECT 1 FROM schedule_location sl WHERE sl.schedule_id = s.id AND sl.tiploc_code IN (SELECT tiploc_code FROM tiploc WHERE stanox = $%d) AND (sl.arrival::time BETWEEN $%d AND $%d OR sl.departure::time BETWEEN $%d AND $%d))", argIndex-1, argIndex, argIndex+1, argIndex, argIndex+1)
			args = append(args, timeFrom, timeTo)
			argIndex += 2
		} else if locFilter.TimeFrom != nil {
			existsClause = fmt.Sprintf("EXISTS (SELECT 1 FROM schedule_location sl WHERE sl.schedule_id = s.id AND sl.tiploc_code IN (SELECT tiploc_code FROM tiploc WHERE stanox = $%d) AND (sl.arrival::time >= $%d OR sl.departure::time >= $%d))", argIndex-1, argIndex, argIndex)
			args = append(args, locFilter.TimeFrom.Format("15:04:05"))
			argIndex++
		} else if locFilter.TimeTo != nil {
			existsClause = fmt.Sprintf("EXISTS (SELECT 1 FROM schedule_location sl WHERE sl.schedule_id = s.id AND sl.tiploc_code IN (SELECT tiploc_code FROM tiploc WHERE stanox = $%d) AND (sl.arrival::time <= $%d OR sl.departure::time <= $%d))", argIndex-1, argIndex, argIndex)
			args = append(args, locFilter.TimeTo.Format("15:04:05"))
			argIndex++
		}

		conditions = append(conditions, existsClause)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	return whereClause, args
}

func scanServiceRow(scanner interface {
	Scan(dest ...interface{}) error
}) (*api_types.ServiceResponse, error) {
	var service api_types.ServiceResponse
	var scheduleStartDate, scheduleEndDate time.Time
	var scheduleDaysRuns string
	var trainCategory, trainStatus, atocCode, tocName sql.NullString

	err := scanner.Scan(
		&service.Id,
		&service.TrainUid,
		&service.SignallingId,
		&service.Headcode,
		&trainCategory,
		&scheduleStartDate,
		&scheduleEndDate,
		&scheduleDaysRuns,
		&trainStatus,
		&atocCode,
		&tocName,
	)
	if err != nil {
		return nil, err
	}

	if trainCategory.Valid {
		service.TrainCategory = &trainCategory.String
	}
	if trainStatus.Valid {
		service.TrainStatus = &trainStatus.String
	}

	startDate := openapi_types.Date{Time: scheduleStartDate}
	endDate := openapi_types.Date{Time: scheduleEndDate}
	service.ScheduleStartDate = &startDate
	service.ScheduleEndDate = &endDate
	service.ScheduleDaysRuns = &scheduleDaysRuns

	if atocCode.Valid && tocName.Valid {
		service.Operator = &api_types.Operator{
			Code: atocCode.String,
			Name: tocName.String,
		}
	}

	return &service, nil
}

func (dc *DataClient) enrichServiceWithLocationsAndRealtime(service *api_types.ServiceResponse, date *time.Time) error {
	allStops, err := dc.fetchScheduleLocations(service.Id)
	if err != nil {
		return fmt.Errorf("failed to fetch schedule locations: %w", err)
	}
	service.Locations = allStops[service.Id]

	if date != nil {
		services := []api_types.ServiceResponse{*service}
		dc.AddRealtimeData(services, *date)
		*service = services[0]
	}

	return nil
}

func (dc *DataClient) GetServicesWithFilters(filters ServiceFilters) (*ServiceQueryResult, error) {
	filter, args := dc.buildServiceFilter(filters)
	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT s.id)
		FROM schedule s
		JOIN reference_toc toc ON s.atoc_code = toc.code
		%s
	`, filter)

	var totalResults int
	err := dc.pg.QueryRow(context.Background(), countQuery, args...).Scan(&totalResults)
	if err != nil {
		return nil, fmt.Errorf("failed to count services: %w", err)
	}

	mainQueryArgs := append([]interface{}{}, args...)
	query := fmt.Sprintf(`
		SELECT s.id, s.train_uid, s.signalling_id, s.headcode,
			   s.train_category, s.schedule_start_date, s.schedule_end_date, s.schedule_days_runs,
			   s.train_status, s.atoc_code, toc.name
		FROM schedule s
		JOIN reference_toc toc ON s.atoc_code = toc.code
		%s
		ORDER BY s.schedule_start_date ASC, s.signalling_id ASC
		LIMIT $%d OFFSET $%d
	`, filter, len(mainQueryArgs)+1, len(mainQueryArgs)+2)

	mainQueryArgs = append(mainQueryArgs, filters.Limit, filters.Offset)

	rows, err := dc.pg.Query(context.Background(), query, mainQueryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute service query: %w", err)
	}
	defer rows.Close()

	services := []api_types.ServiceResponse{}
	var scheduleIDs []int

	rowCount := 0

	for rows.Next() {
		rowCount++
		service, err := scanServiceRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan service row: %w", err)
		}

		scheduleIDs = append(scheduleIDs, service.Id)
		services = append(services, *service)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating service rows: %w", err)
	}

	if len(scheduleIDs) > 0 {
		allStops, err := dc.fetchScheduleLocations(scheduleIDs...)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch schedule locations: %w", err)
		}

		totalLocations := 0
		for i := range services {
			services[i].Locations = allStops[services[i].Id]
			totalLocations += len(services[i].Locations)
		}
	}

	return &ServiceQueryResult{
		Services:     services,
		TotalResults: totalResults,
	}, nil
}

func (dc *DataClient) GetServiceByUID(uid string, date *time.Time) (*api_types.ServiceResponse, error) {
	query := `
		SELECT s.id, s.train_uid, s.signalling_id, s.headcode,
			   s.train_category, s.schedule_start_date, s.schedule_end_date, s.schedule_days_runs,
			   s.train_status, s.atoc_code, toc.name
		FROM schedule s
		JOIN reference_toc toc ON s.atoc_code = toc.code
		WHERE s.train_uid = $1
	`

	args := []interface{}{uid}

	if date != nil {
		query += " AND s.schedule_start_date <= $2 AND s.schedule_end_date >= $2"
		args = append(args, *date)
	}

	query += " ORDER BY s.schedule_start_date DESC LIMIT 1"

	service, err := scanServiceRow(dc.pg.QueryRow(context.Background(), query, args...))
	if err != nil {
		return nil, err
	}

	if err := dc.enrichServiceWithLocationsAndRealtime(service, date); err != nil {
		return nil, err
	}

	return service, nil
}

func (dc *DataClient) GetServiceByID(id int, date time.Time) (*api_types.ServiceResponse, error) {
	query := `
		SELECT s.id, s.train_uid, s.signalling_id, s.headcode,
			   s.train_category, s.schedule_start_date, s.schedule_end_date, s.schedule_days_runs,
			   s.train_status, s.atoc_code, toc.name
		FROM schedule s
		JOIN reference_toc toc ON s.atoc_code = toc.code
		WHERE s.id = $1
	`

	service, err := scanServiceRow(dc.pg.QueryRow(context.Background(), query, id))
	if err != nil {
		return nil, err
	}

	if service.ScheduleDaysRuns != nil && service.ScheduleStartDate != nil && service.ScheduleEndDate != nil {
		if !isScheduleValidForDate(*service.ScheduleDaysRuns, service.ScheduleStartDate.Time, service.ScheduleEndDate.Time, date) {
			return nil, sql.ErrNoRows
		}
	}

	if err := dc.enrichServiceWithLocationsAndRealtime(service, &date); err != nil {
		return nil, err
	}

	return service, nil
}

// sortServices sorts services based on the specified criteria
func (dc *DataClient) sortServices(services []api_types.ServiceResponse, filters ServiceFilters) {
	if len(filters.PassesThrough) > 0 {
		// Sort by time at first specified pass location
		firstStanox := filters.PassesThrough[0].Stanox

		// Sort services
		sort.Slice(services, func(i, j int) bool {
			timeI := dc.getTimeAtLocation(services[i], firstStanox)
			timeJ := dc.getTimeAtLocation(services[j], firstStanox)

			if timeI == "" && timeJ != "" {
				return false
			}
			if timeI != "" && timeJ == "" {
				return true
			}

			return timeI < timeJ
		})
	} else {
		// Sort by departure time at origin
		sort.Slice(services, func(i, j int) bool {
			timeI := dc.getOriginDepartureTime(services[i])
			timeJ := dc.getOriginDepartureTime(services[j])

			if timeI == "" && timeJ != "" {
				return false
			}
			if timeI != "" && timeJ == "" {
				return true
			}

			return timeI < timeJ
		})
	}
}

// getTimeAtLocation returns the time (departure or arrival) at a specific location
func (dc *DataClient) getTimeAtLocation(service api_types.ServiceResponse, stanox string) string {
	for _, loc := range service.Locations {
		if loc.Location.Stanox == stanox {
			if loc.Departure != nil && *loc.Departure != "" {
				return *loc.Departure
			}
			if loc.Arrival != nil && *loc.Arrival != "" {
				return *loc.Arrival
			}
		}
	}
	return ""
}

// getOriginDepartureTime returns the departure time at the origin (first location)
func (dc *DataClient) getOriginDepartureTime(service api_types.ServiceResponse) string {
	if len(service.Locations) == 0 {
		return ""
	}

	firstLoc := &service.Locations[0]
	if firstLoc.Departure != nil && *firstLoc.Departure != "" {
		return *firstLoc.Departure
	}
	if firstLoc.Arrival != nil && *firstLoc.Arrival != "" {
		return *firstLoc.Arrival
	}

	return ""
}

// fetchScheduleLocations fetches all schedule locations for the given schedule IDs
func (dc *DataClient) fetchScheduleLocations(scheduleIDs ...int) (map[int][]api_types.ScheduleLocation, error) {
	if len(scheduleIDs) == 0 {
		return make(map[int][]api_types.ScheduleLocation), nil
	}

	rows, err := dc.pg.Query(context.Background(), `
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
	locationsBySchedule := make(map[int][]api_types.ScheduleLocation, len(scheduleIDs))
	estimatedLocationsPerSchedule := 30
	for _, schedID := range scheduleIDs {
		locationsBySchedule[schedID] = make([]api_types.ScheduleLocation, 0, estimatedLocationsPerSchedule)
	}

	locationCount := 0

	for rows.Next() {
		locationCount++
		var scheduleID int
		var location api_types.ScheduleLocation
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
		location.Location.TiplocCodes = append(location.Location.TiplocCodes, tiplocCode)
		if stanox.Valid {
			location.Location.Stanox = stanox.String
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

// GetLocationDetails retrieves full location details for a given stanox
func (dc *DataClient) GetLocationDetails(stanox string) (*api_types.Location, error) {
	rows, err := dc.pg.Query(context.Background(), `
		SELECT description, crs_code, tiploc_code FROM tiploc 
		WHERE stanox = $1
		ORDER BY tiploc_code
	`, stanox)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	details := &api_types.Location{
		TiplocCodes: []string{},
	}

	for rows.Next() {
		var desc, crs, tiploc sql.NullString
		err := rows.Scan(&desc, &crs, &tiploc)
		if err != nil {
			return nil, err
		}

		if desc.Valid && desc.String != "" && details.FullName == nil {
			details.FullName = &desc.String
		}

		if crs.Valid && crs.String != "" && details.Crs == nil {
			details.Crs = &crs.String
		}

		if tiploc.Valid && tiploc.String != "" {
			details.TiplocCodes = append(details.TiplocCodes, tiploc.String)
		}
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	if details.FullName == nil && len(details.TiplocCodes) == 0 {
		return nil, sql.ErrNoRows
	}

	return details, nil
}

// isScheduleValidForDate checks if a schedule runs on a specific day of the week
func isScheduleValidForDate(daysRuns string, startDate, endDate, checkDate time.Time) bool {
	// Check if date is within schedule range
	if checkDate.Before(startDate) || checkDate.After(endDate) {
		return false
	}

	// Check if schedule runs on this day of the week
	// daysRuns is a 7-character string where each character is '0' or '1'
	// representing Monday through Sunday
	if len(daysRuns) != 7 {
		return false
	}

	// Calculate days since start date
	daysSinceStart := int(checkDate.Sub(startDate).Hours() / 24)
	dayOfWeek := (int(startDate.Weekday())+daysSinceStart-1)%7 + 1 // 1=Monday, 7=Sunday
	if dayOfWeek == 0 {
		dayOfWeek = 7
	}

	// Check if the schedule runs on this day (1-indexed, Monday=1)
	return daysRuns[dayOfWeek-1] == '1'
}

func matchesLocationFilters(service api_types.ServiceResponse, filters []LocationFilter, baseDate time.Time) bool {
	locationsByStanox := make(map[string][]api_types.ScheduleLocation)
	for _, loc := range service.Locations {
		locationsByStanox[loc.Location.Stanox] = append(locationsByStanox[loc.Location.Stanox], loc)
	}

	locationTimes := make(map[int]time.Time, len(service.Locations))
	locationDates := make(map[int]time.Time, len(service.Locations))
	currentDate := baseDate
	var prevTime time.Time

	for _, loc := range service.Locations {
		var locTime time.Time
		if loc.Departure != nil && *loc.Departure != "" {
			parsed, err := time.Parse("15:04:05", *loc.Departure)
			if err == nil {
				locTime = parsed
			}
		} else if loc.Arrival != nil && *loc.Arrival != "" {
			parsed, err := time.Parse("15:04:05", *loc.Arrival)
			if err == nil {
				locTime = parsed
			}
		}

		if !prevTime.IsZero() && !locTime.IsZero() {
			if locTime.Hour() < prevTime.Hour() || (locTime.Hour() == prevTime.Hour() && locTime.Minute() < prevTime.Minute()) {
				currentDate = currentDate.Add(24 * time.Hour)
			}
		}

		locationDates[loc.LocationOrder] = currentDate
		locationTimes[loc.LocationOrder] = locTime
		if !locTime.IsZero() {
			prevTime = locTime
		}
	}

	for _, filter := range filters {
		matchFound := false
		locations := locationsByStanox[filter.Stanox]

		for _, loc := range locations {
			if filter.TimeFrom == nil && filter.TimeTo == nil {
				matchFound = true
				break
			}

			actualDate := locationDates[loc.LocationOrder]

			if filter.TimeFrom != nil {
				filterDate := filter.TimeFrom.Truncate(24 * time.Hour)

				if !actualDate.Equal(filterDate) {
					continue
				}

				locTime := locationTimes[loc.LocationOrder]

				if !locTime.IsZero() {
					locTimeSeconds := locTime.Hour()*3600 + locTime.Minute()*60 + locTime.Second()

					if filter.TimeFrom != nil {
						filterTimeFrom := filter.TimeFrom.Hour()*3600 + filter.TimeFrom.Minute()*60 + filter.TimeFrom.Second()
						if locTimeSeconds < filterTimeFrom {
							continue
						}
					}

					if filter.TimeTo != nil {
						filterTimeTo := filter.TimeTo.Hour()*3600 + filter.TimeTo.Minute()*60 + filter.TimeTo.Second()
						if locTimeSeconds > filterTimeTo {
							continue
						}
					}

					matchFound = true
					break
				}
			}
		}

		if !matchFound {
			return false
		}
	}

	return true
}

func (dc *DataClient) AddRealtimeData(services []api_types.ServiceResponse, date time.Time) {
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

			journey, err := utils.LoadTrainJourney(context.Background(), dc.pg, dc.rdb, uid, runDate)
			if err == nil {
				journeyMutex.Lock()
				journeys[uid] = journey
				journeyMutex.Unlock()
			}
		}(trainUID)
	}
	journeyWg.Wait()

	servicesWithJourneys := make([]int, 0, len(journeys))
	for i := range services {
		trainUid := strings.TrimSpace(services[i].TrainUid)
		if _, hasJourney := journeys[trainUid]; hasJourney {
			servicesWithJourneys = append(servicesWithJourneys, i)
		}
	}

	tiplocs := make([]string, 0, 300) // Preallocate with estimated size
	tiplocSet := make(map[string]bool)

	for _, idx := range servicesWithJourneys {
		for j := range services[idx].Locations {
			for _, tiplocCode := range services[idx].Locations[j].Location.TiplocCodes {
				if !tiplocSet[tiplocCode] {
					tiplocSet[tiplocCode] = true
					tiplocs = append(tiplocs, tiplocCode)
				}
			}
		}
	}

	tiplocToStanox := make(map[string]string, len(tiplocs))

	if len(tiplocs) > 0 {
		rows, err := dc.pg.Query(context.Background(), `
			SELECT tiploc_code, stanox
			FROM tiploc
			WHERE tiploc_code = ANY($1) AND stanox IS NOT NULL
		`, tiplocs)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var tiplocCode, stanox string
				if err := rows.Scan(&tiplocCode, &stanox); err == nil {
					tiplocToStanox[tiplocCode] = stanox
				}
			}
			rows.Close()
		}
	}

	for _, idx := range servicesWithJourneys {
		trainUid := strings.TrimSpace(services[idx].TrainUid)
		journey := journeys[trainUid]

		var activationTrainID string
		var activationTime string

		if journey.TrainID != "" {
			activationTrainID = journey.TrainID
			if journey.ActivationTime == "" {
				activationKey := utils.BuildActivationKey(journey.TrainID)
				activationData, err := dc.rdb.Get(context.Background(), activationKey).Result()
				if err == nil {
					var activation map[string]string
					if json.Unmarshal([]byte(activationData), &activation) == nil {
						if aTime, ok := activation["activation_time"]; ok {
							activationTime = aTime
						}
					}
				}
			} else {
				activationTime = journey.ActivationTime
			}
		} else {
			uidKey := fmt.Sprintf("activation:uid:%s", trainUid)
			activationData, err := dc.rdb.Get(context.Background(), uidKey).Result()
			if err == nil {
				var activation map[string]string
				if json.Unmarshal([]byte(activationData), &activation) == nil {
					activationTrainID = activation["train_id"]
					activationTime = activation["activation_time"]
				}
			}
		}

		if activationTrainID != "" {
			services[idx].TrustId = &activationTrainID
		}
		if activationTime != "" {
			services[idx].ActivationTime = &activationTime
		}

		stanoxToStop := make(map[string]types.Stop)
		for _, stop := range journey.Stops {
			stanoxToStop[stop.Stanox] = stop
		}

		for j := range services[idx].Locations {
			location := &services[idx].Locations[j]

			var stanox string
			for _, tiplocCode := range location.Location.TiplocCodes {
				if s, ok := tiplocToStanox[tiplocCode]; ok {
					stanox = s
					break
				}
			}
			if stanox == "" {
				continue
			}

			stop, found := stanoxToStop[stanox]
			if !found {
				continue
			}

			if stop.ActualArr != "" {
				formattedTime := utils.FormatActualTime(stop.ActualArr)
				services[idx].Locations[j].ActualArrival = &formattedTime

				if location.Arrival != nil && *location.Arrival != "" {
					lateness := utils.CalculateLateness(*location.Arrival, stop.ActualArr)
					services[idx].Locations[j].ArrivalLateness = &lateness
				}
			}

			if stop.ActualDep != "" {
				formattedTime := utils.FormatActualTime(stop.ActualDep)
				services[idx].Locations[j].ActualDeparture = &formattedTime

				if location.Departure != nil && *location.Departure != "" {
					lateness := utils.CalculateLateness(*location.Departure, stop.ActualDep)
					services[idx].Locations[j].DepartureLateness = &lateness
				}
			}
		}
	}
}
