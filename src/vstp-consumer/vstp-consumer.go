package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()

	log.Println("Starting VSTP consumer...")

	db, err := utils.NewPostgresConnection()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	conn, channel, err := utils.NewRabbitConnection()
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	defer channel.Close()

	_, err = channel.QueueDeclare("vstp", false, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	msgs, err := channel.Consume("vstp", "", true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Processing VSTP schedule messages...")

	for msg := range msgs {
		var vstpMsg types.VSTPMessage
		if err := json.Unmarshal(msg.Body, &vstpMsg); err != nil {
			log.Printf("Bad JSON: %v", err)
			continue
		}

		if err := processVSTPMessage(ctx, db, &vstpMsg); err != nil {
			log.Printf("Error processing VSTP message: %v", err)
			continue
		}

		fmt.Printf("Processed VSTP schedule: %s\n", vstpMsg.VSTPCIFMsgV1.Schedule.TrainUID)
	}
}

func processVSTPMessage(ctx context.Context, db *pgxpool.Pool, vstpMsg *types.VSTPMessage) error {
	schedule := &vstpMsg.VSTPCIFMsgV1.Schedule

	// Parse dates
	startDate, err := time.Parse("2006-01-02", schedule.ScheduleStartDate)
	if err != nil {
		return fmt.Errorf("invalid start date: %v", err)
	}

	endDate, err := time.Parse("2006-01-02", schedule.ScheduleEndDate)
	if err != nil {
		return fmt.Errorf("invalid end date: %v", err)
	}

	tx, err := db.Begin(ctx)
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

	return tx.Commit(ctx)
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
		"LO", // Default location type for VSTP
		"LO", // Default record identity for VSTP
		location.Location.Tiploc.TiplocId,
		nil, // tiploc_instance
		utils.ParseTime(location.ScheduledArrivalTime),   // arrival
		utils.ParseTime(location.PublicArrivalTime),      // public_arrival
		utils.ParseTime(location.ScheduledDepartureTime), // departure
		utils.ParseTime(location.PublicDepartureTime),    // public_departure
		utils.ParseTime(location.ScheduledPassTime),      // pass
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
