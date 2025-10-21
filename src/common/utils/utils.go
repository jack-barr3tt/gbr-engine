package utils

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-stomp/stomp/v3"
	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
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

func NewRabbitConnection() (*amqp.Connection, *amqp.Channel, error) {
	mqUser := os.Getenv("MQ_USER")
	mqPassword := os.Getenv("MQ_PASSWORD")
	mqHost := os.Getenv("MQ_HOST")
	mqPort := os.Getenv("MQ_PORT")

	config := amqp.Config{
		Heartbeat: 60 * time.Second,
		Locale:    "en_US",
	}

	connection, err := amqp.DialConfig(fmt.Sprintf("amqp://%s:%s@%s:%s/", mqUser, mqPassword, mqHost, mqPort), config)
	if err != nil {
		return nil, nil, err
	}
	channel, err := connection.Channel()
	if err != nil {
		return nil, nil, err
	}

	return connection, channel, nil
}

func NewRabbitConnectionOnly() (*amqp.Connection, error) {
	mqUser := os.Getenv("MQ_USER")
	mqPassword := os.Getenv("MQ_PASSWORD")
	mqHost := os.Getenv("MQ_HOST")
	mqPort := os.Getenv("MQ_PORT")

	config := amqp.Config{
		Heartbeat: 60 * time.Second,
		Locale:    "en_US",
	}

	connection, err := amqp.DialConfig(fmt.Sprintf("amqp://%s:%s@%s:%s/", mqUser, mqPassword, mqHost, mqPort), config)
	if err != nil {
		return nil, err
	}

	return connection, nil
}

func NewNRStompConnection() (*stomp.Conn, error) {
	url := os.Getenv("NR_FEEDS_ENDPOINT")
	username := os.Getenv("NR_FEEDS_USERNAME")
	password := os.Getenv("NR_FEEDS_PASSWORD")

	conn, err := stomp.Dial("tcp", url,
		stomp.ConnOpt.Login(username, password),
	)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func NewRedisClient() *redis.Client {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		// default to the redis service in the cluster
		redisAddr = "redis:6379"
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
		DB:   0,
	})

	return rdb
}

func NewPostgresConnection() (*pgxpool.Pool, error) {
	host := os.Getenv("POSTGRES_HOST")
	port := os.Getenv("POSTGRES_PORT")
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	dbname := os.Getenv("POSTGRES_DB")

	dbConnectionString := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname,
	)

	connection, err := pgxpool.New(context.Background(), dbConnectionString)
	if err != nil {
		return nil, err
	}

	return connection, nil
}

func UnmarshalTrustMessages(data string) ([]types.TrustMessage, error) {
	var messages []types.TrustMessage
	err := json.Unmarshal([]byte(data), &messages)
	return messages, err
}

func UnmarshalTDMessages(data string) ([]types.TDCMsgBody, []types.TDSMsgBody, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal([]byte(data), &raws); err != nil {
		return nil, nil, err
	}

	var tdCMsgs []types.TDCMsgBody
	var tdSMsgs []types.TDSMsgBody

	for _, raw := range raws {
		var tdC types.TDCMsgEnvelope
		if err := json.Unmarshal(raw, &tdC); err == nil {
			if tdC.CAMsgBody != nil {
				tdCMsgs = append(tdCMsgs, *tdC.CAMsgBody)
			}
			if tdC.CBMsgBody != nil {
				tdCMsgs = append(tdCMsgs, *tdC.CBMsgBody)
			}
			if tdC.CCMsgBody != nil {
				tdCMsgs = append(tdCMsgs, *tdC.CCMsgBody)
			}
			if tdC.CTMsgBody != nil {
				tdCMsgs = append(tdCMsgs, *tdC.CTMsgBody)
			}
			continue
		}

		var tdS types.TDSMsgEnvelope
		if err := json.Unmarshal(raw, &tdS); err == nil {
			if tdS.SFMsgBody != nil {
				tdSMsgs = append(tdSMsgs, *tdS.SFMsgBody)
			}
			if tdS.SGMsgBody != nil {
				tdSMsgs = append(tdSMsgs, *tdS.SGMsgBody)
			}
			if tdS.SHMsgBody != nil {
				tdSMsgs = append(tdSMsgs, *tdS.SHMsgBody)
			}
			continue
		}
	}

	return tdCMsgs, tdSMsgs, nil
}

func UnmarshalVSTP(jsonStr string) (*types.VSTPMessage, error) {
	var vstpMsg types.VSTPMessage
	err := json.Unmarshal([]byte(jsonStr), &vstpMsg)
	if err != nil {
		return nil, err
	}
	return &vstpMsg, nil
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

func BuildTiplocKey(tiplocCode string) string {
	return fmt.Sprintf("tiploc:%s", tiplocCode)
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

// GetStanoxByCRS returns the STANOX for a given CRS code.
func GetStanoxByCRS(ctx context.Context, db *pgxpool.Pool, crsCode string) (string, error) {
	var stanox sql.NullString
	err := db.QueryRow(ctx, `
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

// GetStanoxByTiploc returns the STANOX for a given TIPLOC code.
func GetStanoxByTiploc(ctx context.Context, db *pgxpool.Pool, tiploc string) (string, error) {
	var stanox sql.NullString
	err := db.QueryRow(ctx, `
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

// GetStanoxByTiplocCached caches TIPLOC->STANOX lookups in Redis under key tiploc:<code> for the provided TTL.
func GetStanoxByTiplocCached(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client, tiploc string, ttl time.Duration) (string, error) {
	if rdb != nil {
		if cached, err := rdb.Get(ctx, BuildTiplocKey(tiploc)).Result(); err == nil && cached != "" {
			return cached, nil
		}
	}
	stanox, err := GetStanoxByTiploc(ctx, db, tiploc)
	if err != nil {
		return "", err
	}
	if rdb != nil {
		_ = rdb.Set(ctx, BuildTiplocKey(tiploc), stanox, ttl).Err()
	}
	return stanox, nil
}
