package broker

import "context"

// Broker is the abstraction every broker implementation must satisfy.
// Implementations: internal/kite.Broker (Zerodha), and future adapters
// for other brokers. The interface is broker-agnostic; each implementation
// translates between broker-native types and the types defined in this
// package (PLAN.md §1).
type Broker interface {
	PlaceOrder(ctx context.Context, variety string, params OrderParams) (OrderResponse, error)
}
