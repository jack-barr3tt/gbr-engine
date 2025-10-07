package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	log.Println("Starting schedule initialization...")

	pg, err := utils.NewPostgresConnection()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer pg.Close()

	url := "https://publicdatafeeds.networkrail.co.uk/ntrod/CifFileAuthenticate?type=CIF_ALL_FULL_DAILY&day=toc-full"

	username := os.Getenv("NR_FEEDS_USERNAME")
	password := os.Getenv("NR_FEEDS_PASSWORD")

	if username == "" || password == "" {
		log.Fatal("NR_FEEDS_USERNAME and NR_FEEDS_PASSWORD environment variables must be set")
	}

	log.Println("Downloading schedule data...")
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal("Failed to create request:", err)
	}
	req.SetBasicAuth(username, password)

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal("Failed to download schedule data:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Fatalf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}

	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		log.Fatal("Failed to create gzip reader:", err)
	}
	defer gzReader.Close()

	log.Println("Processing schedule data...")

	var processedCount int
	var tiplocCount int
	var associationCount int
	var scheduleCount int

	scanner := bufio.NewScanner(gzReader)
	for scanner.Scan() {
		line := scanner.Text()
		var entry types.TimetableEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			log.Println("Error unmarshalling JSON:", err)
			continue
		}

		processedCount++
		if processedCount%10000 == 0 {
			log.Printf("Processed %d entries (TIPLOCs: %d, Associations: %d, Schedules: %d)",
				processedCount, tiplocCount, associationCount, scheduleCount)
		}

		switch {
		case entry.JsonTimetableV1 != nil:
			// Not needed currently

		case entry.TiplocV1 != nil:
			_, err := pg.Exec(context.Background(), `INSERT INTO tiploc (tiploc_code, nalco, stanox, crs_code, description, tps_description)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				entry.TiplocV1.TiplocCode,
				entry.TiplocV1.Nalco,
				entry.TiplocV1.Stanox,
				entry.TiplocV1.CrsCode,
				entry.TiplocV1.Description,
				entry.TiplocV1.TpsDescription,
			)
			if err != nil {
				log.Printf("Error inserting Tiploc %s: %v", entry.TiplocV1.TiplocCode, err)
			} else {
				tiplocCount++
			}

		case entry.JsonAssociationV1 != nil:
			assoc := entry.JsonAssociationV1

			startDate, err := parseDate(assoc.AssocStartDate)
			if err != nil {
				log.Printf("Error parsing association start date %s: %v", assoc.AssocStartDate, err)
				continue
			}

			endDate, err := parseDate(assoc.AssocEndDate)
			if err != nil {
				log.Printf("Error parsing association end date %s: %v", assoc.AssocEndDate, err)
				continue
			}

			_, err = pg.Exec(context.Background(), `
				INSERT INTO association (
					transaction_type, main_train_uid, assoc_train_uid, assoc_start_date,
					assoc_end_date, assoc_days, category, date_indicator,
					location, base_location_suffix, assoc_location_suffix,
					diagram_type, stp_indicator
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
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
				log.Printf("Error inserting association %s-%s: %v", assoc.MainTrainUID, assoc.AssocTrainUID, err)
			} else {
				associationCount++
			}

		case entry.JsonScheduleV1 != nil:
			tx, err := pg.Begin(context.Background())
			if err != nil {
				log.Printf("Error starting transaction for schedule %s: %v", entry.JsonScheduleV1.TrainUID, err)
				continue
			}

			schedule := entry.JsonScheduleV1

			startDate, err := parseDate(schedule.ScheduleStartDate)
			if err != nil {
				log.Printf("Error parsing schedule start date %s: %v", schedule.ScheduleStartDate, err)
				tx.Rollback(context.Background())
				continue
			}

			endDate, err := parseDate(schedule.ScheduleEndDate)
			if err != nil {
				log.Printf("Error parsing schedule end date %s: %v", schedule.ScheduleEndDate, err)
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
				log.Printf("Error inserting schedule %s: %v", schedule.TrainUID, err)
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
					log.Printf("Error parsing arrival time for schedule %s location %d: %v", schedule.TrainUID, i, err)
					continue
				}

				publicArrival, err := parseTime(func() string {
					if location.PublicArrival != nil {
						return *location.PublicArrival
					}
					return ""
				}())
				if err != nil {
					log.Printf("Error parsing public arrival time for schedule %s location %d: %v", schedule.TrainUID, i, err)
					continue
				}

				departure, err := parseTime(func() string {
					if location.Departure != nil {
						return *location.Departure
					}
					return ""
				}())
				if err != nil {
					log.Printf("Error parsing departure time for schedule %s location %d: %v", schedule.TrainUID, i, err)
					continue
				}

				publicDeparture, err := parseTime(func() string {
					if location.PublicDeparture != nil {
						return *location.PublicDeparture
					}
					return ""
				}())
				if err != nil {
					log.Printf("Error parsing public departure time for schedule %s location %d: %v", schedule.TrainUID, i, err)
					continue
				}

				pass, err := parseTime(func() string {
					if location.Pass != nil {
						return *location.Pass
					}
					return ""
				}())
				if err != nil {
					log.Printf("Error parsing pass time for schedule %s location %d: %v", schedule.TrainUID, i, err)
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
					log.Printf("Error inserting schedule location %d for schedule %s: %v", i, schedule.TrainUID, err)
					break
				}
			}

			if err := tx.Commit(context.Background()); err != nil {
				log.Printf("Error committing schedule transaction for %s: %v", schedule.TrainUID, err)
			} else {
				scheduleCount++
			}

		case entry.EOF != nil && entry.EOF.EOF:
			log.Println("End of schedule data reached.")
			goto endProcessing

		default:
			// Skip unknown entry types
		}
	}

endProcessing:
	if err := scanner.Err(); err != nil {
		log.Fatal("Error reading schedule file:", err)
	}

	log.Printf("Schedule initialization completed successfully!")
	log.Printf("Final counts - TIPLOCs: %d, Associations: %d, Schedules: %d",
		tiplocCount, associationCount, scheduleCount)
}
