package postgres

import (
	"context"

	"envoytrade/internal/domain"
	"envoytrade/internal/store/postgres/sqlcgen"

	"github.com/google/uuid"
)

// PositionSyncParam represents one position row to sync.
type PositionSyncParam = domain.PositionSyncParam

// HoldingSyncParam represents one holding row to sync.
type HoldingSyncParam = domain.HoldingSyncParam

// MarginSyncParam represents account margin and summary metrics to sync.
type MarginSyncParam = domain.MarginSyncParam

// SyncAccountPositions upserts all positions for an account.
func (s *Store) SyncAccountPositions(ctx context.Context, accountID uuid.UUID, positions []PositionSyncParam) error {
	for _, p := range positions {
		act := p.Action
		if act == "" {
			act = "exit"
		}
		err := s.queries.UpsertAccountPosition(ctx, sqlcgen.UpsertAccountPositionParams{
			AccountID:    accountID,
			Product:      p.Product,
			Instrument:   p.Instrument,
			Quantity:     int32(p.Quantity),
			BuyPrice:     p.BuyPrice,
			SellPrice:    p.SellPrice,
			BuyQuantity:  int32(p.BuyQuantity),
			SellQuantity: int32(p.SellQuantity),
			Ltp:          p.Ltp,
			Mtm:          p.Mtm,
			Pnl:          p.Pnl,
			Action:       act,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// SyncAccountHoldings upserts all holdings for an account.
func (s *Store) SyncAccountHoldings(ctx context.Context, accountID uuid.UUID, holdings []HoldingSyncParam) error {
	for _, h := range holdings {
		act := h.Action
		if act == "" {
			act = "exit"
		}
		err := s.queries.UpsertAccountHolding(ctx, sqlcgen.UpsertAccountHoldingParams{
			AccountID:        accountID,
			Instrument:       h.Instrument,
			SellableQuantity: int32(h.SellableQuantity),
			BuyAveragePrice:  h.BuyAveragePrice,
			Ltp:              h.Ltp,
			Pnl:              h.Pnl,
			Action:           act,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// SyncAccountMargins upserts the aggregated margins and metrics for an account.
func (s *Store) SyncAccountMargins(ctx context.Context, accountID uuid.UUID, m MarginSyncParam) error {
	status := m.Status
	if status == "" {
		status = "online"
	}
	return s.queries.UpsertAccountMargins(ctx, sqlcgen.UpsertAccountMarginsParams{
		AccountID:    accountID,
		NetQty:       int32(m.NetQty),
		TotalMtm:     m.TotalMtm,
		RealizedPnl:  m.RealizedPnl,
		AccountValue: m.AccountValue,
		Status:       status,
	})
}
