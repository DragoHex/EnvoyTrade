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
	Latency time.Duration
	Err     error
	OrderID string

	mu    sync.Mutex
	Calls []broker.OrderParams
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
