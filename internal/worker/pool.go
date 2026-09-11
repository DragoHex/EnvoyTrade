// Package worker isolates follower order placement: one goroutine and
// one buffered channel per follower, each with its own broker (own
// egress). A slow, hanging, or panicking follower can never affect
// another's dispatch — that isolation is the point (PLAN.md §4.3).
//
// Rate limiting is enforced per-account to respect Kite's ~10 orders/sec
// limit. Timeouts are retried with backoff up to a cap; margin/RMS
// rejections are terminal on first attempt (Design Doc §6).
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

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
	mu                sync.Mutex
	workers           map[uuid.UUID]chan domain.Job
	wg                sync.WaitGroup
	done              chan struct{}
	shutdown          sync.Once
	RateLimitInterval time.Duration
	MaxRetries        int
	InitialBackoff    time.Duration
	Logger            *slog.Logger
}

// NewPool creates an empty pool. Followers are added with Register.
func NewPool() *Pool {
	return &Pool{
		workers:           make(map[uuid.UUID]chan domain.Job),
		done:              make(chan struct{}),
		RateLimitInterval: 100 * time.Millisecond,
		MaxRetries:        3,
		InitialBackoff:    50 * time.Millisecond,
	}
}

func (p *Pool) log() *slog.Logger {
	if p.Logger != nil {
		return p.Logger
	}
	return slog.Default()
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
	var lastCall time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.done:
			return
		case job := <-in:
			if p.RateLimitInterval > 0 && !lastCall.IsZero() {
				elapsed := time.Since(lastCall)
				if elapsed < p.RateLimitInterval {
					select {
					case <-time.After(p.RateLimitInterval - elapsed):
					case <-ctx.Done():
						return
					case <-p.done:
						return
					}
				}
			}
			lastCall = time.Now()
			p.place(ctx, job, b, store)
		}
	}
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded")
}

func (p *Pool) place(ctx context.Context, job domain.Job, b broker.Broker, store Store) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("panic placing order: %v", r)
			p.log().Error("worker: placement panic recovered",
				"follower_id", job.FollowerID,
				"order_id", job.FollowerOrderID,
				"error", err,
			)
			p.recordFailure(ctx, job, store, err)
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

	maxAttempts := p.MaxRetries
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	backoff := p.InitialBackoff
	if backoff <= 0 {
		backoff = 10 * time.Millisecond
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := b.PlaceOrder(ctx, "regular", params)
		if err == nil {
			p.log().Info("worker: order placed successfully",
				"follower_id", job.FollowerID,
				"order_id", job.FollowerOrderID,
				"broker_order_id", resp.OrderID,
				"symbol", job.Tradingsymbol,
				"qty", job.Quantity,
				"attempt", attempt,
			)

			_ = store.UpdateFollowerOrderPlaced(ctx, job.FollowerOrderID, resp.OrderID, job.Quantity)
			orderID := job.FollowerOrderID
			_ = store.AppendOrderEvent(ctx, domain.OrderEvent{
				FollowerOrderID: &orderID,
				MasterFillID:    &job.MasterFillID,
				AccountID:       job.FollowerID,
				EventType:       "api_response",
				Payload:         []byte(fmt.Sprintf(`{"broker_order_id":%q,"quantity":%d}`, resp.OrderID, job.Quantity)),
			})
			return
		}

		// Rejections (margin/RMS/input) are terminal on first attempt — no retry (Design Doc §6).
		if !isTimeout(err) {
			p.log().Error("worker: placement rejected by broker, terminal failure",
				"follower_id", job.FollowerID,
				"order_id", job.FollowerOrderID,
				"symbol", job.Tradingsymbol,
				"qty", job.Quantity,
				"error", err,
			)
			p.recordFailure(ctx, job, store, err)
			return
		}

		// If this was the last attempt, record the timeout failure.
		if attempt == maxAttempts {
			p.log().Error("worker: placement timeout retries exhausted",
				"follower_id", job.FollowerID,
				"order_id", job.FollowerOrderID,
				"symbol", job.Tradingsymbol,
				"attempts", attempt,
				"error", err,
			)
			p.recordFailure(ctx, job, store, err)
			return
		}

		p.log().Warn("worker: placement timeout, retrying with backoff",
			"follower_id", job.FollowerID,
			"order_id", job.FollowerOrderID,
			"attempt", attempt,
			"backoff_ms", backoff.Milliseconds(),
			"error", err,
		)

		select {
		case <-time.After(backoff):
			backoff *= 2
		case <-ctx.Done():
			p.recordFailure(ctx, job, store, ctx.Err())
			return
		case <-p.done:
			p.recordFailure(ctx, job, store, errors.New("pool shutdown"))
			return
		}
	}
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
