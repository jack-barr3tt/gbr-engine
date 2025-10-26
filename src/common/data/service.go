package data

import (
	"context"
	"database/sql"
	"fmt"
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
}

type LocationFilter struct {
	Stanox   string
	TimeFrom *time.Time
	TimeTo   *time.Time
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
		args = append(args, maxDate)
		argIndex++
		conditions = append(conditions, fmt.Sprintf("s.schedule_end_date >= $%d", argIndex))
		args = append(args, minDate)
		argIndex++
	}

	for _, locFilter := range filters.PassesThrough {
		condParts := []string{fmt.Sprintf("EXISTS (SELECT 1 FROM schedule_location sl WHERE sl.schedule_id = s.id AND sl.tiploc_code IN (SELECT t.tiploc_code FROM tiploc t WHERE t.stanox = $%d)", argIndex)}
		args = append(args, locFilter.Stanox)
		argIndex++

		if locFilter.TimeFrom != nil && locFilter.TimeTo != nil {
			timeFrom := locFilter.TimeFrom.Format("15:04:05")
			timeTo := locFilter.TimeTo.Format("15:04:05")

			condParts = append(condParts, fmt.Sprintf("((sl.arrival::time BETWEEN $%d AND $%d) OR (sl.departure::time BETWEEN $%d AND $%d))", argIndex, argIndex+1, argIndex, argIndex+1))
			args = append(args, timeFrom, timeTo)
			argIndex += 2
		} else if locFilter.TimeFrom != nil {
			condParts = append(condParts, fmt.Sprintf("((sl.arrival::time >= $%d) OR (sl.departure::time >= $%d))", argIndex, argIndex))
			args = append(args, locFilter.TimeFrom.Format("15:04:05"))
			argIndex++
		} else if locFilter.TimeTo != nil {
			condParts = append(condParts, fmt.Sprintf("((sl.arrival::time <= $%d) OR (sl.departure::time <= $%d))", argIndex, argIndex))
			args = append(args, locFilter.TimeTo.Format("15:04:05"))
			argIndex++
		}

		conditions = append(conditions, strings.Join(condParts, " AND ")+")")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	return whereClause, args
}

func (dc *DataClient) GetServicesWithFilters(filters ServiceFilters) ([]api_types.ServiceResponse, error) {
	filter, args := dc.buildServiceFilter(filters)

	query := fmt.Sprintf(`
		SELECT s.id, s.train_uid, s.signalling_id, s.headcode,
			   s.train_category, s.schedule_start_date, s.schedule_end_date, s.schedule_days_runs,
			   s.train_status, s.atoc_code, toc.name
		FROM schedule s
		JOIN reference_toc toc ON s.atoc_code = toc.code
		%s
	`, filter)

	rows, err := dc.pg.Query(context.Background(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute service query: %w", err)
	}
	defer rows.Close()

	services := []api_types.ServiceResponse{}
	var scheduleIDs []int

	rowCount := 0

	for rows.Next() {
		rowCount++
		var service api_types.ServiceResponse
		var scheduleStartDate, scheduleEndDate time.Time
		var scheduleDaysRuns string
		var trainCategory, trainStatus, atocCode, tocName sql.NullString

		err := rows.Scan(
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
			return nil, fmt.Errorf("failed to scan service row: %w", err)
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

		scheduleIDs = append(scheduleIDs, service.Id)
		services = append(services, service)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating service rows: %w", err)
	}

	// Fetch all stops for all services
	if len(scheduleIDs) > 0 {
		allStops, err := dc.fetchScheduleLocations(scheduleIDs...)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch schedule locations: %w", err)
		}

		for i := range services {
			services[i].Locations = allStops[services[i].Id]
		}
	}

	if len(filters.PassesThrough) > 0 {
		var earliestDate time.Time
		for _, locFilter := range filters.PassesThrough {
			if locFilter.TimeFrom != nil {
				checkDate := locFilter.TimeFrom.Truncate(24 * time.Hour)
				if earliestDate.IsZero() || checkDate.Before(earliestDate) {
					earliestDate = checkDate
				}
			}
		}

		if !earliestDate.IsZero() {
			validServices := make([]api_types.ServiceResponse, 0, len(services))
			for _, service := range services {
				if service.ScheduleDaysRuns == nil || service.ScheduleStartDate == nil || service.ScheduleEndDate == nil {
					continue
				}

				if !isScheduleValidForDate(*service.ScheduleDaysRuns, service.ScheduleStartDate.Time, service.ScheduleEndDate.Time, earliestDate) {
					continue
				}

				if matchesLocationFilters(service, filters.PassesThrough, earliestDate) {
					validServices = append(validServices, service)
				}
			}
			services = validServices
		}
	}

	return services, nil
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

	locationsBySchedule := make(map[int][]api_types.ScheduleLocation)
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

	locationDates := make(map[int]time.Time)
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

				var locTime time.Time
				if loc.Arrival != nil && *loc.Arrival != "" {
					parsed, _ := time.Parse("15:04:05", *loc.Arrival)
					locTime = parsed
				}
				if loc.Departure != nil && *loc.Departure != "" {
					parsed, _ := time.Parse("15:04:05", *loc.Departure)
					if locTime.IsZero() || parsed.After(locTime) {
						locTime = parsed
					}
				}

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
	tiplocs := make(map[string]bool)
	for i := range services {
		for j := range services[i].Locations {
			for _, tiplocCode := range services[i].Locations[j].Location.TiplocCodes {
				tiplocs[tiplocCode] = true
			}
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

			stanox, err := dc.GetStanoxByTiploc(t)
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
