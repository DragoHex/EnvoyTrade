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
	Tag             string // idempotency tag, sent as broker's order tag field
}

// OrderResponse is the broker-native response to PlaceOrder. Currently
// only carries the order ID returned by the broker, but can be extended
// as needed (e.g., for other broker-specific metadata).
type OrderResponse struct {
	OrderID string
}
