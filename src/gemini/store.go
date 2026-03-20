package main

import (
	"context"
	"strings"
	"time"

	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StoreConsistMessage persists a PassengerTrainConsistMessage and its allocations in an append-only fashion.
// It is idempotent on MessageIdentifier: if we've already seen this message, it is ignored.
func StoreConsistMessage(ctx context.Context, pg *pgxpool.Pool, msg *types.PassengerTrainConsistMessage, toc string) error {
	if msg == nil {
		return nil
	}

	ref := strings.TrimSpace(msg.MessageHeader.MessageReference.MessageIdentifier)
	if ref == "" {
		// No identifier; do not attempt to store to avoid unbounded duplicates.
		return nil
	}

	// Parse message date/time if present.
	var msgTime *time.Time
	if ts := strings.TrimSpace(msg.MessageHeader.MessageReference.MessageDateTime); ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			msgTime = &t
		}
	}

	tx, err := pg.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Idempotency check.
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM gemini_message WHERE message_identifier = $1)`, ref).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return tx.Rollback(ctx)
	}

	// Insert gemini_message row.
	_, err = tx.Exec(ctx, `
		INSERT INTO gemini_message (message_identifier, message_date_time, toc)
		VALUES ($1, $2, $3)
	`, ref, msgTime, nullableString(toc))
	if err != nil {
		return err
	}

	// Extract common train identity from the message.
	op := strings.TrimSpace(msg.OperationalTrainNumberIdentifier.OperationalTrainNumber)
	var core, startDateStr, company string
	if transports := msg.TrainOperationalIdentification.Transports; len(transports) > 0 {
		core = strings.TrimSpace(transports[0].Core)
		startDateStr = strings.TrimSpace(transports[0].StartDate)
		company = strings.TrimSpace(transports[0].Company)
	}

	startDate, _ := parseDate(startDateStr)

	for _, a := range msg.Allocations {
		rg := a.ResourceGroup
		rgID := strings.TrimSpace(rg.ResourceGroupId)
		if rgID == "" {
			continue
		}

		diagramDate, _ := parseDate(strings.TrimSpace(a.DiagramDate))

		trainOrigin := tiplocFrom(a.TrainOriginLocation)
		trainDest := tiplocFrom(a.TrainDestLocation)
		allocationOrigin := tiplocFrom(a.AllocationOriginLocation)
		allocationDest := tiplocFrom(a.AllocationDestinationLocation)

		_, err = tx.Exec(ctx, `
			INSERT INTO gemini_allocation (
				resource_group_id,
				diagram_date,
				operational_train_number,
				core,
				start_date,
				company,
				allocation_sequence_number,
				train_origin_tiploc,
				train_dest_tiploc,
				allocation_origin_tiploc,
				allocation_dest_tiploc,
				resource_group_position,
				reversed,
				type_of_resource,
				message_identifier,
				message_date_time
			)
			VALUES (
				$1, $2, $3, $4, $5, $6,
				$7, $8, $9, $10, $11,
				$12, $13, $14, $15, $16
			)
		`,
			rgID,
			diagramDate,
			op,
			core,
			startDate,
			nullableString(company),
			a.AllocationSequenceNumber,
			nullableString(trainOrigin),
			nullableString(trainDest),
			nullableString(allocationOrigin),
			nullableString(allocationDest),
			strings.TrimSpace(a.ResourceGroupPosition),
			strings.TrimSpace(a.Reversed),
			strings.TrimSpace(rg.TypeOfResource),
			ref,
			msgTime,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func parseDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

