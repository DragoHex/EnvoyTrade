// Package engine is the fan-out orchestrator: it turns one master fill
// into correctly-sized, idempotent follower orders and hands each one to
// its follower's worker. This is the "core copy trade mechanism"
// (PLAN.md §4.1–§4.3).
package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

// Store is everything the engine needs from persistence. Defined here
// (the consumer), not in the store package, per PLAN.md §1's seam rule.
// Methods return domain.ErrDuplicate on a unique-constraint collision.
type Store interface {
	MasterActive(ctx context.Context, masterID uuid.UUID) (bool, error)
	EnabledFollowLinks(ctx context.Context, masterID uuid.UUID) ([]domain.FollowLink, error)
	InstrumentLotSize(ctx context.Context, exchange, tradingsymbol string) (int, error)
	InsertFollowerOrder(ctx context.Context, o domain.FollowerOrder) (int64, error)
	SetMasterFillDispatchState(ctx context.Context, id int64, state domain.DispatchState) error
	UpdateFollowerOrderFailed(ctx context.Context, id int64, terminalStatus string, errMsg string) error
}

// Dispatcher hands a sized Job off to the given follower's worker.
// Dispatch must not block: it reports false immediately if the
// follower's inbound channel has no room, so one slow/backed-up follower
// can never stall fan-out to the others (PLAN.md §4.3).
type Dispatcher interface {
	Dispatch(followerID uuid.UUID, job domain.Job) bool
}

// Engine fans a master fill out to every enabled follower.
type Engine struct {
	store      Store
	dispatcher Dispatcher
	Logger     *slog.Logger
}

// New wires an Engine to its store and dispatcher.
func New(store Store, dispatcher Dispatcher) *Engine {
	return &Engine{store: store, dispatcher: dispatcher}
}

func (e *Engine) log() *slog.Logger {
	if e.Logger != nil {
		return e.Logger
	}
	return slog.Default()
}

// HandleMasterFill is the engine's one job: size the fill for every
// enabled follower of its master, persist a follower_order per follower,
// and dispatch each non-zero-quantity order to its worker. It is safe to
// call more than once for the same fill (WS redelivery, outbox
// redrive) — duplicate follower_orders are recognized and skipped, not
// re-dispatched.
func (e *Engine) HandleMasterFill(ctx context.Context, fill domain.MasterFill) error {
	e.log().Info("engine: processing master fill",
		"master_id", fill.MasterID,
		"broker_order_id", fill.BrokerOrderID,
		"symbol", fill.Tradingsymbol,
		"qty", fill.FilledQuantity,
	)

	active, err := e.store.MasterActive(ctx, fill.MasterID)
	if err != nil {
		return fmt.Errorf("check master active: %w", err)
	}
	if !active {
		// Master manually stopped via the Dashboard — no-op, same as a
		// duplicate-fill redelivery: safe to keep receiving fills, just
		// nothing dispatches until Start is clicked again.
		e.log().Info("engine: master inactive, skipping fan-out", "master_id", fill.MasterID)
		return nil
	}

	links, err := e.store.EnabledFollowLinks(ctx, fill.MasterID)
	if err != nil {
		return fmt.Errorf("load follow links: %w", err)
	}

	for _, link := range links {
		if err := e.fanOutToFollower(ctx, fill, link); err != nil {
			return fmt.Errorf("fan out to follower %s: %w", link.FollowerID, err)
		}
	}

	return e.store.SetMasterFillDispatchState(ctx, fill.ID, domain.DispatchDispatched)
}

func (e *Engine) fanOutToFollower(ctx context.Context, fill domain.MasterFill, link domain.FollowLink) error {
	// Lot size comes from the instrument master, never from the fill
	// itself — an F&O contract's lot size can change over its life, so a
	// signal must never carry its own (possibly stale) claim about it. A
	// symbol missing from the instrument master resolves to lot size 0,
	// which SizeOrder already treats as ReasonBadInstrument.
	lotSize, err := e.store.InstrumentLotSize(ctx, fill.Exchange, fill.Tradingsymbol)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("lookup instrument lot size: %w", err)
	}

	qty, reason := domain.SizeOrder(fill.FilledQuantity, link.CapitalRatio, lotSize, link.MaxQtyPerOrder)
	tag := domain.IdempotencyTag(fill.ID, link.FollowerID)

	e.log().Debug("engine: sized follower order",
		"master_id", fill.MasterID,
		"follower_id", link.FollowerID,
		"intended_qty", qty,
		"lot_size", lotSize,
		"sizing_reason", reason,
	)

	orderID, err := e.store.InsertFollowerOrder(ctx, domain.FollowerOrder{
		MasterFillID:   fill.ID,
		FollowerID:     link.FollowerID,
		IdempotencyTag: tag,
		IntendedQty:    qty,
		LotSize:        lotSize,
		SizingReason:   reason,
	})
	if errors.Is(err, domain.ErrDuplicate) {
		// Already fanned out for this (fill, follower) pair — a
		// redelivery, not a new signal. The original pass already
		// dispatched (or will, via the outbox drainer); doing it again
		// here would risk a second live order.
		e.log().Debug("engine: duplicate follower order, skipping redelivery",
			"follower_id", link.FollowerID,
			"fill_id", fill.ID,
		)
		return nil
	}
	if err != nil {
		return err
	}

	if qty == 0 {
		// Below one lot (or a bad instrument/ratio): visible on the
		// follower_order row, but there is nothing to place.
		e.log().Info("engine: zero quantity follower order, skipping dispatch",
			"follower_id", link.FollowerID,
			"order_id", orderID,
			"reason", reason,
		)
		return nil
	}

	job := domain.Job{
		FollowerOrderID: orderID,
		FollowerID:      link.FollowerID,
		MasterFillID:    fill.ID,
		IdempotencyTag:  tag,
		Exchange:        fill.Exchange,
		Tradingsymbol:   fill.Tradingsymbol,
		TransactionType: fill.TransactionType,
		Product:         fill.Product,
		OrderType:       fill.OrderType,
		Quantity:        qty,
	}
	if !e.dispatcher.Dispatch(link.FollowerID, job) {
		e.log().Error("engine: follower worker channel full, dead-lettered",
			"follower_id", link.FollowerID,
			"order_id", orderID,
		)
		return e.store.UpdateFollowerOrderFailed(ctx, orderID, domain.TerminalDeadLettered,
			"worker channel full: fan-out could not hand off in time")
	}

	e.log().Info("engine: follower order dispatched",
		"follower_id", link.FollowerID,
		"order_id", orderID,
		"qty", qty,
	)
	return nil
}
