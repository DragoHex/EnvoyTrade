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

func TestCalculatePositionMetrics_BrokerMarginPrecedenceAndBreakdown(t *testing.T) {
	positions := []kiteconnect.Position{
		{
			Exchange:      "NFO",
			Tradingsymbol: "NIFTY26OCTFUT",
			Product:       "NRML",
			Quantity:      75,
			M2M:           1500.0,
			Realised:      0.0,
		},
		{
			Exchange:      "NSE",
			Tradingsymbol: "RELIANCE",
			Product:       "MIS",
			Quantity:      -10,
			M2M:           350.0,
			Realised:      0.0,
		},
		{
			Exchange:      "NSE",
			Tradingsymbol: "TCS",
			Product:       "CNC",
			Quantity:      5,
			M2M:           -50.0,
			Realised:      0.0,
		},
	}

	// Broker margins returns account-level MTM in utilised fields
	margins := kiteconnect.AllMargins{
		Equity: kiteconnect.Margins{
			Used: kiteconnect.UsedMargins{
				M2MRealised:   100.0,
				M2MUnrealised: 1700.0, // Total equity M2M = 1800.0
			},
		},
		Commodity: kiteconnect.Margins{
			Used: kiteconnect.UsedMargins{
				M2MRealised:   0.0,
				M2MUnrealised: 0.0,
			},
		},
	}

	summary := kite.CalculatePositionMetrics(positions, margins)

	// Total MTM must be sourced directly from F&O positions: 1500 + 350 = 1850 (CNC is excluded)
	expectedTotalMtm := decimal.NewFromFloat(1850.0)
	if !summary.TotalMtm.Equal(expectedTotalMtm) {
		t.Errorf("TotalMtm = %s, want %s (F&O positions source)", summary.TotalMtm, expectedTotalMtm)
	}

	// Product breakdown must be grouped and summed for F&O only
	expectedNrml := decimal.NewFromFloat(1500.0)
	if !summary.ProductMtm["NRML"].Equal(expectedNrml) {
		t.Errorf("ProductMtm[NRML] = %s, want %s", summary.ProductMtm["NRML"], expectedNrml)
	}

	expectedMis := decimal.NewFromFloat(350.0)
	if !summary.ProductMtm["MIS"].Equal(expectedMis) {
		t.Errorf("ProductMtm[MIS] = %s, want %s", summary.ProductMtm["MIS"], expectedMis)
	}

	// CNC must be excluded from product MTM
	if _, ok := summary.ProductMtm["CNC"]; ok {
		t.Errorf("ProductMtm should not contain CNC, got %s", summary.ProductMtm["CNC"])
	}

	// When no F&O positions exist (e.g. only CNC), fallback to broker margins
	onlyCncPositions := []kiteconnect.Position{
		{
			Exchange:      "NSE",
			Tradingsymbol: "TCS",
			Product:       "CNC",
			Quantity:      5,
			M2M:           -50.0,
			Realised:      0.0,
		},
	}
	summaryFallback := kite.CalculatePositionMetrics(onlyCncPositions, margins)
	expectedBrokerMtm := decimal.NewFromFloat(1800.0) // 100 + 1700
	if !summaryFallback.TotalMtm.Equal(expectedBrokerMtm) {
		t.Errorf("Fallback TotalMtm = %s, want %s (broker margin source)", summaryFallback.TotalMtm, expectedBrokerMtm)
	}
	expectedBrokerRealized := decimal.NewFromFloat(100.0)
	if !summaryFallback.RealizedPnl.Equal(expectedBrokerRealized) {
		t.Errorf("Fallback RealizedPnl = %s, want %s (broker margin source)", summaryFallback.RealizedPnl, expectedBrokerRealized)
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

func TestCalculatePositionMetrics_DynamicMTMScenarios(t *testing.T) {
	margins := kiteconnect.AllMargins{}

	t.Run("Scenario A: Multi-position mixed gains and losses", func(t *testing.T) {
		positions := []kiteconnect.Position{
			{Tradingsymbol: "NIFTY26OCTFUT", Quantity: 150, M2M: 22500.0, Realised: 0.0},
			{Tradingsymbol: "RELIANCE", Quantity: -10, M2M: 300.0, Realised: 0.0},
			{Tradingsymbol: "INFY", Quantity: 100, M2M: -1500.0, Realised: 0.0},
		}
		summary := kite.CalculatePositionMetrics(positions, margins)
		expectedMtm := decimal.NewFromFloat(21300.0) // 22500 + 300 - 1500
		if !summary.TotalMtm.Equal(expectedMtm) {
			t.Errorf("TotalMtm = %s, want %s", summary.TotalMtm, expectedMtm)
		}
		if summary.NetQty != 240 { // 150 - 10 + 100
			t.Errorf("NetQty = %d, want 240", summary.NetQty)
		}
		if summary.OpenPositions != 3 || summary.ClosedPositions != 0 {
			t.Errorf("Open=%d Closed=%d, want 3 and 0", summary.OpenPositions, summary.ClosedPositions)
		}
	})

	t.Run("Scenario B: Mix of open and closed positions with realized PnL", func(t *testing.T) {
		positions := []kiteconnect.Position{
			{Tradingsymbol: "NIFTY26OCTFUT", Quantity: 75, M2M: 37500.0, Realised: 15000.0},
			{Tradingsymbol: "GOLD26OCTFUT", Quantity: 0, M2M: -5000.0, Realised: -5000.0},
		}
		summary := kite.CalculatePositionMetrics(positions, margins)
		expectedMtm := decimal.NewFromFloat(32500.0) // 37500 + (-5000)
		expectedRealized := decimal.NewFromFloat(10000.0) // 15000 + (-5000)
		if !summary.TotalMtm.Equal(expectedMtm) {
			t.Errorf("TotalMtm = %s, want %s", summary.TotalMtm, expectedMtm)
		}
		if !summary.RealizedPnl.Equal(expectedRealized) {
			t.Errorf("RealizedPnl = %s, want %s", summary.RealizedPnl, expectedRealized)
		}
		if summary.OpenPositions != 1 || summary.ClosedPositions != 1 {
			t.Errorf("Open=%d Closed=%d, want 1 and 1", summary.OpenPositions, summary.ClosedPositions)
		}
	})

	t.Run("Scenario C: Adverse movements across portfolio", func(t *testing.T) {
		positions := []kiteconnect.Position{
			{Tradingsymbol: "NIFTY26OCTFUT", Quantity: 150, M2M: -15000.0, Realised: 0.0},
			{Tradingsymbol: "RELIANCE", Quantity: -10, M2M: -500.0, Realised: 0.0},
		}
		summary := kite.CalculatePositionMetrics(positions, margins)
		expectedMtm := decimal.NewFromFloat(-15500.0)
		if !summary.TotalMtm.Equal(expectedMtm) {
			t.Errorf("TotalMtm = %s, want %s", summary.TotalMtm, expectedMtm)
		}
	})

	t.Run("Scenario D: Empty portfolio", func(t *testing.T) {
		summary := kite.CalculatePositionMetrics(nil, margins)
		if !summary.TotalMtm.IsZero() {
			t.Errorf("TotalMtm = %s, want 0", summary.TotalMtm)
		}
		if summary.NetQty != 0 || summary.OpenPositions != 0 || summary.ClosedPositions != 0 {
			t.Errorf("unexpected counts: %+v", summary)
		}
	})

	t.Run("Scenario E: Overnight carry-forward positions where M2M is zero but PnL is non-zero", func(t *testing.T) {
		positions := []kiteconnect.Position{
			{
				Exchange:      "MCX",
				Tradingsymbol: "CRUDEOIL26OCT10200CE",
				Product:       "NRML",
				Quantity:      -2,
				AveragePrice:  72.65,
				LastPrice:     20.40,
				M2M:           0.0,      // Zerodha returns 0.0 overnight / pre-market
				PnL:           10450.00, // True floating PnL from entry
			},
			{
				Exchange:      "MCX",
				Tradingsymbol: "CRUDEOIL26OCT7500PE",
				Product:       "NRML",
				Quantity:      -2,
				AveragePrice:  69.35,
				LastPrice:     23.00,
				M2M:           0.0,     // Zerodha returns 0.0 overnight / pre-market
				PnL:           9270.00, // True floating PnL from entry
			},
		}
		summary := kite.CalculatePositionMetrics(positions, margins)
		expectedMtm := decimal.NewFromFloat(19720.00) // 10450 + 9270
		if !summary.TotalMtm.Equal(expectedMtm) {
			t.Errorf("TotalMtm = %s, want %s (must use PnL for open positions when M2M is 0)", summary.TotalMtm, expectedMtm)
		}
		if !summary.ProductMtm["NRML"].Equal(expectedMtm) {
			t.Errorf("ProductMtm[NRML] = %s, want %s", summary.ProductMtm["NRML"], expectedMtm)
		}
		if summary.OpenPositions != 2 {
			t.Errorf("OpenPositions = %d, want 2", summary.OpenPositions)
		}
	})
}

func TestConvertPosition_OvernightPosition(t *testing.T) {
	accID := uuid.New()
	p := kiteconnect.Position{
		Product:       "NRML",
		Tradingsymbol: "CRUDEOIL26OCT10200CE",
		Quantity:      -2,
		AveragePrice:  72.65,
		LastPrice:     20.40,
		M2M:           0.0,
		PnL:           10450.00,
	}

	converted := kite.ConvertPosition(accID, p)

	expectedPnl := decimal.NewFromFloat(10450.00)
	if !converted.Mtm.Equal(expectedPnl) {
		t.Errorf("converted.Mtm = %s, want %s (must reflect PnL when M2M is 0)", converted.Mtm, expectedPnl)
	}
	if !converted.Pnl.Equal(expectedPnl) {
		t.Errorf("converted.Pnl = %s, want %s", converted.Pnl, expectedPnl)
	}
}

