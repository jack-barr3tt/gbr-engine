package main

import (
	"context"
	"fmt"

	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const insertBatchSize = 5000

// StoreBPLAN truncates BPLAN tables and inserts all data in a single transaction.
func StoreBPLAN(ctx context.Context, pg *pgxpool.Pool, data bplanData) error {
	tx, err := pg.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tables := []string{"bplan_tlk", "bplan_nwk", "bplan_plt", "bplan_tld", "bplan_loc", "bplan_ref", "bplan_pif"}
	for _, t := range tables {
		if _, err := tx.Exec(ctx, "TRUNCATE TABLE "+t); err != nil {
			return fmt.Errorf("truncate %s: %w", t, err)
		}
	}

	for _, r := range data.Pif {
		_, err := tx.Exec(ctx, `INSERT INTO bplan_pif (version, source, toc, timetable_start, timetable_end, cycle_type, cycle_indicator, file_creation_time, sequence)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			r.Version, r.Source, r.TOC, r.TimetableStart, r.TimetableEnd, r.CycleType, r.CycleIndicator, r.FileCreationTime, r.Sequence)
		if err != nil {
			return fmt.Errorf("insert bplan_pif: %w", err)
		}
	}

	for _, r := range data.Ref {
		_, err := tx.Exec(ctx, `INSERT INTO bplan_ref (category, subcode, description) VALUES ($1,$2,$3)`,
			r.Category, r.Subcode, r.Description)
		if err != nil {
			return fmt.Errorf("insert bplan_ref: %w", err)
		}
	}

	for _, r := range data.Loc {
		_, err := tx.Exec(ctx, `INSERT INTO bplan_loc (tiploc, name, start_date, end_date, x_coord, y_coord, optionality, zone, stanox, stanox_flag, extra)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			r.Tiploc, r.Name, r.StartDate, nullEmpty(r.EndDate), r.XCoord, r.YCoord, r.Optionality, r.Zone, nullEmpty(r.Stanox), nullEmpty(r.StanoxFlag), nullEmpty(r.Extra))
		if err != nil {
			return fmt.Errorf("insert bplan_loc: %w", err)
		}
	}

	for _, r := range data.Tld {
		_, err := tx.Exec(ctx, `INSERT INTO bplan_tld (load_code, load_subcode, speed, reserved, description, train_type, sub_type, speed_or_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			r.LoadCode, r.LoadSubcode, r.Speed, r.Reserved, r.Description, r.TrainType, r.SubType, r.SpeedOrID)
		if err != nil {
			return fmt.Errorf("insert bplan_tld: %w", err)
		}
	}

	for _, r := range data.Plt {
		_, err := tx.Exec(ctx, `INSERT INTO bplan_plt (tiploc, platform_id, start_date, end_date, platform_num, reserved, flag, public_flag)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			r.Tiploc, r.PlatformID, r.StartDate, nullEmpty(r.EndDate), nullEmpty(r.PlatformNum), nullEmpty(r.Reserved), nullEmpty(r.Flag), r.PublicFlag)
		if err != nil {
			return fmt.Errorf("insert bplan_plt: %w", err)
		}
	}

	for i := 0; i < len(data.Nwk); i += insertBatchSize {
		end := i + insertBatchSize
		if end > len(data.Nwk) {
			end = len(data.Nwk)
		}
		batch := data.Nwk[i:end]
		if err := insertNwkBatch(ctx, tx, batch); err != nil {
			return err
		}
	}

	for i := 0; i < len(data.Tlk); i += insertBatchSize {
		end := i + insertBatchSize
		if end > len(data.Tlk) {
			end = len(data.Tlk)
		}
		batch := data.Tlk[i:end]
		if err := insertTlkBatch(ctx, tx, batch); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func nullEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func insertNwkBatch(ctx context.Context, tx pgx.Tx, batch []types.NWK) error {
	for _, r := range batch {
		_, err := tx.Exec(ctx, `INSERT INTO bplan_nwk (from_tiploc, to_tiploc, link_type, reserved, start_date, end_date, direction, direction2, distance, flag1, flag2, flag3, zone, flag4, reserved2, extra, reserved3)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
			r.FromTiploc, r.ToTiploc, nullEmpty(r.LinkType), nullEmpty(r.Reserved), r.StartDate, nullEmpty(r.EndDate), r.Direction, r.Direction2, r.Distance,
			nullEmpty(r.Flag1), nullEmpty(r.Flag2), nullEmpty(r.Flag3), nullEmpty(r.Zone), nullEmpty(r.Flag4), nullEmpty(r.Reserved2), nullEmpty(r.Extra), nullEmpty(r.Reserved3))
		if err != nil {
			return fmt.Errorf("insert bplan_nwk: %w", err)
		}
	}
	return nil
}

func insertTlkBatch(ctx context.Context, tx pgx.Tx, batch []types.TLK) error {
	for _, r := range batch {
		_, err := tx.Exec(ctx, `INSERT INTO bplan_tlk (from_tiploc, to_tiploc, route_link_type, timing_load, sub_type, speed, load_variant, penalty1, penalty2, start_date, end_date, run_time, reserved)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			r.FromTiploc, r.ToTiploc, nullEmpty(r.RouteLinkType), nullEmpty(r.TimingLoad), nullEmpty(r.SubType), nullEmpty(r.Speed), nullEmpty(r.LoadVariant),
			nullEmpty(r.Penalty1), nullEmpty(r.Penalty2), r.StartDate, nullEmpty(r.EndDate), r.RunTime, nullEmpty(r.Reserved))
		if err != nil {
			return fmt.Errorf("insert bplan_tlk: %w", err)
		}
	}
	return nil
}
