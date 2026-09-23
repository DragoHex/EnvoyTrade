package kite_test

import (
	"testing"

	"envoytrade/internal/kite"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

func TestIsFOOrCommodity(t *testing.T) {
	tests := []struct {
		exchange string
		want     bool
	}{
		{"NFO", true},
		{"BFO", true},
		{"CDS", true},
		{"BCD", true},
		{"MCX", true},
		{"NSE", false},
		{"BSE", false},
		{"", false},
	}

	for _, tt := range tests {
		got := kite.IsFOOrCommodity(tt.exchange)
		if got != tt.want {
			t.Errorf("IsFOOrCommodity(%q) = %v, want %v", tt.exchange, got, tt.want)
		}
	}
}

func TestFilterFOPositions(t *testing.T) {
	positions := []kiteconnect.Position{
		{Exchange: "NFO", Tradingsymbol: "NIFTY26SEP24000CE", Quantity: 50},
		{Exchange: "NSE", Tradingsymbol: "RELIANCE", Quantity: 10},
		{Exchange: "MCX", Tradingsymbol: "CRUDEOIL26OCTFUT", Quantity: 100},
		{Exchange: "BSE", Tradingsymbol: "TCS", Quantity: 5},
	}

	filtered := kite.FilterFOPositions(positions)
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	if filtered[0].Tradingsymbol != "NIFTY26SEP24000CE" || filtered[1].Tradingsymbol != "CRUDEOIL26OCTFUT" {
		t.Errorf("unexpected filtered positions: %+v", filtered)
	}
}

func TestCalculatePositionMetrics(t *testing.T) {
	positions := []kiteconnect.Position{
		{
			Exchange:      "NFO",
			Tradingsymbol: "NIFTY26SEP24000CE",
			Quantity:      50,
			M2M:           1500.50,
			Realised:      200.0,
		},
		{
			Exchange:      "MCX",
			Tradingsymbol: "GOLD26OCTFUT",
			Quantity:      0, // Closed
			M2M:           -300.0,
			Realised:      -300.0,
		},
	}

	margins := kiteconnect.AllMargins{
		Equity: kiteconnect.Margins{
			Available: kiteconnect.AvailableMargins{
				Cash:        25000.0,
				LiveBalance: 60000.0,
			},
		},
		Commodity: kiteconnect.Margins{
			Available: kiteconnect.AvailableMargins{
				Cash:        10000.0,
				LiveBalance: 15000.0,
			},
		},
	}

	summary := kite.CalculatePositionMetrics(positions, margins)

	if summary.NetQty != 50 {
		t.Errorf("NetQty = %d, want 50", summary.NetQty)
	}
	if summary.OpenPositions != 1 {
		t.Errorf("OpenPositions = %d, want 1", summary.OpenPositions)
	}
	if summary.ClosedPositions != 1 {
		t.Errorf("ClosedPositions = %d, want 1", summary.ClosedPositions)
	}
	expectedMtm := decimal.NewFromFloat(1200.50)
	if !summary.TotalMtm.Equal(expectedMtm) {
		t.Errorf("TotalMtm = %s, want %s", summary.TotalMtm, expectedMtm)
	}
	expectedRealized := decimal.NewFromFloat(-100.0)
	if !summary.RealizedPnl.Equal(expectedRealized) {
		t.Errorf("RealizedPnl = %s, want %s", summary.RealizedPnl, expectedRealized)
	}
	expectedCash := decimal.NewFromFloat(35000.0)
	if !summary.AvailableCash.Equal(expectedCash) {
		t.Errorf("AvailableCash = %s, want %s", summary.AvailableCash, expectedCash)
	}
	expectedMargin := decimal.NewFromFloat(75000.0)
	if !summary.AvailableMargin.Equal(expectedMargin) {
		t.Errorf("AvailableMargin = %s, want %s", summary.AvailableMargin, expectedMargin)
	}
}

func TestHoldingConversion(t *testing.T) {
	accID := uuid.New()
	h := kiteconnect.Holding{
		Tradingsymbol: "INFY",
		Quantity:      100,
		UsedQuantity:  20,
		AveragePrice:  1450.75,
		LastPrice:     1520.00,
		PnL:           6925.00,
	}

	converted := kite.ConvertHolding(accID, h)

	if converted.Instrument != "INFY" {
		t.Errorf("Instrument = %q, want INFY", converted.Instrument)
	}
	if converted.SellableQuantity != 80 {
		t.Errorf("SellableQuantity = %d, want 80", converted.SellableQuantity)
	}
	if !converted.BuyAveragePrice.Equal(decimal.NewFromFloat(1450.75)) {
		t.Errorf("BuyAveragePrice = %s, want 1450.75", converted.BuyAveragePrice)
	}
	if !converted.Ltp.Equal(decimal.NewFromFloat(1520.00)) {
		t.Errorf("Ltp = %s, want 1520.00", converted.Ltp)
	}
	if !converted.Pnl.Equal(decimal.NewFromFloat(6925.00)) {
		t.Errorf("Pnl = %s, want 6925.00", converted.Pnl)
	}
}
