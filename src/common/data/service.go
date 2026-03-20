package data

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
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
	Timings      map[string]int64 // stage name → milliseconds
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

func buildSingleTimeClause(col string, timeFromIdx, timeToIdx int) string {
	switch {
	case timeFromIdx > 0 && timeToIdx > 0:
		return fmt.Sprintf("%s BETWEEN $%d AND $%d", col, timeFromIdx, timeToIdx)
	case timeFromIdx > 0:
		return fmt.Sprintf("%s >= $%d", col, timeFromIdx)
	case timeToIdx > 0:
		return fmt.Sprintf("%s <= $%d", col, timeToIdx)
	default:
		return "TRUE"
	}
}

func buildLocationExistsClause(stanoxParamIndex, timeFromIdx, timeToIdx int) string {
	tiplocSubq := fmt.Sprintf("SELECT tiploc_code FROM tiploc WHERE stanox = $%d", stanoxParamIndex)

	if timeFromIdx == 0 && timeToIdx == 0 {
		return fmt.Sprintf(`
			s.id IN (
				SELECT sl.schedule_id
				FROM schedule_location sl
				WHERE sl.tiploc_code IN (%s)
			)
		`, tiplocSubq)
	}

	arrivalCond := buildSingleTimeClause("sl.arrival", timeFromIdx, timeToIdx)
	departureCond := buildSingleTimeClause("sl.departure", timeFromIdx, timeToIdx)

	return fmt.Sprintf(`
		s.id IN (
			SELECT sl.schedule_id FROM schedule_location sl
			WHERE sl.tiploc_code IN (%s) AND %s
			UNION
			SELECT sl.schedule_id FROM schedule_location sl
			WHERE sl.tiploc_code IN (%s) AND %s
		)
	`, tiplocSubq, arrivalCond, tiplocSubq, departureCond)
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

func scanServiceRowWithVisitTime(scanner interface {
	Scan(dest ...interface{}) error
}) (*api_types.ServiceResponse, string, error) {
	var service api_types.ServiceResponse
	var scheduleStartDate, scheduleEndDate time.Time
	var scheduleDaysRuns string
	var trainCategory, trainCategoryDescription, trainStatus, atocCode, tocName sql.NullString
	var stpIndicator string
	var visitTime sql.NullString

	err := scanner.Scan(
		&service.Id,
		&service.TrainUid,
		&service.SignallingId,
		&service.Headcode,
		&trainCategory,
		&trainCategoryDescription,
		&scheduleStartDate,
		&scheduleEndDate,
		&scheduleDaysRuns,
		&trainStatus,
		&atocCode,
		&tocName,
		&stpIndicator,
		&visitTime,
	)
	if err != nil {
		return nil, "", err
	}

	if trainCategory.Valid {
		service.TrainCategory = &trainCategory.String
	}
	if trainCategoryDescription.Valid {
		service.TrainCategoryDescription = &trainCategoryDescription.String
	}
	if trainStatus.Valid {
		service.TrainStatus = &trainStatus.String
	}

	startDate := openapi_types.Date{Time: scheduleStartDate}
	endDate := openapi_types.Date{Time: scheduleEndDate}
	service.ScheduleStartDate = &startDate
	service.ScheduleEndDate = &endDate
	service.ScheduleDaysRuns = &scheduleDaysRuns

	operatorCode := strings.TrimSpace(atocCode.String)
	operatorName := resolveOperatorName(tocName)
	if atocCode.Valid && operatorCode != "" && operatorName != "" {
		service.Operator = &api_types.Operator{
			Code: operatorCode,
			Name: operatorName,
		}
	}

	cancelled := stpIndicator == "C"
	service.Cancelled = &cancelled

	vt := ""
	if visitTime.Valid {
		vt = visitTime.String
	}
	return &service, vt, nil
}

func scanServiceRow(scanner interface {
	Scan(dest ...interface{}) error
}) (*api_types.ServiceResponse, error) {
	var service api_types.ServiceResponse
	var scheduleStartDate, scheduleEndDate time.Time
	var scheduleDaysRuns string
	var trainCategory, trainCategoryDescription, trainStatus, atocCode, tocName sql.NullString
	var stpIndicator string

	err := scanner.Scan(
		&service.Id,
		&service.TrainUid,
		&service.SignallingId,
		&service.Headcode,
		&trainCategory,
		&trainCategoryDescription,
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
	if trainCategoryDescription.Valid {
		service.TrainCategoryDescription = &trainCategoryDescription.String
	}
	if trainStatus.Valid {
		service.TrainStatus = &trainStatus.String
	}

	startDate := openapi_types.Date{Time: scheduleStartDate}
	endDate := openapi_types.Date{Time: scheduleEndDate}
	service.ScheduleStartDate = &startDate
	service.ScheduleEndDate = &endDate
	service.ScheduleDaysRuns = &scheduleDaysRuns

	operatorCode := strings.TrimSpace(atocCode.String)
	operatorName := resolveOperatorName(tocName)
	if atocCode.Valid && operatorCode != "" && operatorName != "" {
		service.Operator = &api_types.Operator{
			Code: operatorCode,
			Name: operatorName,
		}
	}

	cancelled := stpIndicator == "C"
	service.Cancelled = &cancelled

	return &service, nil
}

func resolveOperatorName(tocName sql.NullString) string {
	if tocName.Valid {
		trimmed := strings.TrimSpace(tocName.String)
		normalized := strings.ToLower(trimmed)
		if trimmed != "" && !strings.Contains(normalized, "unknown") && !strings.Contains(normalized, "unkown") {
			return trimmed
		}
	}

	return ""
}

func isUnknownOperatorName(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	return normalized == "" || strings.Contains(normalized, "unknown") || strings.Contains(normalized, "unkown")
}

func (dc *DataClient) resolveTOCNames(codes []string) map[string]string {
	if len(codes) == 0 {
		return map[string]string{}
	}

	rows, err := dc.pg.Query(context.Background(), `
		SELECT code, name
		FROM (
			SELECT DISTINCT UPPER(TRIM(c.code)) AS code,
				COALESCE(
					CASE
						WHEN LOWER(TRIM(toc.name)) LIKE '%unknown%' OR LOWER(TRIM(toc.name)) LIKE '%unkown%' THEN NULL
						ELSE NULLIF(TRIM(toc.name), '')
					END,
					btoc.description
				) AS name
			FROM (
				SELECT DISTINCT UPPER(TRIM(unnest($1::text[]))) AS code
			) c
			LEFT JOIN reference_toc toc ON UPPER(TRIM(toc.code)) = c.code
			LEFT JOIN LATERAL (
				SELECT r.description
				FROM bplan_ref r
				WHERE r.category = 'TOC'
				  AND UPPER(TRIM(r.subcode)) = c.code
				  AND NULLIF(TRIM(r.description), '') IS NOT NULL
				ORDER BY r.id
				LIMIT 1
			) btoc ON TRUE
		) resolved
		WHERE name IS NOT NULL
	`, codes)
	if err != nil {
		return map[string]string{}
	}
	defer rows.Close()

	resolved := make(map[string]string, len(codes))
	for rows.Next() {
		var code, name string
		if rows.Scan(&code, &name) == nil {
			resolved[code] = name
		}
	}
	return resolved
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

func buildStopsCTE(loc LocationFilter, idx int, argIdx *int, args *[]interface{}, maxDateIdx, minDateIdx int, dayClause string) string {
	tiplocAlias := fmt.Sprintf("tiploc_codes_%d", idx)
	ssAlias := fmt.Sprintf("ss%d", idx)

	stanoxIdx := *argIdx
	*args = append(*args, loc.Stanox)
	*argIdx++

	tiplocCTE := fmt.Sprintf(`
		%s AS MATERIALIZED (
			SELECT tiploc_code FROM tiploc WHERE stanox = $%d
		)
	`, tiplocAlias, stanoxIdx)

	var schedConds []string
	if maxDateIdx > 0 {
		schedConds = append(schedConds, fmt.Sprintf("s.schedule_start_date <= $%d", maxDateIdx))
	}
	if minDateIdx > 0 {
		schedConds = append(schedConds, fmt.Sprintf("s.schedule_end_date >= $%d", minDateIdx))
	}
	if dayClause != "" {
		schedConds = append(schedConds, dayClause)
	}
	schedJoinExtra := ""
	if len(schedConds) > 0 {
		schedJoinExtra = "AND " + strings.Join(schedConds, " AND ")
	}

	var timeCond string
	if loc.TimeFrom != nil && loc.TimeTo != nil {
		fromIdx := *argIdx
		*args = append(*args, loc.TimeFrom.Format("15:04:05"))
		*argIdx++
		toIdx := *argIdx
		*args = append(*args, loc.TimeTo.Format("15:04:05"))
		*argIdx++
		timeCond = fmt.Sprintf(
			"AND (sl.departure BETWEEN $%d AND $%d OR sl.arrival BETWEEN $%d AND $%d)",
			fromIdx, toIdx, fromIdx, toIdx,
		)
	} else if loc.TimeFrom != nil {
		fromIdx := *argIdx
		*args = append(*args, loc.TimeFrom.Format("15:04:05"))
		*argIdx++
		timeCond = fmt.Sprintf("AND (sl.departure >= $%d OR sl.arrival >= $%d)", fromIdx, fromIdx)
	} else if loc.TimeTo != nil {
		toIdx := *argIdx
		*args = append(*args, loc.TimeTo.Format("15:04:05"))
		*argIdx++
		timeCond = fmt.Sprintf("AND (sl.departure <= $%d OR sl.arrival <= $%d)", toIdx, toIdx)
	}

	ssCTE := fmt.Sprintf(`
		%s AS MATERIALIZED (
			SELECT sl.schedule_id, MIN(COALESCE(sl.departure, sl.arrival)) AS visit_time
			FROM %s tc
			JOIN schedule_location sl ON sl.tiploc_code = tc.tiploc_code
			JOIN schedule s ON s.id = sl.schedule_id %s
			%s
			GROUP BY sl.schedule_id
		)
	`, ssAlias, tiplocAlias, schedJoinExtra, timeCond)

	return tiplocCTE + ",\n" + ssCTE
}

func passThroughDataCacheKey(filters ServiceFilters) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("ptdata|%+v|hc=%v|op=%v|dt=%v",
		filters.PassesThrough,
		filters.Headcode,
		filters.OperatorCode,
		filters.Date,
	)))
	return fmt.Sprintf("svc:data:%x", h)
}

