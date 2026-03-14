package main

import (
	"context"
	"encoding/json"
	"io"
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
	utils.InitLogger()
	defer utils.SyncLogger()
	log := utils.GetLogger()

	pg, err := utils.NewPostgresConnection()
	if err != nil {
		log.Fatalw("failed to connect to Postgres", "error", err)
	}

	// Ensure reference_fetch has the toc row (schema may not have run the seed INSERT)
	_, err = pg.Exec(context.Background(), `
		INSERT INTO reference_fetch (key, last_fetched, max_age)
		VALUES ('toc', '2000-01-01 00:00:00', '1 week')
		ON CONFLICT (key) DO NOTHING`)
	if err != nil {
		log.Fatalw("failed to seed reference_fetch", "error", err)
	}
	log.Info("data-fetcher started, checking for stale reference data...")

	for {
		rows, err := pg.Query(context.Background(), "SELECT key FROM reference_fetch WHERE last_fetched + max_age < NOW()")
		if err != nil {
			log.Fatalw("failed to query reference_fetch", "error", err)
		}

		var key string
		updated := false
		for rows.Next() {
			if err := rows.Scan(&key); err != nil {
				rows.Close()
				log.Fatalw("failed to scan key", "error", err)
			}

			switch key {
			case "toc":
				log.Info("Updating TOC reference data...")
				err := UpdateTOCs(pg)
				if err != nil {
					log.Warnw("Error updating TOC reference data", "error", err)
				} else {
					log.Info("TOC reference data updated successfully.")
				}
				updated = true
			default:
				log.Infow("unknown reference key", "key", key)
			}
		}

		rows.Close()

		if !updated {
			log.Info("No stale reference data, sleeping 1h")
		}
		time.Sleep(1 * time.Hour)
	}
}
