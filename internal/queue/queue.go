// Package queue defines the dispatch-transport seam (PLAN.md §1). This is
// one of only two interfaces meant to survive a V2 rewrite: swapping
// memchan for Redis Streams/Kafka should change latency and process
// topology, never correctness or the engine's call sites.
package queue

import (
	"context"

	"envoytrade/internal/domain"
)

// Publisher hands a master fill to the dispatch transport. Publish must
// not block indefinitely — PLAN.md's outbox pattern treats the channel as
// a latency optimisation only, so a full transport should fail fast and
// let the durable outbox drainer redeliver later, not stall the caller.
type Publisher interface {
	Publish(ctx context.Context, ev domain.MasterFill) error
}

// Consumer delivers published events to fn, one at a time, in publish
// order. Consume blocks until ctx is cancelled or the underlying
// transport is closed.
type Consumer interface {
	Consume(ctx context.Context, fn func(context.Context, domain.MasterFill) error) error
}
