package kite

import (
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

// AccountPortfolioSummary holds aggregated metrics for an account's portfolio.
type AccountPortfolioSummary struct {
	NetQty          int
	OpenPositions   int
	ClosedPositions int
	TotalMtm        decimal.Decimal
	RealizedPnl     decimal.Decimal
	AvailableCash   decimal.Decimal
	AvailableMargin decimal.Decimal
}

// ConvertedHolding holds database-ready holding fields.
type ConvertedHolding struct {
	AccountID        uuid.UUID
	Instrument       string
	SellableQuantity int
	BuyAveragePrice  decimal.Decimal
	Ltp              decimal.Decimal
	Pnl              decimal.Decimal
	Action           string
}

// ConvertedPosition holds database-ready position fields.
type ConvertedPosition struct {
	AccountID    uuid.UUID
	Product      string
	Instrument   string
	Quantity     int
	BuyPrice     decimal.Decimal
	SellPrice    decimal.Decimal
	BuyQuantity  int
	SellQuantity int
	Ltp          decimal.Decimal
	Mtm          decimal.Decimal
	Pnl          decimal.Decimal
	Action       string
}

// IsFOOrCommodity checks if the given exchange belongs to F&O or Commodity markets.
func IsFOOrCommodity(exchange string) bool {
	switch strings.ToUpper(exchange) {
	case "NFO", "BFO", "CDS", "BCD", "MCX":
		return true
	default:
		return false
	}
}

// FilterFOPositions returns only positions that belong to F&O or Commodity segments.
func FilterFOPositions(positions []kiteconnect.Position) []kiteconnect.Position {
	out := make([]kiteconnect.Position, 0, len(positions))
	for _, p := range positions {
		if IsFOOrCommodity(p.Exchange) {
			out = append(out, p)
		}
	}
	return out
}

// CalculatePositionMetrics aggregates portfolio metrics from positions and margins.
func CalculatePositionMetrics(positions []kiteconnect.Position, margins kiteconnect.AllMargins) AccountPortfolioSummary {
	var netQty, openCount, closedCount int
	totalMtm := decimal.Zero
	realizedPnl := decimal.Zero

	for _, p := range positions {
		netQty += p.Quantity
		if p.Quantity != 0 {
			openCount++
		} else {
			closedCount++
		}
		totalMtm = totalMtm.Add(decimal.NewFromFloat(p.M2M))
		realizedPnl = realizedPnl.Add(decimal.NewFromFloat(p.Realised))
	}

	availableCash := decimal.NewFromFloat(margins.Equity.Available.Cash).Add(decimal.NewFromFloat(margins.Commodity.Available.Cash))
	availableMargin := decimal.NewFromFloat(margins.Equity.Available.LiveBalance).Add(decimal.NewFromFloat(margins.Commodity.Available.LiveBalance))

	return AccountPortfolioSummary{
		NetQty:          netQty,
		OpenPositions:   openCount,
		ClosedPositions: closedCount,
		TotalMtm:        totalMtm,
		RealizedPnl:     realizedPnl,
		AvailableCash:   availableCash,
		AvailableMargin: availableMargin,
	}
}

// ConvertHolding maps kiteconnect.Holding to ConvertedHolding.
func ConvertHolding(accountID uuid.UUID, h kiteconnect.Holding) ConvertedHolding {
	sellable := h.Quantity - h.UsedQuantity
	if sellable < 0 {
		sellable = 0
	}
	return ConvertedHolding{
		AccountID:        accountID,
		Instrument:       h.Tradingsymbol,
		SellableQuantity: sellable,
		BuyAveragePrice:  decimal.NewFromFloat(h.AveragePrice),
		Ltp:              decimal.NewFromFloat(h.LastPrice),
		Pnl:              decimal.NewFromFloat(h.PnL),
		Action:           "exit",
	}
}

// ConvertPosition maps kiteconnect.Position to ConvertedPosition.
func ConvertPosition(accountID uuid.UUID, p kiteconnect.Position) ConvertedPosition {
	buyPrice := p.BuyPrice
	if buyPrice == 0 && p.Quantity > 0 {
		buyPrice = p.AveragePrice
	}
	sellPrice := p.SellPrice
	if sellPrice == 0 && p.Quantity < 0 {
		sellPrice = p.AveragePrice
	}
	return ConvertedPosition{
		AccountID:    accountID,
		Product:      p.Product,
		Instrument:   p.Tradingsymbol,
		Quantity:     p.Quantity,
		BuyPrice:     decimal.NewFromFloat(buyPrice),
		SellPrice:    decimal.NewFromFloat(sellPrice),
		BuyQuantity:  p.BuyQuantity,
		SellQuantity: p.SellQuantity,
		Ltp:          decimal.NewFromFloat(p.LastPrice),
		Mtm:          decimal.NewFromFloat(p.M2M),
		Pnl:          decimal.NewFromFloat(p.PnL),
		Action:       "exit",
	}
}
