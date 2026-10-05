// Package fake is a programmable in-memory implementation of broker.Broker
// for tests — latency injection, error injection, and call recording.
package fake

import (
	"context"
	"sync"
	"time"

	"envoytrade/internal/broker"
)

// Broker is a single-account fake broker. Zero value behaves as an
// always-succeeding broker with an empty OrderID; set Err to make every
// call fail, or Latency to simulate a slow broker.
type Broker struct {
	Latency    time.Duration
	Err        error
	OrderID    string
	Positions  []broker.Position
	OpenOrders []broker.Order

	mu          sync.Mutex
	Calls       []broker.OrderParams
	CancelCalls []string
}

// PlaceOrder waits for Latency (or ctx cancellation, whichever comes
// first), records the call, then returns Err if set or an OrderResponse
// carrying OrderID.
func (b *Broker) PlaceOrder(ctx context.Context, variety string, params broker.OrderParams) (broker.OrderResponse, error) {
	if b.Latency > 0 {
		select {
		case <-time.After(b.Latency):
		case <-ctx.Done():
			return broker.OrderResponse{}, ctx.Err()
		}
	}

	b.mu.Lock()
	b.Calls = append(b.Calls, params)
	b.mu.Unlock()

	if b.Err != nil {
		return broker.OrderResponse{}, b.Err
	}
	return broker.OrderResponse{OrderID: b.OrderID}, nil
}

// GetPositions returns configured fake positions.
func (b *Broker) GetPositions(ctx context.Context) ([]broker.Position, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Err != nil {
		return nil, b.Err
	}
	out := make([]broker.Position, len(b.Positions))
	copy(out, b.Positions)
	return out, nil
}

// GetOpenOrders returns configured open orders.
func (b *Broker) GetOpenOrders(ctx context.Context) ([]broker.Order, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Err != nil {
		return nil, b.Err
	}
	out := make([]broker.Order, len(b.OpenOrders))
	copy(out, b.OpenOrders)
	return out, nil
}

// CancelOrder records the cancellation call and returns success.
func (b *Broker) CancelOrder(ctx context.Context, variety, orderID string) (broker.OrderResponse, error) {
	b.mu.Lock()
	b.CancelCalls = append(b.CancelCalls, orderID)
	b.mu.Unlock()
	if b.Err != nil {
		return broker.OrderResponse{}, b.Err
	}
	return broker.OrderResponse{OrderID: orderID}, nil
}
