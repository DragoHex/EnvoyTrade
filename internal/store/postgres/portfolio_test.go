//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"envoytrade/internal/store/postgres"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestStore_SyncPositionsAndHoldings(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	accID := uuid.New()
	err := store.CreateAccount(ctx, accID, "Test Sync Acc", "follower", "kite", "SYNC123", "test_api_key", "secret", "127.0.0.1")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	positions := []postgres.PositionSyncParam{
		{
			Product:      "NRML",
			Instrument:   "NIFTY26SEP24000CE",
			Quantity:     50,
			BuyPrice:     decimal.NewFromFloat(150.0),
			SellPrice:    decimal.Zero,
			BuyQuantity:  50,
			SellQuantity: 0,
			Ltp:          decimal.NewFromFloat(165.5),
			Mtm:          decimal.NewFromFloat(775.0),
			Pnl:          decimal.NewFromFloat(775.0),
			Action:       "exit",
		},
	}
	if err := store.SyncAccountPositions(ctx, accID, positions); err != nil {
		t.Fatalf("SyncAccountPositions: %v", err)
	}

	holdings := []postgres.HoldingSyncParam{
		{
			Instrument:       "INFY",
			SellableQuantity: 100,
			BuyAveragePrice:  decimal.NewFromFloat(1400.0),
			Ltp:              decimal.NewFromFloat(1450.0),
			Pnl:              decimal.NewFromFloat(5000.0),
			Action:           "exit",
		},
	}
	if err := store.SyncAccountHoldings(ctx, accID, holdings); err != nil {
		t.Fatalf("SyncAccountHoldings: %v", err)
	}

	margin := postgres.MarginSyncParam{
		NetQty:       50,
		TotalMtm:     decimal.NewFromFloat(775.0),
		RealizedPnl:  decimal.Zero,
		AccountValue: decimal.NewFromFloat(100775.0),
		Status:       "online",
	}
	if err := store.SyncAccountMargins(ctx, accID, margin); err != nil {
		t.Fatalf("SyncAccountMargins: %v", err)
	}

	detail, err := store.AccountOrders(ctx, accID, "open_positions", 1, 10)
	if err != nil {
		t.Fatalf("AccountOrders(open_positions): %v", err)
	}
	if len(detail.OpenPositions) != 1 {
		t.Fatalf("OpenPositions len = %d, want 1", len(detail.OpenPositions))
	}
	if detail.OpenPositions[0].Instrument != "NIFTY26SEP24000CE" {
		t.Errorf("Instrument = %s, want NIFTY26SEP24000CE", detail.OpenPositions[0].Instrument)
	}
	if detail.Summary.NetQty != 50 {
		t.Errorf("Summary.NetQty = %d, want 50", detail.Summary.NetQty)
	}

	holdingsDetail, err := store.AccountOrders(ctx, accID, "holdings", 1, 10)
	if err != nil {
		t.Fatalf("AccountOrders(holdings): %v", err)
	}
	if len(holdingsDetail.Holdings) != 1 {
		t.Fatalf("Holdings len = %d, want 1", len(holdingsDetail.Holdings))
	}
	if holdingsDetail.Holdings[0].Instrument != "INFY" {
		t.Errorf("Instrument = %s, want INFY", holdingsDetail.Holdings[0].Instrument)
	}
}
