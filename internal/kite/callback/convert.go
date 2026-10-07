package callback

import (
	"encoding/json"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

// IsTerminal reports whether a Kite order status is one fan-out and
// status-tracking care about. Intermediate states (OPEN, TRIGGER PENDING,
// PUT ORDER REQ RECEIVED, ...) are dropped by both delivery paths — only a
// terminal order is a signal or a final status worth recording.
func IsTerminal(status string) bool {
	switch status {
	case domain.TerminalComplete, domain.TerminalRejected, domain.TerminalCancelled:
		return true
	default:
		return false
	}
}

// ToMasterFill converts a Kite order update into the signal the engine
// fans out. masterID is supplied by the caller (resolved from the
// account the order belongs to), never trusted from the payload.
func ToMasterFill(o kiteconnect.Order, masterID uuid.UUID) domain.MasterFill {
	raw, _ := json.Marshal(o)
	price := o.Price
	if o.OrderType == "LIMIT" && price <= 0 && o.AveragePrice > 0 {
		price = o.AveragePrice
	}
	return domain.MasterFill{
		MasterID:        masterID,
		BrokerOrderID:   o.OrderID,
		Exchange:        o.Exchange,
		Tradingsymbol:   o.TradingSymbol,
		InstrumentToken: int64(o.InstrumentToken),
		TransactionType: o.TransactionType,
		Product:         o.Product,
		OrderType:       o.OrderType,
		FilledQuantity:  int(o.FilledQuantity),
		Price:           decimal.NewFromFloat(price),
		TriggerPrice:    decimal.NewFromFloat(o.TriggerPrice),
		AveragePrice:    decimal.NewFromFloat(o.AveragePrice),
		Status:          o.Status,
		OrderTimestamp:  o.OrderTimestamp.Time,
		RawPayload:      raw,
		Tag:             o.Tag,
	}
}

// ToOrderUpdate converts a Kite order update into an OrderUpdate event for a follower.
// It maps the full order parameters, facilitating both in-flight status updates
// and direct/manual order recording.
func ToOrderUpdate(o kiteconnect.Order, followerID uuid.UUID) domain.OrderUpdate {
	raw, _ := json.Marshal(o)
	return domain.OrderUpdate{
		FollowerID:      followerID,
		BrokerOrderID:   o.OrderID,
		Tradingsymbol:   o.TradingSymbol,
		Exchange:        o.Exchange,
		Product:         o.Product,
		TransactionType: o.TransactionType,
		OrderType:       o.OrderType,
		Quantity:        int(o.Quantity),
		Status:          o.Status,
		FilledQuantity:  int(o.FilledQuantity),
		AveragePrice:    decimal.NewFromFloat(o.AveragePrice),
		OrderTimestamp:  o.OrderTimestamp.Time,
		Tag:             o.Tag,
		RawPayload:      raw,
	}
}
