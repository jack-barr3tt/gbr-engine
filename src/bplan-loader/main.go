package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
)

const (
	dataDir       = "data"
	stateFileName = ".bplan-state"
	checkInterval = 1 * time.Hour
)

var errNoTxtFile = errors.New("no .txt file found in data directory")

// findFirstTxtFile returns the path to the first .txt file in dir (by name order), or an error if dir is missing or contains no .txt files.
func findFirstTxtFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("data directory does not exist: %s", dir)
		}
		return "", err
	}
	var txtNames []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(e.Name()), ".txt") {
			txtNames = append(txtNames, e.Name())
		}
	}
	if len(txtNames) == 0 {
		return "", errNoTxtFile
	}
	sort.Strings(txtNames)
	return filepath.Join(dir, txtNames[0]), nil
}

func main() {
	utils.InitLogger()
	defer utils.SyncLogger()
	log := utils.GetLogger()

	pg, err := utils.NewPostgresConnection()
	if err != nil {
		log.Fatalw("failed to connect to Postgres", "error", err)
	}
	defer pg.Close()

	log.Infow("bplan-loader started", "data_dir", dataDir, "check_interval", checkInterval)

	for {
		datasetPath, err := findFirstTxtFile(dataDir)
		if err != nil {
			if errors.Is(err, errNoTxtFile) {
				log.Warnw("no .txt file in data directory, will retry", "data_dir", dataDir)
			} else {
				log.Warnw("cannot find dataset, will retry", "data_dir", dataDir, "error", err)
			}
			time.Sleep(checkInterval)
			continue
		}

		stat, err := os.Stat(datasetPath)
		if err != nil {
			log.Warnw("dataset not readable, will retry", "path", datasetPath, "error", err)
			time.Sleep(checkInterval)
			continue
		}
		datasetMod := stat.ModTime()
		statePath := filepath.Join(dataDir, stateFileName)

		needParse := true
		if b, err := os.ReadFile(statePath); err == nil {
			var stored time.Time
			if err := stored.UnmarshalText(b); err == nil && stored.Equal(datasetMod) {
				needParse = false
				log.Infow("dataset unchanged, skipping parse", "path", datasetPath)
			}
		}

		if needParse {
			log.Infow("parsing BPLAN dataset", "path", datasetPath)

			data, c, pit, err := LoadBPLAN(datasetPath)
			if err != nil {
				if errors.Is(err, errNotBPLAN) || errors.Is(err, errNoPITTrailer) {
					log.Errorw("invalid BPLAN file", "path", datasetPath, "error", err)
				} else {
					log.Errorw("load BPLAN failed", "path", datasetPath, "error", err)
				}
				time.Sleep(checkInterval)
				continue
			}

			ok, details := ValidatePIT(c, pit)
			if ok {
				log.Infow("PIT validation passed", "ref", c.ref, "tld", c.tld, "loc", c.loc, "plt", c.plt, "nwk", c.nwk, "tlk", c.tlk)
			} else {
				log.Warnw("PIT validation failed", "details", details)
			}

			ctx := context.Background()
			if err := StoreBPLAN(ctx, pg, data); err != nil {
				log.Errorw("failed to store BPLAN to database", "error", err)
				time.Sleep(checkInterval)
				continue
			}
			log.Infow("BPLAN data stored", "pif", len(data.Pif), "ref", len(data.Ref), "loc", len(data.Loc), "tld", len(data.Tld), "plt", len(data.Plt), "nwk", len(data.Nwk), "tlk", len(data.Tlk))

			mt, _ := datasetMod.MarshalText()
			if err := os.WriteFile(statePath, mt, 0600); err != nil {
				log.Warnw("could not write state file", "path", statePath, "error", err)
			}
		}

		time.Sleep(checkInterval)
	}
}
