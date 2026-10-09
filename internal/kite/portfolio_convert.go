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
	ProductMtm      map[string]decimal.Decimal
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

// FilterFOPositions returns only positions that belong to F&O or Commodity segments, strictly excluding equity holdings (CNC).
func FilterFOPositions(positions []kiteconnect.Position) []kiteconnect.Position {
	out := make([]kiteconnect.Position, 0, len(positions))
	for _, p := range positions {
		prod := strings.ToUpper(strings.TrimSpace(p.Product))
		if prod == "CNC" {
			continue
		}
		if IsFOOrCommodity(p.Exchange) || prod == "NRML" || prod == "MIS" {
			out = append(out, p)
		}
	}
	return out
}

// CalculatePositionMetrics aggregates portfolio metrics from positions and margins, strictly for F&O / Commodity segments (ignoring equity holdings / CNC).
func CalculatePositionMetrics(positions []kiteconnect.Position, margins kiteconnect.AllMargins) AccountPortfolioSummary {
	var netQty, openCount, closedCount int
	positionsRealisedSum := decimal.Zero
	productMtm := make(map[string]decimal.Decimal)
	positionsMtmSum := decimal.Zero

	foCount := 0
	for _, p := range positions {
		prod := strings.ToUpper(strings.TrimSpace(p.Product))
		// Strictly exclude equity holdings (CNC)
		if prod == "CNC" {
			continue
		}
		if prod == "" {
			prod = "NRML"
		}
		foCount++
		netQty += p.Quantity
		if p.Quantity != 0 {
			openCount++
		} else {
			closedCount++
		}
		positionsRealisedSum = positionsRealisedSum.Add(decimal.NewFromFloat(p.Realised))

		posMtmVal := p.PnL
		if posMtmVal == 0 && p.M2M != 0 {
			posMtmVal = p.M2M
		}
		posMtm := decimal.NewFromFloat(posMtmVal)
		productMtm[prod] = productMtm[prod].Add(posMtm)
		positionsMtmSum = positionsMtmSum.Add(posMtm)
	}

	// Broker margins M2M (Equity + Commodity)
	eqMtm := decimal.NewFromFloat(margins.Equity.Used.M2MRealised).Add(decimal.NewFromFloat(margins.Equity.Used.M2MUnrealised))
	commMtm := decimal.NewFromFloat(margins.Commodity.Used.M2MRealised).Add(decimal.NewFromFloat(margins.Commodity.Used.M2MUnrealised))
	brokerMarginMtm := eqMtm.Add(commMtm)

	// Sourced directly from broker's GetUserMargins via gokiteconnect (*kiteconnect.Client)
	brokerM2MRealised := decimal.NewFromFloat(margins.Equity.Used.M2MRealised).Add(decimal.NewFromFloat(margins.Commodity.Used.M2MRealised))

	// For F&O segments: if individual F&O positions exist, their aggregation is the exact truth for F&O (since broker equity margin can include cash equity / holdings).
	// If no individual F&O positions are present, fallback to broker margins.
	totalMtm := positionsMtmSum
	realizedPnl := positionsRealisedSum

	if foCount == 0 {
		totalMtm = brokerMarginMtm
		realizedPnl = brokerM2MRealised
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
		ProductMtm:      productMtm,
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
	posMtm := p.PnL
	if posMtm == 0 && p.M2M != 0 {
		posMtm = p.M2M
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
		Mtm:          decimal.NewFromFloat(posMtm),
		Pnl:          decimal.NewFromFloat(p.PnL),
		Action:       "exit",
	}
}
