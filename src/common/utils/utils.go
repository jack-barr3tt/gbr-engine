package utils

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func LoadTrainJourney(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client, trainUID, runDate string) (types.TrainJourney, error) {
	schedKey := BuildScheduleKey(trainUID, runDate)
	raw, err := rdb.Get(ctx, schedKey).Result()
	var journey types.TrainJourney
	if err != nil {
		journey, err = LoadScheduleFromDatabase(ctx, db, trainUID, runDate)
		if err != nil {
			return types.TrainJourney{}, err
		}

		if b, err := json.Marshal(journey); err == nil {
			rdb.Set(ctx, schedKey, b, 48*time.Hour)
		}
	} else {
		if err := json.Unmarshal([]byte(raw), &journey); err != nil {
			return types.TrainJourney{}, fmt.Errorf("failed to unmarshal schedule: %w", err)
		}
	}
	return journey, nil
}

// LoadTrainJourneysBatch fetches schedules and locations for multiple train UIDs in two queries
// (one for schedules, one for all their locations) instead of 2N individual queries. Results are
// written back to Redis as a pipeline so subsequent requests hit the cache.
func LoadTrainJourneysBatch(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client, trainUIDs []string, runDateStr string) (map[string]types.TrainJourney, error) {
	if len(trainUIDs) == 0 {
		return make(map[string]types.TrainJourney), nil
	}

	runDate, err := time.Parse("20060102", runDateStr)
	if err != nil {
		return nil, fmt.Errorf("invalid run date: %w", err)
	}

	// One query: best schedule per train UID that covers the run date.
	rows, err := db.Query(ctx, `
		SELECT DISTINCT ON (train_uid)
			id, train_uid, schedule_days_runs, schedule_start_date, schedule_end_date
		FROM schedule
		WHERE train_uid = ANY($1)
		  AND schedule_start_date <= $2
		  AND schedule_end_date >= $2
		ORDER BY train_uid, schedule_start_date DESC
	`, trainUIDs, runDate)
	if err != nil {
		return nil, fmt.Errorf("failed to batch-load schedules: %w", err)
	}

	type schedInfo struct {
		id        int
		uid       string
		daysRuns  string
		startDate time.Time
		endDate   time.Time
	}

	var validSchedules []schedInfo
	var scheduleIDs []int
	uidToSchedID := make(map[string]int)

	for rows.Next() {
		var s schedInfo
		if err := rows.Scan(&s.id, &s.uid, &s.daysRuns, &s.startDate, &s.endDate); err != nil {
			continue
		}
		if IsScheduleValidForDate(s.daysRuns, s.startDate, s.endDate, runDate) {
			validSchedules = append(validSchedules, s)
			scheduleIDs = append(scheduleIDs, s.id)
			uidToSchedID[s.uid] = s.id
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating batch schedules: %w", err)
	}

	if len(scheduleIDs) == 0 {
		return make(map[string]types.TrainJourney), nil
	}

	// One query: all locations for valid schedules.
	locRows, err := db.Query(ctx, `
		SELECT sl.schedule_id, sl.tiploc_code, sl.arrival::text, sl.departure::text, t.stanox
		FROM schedule_location sl
		LEFT JOIN tiploc t ON sl.tiploc_code = t.tiploc_code
		WHERE sl.schedule_id = ANY($1)
		ORDER BY sl.schedule_id, sl.location_order
	`, scheduleIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to batch-load locations: %w", err)
	}

	stopsBySchedID := make(map[int][]types.Stop, len(scheduleIDs))
	for locRows.Next() {
		var schedID int
		var tiplocCode string
		var arrival, departure, stanox sql.NullString

		if err := locRows.Scan(&schedID, &tiplocCode, &arrival, &departure, &stanox); err != nil {
			continue
		}
		if !stanox.Valid || stanox.String == "" {
			continue
		}

		stop := types.Stop{Stanox: stanox.String}
		if arrival.Valid {
			if len(arrival.String) >= 5 {
				stop.PlannedArr = arrival.String[:5]
			} else {
				stop.PlannedArr = arrival.String
			}
		}
		if departure.Valid {
			if len(departure.String) >= 5 {
				stop.PlannedDep = departure.String[:5]
			} else {
				stop.PlannedDep = departure.String
			}
		}
		stopsBySchedID[schedID] = append(stopsBySchedID[schedID], stop)
	}
	locRows.Close()
	if err := locRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating batch locations: %w", err)
	}

	result := make(map[string]types.TrainJourney, len(validSchedules))
	for _, s := range validSchedules {
		result[s.uid] = types.TrainJourney{
			UID:     s.uid,
			RunDate: runDateStr,
			Stops:   stopsBySchedID[s.id],
		}
	}

	// Warm Redis cache via pipeline so future requests hit the cache.
	if rdb != nil {
		pipe := rdb.Pipeline()
		for uid, journey := range result {
			if b, err := json.Marshal(journey); err == nil {
				pipe.Set(ctx, BuildScheduleKey(uid, runDateStr), b, 48*time.Hour)
			}
		}
		pipe.Exec(ctx)
	}

	return result, nil
}

func MergeTrustEvent(journey *types.TrainJourney, trust *types.TrustBody) bool {
	merged := false
	for i, stop := range journey.Stops {
		if stop.Stanox == trust.LocStanox {
			if trust.EventType == "ARRIVAL" {
				journey.Stops[i].ActualArr = trust.ActualTimestamp
			} else if trust.EventType == "DEPARTURE" {
				journey.Stops[i].ActualDep = trust.ActualTimestamp
			}
			merged = true
			break
		}
	}
	return merged
}

