// Package worker isolates follower order placement: one goroutine and
// one buffered channel per follower, each with its own broker (own
// egress). A slow, hanging, or panicking follower can never affect
// another's dispatch — that isolation is the point (PLAN.md §4.3).
//
// Retry, backoff, rate limiting, and circuit breaking (PLAN.md §4.4) are
// a later milestone: this package places each order at most once and
// records the outcome, success or terminal failure.
package worker

import (
	"context"
	"fmt"
	"sync"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

// Store is everything a worker needs to record a placement outcome.
// Defined here (the consumer), not in the store package, per PLAN.md
// §1's seam rule.
type Store interface {
	UpdateFollowerOrderPlaced(ctx context.Context, id int64, brokerOrderID string, placedQty int) error
	UpdateFollowerOrderFailed(ctx context.Context, id int64, terminalStatus string, errMsg string) error
	AppendOrderEvent(ctx context.Context, ev domain.OrderEvent) error
}

// Pool owns one goroutine and one bounded channel per registered
// follower. It satisfies engine.Dispatcher structurally — engine never
// imports this package, only the interface it declared.
type Pool struct {
	mu       sync.Mutex
	workers  map[uuid.UUID]chan domain.Job
	wg       sync.WaitGroup
	done     chan struct{}
	shutdown sync.Once
}

// NewPool creates an empty pool. Followers are added with Register.
func NewPool() *Pool {
	return &Pool{
		workers: make(map[uuid.UUID]chan domain.Job),
		done:    make(chan struct{}),
	}
}

// Register starts a follower's dedicated goroutine, listening on a
// channel of the given buffer size until ctx is cancelled or Shutdown is
// called — Shutdown is authoritative regardless of which context (if
// any) a caller passes in, so it always terminates every worker.
func (p *Pool) Register(ctx context.Context, followerID uuid.UUID, b broker.Broker, store Store, bufferSize int) {
	ch := make(chan domain.Job, bufferSize)

	p.mu.Lock()
	p.workers[followerID] = ch
	p.mu.Unlock()

	p.wg.Add(1)
	go p.run(ctx, ch, b, store)
}

// Dispatch attempts a non-blocking hand-off to the given follower's
// channel. It returns false if the follower isn't registered or its
// channel has no room — the caller (engine) treats that as a dead
// letter rather than blocking fan-out to other followers.
func (p *Pool) Dispatch(followerID uuid.UUID, job domain.Job) bool {
	p.mu.Lock()
	ch, ok := p.workers[followerID]
	p.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- job:
		return true
	default:
		return false
	}
}

// Shutdown signals every worker goroutine to stop and waits for them to
// exit. Safe to call more than once.
func (p *Pool) Shutdown() {
	p.shutdown.Do(func() { close(p.done) })
	p.wg.Wait()
}

func (p *Pool) run(ctx context.Context, in <-chan domain.Job, b broker.Broker, store Store) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.done:
			return
		case job := <-in:
			p.place(ctx, job, b, store)
		}
	}
}

func (p *Pool) place(ctx context.Context, job domain.Job, b broker.Broker, store Store) {
	defer func() {
		if r := recover(); r != nil {
			p.recordFailure(ctx, job, store, fmt.Errorf("panic placing order: %v", r))
		}
	}()

	params := broker.OrderParams{
		Exchange:        job.Exchange,
		Tradingsymbol:   job.Tradingsymbol,
		TransactionType: job.TransactionType,
		Product:         job.Product,
		OrderType:       job.OrderType,
		Quantity:        job.Quantity,
		Tag:             job.IdempotencyTag,
	}

	resp, err := b.PlaceOrder(ctx, "regular", params)
	if err != nil {
		p.recordFailure(ctx, job, store, err)
		return
	}

	_ = store.UpdateFollowerOrderPlaced(ctx, job.FollowerOrderID, resp.OrderID, job.Quantity)
	orderID := job.FollowerOrderID
	_ = store.AppendOrderEvent(ctx, domain.OrderEvent{
		FollowerOrderID: &orderID,
		MasterFillID:    &job.MasterFillID,
		AccountID:       job.FollowerID,
		EventType:       "api_response",
		Payload:         []byte(fmt.Sprintf(`{"broker_order_id":%q,"quantity":%d}`, resp.OrderID, job.Quantity)),
	})
}

func (p *Pool) recordFailure(ctx context.Context, job domain.Job, store Store, cause error) {
	_ = store.UpdateFollowerOrderFailed(ctx, job.FollowerOrderID, "", cause.Error())
	orderID := job.FollowerOrderID
	_ = store.AppendOrderEvent(ctx, domain.OrderEvent{
		FollowerOrderID: &orderID,
		MasterFillID:    &job.MasterFillID,
		AccountID:       job.FollowerID,
		EventType:       "api_response",
		Payload:         []byte(fmt.Sprintf(`{"error":%q}`, cause.Error())),
	})
}
