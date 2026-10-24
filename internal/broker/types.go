// Package broker defines the broker abstraction — broker-agnostic types
// and interfaces that all broker implementations (Kite, Stripe, etc.)
// satisfy. Concrete broker implementations live in their own packages
// (internal/kite, internal/stripe, etc.) and adapt their native types
// to these types.
package broker

import (
	"encoding/json"
	"time"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// OrderParams are the broker-agnostic inputs to PlaceOrder. Each broker
// adapter translates these to its native type.
type OrderParams struct {
	Exchange        string
	Tradingsymbol   string
	TransactionType string // BUY|SELL
	Product         string // CNC|MIS|NRML (Kite), or broker-native equivalent
	OrderType       string // MARKET|LIMIT|etc (broker-native)
	Quantity        int
	Price           float64
	TriggerPrice    float64
	Tag             string // idempotency tag, sent as broker's order tag field
}

// OrderResponse is the broker-native response to PlaceOrder. Currently
// only carries the order ID returned by the broker, but can be extended
// as needed (e.g., for other broker-specific metadata).
type OrderResponse struct {
	OrderID string
}

// Position represents an open or closed position with the broker.
type Position struct {
	Exchange      string  `json:"exchange"`
	Tradingsymbol string  `json:"tradingsymbol"`
	Product       string  `json:"product"`
	Quantity      int     `json:"quantity"` // net quantity (>0 long, <0 short, 0 closed)
	AveragePrice  float64 `json:"average_price"`
	LastPrice     float64 `json:"last_price"`
	M2M           float64 `json:"m2m"`
	PnL           float64 `json:"pnl"`
}

// Order represents an existing order on the broker (e.g., for status check, history, or reconciliation).
type Order struct {
	OrderID         string          `json:"order_id"`
	Exchange        string          `json:"exchange"`
	Tradingsymbol   string          `json:"tradingsymbol"`
	TransactionType string          `json:"transaction_type"` // BUY | SELL
	Product         string          `json:"product"`          // CNC | MIS | NRML
	OrderType       string          `json:"order_type"`        // MARKET | LIMIT | etc.
	Status          string          `json:"status"`           // COMPLETE | REJECTED | CANCELLED | OPEN
	StatusMessage   string          `json:"status_message,omitempty"`
	Quantity        int             `json:"quantity"`
	FilledQuantity  int             `json:"filled_quantity"`
	Price           decimal.Decimal `json:"price"`
	TriggerPrice    decimal.Decimal `json:"trigger_price"`
	AveragePrice    decimal.Decimal `json:"average_price"`
	OrderTimestamp  time.Time       `json:"order_timestamp"`
	Tag             string          `json:"tag,omitempty"`
	RawPayload      json.RawMessage `json:"raw_payload,omitempty"`
}

// ToMasterFill converts a broker Order into domain.MasterFill.
func (o Order) ToMasterFill(masterID uuid.UUID) domain.MasterFill {
	price := o.Price
	if o.OrderType == "LIMIT" && price.IsZero() && o.AveragePrice.GreaterThan(decimal.Zero) {
		price = o.AveragePrice
	}
	raw := o.RawPayload
	if len(raw) == 0 {
		raw, _ = json.Marshal(o)
	}
	return domain.MasterFill{
		MasterID:        masterID,
		BrokerOrderID:   o.OrderID,
		Exchange:        o.Exchange,
		Tradingsymbol:   o.Tradingsymbol,
		TransactionType: o.TransactionType,
		Product:         o.Product,
		OrderType:       o.OrderType,
		FilledQuantity:  o.FilledQuantity,
		Price:           price,
		TriggerPrice:    o.TriggerPrice,
		AveragePrice:    o.AveragePrice,
		Status:          o.Status,
		OrderTimestamp:  o.OrderTimestamp,
		RawPayload:      raw,
		Tag:             o.Tag,
	}
}

// ToOrderUpdate converts a broker Order into domain.OrderUpdate for followers.
func (o Order) ToOrderUpdate(followerID uuid.UUID) domain.OrderUpdate {
	raw := o.RawPayload
	if len(raw) == 0 {
		raw, _ = json.Marshal(o)
	}
	return domain.OrderUpdate{
		FollowerID:      followerID,
		BrokerOrderID:   o.OrderID,
		Tradingsymbol:   o.Tradingsymbol,
		Exchange:        o.Exchange,
		Product:         o.Product,
		TransactionType: o.TransactionType,
		OrderType:       o.OrderType,
		Quantity:        o.Quantity,
		Status:          o.Status,
		FilledQuantity:  o.FilledQuantity,
		AveragePrice:    o.AveragePrice,
		OrderTimestamp:  o.OrderTimestamp,
		Tag:             o.Tag,
		RawPayload:      raw,
	}
}
