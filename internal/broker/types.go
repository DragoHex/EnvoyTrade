// Package broker defines the broker abstraction — broker-agnostic types
// and interfaces that all broker implementations (Kite, Stripe, etc.)
// satisfy. Concrete broker implementations live in their own packages
// (internal/kite, internal/stripe, etc.) and adapt their native types
// to these types.
package broker

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

// Order represents an existing order on the broker (e.g., for status check or cancellation).
type Order struct {
	OrderID       string  `json:"order_id"`
	Exchange      string  `json:"exchange"`
	Tradingsymbol string  `json:"tradingsymbol"`
	Status        string  `json:"status"` // OPEN, COMPLETE, CANCELLED, REJECTED
	Quantity      int     `json:"quantity"`
	FilledQuantity int    `json:"filled_quantity"`
}
