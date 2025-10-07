package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ReferenceRequest(endpoint string) (*http.Response, error) {
	baseUrl := os.Getenv("NR_REFERENCE_API")
	apiKey := os.Getenv("NR_REFERENCE_API_KEY")

	client := &http.Client{}

	req, err := http.NewRequest("GET", baseUrl+endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-apikey", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func UpdateStations(pg *pgxpool.Pool) error {
	res, err := ReferenceRequest("/LDBSVWS/api/ref/20211101/GetStationList/1")
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	var referenceData types.StationReference
	if err := json.Unmarshal(body, &referenceData); err != nil {
		return err
	}

	tx, err := pg.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	if _, err = tx.Exec(context.Background(), "TRUNCATE TABLE reference_station"); err != nil {
		return err
	}

	for _, station := range referenceData.StationList {
		_, err := tx.Exec(context.Background(), "INSERT INTO reference_station (crs, name) VALUES ($1, $2)", station.Crs, station.Value)
		if err != nil {
			return err
		}
	}

	tx.Exec(context.Background(), "UPDATE reference_fetch SET last_fetched = NOW() WHERE key = 'stations'")

	if err := tx.Commit(context.Background()); err != nil {
		return err
	}

	return nil
}

func UpdateTOCs(pg *pgxpool.Pool) error {
	res, err := ReferenceRequest("/LDBSVWS/api/ref/20211101/GetTOCList/1")
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	var tocData types.TOCReference
	if err := json.Unmarshal(body, &tocData); err != nil {
		return err
	}

	tx, err := pg.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	if _, err = tx.Exec(context.Background(), "TRUNCATE TABLE reference_toc"); err != nil {
		return err
	}

	for _, toc := range tocData.TOCList {
		_, err := tx.Exec(context.Background(), "INSERT INTO reference_toc (code, name) VALUES ($1, $2)", toc.TOC, toc.Value)
		if err != nil {
			return err
		}
	}

	tx.Exec(context.Background(), "UPDATE reference_fetch SET last_fetched = NOW() WHERE key = 'toc'")

	if err := tx.Commit(context.Background()); err != nil {
		return err
	}

	return nil
}

func main() {
	pg, err := utils.NewPostgresConnection()
	if err != nil {
		log.Fatal(err)
	}

	for {
		rows, err := pg.Query(context.Background(), "SELECT key FROM reference_fetch WHERE last_fetched + max_age < NOW()")
		if err != nil {
			log.Fatal(err)
		}

		var key string
		for rows.Next() {
			if err := rows.Scan(&key); err != nil {
				log.Fatal(err)
			}

			switch key {
			case "stations":
				log.Println("Updating stations reference data...")
				err := UpdateStations(pg)
				if err != nil {
					log.Printf("Error updating stations reference data: %v\n", err)
				} else {
					log.Println("Stations reference data updated successfully.")
				}
			case "toc":
				log.Println("Updating TOC reference data...")
				err := UpdateTOCs(pg)
				if err != nil {
					log.Printf("Error updating TOC reference data: %v\n", err)
				} else {
					log.Println("TOC reference data updated successfully.")
				}
			default:
				fmt.Printf("Unknown key: %s\n", key)
			}
		}

		rows.Close()

		// Sleep for a while before checking again
		time.Sleep(1 * time.Hour)
	}
}
