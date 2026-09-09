// Package listener drains the queues internal/kite/callback publishes
// onto and applies each event to the existing store/engine — the "worker"
// side of the postback/WS → queue → worker pipeline (PLAN.md §4.1).
package listener

import (
	"context"
	"errors"
	"log/slog"

	"envoytrade/internal/domain"
	"envoytrade/internal/queue"

	"github.com/shopspring/decimal"
)

// MasterFillStore is what MasterFillConsumer needs from persistence.
// Defined here (the consumer), not in the store package, per PLAN.md
// §1's seam rule.
type MasterFillStore interface {
	InsertMasterFill(ctx context.Context, f domain.MasterFill) (int64, error)
}

// MasterFillEngine is what MasterFillConsumer needs from the fan-out
// engine.
type MasterFillEngine interface {
	HandleMasterFill(ctx context.Context, fill domain.MasterFill) error
}

// MasterFillConsumer turns a queued domain.MasterFill (from postback or
// WS ticker) into a durable row plus a fan-out pass. It's safe to run
// more than once for the same fill — a duplicate insert is recognized
// and skipped, never re-dispatched.
type MasterFillConsumer struct {
	Store  MasterFillStore
	Engine MasterFillEngine
	Logger *slog.Logger
}

func (c *MasterFillConsumer) log() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// Handle persists fill and fans it out. Called directly by tests, or via
// Run for a live queue.Consumer.
func (c *MasterFillConsumer) Handle(ctx context.Context, fill domain.MasterFill) error {
	id, err := c.Store.InsertMasterFill(ctx, fill)
	if errors.Is(err, domain.ErrDuplicate) {
		// Already durable from an earlier delivery (WS + postback both
		// fired, or a retry) — the original pass already fanned out.
		return nil
	}
	if err != nil {
		c.log().Error("listener: insert master fill failed", "broker_order_id", fill.BrokerOrderID, "error", err)
		return err
	}
	fill.ID = id
	return c.Engine.HandleMasterFill(ctx, fill)
}

// Run drains c until ctx is cancelled. An error handling one event is
// logged, not fatal — Run keeps consuming the next one.
func (c *MasterFillConsumer) Run(ctx context.Context, consumer queue.Consumer[domain.MasterFill]) error {
	return consumer.Consume(ctx, func(ctx context.Context, fill domain.MasterFill) error {
		return c.Handle(ctx, fill)
	})
}

// FollowerStatusStore is what FollowerStatusConsumer needs from
// persistence.
type FollowerStatusStore interface {
	// UpdateFollowerOrderStatus returns domain.ErrNotFound if no
	// follower_order has this broker_order_id yet — the postback can
	// race the worker's own placement write.
	UpdateFollowerOrderStatus(ctx context.Context, brokerOrderID, status string, filledQty int, averagePrice decimal.Decimal) (int64, error)
	GetFollowerOrder(ctx context.Context, id int64) (domain.FollowerOrder, error)
	AppendOrderEvent(ctx context.Context, ev domain.OrderEvent) error
}

// FollowerStatusConsumer applies a queued domain.OrderUpdate (postback
// only — followers have no WS ticker, PLAN.md §3.4) to the matching
// follower_orders row and appends it to the audit trail.
type FollowerStatusConsumer struct {
	Store  FollowerStatusStore
	Logger *slog.Logger
}

func (c *FollowerStatusConsumer) log() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// Handle applies upd. An unknown broker_order_id is dropped, not an
// error — it can arrive before the worker's own placement write commits.
func (c *FollowerStatusConsumer) Handle(ctx context.Context, upd domain.OrderUpdate) error {
	id, err := c.Store.UpdateFollowerOrderStatus(ctx, upd.BrokerOrderID, upd.Status, upd.FilledQuantity, upd.AveragePrice)
	if errors.Is(err, domain.ErrNotFound) {
		c.log().Warn("listener: order update for unknown broker_order_id, dropping", "broker_order_id", upd.BrokerOrderID)
		return nil
	}
	if err != nil {
		c.log().Error("listener: update follower order status failed", "broker_order_id", upd.BrokerOrderID, "error", err)
		return err
	}

	order, err := c.Store.GetFollowerOrder(ctx, id)
	if err != nil {
		c.log().Error("listener: fetch follower order for audit event failed", "id", id, "error", err)
		return err
	}
	return c.Store.AppendOrderEvent(ctx, domain.OrderEvent{
		FollowerOrderID: &id,
		AccountID:       order.FollowerID,
		EventType:       "status_update",
		Payload:         upd.RawPayload,
	})
}

// Run drains c until ctx is cancelled. An error handling one event is
// logged, not fatal — Run keeps consuming the next one.
func (c *FollowerStatusConsumer) Run(ctx context.Context, consumer queue.Consumer[domain.OrderUpdate]) error {
	return consumer.Consume(ctx, func(ctx context.Context, upd domain.OrderUpdate) error {
		return c.Handle(ctx, upd)
	})
}
