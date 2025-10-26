package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jack-barr3tt/gbr-engine/src/common/data"
	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Connections struct {
	DB     *pgxpool.Pool
	Redis  *redis.Client
	Logger *zap.SugaredLogger
	Data   *data.DataClient
}

func main() {
	utils.InitLogger()
	defer utils.SyncLogger()
	log := utils.GetLogger()

	ctx := context.Background()

	log.Info("Starting VSTP consumer...")

	db, err := utils.NewPostgresConnection()
	if err != nil {
		log.Fatalw("failed to connect to Postgres", "error", err)
	}
	defer db.Close()

	rdb := utils.NewRedisClient()
	defer rdb.Close()

	conn, channel, err := utils.NewRabbitConnection()
	if err != nil {
		log.Fatalw("failed to connect to RabbitMQ", "error", err)
	}
	defer conn.Close()
	defer channel.Close()

	_, err = channel.QueueDeclare("vstp", false, false, false, false, nil)
	if err != nil {
		log.Fatalw("failed to declare VSTP queue", "error", err)
	}

	msgs, err := channel.Consume("vstp", "", true, false, false, false, nil)
	if err != nil {
		log.Fatalw("failed to consume VSTP queue", "error", err)
	}

	log.Info("Processing VSTP schedule messages...")

	for msg := range msgs {
		var vstpMsg types.VSTPMessage
		if err := json.Unmarshal(msg.Body, &vstpMsg); err != nil {
			log.Warnw("bad json in VSTP message", "error", err)
			continue
		}

		if err := processVSTPMessage(ctx, &Connections{
			DB:     db,
			Redis:  rdb,
			Logger: log,
			Data:   data.NewDataClient(db, rdb, log),
		}, &vstpMsg); err != nil {
			log.Warnw("error processing VSTP message", "error", err)
			continue
		}
		log.Infow("processed VSTP schedule", "train_uid", vstpMsg.VSTPCIFMsgV1.Schedule.TrainUID)
	}
}

func processVSTPMessage(ctx context.Context, conn *Connections, vstpMsg *types.VSTPMessage) error {
	schedule := &vstpMsg.VSTPCIFMsgV1.Schedule

	startDate, err := time.Parse("2006-01-02", schedule.ScheduleStartDate)
	if err != nil {
		return fmt.Errorf("invalid start date: %v", err)
	}

	endDate, err := time.Parse("2006-01-02", schedule.ScheduleEndDate)
	if err != nil {
		return fmt.Errorf("invalid end date: %v", err)
	}

	tx, err := conn.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Insert main schedule record
	var scheduleID int
	for _, segment := range schedule.ScheduleSegment {
		err = tx.QueryRow(ctx, `
			INSERT INTO schedule (
				train_uid, transaction_type, stp_indicator, bank_holiday_running,
				applicable_timetable, atoc_code, schedule_days_runs, schedule_start_date,
				schedule_end_date, train_status, signalling_id, train_category,
				headcode, course_indicator, train_service_code, business_sector,
				power_type, timing_load, speed, operating_characteristics,
				train_class, sleepers, reservations, connection_indicator,
				catering_code, service_branding, traction_class, uic_code,
				origin_msg_id, schema_location
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
				$17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30
			) RETURNING id`,
			schedule.TrainUID,
			schedule.TransactionType,
			schedule.StpIndicator,
			utils.NullString(schedule.BankHolidayRunning),
			utils.NullString(schedule.ApplicableTimetable),
			utils.NullString(segment.AtocCode),
			schedule.ScheduleDaysRuns,
			startDate,
			endDate,
			schedule.TrainStatus,
			segment.SignallingId,
			segment.TrainCategory,
			segment.Headcode,
			utils.ParseIntOrZero(segment.CourseIndicator),
			segment.TrainServiceCode,
			utils.NullString(segment.BusinessSector),
			utils.NullString(segment.PowerType),
			utils.NullString(segment.TimingLoad),
			utils.NullString(segment.Speed),
			utils.NullString(segment.OperatingCharacteristics),
			utils.NullString(segment.TrainClass),
			utils.NullString(segment.Sleepers),
			utils.NullString(segment.Reservations),
			utils.NullString(segment.ConnectionIndicator),
			utils.NullString(segment.CateringCode),
			segment.ServiceBranding,
			utils.NullString(segment.TractionClass),
			utils.NullString(segment.UicCode),
			vstpMsg.VSTPCIFMsgV1.OriginMsgId,
			vstpMsg.VSTPCIFMsgV1.SchemaLocation,
		).Scan(&scheduleID)

		if err != nil {
			return fmt.Errorf("error inserting schedule: %v", err)
		}

		// Insert schedule locations
		for i, location := range segment.ScheduleLocation {
			err = insertScheduleLocation(ctx, tx, scheduleID, &location, i+1)
			if err != nil {
				return fmt.Errorf("error inserting schedule location: %v", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	runDate := strings.ReplaceAll(schedule.ScheduleStartDate, "-", "")
	trainUID := strings.TrimSpace(schedule.TrainUID)

	var stops []types.Stop
	for _, segment := range schedule.ScheduleSegment {
		for _, loc := range segment.ScheduleLocation {
			stanox, err := conn.Data.GetStanoxByTiploc(loc.Location.Tiploc.TiplocId)

			if err != nil {
				continue
			}

			plannedArr := utils.FormatPlannedTime(loc.ScheduledArrivalTime)
			plannedDep := utils.FormatPlannedTime(loc.ScheduledDepartureTime)
			stops = append(stops, types.Stop{Stanox: stanox, PlannedArr: plannedArr, PlannedDep: plannedDep})
		}
	}

	journey := types.TrainJourney{UID: trainUID, RunDate: runDate, Stops: stops}
	b, _ := json.Marshal(journey)
	key := utils.BuildScheduleKey(trainUID, runDate)
	if err := conn.Redis.Set(ctx, key, b, 72*time.Hour).Err(); err != nil {
		conn.Logger.Warnw("failed to write schedule to Redis", "train_uid", schedule.TrainUID, "error", err)
	} else {
		conn.Logger.Infow("wrote schedule to Redis", "key", key)
	}

	return nil
}

func insertScheduleLocation(ctx context.Context, tx pgx.Tx, scheduleID int, location *types.VSTPScheduleLocation, order int) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO schedule_location (
			schedule_id, location_type, record_identity, tiploc_code, tiploc_instance,
			arrival, public_arrival, departure, public_departure, pass,
			platform, line, path, engineering_allowance, pathing_allowance,
			performance_allowance, location_order, activity
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
		)`,
		scheduleID,
		"LO",
		"LO",
		location.Location.Tiploc.TiplocId,
		nil,
		utils.ParseTime(location.ScheduledArrivalTime),
		utils.ParseTime(location.PublicArrivalTime),
		utils.ParseTime(location.ScheduledDepartureTime),
		utils.ParseTime(location.PublicDepartureTime),
		utils.ParseTime(location.ScheduledPassTime),
		utils.NullString(location.Platform),
		utils.NullString(location.Line),
		utils.NullString(location.Path),
		utils.NullString(location.EngineeringAllowance),
		utils.NullString(location.PathingAllowance),
		utils.NullString(location.PerformanceAllowance),
		order,
		utils.NullString(location.Activity),
	)

	return err
}
