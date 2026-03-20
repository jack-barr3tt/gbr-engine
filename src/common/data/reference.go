package data

import (
	"context"
	"database/sql"

	api_types "github.com/jack-barr3tt/gbr-engine/src/common/api-types"
)

func (dc *DataClient) GetAllLocations() ([]api_types.Location, error) {
	rows, err := dc.pg.Query(context.Background(), `
		SELECT DISTINCT ON (t.stanox)
			t.stanox,
			t.crs_code,
			COALESCE(NULLIF(br.description, ''), NULLIF(bl.name, ''), t.description) AS full_name
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
		WHERE t.stanox IS NOT NULL AND t.stanox != ''
		ORDER BY
			t.stanox,
			(NULLIF(br.description, '') IS NULL),
			(NULLIF(bl.name, '') IS NULL),
			t.crs_code
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	locations := make([]api_types.Location, 0)
	for rows.Next() {
		var stanox string
		var crsCode, fullName sql.NullString

		if err := rows.Scan(&stanox, &crsCode, &fullName); err != nil {
			return nil, err
		}

		loc := api_types.Location{
			Stanox:      stanox,
			TiplocCodes: []string{},
		}

		if crsCode.Valid && crsCode.String != "" {
			loc.Crs = &crsCode.String
		}
		if fullName.Valid && fullName.String != "" {
			loc.FullName = &fullName.String
		}
		locations = append(locations, loc)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return locations, nil
}

func (dc *DataClient) GetAllOperators() ([]api_types.Operator, error) {
	rows, err := dc.pg.Query(context.Background(), `
		SELECT code, name
		FROM reference_toc
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	operators := []api_types.Operator{}

	for rows.Next() {
		var operator api_types.Operator
		if err := rows.Scan(&operator.Code, &operator.Name); err != nil {
			return nil, err
		}
		operators = append(operators, operator)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return operators, nil
}
