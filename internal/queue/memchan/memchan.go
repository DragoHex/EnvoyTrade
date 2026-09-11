// Package memchan is the V1 dispatch transport: an in-memory buffered
// channel implementing queue.Publisher and queue.Consumer. It is not
// durable — the Postgres outbox (master_fills.dispatch_state) is the
// actual at-least-once guarantee (PLAN.md §4.1); this package only
// optimises the common-case latency between listener and engine.
package memchan

import (
	"context"
	"errors"

	"envoytrade/internal/domain"
	"envoytrade/internal/queue"
)

// ErrFull is returned by Publish when the buffer has no room. The caller
// is expected to drop the event and rely on the outbox drainer to
// redeliver it — Publish never blocks waiting for space.
var ErrFull = errors.New("memchan: queue full")

// Queue is a fixed-capacity, in-memory implementation of
// queue.Publisher and queue.Consumer.
type Queue struct {
	ch chan domain.MasterFill
}

var (
	_ queue.Publisher = (*Queue)(nil)
	_ queue.Consumer  = (*Queue)(nil)
)

// New creates a Queue with the given buffer capacity.
func New(capacity int) *Queue {
	return &Queue{ch: make(chan domain.MasterFill, capacity)}
}

// Publish attempts a non-blocking send. If the buffer is full it returns
// ErrFull immediately rather than waiting for space.
func (q *Queue) Publish(ctx context.Context, ev domain.MasterFill) error {
	select {
	case q.ch <- ev:
		return nil
	default:
		return ErrFull
	}
}

// Consume pulls events off the buffer in order and invokes fn for each,
// blocking until ctx is cancelled. memchan has no durability of its own,
// so an error from fn simply drops that in-memory delivery — the caller
// relies on the outbox drainer for redelivery, not on Consume retrying.
func (q *Queue) Consume(ctx context.Context, fn func(context.Context, domain.MasterFill) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-q.ch:
			_ = fn(ctx, ev)
		}
	}
}
