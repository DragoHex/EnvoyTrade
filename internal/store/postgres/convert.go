package postgres

import (
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/store/postgres/sqlcgen"

	"github.com/jackc/pgx/v5/pgtype"
)

func pgtimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func pgdate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func toFollowerOrder(row sqlcgen.FollowerOrder) domain.FollowerOrder {
	o := domain.FollowerOrder{
		ID:             row.ID,
		MasterFillID:   row.MasterFillID,
		FollowerID:     row.FollowerID,
		IdempotencyTag: row.IdempotencyTag,
		IntendedQty:    int(row.IntendedQty),
		LotSize:        int(row.LotSize),
		SizingReason:   domain.SizingReason(row.SizingReason),
		FilledQty:      int(row.FilledQty),
		AttemptCount:   int(row.AttemptCount),
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}
	if row.PlacedQty != nil {
		placedQty := int(*row.PlacedQty)
		o.PlacedQty = &placedQty
	}
	if row.BrokerOrderID != nil {
		o.BrokerOrderID = *row.BrokerOrderID
	}
	if row.TerminalStatus != nil {
		o.TerminalStatus = *row.TerminalStatus
	}
	if row.LastError != nil {
		o.LastError = *row.LastError
	}
	if row.AveragePrice.Valid {
		o.AveragePrice = row.AveragePrice.Decimal
	}
	return o
}
