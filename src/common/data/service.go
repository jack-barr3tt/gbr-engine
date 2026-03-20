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
	Date          *time.Time
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

func appendTimeArg(args *[]interface{}, argIndex *int, t *time.Time) int {
	if t == nil {
		return 0
	}
	idx := *argIndex
	*args = append(*args, t.Format("15:04:05"))
	*argIndex++
	return idx
}

func primaryStanox(filters []LocationFilter) string {
	if len(filters) == 0 {
		return ""
	}
	candidate := filters[0].Stanox
	for _, filter := range filters {
		if filter.TimeFrom != nil || filter.TimeTo != nil {
			return filter.Stanox
		}
	}
	return candidate
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func queryDateFromFilters(filters ServiceFilters) *time.Time {
	for _, filter := range filters.PassesThrough {
		if filter.TimeFrom != nil {
			date := startOfDay(*filter.TimeFrom)
			return &date
		}
		if filter.TimeTo != nil {
			date := startOfDay(*filter.TimeTo)
			return &date
		}
	}

	// `date` is an explicit schedule date used for headcode-only searches.
	// Prefer any time filters from `passes_through` when present.
	if filters.Date != nil {
		date := startOfDay(*filters.Date)
		return &date
	}

	return nil
}

func needsPreviousDay(filters ServiceFilters) bool {
	const cutoffMinutes = 5 * 60 // 05:00
	earliest := 24 * 60
	hasTime := false

	for _, filter := range filters.PassesThrough {
		candidates := []*time.Time{filter.TimeFrom, filter.TimeTo}
		for _, t := range candidates {
			if t == nil {
				continue
			}
			minutes := t.Hour()*60 + t.Minute()
			if minutes < earliest {
				earliest = minutes
			}
			hasTime = true
		}
	}

	if !hasTime {
		return false
	}

	return earliest < cutoffMinutes
}

func buildLocationTimeClause(timeFromIdx, timeToIdx int) string {
	switch {
	case timeFromIdx > 0 && timeToIdx > 0:
		return fmt.Sprintf(`
			AND (
				sl.arrival::time BETWEEN $%d AND $%d
				OR sl.departure::time BETWEEN $%d AND $%d
			)
		`, timeFromIdx, timeToIdx, timeFromIdx, timeToIdx)
	case timeFromIdx > 0:
		return fmt.Sprintf(`
			AND (
				sl.arrival::time >= $%d
				OR sl.departure::time >= $%d
			)
		`, timeFromIdx, timeFromIdx)
	case timeToIdx > 0:
		return fmt.Sprintf(`
			AND (
				sl.arrival::time <= $%d
				OR sl.departure::time <= $%d
			)
		`, timeToIdx, timeToIdx)
	default:
		return ""
	}
}

func buildLocationExistsClause(stanoxParamIndex, timeFromIdx, timeToIdx int) string {
	timeClause := buildLocationTimeClause(timeFromIdx, timeToIdx)
	return fmt.Sprintf(`
		EXISTS (
			SELECT 1
			FROM schedule_location sl
			WHERE sl.schedule_id = s.id
			  AND sl.tiploc_code IN (SELECT tiploc_code FROM tiploc WHERE stanox = $%d)%s
		)
	`, stanoxParamIndex, timeClause)
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

	var (
		minDate        time.Time
		maxDate        time.Time
		queryDateValue time.Time
		queryDate      *time.Time
	)
	includePrevDay := false
	for _, locFilter := range filters.PassesThrough {
		if locFilter.TimeFrom != nil {
			t := locFilter.TimeFrom.Truncate(24 * time.Hour)
			if minDate.IsZero() || t.Before(minDate) {
				minDate = t
			}
			if maxDate.IsZero() || t.After(maxDate) {
				maxDate = t
			}
			if queryDate == nil {
				queryDateValue = t
				queryDate = &queryDateValue
			}
		}
	}

	// For headcode-only searches we may not have time filters from `passes_through`.
	// In that case, fall back to the explicit `date` parameter for schedule filtering
	// and Gemini enrichment.
	if queryDate == nil && filters.Date != nil {
		t := startOfDay(*filters.Date)
		minDate = t
		maxDate = t
		queryDateValue = t
		queryDate = &queryDateValue
	}

	if queryDate != nil {
		includePrevDay = needsPreviousDay(filters)
	}

	if !minDate.IsZero() {
		conditions = append(conditions, fmt.Sprintf("s.schedule_start_date <= $%d", argIndex))
		args = append(args, maxDate.Format("2006-01-02"))
		argIndex++
		conditions = append(conditions, fmt.Sprintf("s.schedule_end_date >= $%d", argIndex))
		args = append(args, minDate.Format("2006-01-02"))
		argIndex++

		if queryDate != nil {
			dayOfWeek := int(queryDate.Weekday())
			if dayOfWeek == 0 {
				dayOfWeek = 6
			} else {
				dayOfWeek--
			}
			dayClause := fmt.Sprintf("SUBSTR(s.schedule_days_runs, %d, 1) = '1'", dayOfWeek+1)
			if includePrevDay {
				prevDay := (dayOfWeek + 6) % 7
				dayClause = fmt.Sprintf("(%s OR SUBSTR(s.schedule_days_runs, %d, 1) = '1')", dayClause, prevDay+1)
			}
			conditions = append(conditions, dayClause)
		}
	}

	for _, locFilter := range filters.PassesThrough {
		stanoxParamIndex := argIndex
		args = append(args, locFilter.Stanox)
		argIndex++

		timeFromIdx := appendTimeArg(&args, &argIndex, locFilter.TimeFrom)
		timeToIdx := appendTimeArg(&args, &argIndex, locFilter.TimeTo)

		conditions = append(conditions, buildLocationExistsClause(stanoxParamIndex, timeFromIdx, timeToIdx))
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	return whereClause, args
}

func buildRankOrderClause(stpPriority string, queryDateParam int) string {
	if queryDateParam == 0 {
		return fmt.Sprintf(`
			%s,
			s.schedule_start_date DESC,
			s.schedule_end_date DESC,
			s.id DESC
		`, stpPriority)
	}

	return fmt.Sprintf(`
		CASE WHEN s.schedule_start_date <= $%d THEN 0 ELSE 1 END,
		CASE WHEN s.schedule_start_date <= $%d THEN s.schedule_start_date END DESC,
		CASE WHEN s.schedule_start_date > $%d THEN s.schedule_start_date END ASC,
		%s,
		s.schedule_end_date DESC,
		s.id DESC
	`, queryDateParam, queryDateParam, queryDateParam, stpPriority)
}

func scanServiceRow(scanner interface {
	Scan(dest ...interface{}) error
}) (*api_types.ServiceResponse, error) {
	var service api_types.ServiceResponse
	var scheduleStartDate, scheduleEndDate time.Time
	var scheduleDaysRuns string
	var trainCategory, trainStatus, atocCode, tocName sql.NullString
	var stpIndicator string

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
		&stpIndicator,
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

	cancelled := stpIndicator == "C"
	service.Cancelled = &cancelled

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

func routeTiplocs(service api_types.ServiceResponse) (originTiploc string, destTiploc string) {
	if len(service.Locations) == 0 {
		return "", ""
	}

	origin := service.Locations[0]
	dest := service.Locations[len(service.Locations)-1]

	if len(origin.Location.TiplocCodes) > 0 {
		originTiploc = origin.Location.TiplocCodes[0]
	}
	if len(dest.Location.TiplocCodes) > 0 {
		destTiploc = dest.Location.TiplocCodes[0]
	}

	return originTiploc, destTiploc
}

func (dc *DataClient) GetServicesWithFilters(filters ServiceFilters) (*ServiceQueryResult, error) {
	filter, args := dc.buildServiceFilter(filters)
	queryDate := queryDateFromFilters(filters)
	queryDateParam := 0
	stpPriority := `
		CASE s.stp_indicator
			WHEN 'C' THEN 0
			WHEN 'V' THEN 1
			WHEN 'O' THEN 2
			WHEN 'N' THEN 3
			WHEN 'P' THEN 4
			ELSE 5
		END
	`
	orderClause := buildRankOrderClause(stpPriority, 0)
	if queryDate != nil {
		queryDateParam = len(args) + 1
		orderClause = buildRankOrderClause(stpPriority, queryDateParam)
	}
	rankedCTE := fmt.Sprintf(`
		WITH filtered AS (
			SELECT
				s.id,
				s.train_uid,
				s.signalling_id,
				s.headcode,
				s.train_category,
				s.schedule_start_date,
				s.schedule_end_date,
				s.schedule_days_runs,
				s.train_status,
				s.atoc_code,
				toc.name AS toc_name,
				s.stp_indicator,
				ROW_NUMBER() OVER (
					PARTITION BY s.train_uid
					ORDER BY
						%s
				) AS stp_rank
			FROM schedule s
			JOIN reference_toc toc ON s.atoc_code = toc.code
			%s
		)
	`, orderClause, filter)
	countQuery := fmt.Sprintf(`
		%s
		SELECT COUNT(*)
		FROM filtered
		WHERE stp_rank = 1
	`, rankedCTE)

	countArgs := append([]interface{}{}, args...)
	if queryDate != nil {
		countArgs = append(countArgs, *queryDate)
	}
	var totalResults int
	err := dc.pg.QueryRow(context.Background(), countQuery, countArgs...).Scan(&totalResults)
	if err != nil {
		return nil, fmt.Errorf("failed to count services: %w", err)
	}

	mainQueryArgs := append([]interface{}{}, args...)
	if queryDate != nil {
		mainQueryArgs = append(mainQueryArgs, *queryDate)
	}

	targetStanox := primaryStanox(filters.PassesThrough)

	orderJoin := ""
	resultOrderClause := "ORDER BY f.schedule_start_date ASC, f.signalling_id ASC"
	if targetStanox != "" {
		targetStanoxParam := len(mainQueryArgs) + 1
		mainQueryArgs = append(mainQueryArgs, targetStanox)
		orderJoin = fmt.Sprintf(`
			LEFT JOIN LATERAL (
				SELECT
					COALESCE(sl.departure::text, sl.arrival::text) AS order_time
				FROM schedule_location sl
				WHERE sl.schedule_id = f.id
					AND sl.tiploc_code IN (SELECT tiploc_code FROM tiploc WHERE stanox = $%d)
				ORDER BY sl.location_order
				LIMIT 1
			) order_loc ON TRUE
		`, targetStanoxParam)
		resultOrderClause = `
			ORDER BY
				order_loc.order_time IS NULL,
				order_loc.order_time,
				f.schedule_start_date,
				f.signalling_id
		`
	}
	query := fmt.Sprintf(`
		%s
		SELECT f.id, f.train_uid, f.signalling_id, f.headcode,
			   f.train_category, f.schedule_start_date, f.schedule_end_date, f.schedule_days_runs,
			   f.train_status, f.atoc_code, f.toc_name, f.stp_indicator
		FROM filtered f
		%s
		WHERE f.stp_rank = 1
		%s
		LIMIT $%d OFFSET $%d
	`, rankedCTE, orderJoin, resultOrderClause, len(mainQueryArgs)+1, len(mainQueryArgs)+2)

	mainQueryArgs = append(mainQueryArgs, filters.Limit, filters.Offset)

	rows, err := dc.pg.Query(context.Background(), query, mainQueryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute service query: %w", err)
	}
	defer rows.Close()

	services := []api_types.ServiceResponse{}
	var scheduleIDs []int

	for rows.Next() {
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

		for i := range services {
			services[i].Locations = allStops[services[i].Id]
		}

		dc.sortServices(services, filters)
		services = filterServicesByTargetDate(services, filters)
	}

	// Best-effort Gemini enrichment for list view: use target date from filters if available, otherwise today.
	if len(services) > 0 {
		targetDate := queryDateFromFilters(filters)
		if targetDate == nil {
			now := time.Now().UTC()
			targetDate = &now
		}

		for i := range services {
			originTiploc, destTiploc := routeTiplocs(services[i])
			current, _, err := dc.GetGeminiForService(services[i].SignallingId, *targetDate, originTiploc, destTiploc)
			if err == nil && len(current) > 0 {
				currentCopy := current
				services[i].GeminiResourceGroups = &currentCopy
			}
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
			   s.train_status, s.atoc_code, toc.name, s.stp_indicator
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

	// Enrich with Gemini allocations when a date is available.
	if date != nil {
		originTiploc, destTiploc := routeTiplocs(*service)
		current, history, err := dc.GetGeminiForService(service.SignallingId, *date, originTiploc, destTiploc)
		if err == nil {
			currentCopy := current
			historyCopy := history
			service.GeminiResourceGroups = &currentCopy
			service.GeminiHistory = &historyCopy
		}
	}

	return service, nil
}

func (dc *DataClient) GetServiceByID(id int, date time.Time) (*api_types.ServiceResponse, error) {
	query := `
		SELECT s.id, s.train_uid, s.signalling_id, s.headcode,
			   s.train_category, s.schedule_start_date, s.schedule_end_date, s.schedule_days_runs,
			   s.train_status, s.atoc_code, toc.name, s.stp_indicator
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

	// Enrich with Gemini allocations for this specific date.
	originTiploc, destTiploc := routeTiplocs(*service)
	if current, history, err := dc.GetGeminiForService(service.SignallingId, date, originTiploc, destTiploc); err == nil {
		currentCopy := current
		historyCopy := history
		service.GeminiResourceGroups = &currentCopy
		service.GeminiHistory = &historyCopy
	}

	return service, nil
}

// sortServices sorts services based on the specified criteria
func (dc *DataClient) sortServices(services []api_types.ServiceResponse, filters ServiceFilters) {
	if len(services) < 2 {
		return
	}

	var extractTime func(api_types.ServiceResponse) string
	if stanox := primaryStanox(filters.PassesThrough); stanox != "" {
		extractTime = func(service api_types.ServiceResponse) string {
			return dc.getTimeAtLocation(service, stanox)
		}
	} else {
		extractTime = dc.getOriginDepartureTime
	}

	sort.SliceStable(services, func(i, j int) bool {
		timeI := extractTime(services[i])
		timeJ := extractTime(services[j])

		switch {
		case timeI == "" && timeJ != "":
			return false
		case timeI != "" && timeJ == "":
			return true
		default:
			return timeI < timeJ
		}
	})
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

func parseScheduleClock(value string) (time.Time, error) {
	layouts := []string{"15:04:05", "15:04"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid schedule time: %s", value)
}

func locationPlannedTime(loc api_types.ScheduleLocation) *string {
	if loc.Departure != nil && *loc.Departure != "" {
		return loc.Departure
	}
	if loc.Arrival != nil && *loc.Arrival != "" {
		return loc.Arrival
	}
	if loc.PublicDeparture != nil && *loc.PublicDeparture != "" {
		return loc.PublicDeparture
	}
	if loc.PublicArrival != nil && *loc.PublicArrival != "" {
		return loc.PublicArrival
	}
	return nil
}

func eventDateTimeForStanox(service api_types.ServiceResponse, stanox string, targetDate time.Time) (*time.Time, bool) {
	for _, loc := range service.Locations {
		if loc.Location.Stanox != stanox {
			continue
		}
		timeStr := locationPlannedTime(loc)
		if timeStr == nil || *timeStr == "" {
			return nil, false
		}
		parsed, err := parseScheduleClock(*timeStr)
		if err != nil {
			return nil, false
		}
		eventTime := time.Date(
			targetDate.Year(),
			targetDate.Month(),
			targetDate.Day(),
			parsed.Hour(),
			parsed.Minute(),
			parsed.Second(),
			0,
			targetDate.Location(),
		)
		return &eventTime, true
	}
	return nil, false
}

func filterServicesByTargetDate(services []api_types.ServiceResponse, filters ServiceFilters) []api_types.ServiceResponse {
	stanox := primaryStanox(filters.PassesThrough)
	if stanox == "" {
		return services
	}
	targetDate := queryDateFromFilters(filters)
	if targetDate == nil {
		return services
	}
	dayStart := startOfDay(*targetDate)
	dayEnd := dayStart.Add(24 * time.Hour)

	filtered := make([]api_types.ServiceResponse, 0, len(services))
	for _, svc := range services {
		if ts, ok := eventDateTimeForStanox(svc, stanox, *targetDate); ok {
			if !ts.Before(dayStart) && ts.Before(dayEnd) {
				filtered = append(filtered, svc)
			}
			continue
		}
		filtered = append(filtered, svc)
	}

	return filtered
}

// GetGeminiForService fetches current and historical Gemini allocations for a service by signalling ID and date.
// Gemini's OperationalTrainNumber matches the operational train ID (signalling_id), not the internal headcode.
func (dc *DataClient) GetGeminiForService(
	signallingID string,
	date time.Time,
	originTiploc string,
	destTiploc string,
) (current []string, history []api_types.GeminiSnapshot, err error) {
	routeClause := ""
	args := []interface{}{signallingID, date}

	// Gemini allocations for a single operational_train_number can include multiple diagrams
	// (different origins/destinations). Filter to the route shown in the UI.
	if originTiploc != "" && destTiploc != "" {
		routeClause = `
		  AND (
		    (ga.train_origin_tiploc = $3 AND ga.train_dest_tiploc = $4)
		     OR
		    (ga.allocation_origin_tiploc = $3 AND ga.allocation_dest_tiploc = $4)
		  )
		`
		args = append(args, originTiploc, destTiploc)
	}

	query := fmt.Sprintf(`
		SELECT
			ga.resource_group_id,
			ga.message_identifier,
			COALESCE(
				ga.message_date_time,
				gm.message_date_time,
				gm.received_at,
				ga.created_at
			) AS effective_message_time
		FROM gemini_allocation ga
		LEFT JOIN gemini_message gm
		  ON ga.message_identifier = gm.message_identifier
		WHERE ga.operational_train_number = $1
		  AND ga.start_date = $2
		%s
		ORDER BY effective_message_time ASC
	`, routeClause)

	rows, err := dc.pg.Query(context.Background(), query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	type snapshotKey struct {
		Identifier string
		Time       time.Time
	}

	snapshotsMap := make(map[snapshotKey]map[string]struct{})

	for rows.Next() {
		var rgID string
		var msgID sql.NullString
		var effectiveTime time.Time
		if err := rows.Scan(&rgID, &msgID, &effectiveTime); err != nil {
			return nil, nil, err
		}

		// message_date_time may be NULL for older ingested rows if parsing failed.
		// Use COALESCE(..., received_at, created_at) so we still get ordering + grouping.
		if rgID == "" || !msgID.Valid {
			continue
		}

		effectiveTime = effectiveTime.UTC()
		key := snapshotKey{
			Identifier: msgID.String,
			Time:       effectiveTime,
		}
		if _, ok := snapshotsMap[key]; !ok {
			snapshotsMap[key] = make(map[string]struct{})
		}
		snapshotsMap[key][rgID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	if len(snapshotsMap) == 0 {
		return nil, nil, nil
	}

	// Convert map to ordered slice
	type kv struct {
		Key  snapshotKey
		Vals []string
	}
	tmp := make([]kv, 0, len(snapshotsMap))
	for k, set := range snapshotsMap {
		var ids []string
		for id := range set {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		tmp = append(tmp, kv{Key: k, Vals: ids})
	}
	sort.Slice(tmp, func(i, j int) bool {
		return tmp[i].Key.Time.Before(tmp[j].Key.Time)
	})

	history = make([]api_types.GeminiSnapshot, 0, len(tmp))
	for _, entry := range tmp {
		t := entry.Key.Time.UTC()
		vals := entry.Vals
		history = append(history, api_types.GeminiSnapshot{
			MessageDateTime:  &t,
			ResourceGroupIds: &vals,
		})
	}

	// Current = resource groups from latest snapshot
	if last := history[len(history)-1].ResourceGroupIds; last != nil {
		current = *last
	}
	return current, history, nil
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

	for rows.Next() {
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
