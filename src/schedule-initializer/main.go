package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
)

func parseDate(dateStr string) (time.Time, error) {
	layouts := []string{
		"2006-01-02T15:04:05Z", // ISO 8601 with timezone
		"2006-01-02T15:04:05",  // ISO 8601 without timezone
		"2006-01-02",           // Simple date format
		"06-01-02",
		"060102",
		"2006/01/02",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("failed to parse date '%s' with any known layout", dateStr)
}

func parseTime(timeStr string) (*time.Time, error) {
	if timeStr == "" {
		return nil, nil
	}

	if len(timeStr) == 5 && timeStr[4] == 'H' {
		t, err := time.Parse("1504", timeStr[:4])
		if err != nil {
			return nil, err
		}
		t = t.Add(30 * time.Second)
		return &t, nil
	}

	t, err := time.Parse("1504", timeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid time format: %s", timeStr)
	}

	return &t, nil
}

func main() {
	utils.InitLogger()
	defer utils.SyncLogger()
	l := utils.GetLogger()
	l.Info("Starting schedule initialization...")

	pg, err := utils.NewPostgresConnection()
	if err != nil {
		l.Fatalw("Failed to connect to database", "error", err)
	}
	defer pg.Close()

	url := "https://publicdatafeeds.networkrail.co.uk/ntrod/CifFileAuthenticate?type=CIF_ALL_FULL_DAILY&day=toc-full"

	username := os.Getenv("NR_FEEDS_USERNAME")
	password := os.Getenv("NR_FEEDS_PASSWORD")

	if username == "" || password == "" {
		l.Fatal("NR_FEEDS_USERNAME and NR_FEEDS_PASSWORD environment variables must be set")
	}

	l.Info("Downloading schedule data...")
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		l.Fatalw("Failed to create request", "error", err)
	}
	req.SetBasicAuth(username, password)

	resp, err := client.Do(req)
	if err != nil {
		l.Fatalw("Failed to download schedule data", "error", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		l.Fatalf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}

	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		l.Fatalw("Failed to create gzip reader", "error", err)
	}
	defer gzReader.Close()

	l.Info("Processing schedule data...")

	var processedCount int
	var tiplocCount int
	var associationCount int
	var scheduleCount int

	scanner := bufio.NewScanner(gzReader)
	for scanner.Scan() {
		line := scanner.Text()
		var entry types.TimetableEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			l.Warnw("Error unmarshalling JSON", "error", err)
			continue
		}

		processedCount++
		if processedCount%10000 == 0 {
			l.Infof("Processed %d entries (TIPLOCs: %d, Associations: %d, Schedules: %d)",
				processedCount, tiplocCount, associationCount, scheduleCount)
		}

		switch {
		case entry.JsonTimetableV1 != nil:
			// Not needed currently

		case entry.TiplocV1 != nil:
			_, err := pg.Exec(context.Background(), `INSERT INTO tiploc (tiploc_code, nalco, stanox, crs_code, description, tps_description)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (tiploc_code) DO UPDATE SET
					nalco = EXCLUDED.nalco,
					stanox = EXCLUDED.stanox,
					crs_code = EXCLUDED.crs_code,
					description = EXCLUDED.description,
					tps_description = EXCLUDED.tps_description`,
				entry.TiplocV1.TiplocCode,
				entry.TiplocV1.Nalco,
				entry.TiplocV1.Stanox,
				entry.TiplocV1.CrsCode,
				entry.TiplocV1.Description,
				entry.TiplocV1.TpsDescription,
			)
			if err != nil {
				l.Warnw("Error inserting Tiploc", "tiploc", entry.TiplocV1.TiplocCode, "error", err)
			} else {
				tiplocCount++
			}

		case entry.JsonAssociationV1 != nil:
			assoc := entry.JsonAssociationV1

			startDate, err := parseDate(assoc.AssocStartDate)
			if err != nil {
				l.Warnw("Error parsing association start date", "assoc_start_date", assoc.AssocStartDate, "error", err)
				continue
			}

			endDate, err := parseDate(assoc.AssocEndDate)
			if err != nil {
				l.Warnw("Error parsing association end date", "assoc_end_date", assoc.AssocEndDate, "error", err)
				continue
			}

			_, err = pg.Exec(context.Background(), `
				INSERT INTO association (
					transaction_type, main_train_uid, assoc_train_uid, assoc_start_date,
					assoc_end_date, assoc_days, category, date_indicator,
					location, base_location_suffix, assoc_location_suffix,
					diagram_type, stp_indicator
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
				ON CONFLICT (main_train_uid, assoc_train_uid, assoc_start_date, location, stp_indicator)
				DO UPDATE SET
					transaction_type = EXCLUDED.transaction_type,
					assoc_end_date = EXCLUDED.assoc_end_date,
					assoc_days = EXCLUDED.assoc_days,
					category = EXCLUDED.category,
					date_indicator = EXCLUDED.date_indicator,
					base_location_suffix = EXCLUDED.base_location_suffix,
					assoc_location_suffix = EXCLUDED.assoc_location_suffix,
					diagram_type = EXCLUDED.diagram_type`,
				assoc.TransactionType,
				assoc.MainTrainUID,
				assoc.AssocTrainUID,
				startDate,
				endDate,
				assoc.AssocDays,
				assoc.Category,
				assoc.DateIndicator,
				assoc.Location,
				assoc.BaseLocationSuffix,
				assoc.AssocLocationSuffix,
				assoc.DiagramType,
				assoc.StpIndicator,
			)

			if err != nil {
				l.Warnw("Error inserting association", "main_train_uid", assoc.MainTrainUID, "assoc_train_uid", assoc.AssocTrainUID, "error", err)
			} else {
				associationCount++
			}

		case entry.JsonScheduleV1 != nil:
			tx, err := pg.Begin(context.Background())
			if err != nil {
				l.Warnw("Error starting transaction for schedule", "train_uid", entry.JsonScheduleV1.TrainUID, "error", err)
				continue
			}

			schedule := entry.JsonScheduleV1

			startDate, err := parseDate(schedule.ScheduleStartDate)
			if err != nil {
				l.Warnw("Error parsing schedule start date", "schedule_start_date", schedule.ScheduleStartDate, "error", err)
				tx.Rollback(context.Background())
				continue
			}

			endDate, err := parseDate(schedule.ScheduleEndDate)
			if err != nil {
				l.Warnw("Error parsing schedule end date", "schedule_end_date", schedule.ScheduleEndDate, "error", err)
				tx.Rollback(context.Background())
				continue
			}

			var scheduleID int
			err = tx.QueryRow(context.Background(), `
				INSERT INTO schedule (
					train_uid, transaction_type, stp_indicator, bank_holiday_running,
					applicable_timetable, atoc_code, schedule_days_runs, schedule_start_date,
					schedule_end_date, train_status, signalling_id, train_category,
					headcode, course_indicator, train_service_code, business_sector,
					power_type, timing_load, speed, operating_characteristics,
					train_class, sleepers, reservations, connection_indicator,
					catering_code, service_branding, traction_class, uic_code
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28)
				RETURNING id`,
				schedule.TrainUID,
				schedule.TransactionType,
				schedule.StpIndicator,
				schedule.BankHolidayRunning,
				schedule.ApplicableTimetable,
				schedule.AtocCode,
				schedule.ScheduleDaysRuns,
				startDate,
				endDate,
				schedule.TrainStatus,
				schedule.ScheduleSegment.SignallingID,
				schedule.ScheduleSegment.TrainCategory,
				schedule.ScheduleSegment.Headcode,
				schedule.ScheduleSegment.CourseIndicator,
				schedule.ScheduleSegment.TrainServiceCode,
				schedule.ScheduleSegment.BusinessSector,
				schedule.ScheduleSegment.PowerType,
				schedule.ScheduleSegment.TimingLoad,
				schedule.ScheduleSegment.Speed,
				schedule.ScheduleSegment.OperatingCharacteristics,
				schedule.ScheduleSegment.TrainClass,
				schedule.ScheduleSegment.Sleepers,
				schedule.ScheduleSegment.Reservations,
				schedule.ScheduleSegment.ConnectionIndicator,
				schedule.ScheduleSegment.CateringCode,
				schedule.ScheduleSegment.ServiceBranding,
				func() *string {
					if schedule.NewScheduleSegment != nil {
						return &schedule.NewScheduleSegment.TractionClass
					}
					return nil
				}(),
				func() *string {
					if schedule.NewScheduleSegment != nil {
						return &schedule.NewScheduleSegment.UICCode
					}
					return nil
				}(),
			).Scan(&scheduleID)

			if err != nil {
				l.Warnw("Error inserting schedule", "train_uid", schedule.TrainUID, "error", err)
				tx.Rollback(context.Background())
				continue
			}

			for i, location := range schedule.ScheduleSegment.ScheduleLocation {
				arrival, err := parseTime(func() string {
					if location.Arrival != nil {
						return *location.Arrival
					}
					return ""
				}())
				if err != nil {
					l.Warnw("Error parsing arrival time", "train_uid", schedule.TrainUID, "location_index", i, "error", err)
					continue
				}

				publicArrival, err := parseTime(func() string {
					if location.PublicArrival != nil {
						return *location.PublicArrival
					}
					return ""
				}())
				if err != nil {
					l.Warnw("Error parsing public arrival time", "train_uid", schedule.TrainUID, "location_index", i, "error", err)
					continue
				}

				departure, err := parseTime(func() string {
					if location.Departure != nil {
						return *location.Departure
					}
					return ""
				}())
				if err != nil {
					l.Warnw("Error parsing departure time", "train_uid", schedule.TrainUID, "location_index", i, "error", err)
					continue
				}

				publicDeparture, err := parseTime(func() string {
					if location.PublicDeparture != nil {
						return *location.PublicDeparture
					}
					return ""
				}())
				if err != nil {
					l.Warnw("Error parsing public departure time", "train_uid", schedule.TrainUID, "location_index", i, "error", err)
					continue
				}

				pass, err := parseTime(func() string {
					if location.Pass != nil {
						return *location.Pass
					}
					return ""
				}())
				if err != nil {
					l.Warnw("Error parsing pass time", "train_uid", schedule.TrainUID, "location_index", i, "error", err)
					continue
				}

				_, err = tx.Exec(context.Background(), `
					INSERT INTO schedule_location (
						schedule_id, location_type, record_identity, tiploc_code,
						tiploc_instance, arrival, public_arrival, departure,
						public_departure, pass, platform, line, path,
						engineering_allowance, pathing_allowance, performance_allowance,
						location_order
					) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
					scheduleID,
					location.LocationType,
					location.RecordIdentity,
					location.TiplocCode,
					location.TiplocInstance,
					arrival,
					publicArrival,
					departure,
					publicDeparture,
					pass,
					location.Platform,
					location.Line,
					location.Path,
					location.EngineeringAllowance,
					location.PathingAllowance,
					location.PerformanceAllowance,
					i+1, // location_order starts from 1
				)

				if err != nil {
					l.Warnw("Error inserting schedule location", "location_index", i, "train_uid", schedule.TrainUID, "error", err)
					break
				}
			}

			if err := tx.Commit(context.Background()); err != nil {
				l.Warnw("Error committing schedule transaction", "train_uid", schedule.TrainUID, "error", err)
			} else {
				scheduleCount++
			}

		case entry.EOF != nil && entry.EOF.EOF:
			l.Info("End of schedule data reached.")
			goto endProcessing

		default:
			// Skip unknown entry types
		}
	}

endProcessing:
	if err := scanner.Err(); err != nil {
		l.Fatalw("Error reading schedule file", "error", err)
	}

	l.Info("Schedule initialization completed successfully!")
	l.Infof("Final counts - TIPLOCs: %d, Associations: %d, Schedules: %d",
		tiplocCount, associationCount, scheduleCount)
}
