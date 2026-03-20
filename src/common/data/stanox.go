package data

import (
	"context"
	"database/sql"
)

func (dc *DataClient) GetStanoxByTiploc(tiploc string) (string, error) {
	var stanox sql.NullString
	err := dc.pg.QueryRow(context.Background(), `
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

func (dc *DataClient) GetStanoxByCRS(crsCode string) (string, error) {
	var stanox sql.NullString
	err := dc.pg.QueryRow(context.Background(), `
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

func (dc *DataClient) GetStanoxByLocationName(name string) (string, error) {
	rows, err := dc.pg.Query(context.Background(), `
		SELECT DISTINCT
			t.stanox,
			COALESCE(NULLIF(br.description, ''), NULLIF(bl.name, ''), t.description, t.tps_description) AS matched_name
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
		WHERE t.stanox IS NOT NULL
		  AND t.stanox != ''
		  AND (
			br.description ILIKE $1
			OR bl.name ILIKE $1
			OR t.description ILIKE $1
			OR t.tps_description ILIKE $1
		  )
	`, "%"+name+"%")

	if err != nil {
		return "", err
	}

	type match struct {
		stanox      string
		description string
		lengthDiff  int
	}
	var bestMatch *match

	for rows.Next() {
		var stanox sql.NullString
		var matchedName sql.NullString

		err := rows.Scan(&stanox, &matchedName)
		if err != nil {
			return "", err
		}

		if !stanox.Valid {
			continue
		}

		if !matchedName.Valid || matchedName.String == "" {
			continue
		}

		lengthDiff := len(matchedName.String) - len(name)
		if lengthDiff < 0 {
			lengthDiff = -lengthDiff
		}
		if bestMatch == nil || lengthDiff < bestMatch.lengthDiff {
			bestMatch = &match{
				stanox:      stanox.String,
				description: matchedName.String,
				lengthDiff:  lengthDiff,
			}
		}
	}

	if err = rows.Err(); err != nil {
		return "", err
	}

	if bestMatch == nil {
		return "", sql.ErrNoRows
	}

	return bestMatch.stanox, nil
}
