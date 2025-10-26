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
		SELECT stanox, description, tps_description FROM tiploc 
		WHERE description ILIKE $1 OR tps_description ILIKE $1
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
		var description, tpsDescription sql.NullString

		err := rows.Scan(&stanox, &description, &tpsDescription)
		if err != nil {
			return "", err
		}

		if !stanox.Valid {
			continue
		}

		var matchedDescription string
		if description.Valid && len(description.String) > 0 {
			matchedDescription = description.String
		} else if tpsDescription.Valid && len(tpsDescription.String) > 0 {
			matchedDescription = tpsDescription.String
		} else {
			continue
		}

		lengthDiff := len(matchedDescription) - len(name)
		if lengthDiff < 0 {
			lengthDiff = -lengthDiff
		}
		if bestMatch == nil || lengthDiff < bestMatch.lengthDiff {
			bestMatch = &match{
				stanox:      stanox.String,
				description: matchedDescription,
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