type cachedIDEntry struct {
	ID        int32  `json:"i"`
	VisitTime string `json:"v,omitempty"` // "HH:MM:SS" or empty
}

func (dc *DataClient) buildPassesThroughQueries(filters ServiceFilters) (countQ string, countA []interface{}, dataQ string, dataA []interface{}) {
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

	baseArgs := []interface{}{}
	argIdx := 1

	var minDate, maxDate, queryDateValue time.Time
	var queryDate *time.Time
	for _, loc := range filters.PassesThrough {
		if loc.TimeFrom != nil {
			t := loc.TimeFrom.Truncate(24 * time.Hour)
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
	if queryDate == nil && filters.Date != nil {
		t := startOfDay(*filters.Date)
		minDate, maxDate, queryDateValue = t, t, t
		queryDate = &queryDateValue
	}
	includePrevDay := queryDate != nil && needsPreviousDay(filters)

	var maxDateIdx, minDateIdx int
	var dayClause string
	if !minDate.IsZero() {
		maxDateIdx = argIdx
		baseArgs = append(baseArgs, maxDate.Format("2006-01-02"))
		argIdx++
		minDateIdx = argIdx
		baseArgs = append(baseArgs, minDate.Format("2006-01-02"))
		argIdx++

		if queryDate != nil {
			dayOfWeek := int(queryDate.Weekday())
			if dayOfWeek == 0 {
				dayOfWeek = 6
			} else {
				dayOfWeek--
			}
			dayClause = fmt.Sprintf("SUBSTR(s.schedule_days_runs, %d, 1) = '1'", dayOfWeek+1)
			if includePrevDay {
				prevDay := (dayOfWeek + 6) % 7
				dayClause = fmt.Sprintf(
					"(SUBSTR(s.schedule_days_runs, %d, 1) = '1' OR SUBSTR(s.schedule_days_runs, %d, 1) = '1')",
					dayOfWeek+1, prevDay+1,
				)
			}
		}
	}

	cteParts := make([]string, 0, len(filters.PassesThrough)*2)
	cteFromParts := make([]string, 0, len(filters.PassesThrough))

	for i, loc := range filters.PassesThrough {
		ssAlias := fmt.Sprintf("ss%d", i)
		cteParts = append(cteParts, buildStopsCTE(loc, i, &argIdx, &baseArgs, maxDateIdx, minDateIdx, dayClause))
		if i == 0 {
			cteFromParts = append(cteFromParts, ssAlias)
		} else {
			cteFromParts = append(cteFromParts, fmt.Sprintf("JOIN %s ON %s.schedule_id = ss0.schedule_id", ssAlias, ssAlias))
		}
	}

	cteSQL := "WITH " + strings.Join(cteParts, ",\n")
	cteFromSQL := strings.Join(cteFromParts, "\n")

	schedArgs := append([]interface{}{}, baseArgs...)
	var schedConds []string

	if filters.Headcode != nil {
		schedConds = append(schedConds, fmt.Sprintf("s.signalling_id = $%d", argIdx))
		schedArgs = append(schedArgs, *filters.Headcode)
		argIdx++
	}
	if filters.OperatorCode != nil {
		schedConds = append(schedConds, fmt.Sprintf("s.atoc_code = $%d", argIdx))
		schedArgs = append(schedArgs, *filters.OperatorCode)
		argIdx++
	}

	schedWhere := ""
	if len(schedConds) > 0 {
		schedWhere = "WHERE " + strings.Join(schedConds, " AND ")
	}

	countA = append([]interface{}{}, schedArgs...)
	countQ = fmt.Sprintf(`
		%s
		SELECT COUNT(DISTINCT s.train_uid)
		FROM %s
		JOIN schedule s ON s.id = ss0.schedule_id
		JOIN reference_toc toc ON s.atoc_code = toc.code
		%s
	`, cteSQL, cteFromSQL, schedWhere)

	dataA = append([]interface{}{}, schedArgs...)
	queryDateParam := 0
	if queryDate != nil {
		queryDateParam = len(dataA) + 1
		dataA = append(dataA, *queryDate)
	}
	rankOrder := buildRankOrderClause(stpPriority, queryDateParam)

	dataQ = fmt.Sprintf(`
		%s
		SELECT d.id, d.train_uid, d.signalling_id, d.headcode,
		       d.train_category, d.train_category_description, d.schedule_start_date, d.schedule_end_date, d.schedule_days_runs,
		       d.train_status, d.atoc_code, d.toc_name, d.stp_indicator,
		       d.visit_time
		FROM (
			SELECT DISTINCT ON (s.train_uid)
				s.id, s.train_uid, s.signalling_id, s.headcode,
				s.train_category, btr.description AS train_category_description, s.schedule_start_date, s.schedule_end_date, s.schedule_days_runs,
				s.train_status, s.atoc_code, toc.name AS toc_name, s.stp_indicator,
				ss0.visit_time
			FROM %s
			JOIN schedule s ON s.id = ss0.schedule_id
			LEFT JOIN LATERAL (
				SELECT r.description
				FROM bplan_ref r
				WHERE r.category = 'TCT'
				  AND UPPER(TRIM(r.subcode)) = UPPER(TRIM(s.train_category))
				  AND NULLIF(TRIM(r.description), '') IS NOT NULL
				ORDER BY r.id
				LIMIT 1
			) btr ON TRUE
			JOIN reference_toc toc ON s.atoc_code = toc.code
			%s
			ORDER BY s.train_uid, %s
		) d
		ORDER BY d.visit_time IS NULL, d.visit_time, d.schedule_start_date, d.signalling_id
	`, cteSQL, cteFromSQL, schedWhere, rankOrder)

	return
}

func (dc *DataClient) buildScheduleFirstQueries(filters ServiceFilters) (countQ string, countA []interface{}, dataQ string, dataA []interface{}) {
	filter, args := dc.buildServiceFilter(filters)
	queryDate := queryDateFromFilters(filters)

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

	rankOrder := buildRankOrderClause(stpPriority, 0)
	countA = append([]interface{}{}, args...)
	countQ = fmt.Sprintf(`
		SELECT COUNT(DISTINCT s.train_uid)
		FROM schedule s
		JOIN reference_toc toc ON s.atoc_code = toc.code
		%s
	`, filter)

	dataA = append([]interface{}{}, args...)
	if queryDate != nil {
		queryDateParam := len(dataA) + 1
		rankOrder = buildRankOrderClause(stpPriority, queryDateParam)
		dataA = append(dataA, *queryDate)
	}

	limitParam := len(dataA) + 1
	offsetParam := len(dataA) + 2
	dataA = append(dataA, filters.Limit, filters.Offset)

	dataQ = fmt.Sprintf(`
		SELECT d.id, d.train_uid, d.signalling_id, d.headcode,
		       d.train_category, d.train_category_description, d.schedule_start_date, d.schedule_end_date, d.schedule_days_runs,
		       d.train_status, d.atoc_code, d.toc_name, d.stp_indicator
		FROM (
			SELECT DISTINCT ON (s.train_uid)
				s.id, s.train_uid, s.signalling_id, s.headcode,
				s.train_category, btr.description AS train_category_description, s.schedule_start_date, s.schedule_end_date, s.schedule_days_runs,
				s.train_status, s.atoc_code, toc.name AS toc_name, s.stp_indicator
			FROM schedule s
			LEFT JOIN LATERAL (
				SELECT r.description
				FROM bplan_ref r
				WHERE r.category = 'TCT'
				  AND UPPER(TRIM(r.subcode)) = UPPER(TRIM(s.train_category))
				  AND NULLIF(TRIM(r.description), '') IS NOT NULL
				ORDER BY r.id
				LIMIT 1
			) btr ON TRUE
			JOIN reference_toc toc ON s.atoc_code = toc.code
			%s
			ORDER BY s.train_uid, %s
		) d
		ORDER BY d.schedule_start_date ASC, d.signalling_id ASC
		LIMIT $%d OFFSET $%d
	`, filter, rankOrder, limitParam, offsetParam)

	return
}

func (dc *DataClient) fetchServicesByIDs(pageEntries []cachedIDEntry) ([]api_types.ServiceResponse, error) {
	if len(pageEntries) == 0 {
		return nil, nil
	}
	ids := make([]int32, len(pageEntries))
	for i, e := range pageEntries {
		ids[i] = e.ID
	}

	rows, err := dc.pg.Query(context.Background(), `
		SELECT s.id, s.train_uid, s.signalling_id, s.headcode,
		       s.train_category, btr.description AS train_category_description, s.schedule_start_date, s.schedule_end_date, s.schedule_days_runs,
		       s.train_status, s.atoc_code, toc.name AS toc_name, s.stp_indicator
		FROM schedule s
		LEFT JOIN LATERAL (
			SELECT r.description
			FROM bplan_ref r
			WHERE r.category = 'TCT'
			  AND UPPER(TRIM(r.subcode)) = UPPER(TRIM(s.train_category))
			  AND NULLIF(TRIM(r.description), '') IS NOT NULL
			ORDER BY r.id
			LIMIT 1
		) btr ON TRUE
		JOIN reference_toc toc ON s.atoc_code = toc.code
		WHERE s.id = ANY($1)
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("fetchServicesByIDs: %w", err)
	}
	defer rows.Close()

	byID := make(map[int32]api_types.ServiceResponse, len(pageEntries))
	for rows.Next() {
		svc, err := scanServiceRow(rows)
		if err != nil {
			return nil, fmt.Errorf("fetchServicesByIDs scan: %w", err)
		}
		byID[int32(svc.Id)] = *svc
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("fetchServicesByIDs rows: %w", err)
	}

	result := make([]api_types.ServiceResponse, 0, len(pageEntries))
	for _, e := range pageEntries {
		if svc, ok := byID[e.ID]; ok {
			result = append(result, svc)
		}
	}
	return result, nil
}

func (dc *DataClient) GetServicesWithFilters(filters ServiceFilters) (*ServiceQueryResult, error) {
	tTotal := time.Now()

	if len(filters.PassesThrough) > 0 {
		dataCacheKey := passThroughDataCacheKey(filters)
		if cached, err := dc.rdb.Get(context.Background(), dataCacheKey).Result(); err == nil {
			var allEntries []cachedIDEntry
			if json.Unmarshal([]byte(cached), &allEntries) == nil {
				tData := time.Now()

				total := len(allEntries)
				start := filters.Offset
				if start > total {
					start = total
				}
				end := start + filters.Limit
				if end > total {
					end = total
				}
				pageEntries := allEntries[start:end]

				services, err := dc.fetchServicesByIDs(pageEntries)
				if err != nil {
					return nil, err
				}
				dataMs := time.Since(tData).Milliseconds()

				var scheduleIDs []int
				for _, svc := range services {
					scheduleIDs = append(scheduleIDs, svc.Id)
				}

				tLocs := time.Now()
				if len(scheduleIDs) > 0 {
					allStops, err := dc.fetchScheduleLocations(scheduleIDs...)
					if err != nil {
						return nil, fmt.Errorf("cache hit: fetch locations: %w", err)
					}
					for i := range services {
						services[i].Locations = allStops[services[i].Id]
					}
					dc.sortServices(services, filters)
				}

				tGemini := time.Now()
				if len(services) > 0 {
					targetDate := queryDateFromFilters(filters)
					if targetDate == nil {
						now := time.Now().UTC()
						targetDate = &now
					}
					dc.GetGeminiForServices(services, *targetDate)
				}

				timings := map[string]int64{
					"count_query":     0,
					"count_cached":    1,
					"data_query":      dataMs,
					"fetch_locations": time.Since(tLocs).Milliseconds(),
					"gemini":          time.Since(tGemini).Milliseconds(),
				}
				timings["total_db"] = time.Since(tTotal).Milliseconds()

				if dc.logger != nil {
					dc.logger.Infow("services: GetServicesWithFilters (data cache hit)",
						"data_query_ms", dataMs,
						"total_db_ms", timings["total_db"],
						"total_results", total,
						"returned", len(services),
					)
				}

				return &ServiceQueryResult{
					Services:     services,
					TotalResults: total,
					Timings:      timings,
				}, nil
			}
		}
	}

	var countQuery string
	var countArgs []interface{}
	var dataQuery string
	var mainQueryArgs []interface{}

	if len(filters.PassesThrough) > 0 {
		countQuery, countArgs, dataQuery, mainQueryArgs = dc.buildPassesThroughQueries(filters)
	} else {
		countQuery, countArgs, dataQuery, mainQueryArgs = dc.buildScheduleFirstQueries(filters)
	}

	tQueries := time.Now()

	countCacheKey := func() string {
		h := sha256.Sum256([]byte(fmt.Sprintf("%s|%v", countQuery, countArgs)))
		return fmt.Sprintf("svc:cnt:%x", h)
	}()

	tCount := time.Now()
	var totalResults int
	countCached := false
	if cached, err := dc.rdb.Get(context.Background(), countCacheKey).Result(); err == nil {
		if n, parseErr := strconv.Atoi(cached); parseErr == nil {
			totalResults = n
			countCached = true
		}
	}

	if !countCached {
		if err := dc.pg.QueryRow(context.Background(), countQuery, countArgs...).Scan(&totalResults); err != nil {
			return nil, fmt.Errorf("failed to count services: %w", err)
		}
		dc.rdb.Set(context.Background(), countCacheKey, strconv.Itoa(totalResults), 60*time.Second)
	}

	countMs := time.Since(tCount).Milliseconds()

	tData := time.Now()
	rows, err := dc.pg.Query(context.Background(), dataQuery, mainQueryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute service query: %w", err)
	}
	defer rows.Close()

	services := []api_types.ServiceResponse{}
	var scheduleIDs []int
	var allEntries []cachedIDEntry // populated only for passes_through path

	for rows.Next() {
		var service *api_types.ServiceResponse
		if len(filters.PassesThrough) > 0 {
			var visitTimeStr string
			var scanErr error
			service, visitTimeStr, scanErr = scanServiceRowWithVisitTime(rows)
			if scanErr != nil {
				return nil, fmt.Errorf("failed to scan service row: %w", scanErr)
			}
			allEntries = append(allEntries, cachedIDEntry{ID: int32(service.Id), VisitTime: visitTimeStr})
		} else {
			var scanErr error
			service, scanErr = scanServiceRow(rows)
			if scanErr != nil {
				return nil, fmt.Errorf("failed to scan service row: %w", scanErr)
			}
		}
		scheduleIDs = append(scheduleIDs, service.Id)
		services = append(services, *service)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating service rows: %w", err)
	}
	dataMs := time.Since(tData).Milliseconds()

	if len(filters.PassesThrough) > 0 && len(allEntries) > 0 {
		if data, marshalErr := json.Marshal(allEntries); marshalErr == nil {
			dc.rdb.Set(context.Background(), passThroughDataCacheKey(filters), data, 10*time.Minute)
		}
		totalResults = len(allEntries)
		start := filters.Offset
		if start > len(services) {
			start = len(services)
		}
		end := start + filters.Limit
		if end > len(services) {
			end = len(services)
		}
		services = services[start:end]
		scheduleIDs = scheduleIDs[start : start+len(services)]
	}

	if dc.logger != nil {
		dc.logger.Infow("services: queries done",
			"count_query_ms", countMs,
			"count_cached", countCached,
			"data_query_ms", dataMs,
			"both_queries_ms", time.Since(tQueries).Milliseconds(),
			"total_results", totalResults,
			"returned", len(services),
		)
	}

	countCachedInt := int64(0)
	if countCached {
		countCachedInt = 1
	}
	timings := map[string]int64{
		"count_query":  countMs,
		"count_cached": countCachedInt,
		"data_query":   dataMs,
	}

	tLocs := time.Now()
	if len(scheduleIDs) > 0 {
		allStops, err := dc.fetchScheduleLocations(scheduleIDs...)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch schedule locations: %w", err)
		}
		for i := range services {
			services[i].Locations = allStops[services[i].Id]
		}
		dc.sortServices(services, filters)
	}
	timings["fetch_locations"] = time.Since(tLocs).Milliseconds()

	// Best-effort Gemini enrichment for list view: batched single query for all services.
	tGemini := time.Now()
	if len(services) > 0 {
		targetDate := queryDateFromFilters(filters)
		if targetDate == nil {
			now := time.Now().UTC()
			targetDate = &now
		}
		dc.GetGeminiForServices(services, *targetDate)
	}
	timings["gemini"] = time.Since(tGemini).Milliseconds()

	timings["total_db"] = time.Since(tTotal).Milliseconds()

	if dc.logger != nil {
		dc.logger.Infow("services: GetServicesWithFilters",
			"count_query_ms", timings["count_query"],
			"data_query_ms", timings["data_query"],
			"fetch_locations_ms", timings["fetch_locations"],
			"gemini_ms", timings["gemini"],
			"total_db_ms", timings["total_db"],
			"total_results", totalResults,
			"returned", len(services),
		)
	}

	return &ServiceQueryResult{
		Services:     services,
		TotalResults: totalResults,
		Timings:      timings,
	}, nil
}

func (dc *DataClient) GetServiceByUID(uid string, date *time.Time) (*api_types.ServiceResponse, error) {
	query := `
		SELECT s.id, s.train_uid, s.signalling_id, s.headcode,
			   s.train_category, btr.description AS train_category_description, s.schedule_start_date, s.schedule_end_date, s.schedule_days_runs,
			   COALESCE(NULLIF(TRIM(tst.description), ''), s.train_status) AS train_status, s.atoc_code, COALESCE(
			   	CASE
			   		WHEN LOWER(TRIM(toc.name)) LIKE '%unknown%' OR LOWER(TRIM(toc.name)) LIKE '%unkown%' THEN NULL
			   		ELSE NULLIF(TRIM(toc.name), '')
			   	END,
			   	btoc.description
			   ), s.stp_indicator
		FROM schedule s
		LEFT JOIN LATERAL (
			SELECT r.description
			FROM bplan_ref r
			WHERE r.category = 'TCT'
			  AND UPPER(TRIM(r.subcode)) = UPPER(TRIM(s.train_category))
			  AND NULLIF(TRIM(r.description), '') IS NOT NULL
			ORDER BY r.id
			LIMIT 1
		) btr ON TRUE
		LEFT JOIN LATERAL (
			SELECT r.description
			FROM bplan_ref r
			WHERE r.category = 'TST'
			  AND UPPER(TRIM(r.subcode)) = UPPER(TRIM(s.train_status))
			  AND NULLIF(TRIM(r.description), '') IS NOT NULL
			ORDER BY r.id
			LIMIT 1
		) tst ON TRUE
		LEFT JOIN reference_toc toc ON s.atoc_code = toc.code
		LEFT JOIN LATERAL (
			SELECT r.description
			FROM bplan_ref r
			WHERE r.category = 'TOC'
			  AND UPPER(TRIM(r.subcode)) = UPPER(TRIM(s.atoc_code))
			  AND NULLIF(TRIM(r.description), '') IS NOT NULL
			ORDER BY r.id
			LIMIT 1
		) btoc ON TRUE
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
			   s.train_category, btr.description AS train_category_description, s.schedule_start_date, s.schedule_end_date, s.schedule_days_runs,
			   COALESCE(NULLIF(TRIM(tst.description), ''), s.train_status) AS train_status, s.atoc_code, COALESCE(
			   	CASE
			   		WHEN LOWER(TRIM(toc.name)) LIKE '%unknown%' OR LOWER(TRIM(toc.name)) LIKE '%unkown%' THEN NULL
			   		ELSE NULLIF(TRIM(toc.name), '')
			   	END,
			   	btoc.description
			   ), s.stp_indicator
		FROM schedule s
		LEFT JOIN LATERAL (
			SELECT r.description
			FROM bplan_ref r
			WHERE r.category = 'TCT'
			  AND UPPER(TRIM(r.subcode)) = UPPER(TRIM(s.train_category))
			  AND NULLIF(TRIM(r.description), '') IS NOT NULL
			ORDER BY r.id
			LIMIT 1
		) btr ON TRUE
		LEFT JOIN LATERAL (
			SELECT r.description
			FROM bplan_ref r
			WHERE r.category = 'TST'
			  AND UPPER(TRIM(r.subcode)) = UPPER(TRIM(s.train_status))
			  AND NULLIF(TRIM(r.description), '') IS NOT NULL
			ORDER BY r.id
			LIMIT 1
		) tst ON TRUE
		LEFT JOIN reference_toc toc ON s.atoc_code = toc.code
		LEFT JOIN LATERAL (
			SELECT r.description
			FROM bplan_ref r
			WHERE r.category = 'TOC'
			  AND UPPER(TRIM(r.subcode)) = UPPER(TRIM(s.atoc_code))
			  AND NULLIF(TRIM(r.description), '') IS NOT NULL
			ORDER BY r.id
			LIMIT 1
		) btoc ON TRUE
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

// geminiAllocationRow holds one row from a gemini_allocation + gemini_message join.
type geminiAllocationRow struct {
	ResourceGroupID        string
	MessageIdentifier      sql.NullString
	TrainOriginTiploc      sql.NullString
	TrainDestTiploc        sql.NullString
	AllocationOriginTiploc sql.NullString
	AllocationDestTiploc   sql.NullString
	EffectiveTime          time.Time
}

// buildGeminiResult converts a slice of raw allocation rows into the current resource-group list
// (latest snapshot) and the full ordered history. Route filtering is applied in-memory using
// originTiploc/destTiploc when both are non-empty.
func buildGeminiResult(rows []geminiAllocationRow, originTiploc, destTiploc string) (current []string, history []api_types.GeminiSnapshot) {
	filtered := rows
	if originTiploc != "" && destTiploc != "" {
		filtered = make([]geminiAllocationRow, 0, len(rows))
		for _, r := range rows {
			trainMatch := r.TrainOriginTiploc.Valid && r.TrainOriginTiploc.String == originTiploc &&
				r.TrainDestTiploc.Valid && r.TrainDestTiploc.String == destTiploc
			allocMatch := r.AllocationOriginTiploc.Valid && r.AllocationOriginTiploc.String == originTiploc &&
				r.AllocationDestTiploc.Valid && r.AllocationDestTiploc.String == destTiploc
			if trainMatch || allocMatch {
				filtered = append(filtered, r)
			}
		}
	}

	type snapshotKey struct {
		Identifier string
		Time       time.Time
	}
	snapshotsMap := make(map[snapshotKey]map[string]struct{})
	for _, r := range filtered {
		if r.ResourceGroupID == "" || !r.MessageIdentifier.Valid {
			continue
		}
		key := snapshotKey{r.MessageIdentifier.String, r.EffectiveTime.UTC()}
		if _, ok := snapshotsMap[key]; !ok {
			snapshotsMap[key] = make(map[string]struct{})
		}
		snapshotsMap[key][r.ResourceGroupID] = struct{}{}
	}
	if len(snapshotsMap) == 0 {
		return nil, nil
	}

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
	if last := history[len(history)-1].ResourceGroupIds; last != nil {
		current = *last
	}
	return current, history
}

// GetGeminiForServices enriches a slice of services with current Gemini resource groups in a
// single batched query instead of one query per service. History is not populated for list views.
func (dc *DataClient) GetGeminiForServices(services []api_types.ServiceResponse, date time.Time) {
	if len(services) == 0 {
		return
	}

	signallingIDs := make([]string, 0, len(services))
	idSet := make(map[string]bool)
	for _, svc := range services {
		if !idSet[svc.SignallingId] {
			idSet[svc.SignallingId] = true
			signallingIDs = append(signallingIDs, svc.SignallingId)
		}
	}

	rows, err := dc.pg.Query(context.Background(), `
		SELECT
			ga.operational_train_number,
			ga.resource_group_id,
			ga.message_identifier,
			ga.train_origin_tiploc,
			ga.train_dest_tiploc,
			ga.allocation_origin_tiploc,
			ga.allocation_dest_tiploc,
			COALESCE(
				ga.message_date_time,
				gm.message_date_time,
				gm.received_at,
				ga.created_at
			) AS effective_message_time
		FROM gemini_allocation ga
		LEFT JOIN gemini_message gm ON ga.message_identifier = gm.message_identifier
		WHERE ga.operational_train_number = ANY($1)
		  AND ga.start_date = $2
		ORDER BY ga.operational_train_number, effective_message_time ASC
	`, signallingIDs, date)
	if err != nil {
		return
	}
	defer rows.Close()

	bySignallingID := make(map[string][]geminiAllocationRow)
	for rows.Next() {
		var opTrainNum string
		var r geminiAllocationRow
		if err := rows.Scan(
			&opTrainNum,
			&r.ResourceGroupID,
			&r.MessageIdentifier,
			&r.TrainOriginTiploc,
			&r.TrainDestTiploc,
			&r.AllocationOriginTiploc,
			&r.AllocationDestTiploc,
			&r.EffectiveTime,
		); err != nil {
			continue
		}
		r.EffectiveTime = r.EffectiveTime.UTC()
		bySignallingID[opTrainNum] = append(bySignallingID[opTrainNum], r)
	}
	if rows.Err() != nil {
		return
	}

	for i := range services {
		allocRows := bySignallingID[services[i].SignallingId]
		if len(allocRows) == 0 {
			continue
		}
		originTiploc, destTiploc := routeTiplocs(services[i])
		current, _ := buildGeminiResult(allocRows, originTiploc, destTiploc)
		if len(current) > 0 {
			currentCopy := current
			services[i].GeminiResourceGroups = &currentCopy
		}
	}
}

// GetGeminiForService fetches current and historical Gemini allocations for a service by signalling ID and date.
// Gemini's OperationalTrainNumber matches the operational train ID (signalling_id), not the internal headcode.
func (dc *DataClient) GetGeminiForService(
	signallingID string,
	date time.Time,
	originTiploc string,
	destTiploc string,
) (current []string, history []api_types.GeminiSnapshot, err error) {
	rows, err := dc.pg.Query(context.Background(), `
		SELECT
			ga.resource_group_id,
			ga.message_identifier,
			ga.train_origin_tiploc,
			ga.train_dest_tiploc,
			ga.allocation_origin_tiploc,
			ga.allocation_dest_tiploc,
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
		ORDER BY effective_message_time ASC
	`, signallingID, date)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var allocRows []geminiAllocationRow
	for rows.Next() {
		var r geminiAllocationRow
		if err := rows.Scan(
			&r.ResourceGroupID,
			&r.MessageIdentifier,
			&r.TrainOriginTiploc,
			&r.TrainDestTiploc,
			&r.AllocationOriginTiploc,
			&r.AllocationDestTiploc,
			&r.EffectiveTime,
		); err != nil {
			return nil, nil, err
		}
		r.EffectiveTime = r.EffectiveTime.UTC()
		allocRows = append(allocRows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	current, history = buildGeminiResult(allocRows, originTiploc, destTiploc)
	if current == nil && history == nil {
		return nil, nil, nil
	}
	return current, history, nil
}

// fetchScheduleLocations fetches all schedule locations for the given schedule IDs
func (dc *DataClient) fetchScheduleLocations(scheduleIDs ...int) (map[int][]api_types.ScheduleLocation, error) {
	if len(scheduleIDs) == 0 {
		return make(map[int][]api_types.ScheduleLocation), nil
	}

	rows, err := dc.pg.Query(context.Background(), scheduleLocationsQuery(), scheduleIDs)
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

func scheduleLocationsQuery() string {
	return `
		SELECT sl.schedule_id, sl.id, sl.location_type, sl.tiploc_code,
			   sl.arrival::text, sl.public_arrival::text,
			   sl.departure::text, sl.public_departure::text,
			   sl.platform, sl.location_order,
			   t.stanox, t.crs_code, COALESCE(NULLIF(br.description, ''), NULLIF(bl.name, ''), t.description)
		FROM schedule_location sl
		LEFT JOIN tiploc t ON sl.tiploc_code = t.tiploc_code
		LEFT JOIN bplan_loc bl
		  ON UPPER(TRIM(sl.tiploc_code)) = UPPER(TRIM(bl.tiploc))
		LEFT JOIN LATERAL (
			SELECT r.description
			FROM bplan_ref r
			WHERE UPPER(TRIM(r.subcode)) = UPPER(TRIM(bl.name))
			  AND NULLIF(TRIM(r.description), '') IS NOT NULL
			ORDER BY CASE WHEN r.category = 'LOC' THEN 0 ELSE 1 END, r.id
			LIMIT 1
		) br ON TRUE
		WHERE sl.schedule_id = ANY($1)
		ORDER BY sl.schedule_id, sl.location_order
	`
}

// GetLocationDetails retrieves full location details for a given stanox
func (dc *DataClient) GetLocationDetails(stanox string) (*api_types.Location, error) {
	rows, err := dc.pg.Query(context.Background(), locationDetailsQuery(), stanox)
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

func locationDetailsQuery() string {
	return `
		SELECT COALESCE(NULLIF(br.description, ''), NULLIF(bl.name, ''), t.description) AS full_name, t.crs_code, t.tiploc_code
		FROM tiploc t
		LEFT JOIN bplan_loc bl
		  ON UPPER(TRIM(t.tiploc_code)) = UPPER(TRIM(bl.tiploc))
		LEFT JOIN LATERAL (
			SELECT r.description
			FROM bplan_ref r
			WHERE UPPER(TRIM(r.subcode)) = UPPER(TRIM(bl.name))
			  AND NULLIF(TRIM(r.description), '') IS NOT NULL
			ORDER BY CASE WHEN r.category = 'LOC' THEN 0 ELSE 1 END, r.id
			LIMIT 1
		) br ON TRUE
		WHERE t.stanox = $1
		ORDER BY t.tiploc_code
	`
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

	t0 := time.Now()
	runDate := utils.FormatRunDate(date)

	// Collect unique trainUIDs in a stable order so MGET indices correlate correctly.
	trainUIDs := make([]string, 0, len(services))
	uidSet := make(map[string]bool)
	for i := range services {
		uid := strings.TrimSpace(services[i].TrainUid)
		if uid != "" && !uidSet[uid] {
			uidSet[uid] = true
			trainUIDs = append(trainUIDs, uid)
		}
	}

	journeys := make(map[string]types.TrainJourney, len(trainUIDs))
	var missedUIDs []string

	if len(trainUIDs) > 0 {
		// Single MGET round-trip for all schedule keys.
		scheduleKeys := make([]string, len(trainUIDs))
		for i, uid := range trainUIDs {
			scheduleKeys[i] = utils.BuildScheduleKey(uid, runDate)
		}

		mgetResults, err := dc.rdb.MGet(context.Background(), scheduleKeys...).Result()
		if err == nil {
			for i, raw := range mgetResults {
				if raw == nil {
					missedUIDs = append(missedUIDs, trainUIDs[i])
					continue
				}
				str, ok := raw.(string)
				if !ok {
					missedUIDs = append(missedUIDs, trainUIDs[i])
					continue
				}
				var journey types.TrainJourney
				if json.Unmarshal([]byte(str), &journey) == nil {
					journeys[trainUIDs[i]] = journey
				} else {
					missedUIDs = append(missedUIDs, trainUIDs[i])
				}
			}
		} else {
			missedUIDs = trainUIDs
		}
	}

	// Batch DB fallback: one schedule query + one locations query for all cache misses.
	if len(missedUIDs) > 0 {
		loaded, err := utils.LoadTrainJourneysBatch(context.Background(), dc.pg, dc.rdb, missedUIDs, runDate)
		if err == nil {
			for uid, journey := range loaded {
				journeys[uid] = journey
			}
		}
	}

	if dc.logger != nil {
		hits := len(journeys)
		dc.logger.Infow("services: realtime journey load",
			"duration_ms", time.Since(t0).Milliseconds(),
			"total_uids", len(trainUIDs),
			"cache_hits", hits,
			"cache_misses", len(missedUIDs),
		)
	}

	servicesWithJourneys := make([]int, 0, len(journeys))
	for i := range services {
		uid := strings.TrimSpace(services[i].TrainUid)
		if _, has := journeys[uid]; has {
			servicesWithJourneys = append(servicesWithJourneys, i)
		}
	}

	// Build tiploc→stanox from Location.Stanox populated by fetchScheduleLocations (no extra DB
	// query needed for tiplocs that already have stanox set). Only fall back to Postgres for gaps.
	tiplocToStanox := make(map[string]string)
	var missingTiplocs []string
	missingTiplocSet := make(map[string]bool)

	for _, idx := range servicesWithJourneys {
		for j := range services[idx].Locations {
			loc := &services[idx].Locations[j]
			if loc.Location.Stanox != "" {
				for _, tc := range loc.Location.TiplocCodes {
					tiplocToStanox[tc] = loc.Location.Stanox
				}
			} else {
				for _, tc := range loc.Location.TiplocCodes {
					if tiplocToStanox[tc] == "" && !missingTiplocSet[tc] {
						missingTiplocSet[tc] = true
						missingTiplocs = append(missingTiplocs, tc)
					}
				}
			}
		}
	}

	if len(missingTiplocs) > 0 {
		rows, err := dc.pg.Query(context.Background(), `
			SELECT tiploc_code, stanox
			FROM tiploc
			WHERE tiploc_code = ANY($1) AND stanox IS NOT NULL
		`, missingTiplocs)
		if err == nil {
			for rows.Next() {
				var tiplocCode, stanox string
				if err := rows.Scan(&tiplocCode, &stanox); err == nil {
					tiplocToStanox[tiplocCode] = stanox
				}
			}
			rows.Close()
		}
	}

	// Collect activation keys and resolve in a single MGET call.
	type pendingActivation struct {
		serviceIdx int
		key        string
		useTrainID bool
		trainID    string
	}
	var pending []pendingActivation

	for _, idx := range servicesWithJourneys {
		trainUid := strings.TrimSpace(services[idx].TrainUid)
		journey := journeys[trainUid]

		if journey.TrainID != "" {
			if journey.ActivationTime != "" {
				// Already cached in the journey; set directly.
				trainID := journey.TrainID
				activationTime := journey.ActivationTime
				services[idx].TrustId = &trainID
				services[idx].ActivationTime = &activationTime
			} else {
				pending = append(pending, pendingActivation{
					serviceIdx: idx,
					key:        utils.BuildActivationKey(journey.TrainID),
					useTrainID: true,
					trainID:    journey.TrainID,
				})
			}
		} else {
			pending = append(pending, pendingActivation{
				serviceIdx: idx,
				key:        fmt.Sprintf("activation:uid:%s", trainUid),
				useTrainID: false,
			})
		}
	}

	type pendingOperator struct {
		serviceIdx int
		tocCode    string
	}
	var pendingOperators []pendingOperator

	if len(pending) > 0 {
		activationKeys := make([]string, len(pending))
		for i, p := range pending {
			activationKeys[i] = p.key
		}
		mgetResults, err := dc.rdb.MGet(context.Background(), activationKeys...).Result()
		if err == nil {
			for i, raw := range mgetResults {
				if raw == nil {
					continue
				}
				str, ok := raw.(string)
				if !ok {
					continue
				}
				p := pending[i]
				var activation map[string]string
				if json.Unmarshal([]byte(str), &activation) != nil {
					continue
				}
				if p.useTrainID {
					trainID := p.trainID
					services[p.serviceIdx].TrustId = &trainID
					if aTime, ok := activation["activation_time"]; ok {
						services[p.serviceIdx].ActivationTime = &aTime
					}
					if tocID, ok := activation["toc_id"]; ok && strings.TrimSpace(tocID) != "" {
						pendingOperators = append(pendingOperators, pendingOperator{
							serviceIdx: p.serviceIdx,
							tocCode:    strings.ToUpper(strings.TrimSpace(tocID)),
						})
					}
				} else {
					if tID, ok := activation["train_id"]; ok {
						services[p.serviceIdx].TrustId = &tID
					}
					if aTime, ok := activation["activation_time"]; ok {
						services[p.serviceIdx].ActivationTime = &aTime
					}
					if tocID, ok := activation["toc_id"]; ok && strings.TrimSpace(tocID) != "" {
						pendingOperators = append(pendingOperators, pendingOperator{
							serviceIdx: p.serviceIdx,
							tocCode:    strings.ToUpper(strings.TrimSpace(tocID)),
						})
					}
				}
			}
		}
	}

	if len(pendingOperators) > 0 {
		tocSet := make(map[string]struct{}, len(pendingOperators))
		tocCodes := make([]string, 0, len(pendingOperators))
		for _, p := range pendingOperators {
			if _, ok := tocSet[p.tocCode]; !ok {
				tocSet[p.tocCode] = struct{}{}
				tocCodes = append(tocCodes, p.tocCode)
			}
		}

		tocNames := dc.resolveTOCNames(tocCodes)
		for _, p := range pendingOperators {
			name := tocNames[p.tocCode]
			if name == "" {
				continue
			}
			if services[p.serviceIdx].Operator == nil || isUnknownOperatorName(services[p.serviceIdx].Operator.Name) || strings.EqualFold(strings.TrimSpace(services[p.serviceIdx].Operator.Code), "ZZ") {
				code := p.tocCode
				services[p.serviceIdx].Operator = &api_types.Operator{
					Code: code,
					Name: name,
				}
			}
		}
	}

	// Match journey stops to service locations and attach actual times.
	for _, idx := range servicesWithJourneys {
		trainUid := strings.TrimSpace(services[idx].TrainUid)
		journey := journeys[trainUid]

		stanoxToStop := make(map[string]types.Stop, len(journey.Stops))
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

	if dc.logger != nil {
		dc.logger.Infow("services: AddRealtimeData total", "duration_ms", time.Since(t0).Milliseconds())
	}
}
