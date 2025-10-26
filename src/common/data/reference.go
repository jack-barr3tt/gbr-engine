package data

import (
	"context"
	"database/sql"

	api_types "github.com/jack-barr3tt/gbr-engine/src/common/api-types"
)

func (dc *DataClient) GetAllLocations() ([]api_types.Location, error) {
	rows, err := dc.pg.Query(context.Background(), `
		SELECT DISTINCT stanox, crs_code, description
		FROM tiploc
		WHERE stanox IS NOT NULL AND stanox != ''
		ORDER BY description, crs_code
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	locationsMap := make(map[string]*api_types.Location)

	for rows.Next() {
		var stanox string
		var crsCode, description sql.NullString

		if err := rows.Scan(&stanox, &crsCode, &description); err != nil {
			return nil, err
		}

		loc, exists := locationsMap[stanox]
		if !exists {
			loc = &api_types.Location{
				Stanox:      stanox,
				TiplocCodes: []string{},
			}
			locationsMap[stanox] = loc
		}

		if crsCode.Valid && crsCode.String != "" && loc.Crs == nil {
			loc.Crs = &crsCode.String
		}
		if description.Valid && description.String != "" && loc.FullName == nil {
			loc.FullName = &description.String
		}
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	locations := make([]api_types.Location, 0, len(locationsMap))
	for _, loc := range locationsMap {
		locations = append(locations, *loc)
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
