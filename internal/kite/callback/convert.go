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
		AveragePrice:    decimal.NewFromFloat(o.AveragePrice),
		Status:          o.Status,
		OrderTimestamp:  o.OrderTimestamp.Time,
		RawPayload:      raw,
	}
}

// ToOrderUpdate converts a Kite order update into a status change for an
// order already placed — used to update a follower_orders row located by
// BrokerOrderID.
func ToOrderUpdate(o kiteconnect.Order) domain.OrderUpdate {
	raw, _ := json.Marshal(o)
	return domain.OrderUpdate{
		BrokerOrderID:  o.OrderID,
		Status:         o.Status,
		FilledQuantity: int(o.FilledQuantity),
		AveragePrice:   decimal.NewFromFloat(o.AveragePrice),
		OrderTimestamp: o.OrderTimestamp.Time,
		RawPayload:     raw,
	}
}
