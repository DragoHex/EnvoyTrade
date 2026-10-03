package postgres

import (
	"context"

	"envoytrade/internal/domain"
	"envoytrade/internal/store/postgres/sqlcgen"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PositionSyncParam represents one position row to sync.
type PositionSyncParam = domain.PositionSyncParam

// HoldingSyncParam represents one holding row to sync.
type HoldingSyncParam = domain.HoldingSyncParam

// MarginSyncParam represents account margin and summary metrics to sync.
type MarginSyncParam = domain.MarginSyncParam

// SyncAccountPositions upserts all positions for an account and prunes stale positions.
func (s *Store) SyncAccountPositions(ctx context.Context, accountID uuid.UUID, positions []PositionSyncParam) error {
	seenKeys := make([]string, 0, len(positions))
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
		seenKeys = append(seenKeys, p.Product+":"+p.Instrument)
	}

	if len(seenKeys) == 0 {
		_, err := s.pool.Exec(ctx, "DELETE FROM account_positions WHERE account_id = $1", accountID)
		return err
	}
	_, err := s.pool.Exec(ctx, `
		DELETE FROM account_positions
		WHERE account_id = $1
		  AND (product || ':' || instrument) != ALL($2)
	`, accountID, seenKeys)
	return err
}

// SyncAccountHoldings upserts all holdings for an account and prunes stale holdings.
func (s *Store) SyncAccountHoldings(ctx context.Context, accountID uuid.UUID, holdings []HoldingSyncParam) error {
	seenInstruments := make([]string, 0, len(holdings))
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
		seenInstruments = append(seenInstruments, h.Instrument)
	}

	if len(seenInstruments) == 0 {
		_, err := s.pool.Exec(ctx, "DELETE FROM account_holdings WHERE account_id = $1", accountID)
		return err
	}
	_, err := s.pool.Exec(ctx, `
		DELETE FROM account_holdings
		WHERE account_id = $1
		  AND instrument != ALL($2)
	`, accountID, seenInstruments)
	return err
}

// SyncAccountMargins upserts the aggregated margins and metrics for an account.
func (s *Store) SyncAccountMargins(ctx context.Context, accountID uuid.UUID, m MarginSyncParam) error {
	status := m.Status
	if status == "" {
		status = "online"
	}
	cash := decimal.Zero
	if m.AvailableCash != nil {
		cash = *m.AvailableCash
	}
	margin := decimal.Zero
	if m.AvailableMargin != nil {
		margin = *m.AvailableMargin
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO account_margins
		  (account_id, net_qty, total_mtm, realized_pnl, account_value, available_cash, available_margin, status, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (account_id)
		DO UPDATE SET
		  net_qty = EXCLUDED.net_qty,
		  total_mtm = EXCLUDED.total_mtm,
		  realized_pnl = EXCLUDED.realized_pnl,
		  account_value = EXCLUDED.account_value,
		  available_cash = EXCLUDED.available_cash,
		  available_margin = EXCLUDED.available_margin,
		  status = EXCLUDED.status,
		  updated_at = now();
	`, accountID, m.NetQty, m.TotalMtm, m.RealizedPnl, m.AccountValue, cash, margin, status)
	return err
}
