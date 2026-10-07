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
	var masterFillID int64
	if row.MasterFillID != nil {
		masterFillID = *row.MasterFillID
	}
	o := domain.FollowerOrder{
		ID:              row.ID,
		MasterFillID:    masterFillID,
		FollowerID:      row.FollowerID,
		IdempotencyTag:  row.IdempotencyTag,
		IntendedQty:     int(row.IntendedQty),
		LotSize:         int(row.LotSize),
		SizingReason:    domain.SizingReason(row.SizingReason),
		FilledQty:       int(row.FilledQty),
		AttemptCount:    int(row.AttemptCount),
		Origin:          row.Origin,
		Tradingsymbol:   row.Tradingsymbol,
		Exchange:        row.Exchange,
		Product:         row.Product,
		TransactionType: row.TransactionType,
		OrderType:       row.OrderType,
		CreatedAt:       row.CreatedAt.Time.In(domain.IST),
		UpdatedAt:       row.UpdatedAt.Time.In(domain.IST),
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