func LoadScheduleFromDatabase(ctx context.Context, db *pgxpool.Pool, trainUID string, runDateStr string) (types.TrainJourney, error) {
	runDate, err := time.Parse("20060102", runDateStr)
	if err != nil {
		return types.TrainJourney{}, fmt.Errorf("invalid run date: %w", err)
	}

	var scheduleID int
	var scheduleDaysRuns string
	var startDate, endDate time.Time
	err = db.QueryRow(ctx, `
		SELECT id, schedule_days_runs, schedule_start_date, schedule_end_date
		FROM schedule
		WHERE train_uid = $1
		  AND schedule_start_date <= $2
		  AND schedule_end_date >= $2
		ORDER BY schedule_start_date DESC
		LIMIT 1
	`, trainUID, runDate).Scan(&scheduleID, &scheduleDaysRuns, &startDate, &endDate)

	if err != nil {
		return types.TrainJourney{}, fmt.Errorf("no schedule found for train %s on %s", trainUID, runDateStr)
	}

	if !IsScheduleValidForDate(scheduleDaysRuns, startDate, endDate, runDate) {
		return types.TrainJourney{}, fmt.Errorf("schedule does not run on this day")
	}

	rows, err := db.Query(ctx, `
		SELECT sl.tiploc_code, sl.arrival::text, sl.departure::text, t.stanox
		FROM schedule_location sl
		LEFT JOIN tiploc t ON sl.tiploc_code = t.tiploc_code
		WHERE sl.schedule_id = $1
		ORDER BY sl.location_order
	`, scheduleID)
	if err != nil {
		return types.TrainJourney{}, fmt.Errorf("failed to load locations: %w", err)
	}
	defer rows.Close()

	var stops []types.Stop
	for rows.Next() {
		var tiplocCode string
		var arrival, departure sql.NullString
		var stanox sql.NullString

		if err := rows.Scan(&tiplocCode, &arrival, &departure, &stanox); err != nil {
			return types.TrainJourney{}, fmt.Errorf("failed to scan location: %w", err)
		}

		if !stanox.Valid || stanox.String == "" {
			continue
		}

		stop := types.Stop{
			Stanox: stanox.String,
		}
		if arrival.Valid {
			if len(arrival.String) >= 5 {
				stop.PlannedArr = arrival.String[:5]
			} else {
				stop.PlannedArr = arrival.String
			}
		}
		if departure.Valid {
			if len(departure.String) >= 5 {
				stop.PlannedDep = departure.String[:5]
			} else {
				stop.PlannedDep = departure.String
			}
		}
		stops = append(stops, stop)
	}

	if err = rows.Err(); err != nil {
		return types.TrainJourney{}, fmt.Errorf("error iterating locations: %w", err)
	}

	return types.TrainJourney{
		UID:     trainUID,
		RunDate: runDateStr,
		Stops:   stops,
	}, nil
}

func NullString(s string) *string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func ParseTime(timeStr string) *time.Time {
	trimmed := strings.TrimSpace(timeStr)
	if trimmed == "" || trimmed == "      " {
		return nil
	}

	if len(trimmed) == 6 {
		if t, err := time.Parse("150405", trimmed); err == nil {
			return &t
		}
	}

	return nil
}

func ParseIntOrZero(s string) int {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0
	}

	var result int
	fmt.Sscanf(trimmed, "%d", &result)
	return result
}

func IsScheduleValidForDate(runsOn string, startDate, endDate, checkDate time.Time) bool {
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

func BuildActivationKey(trainID string) string {
	return fmt.Sprintf("activation:%s", trainID)
}

func BuildScheduleKey(trainUID, runDate string) string {
	return fmt.Sprintf("schedule:%s:%s", trainUID, runDate)
}

func FormatRunDate(t time.Time) string {
	return t.Format("20060102")
}

func ParseTimeForComparison(timeStr string) (time.Time, error) {
	timeStr = strings.TrimSpace(timeStr)

	now := time.Now().UTC()
	refDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	if len(timeStr) > 0 {
		var timestampMs int64
		if _, err := fmt.Sscanf(timeStr, "%d", &timestampMs); err == nil && timestampMs > 1000000000 {
			t := time.Unix(timestampMs/1000, (timestampMs%1000)*1000000).UTC()
			return time.Date(refDate.Year(), refDate.Month(), refDate.Day(),
				t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC), nil
		}
	}

	formats := []string{
		"15:04",
		"15:04:05",
		"15:04:05.000000",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return time.Date(refDate.Year(), refDate.Month(), refDate.Day(),
				t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC), nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse time: %s", timeStr)
}

func FormatActualTime(timeStr string) string {
	timeStr = strings.TrimSpace(timeStr)

	var timestampMs int64
	if _, err := fmt.Sscanf(timeStr, "%d", &timestampMs); err == nil && timestampMs > 1000000000 {
		t := time.Unix(timestampMs/1000, (timestampMs%1000)*1000000)
		return t.Format("15:04:05.000000")
	}

	return timeStr
}

func CalculateLateness(planned, actual string) int {
	plannedTime, err := ParseTimeForComparison(planned)
	if err != nil {
		return 0
	}

	actualTime, err := ParseTimeForComparison(actual)
	if err != nil {
		return 0
	}

	diff := actualTime.Sub(plannedTime)
	return int(diff.Minutes())
}

func FormatPlannedTime(s string) string {
	if len(s) == 6 {
		if t, err := time.Parse("150405", s); err == nil {
			return t.Format("15:04")
		}
	}
	return s
}

func Ptr[T any](v T) *T {
	return &v
}
