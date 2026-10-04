// Package engine is the fan-out orchestrator: it turns one master fill
// into correctly-sized, idempotent follower orders and hands each one to
// its follower's worker. This is the "core copy trade mechanism"
// (PLAN.md §4.1–§4.3).
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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

	if fill.Status != "" && fill.Status != domain.TerminalComplete {
		e.log().Info("engine: master fill is not complete, skipping fan-out",
			"master_id", fill.MasterID,
			"broker_order_id", fill.BrokerOrderID,
			"status", fill.Status,
		)
		return nil
	}

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
	var (
		qty     int
		reason  domain.SizingReason
		lotSize int
	)

	one := decimal.NewFromInt(1)
	if link.CapitalRatio.Equal(one) {
		// Fast-path (main flow): 1:1 ratio forwards master quantity directly.
		// Bypasses local DB instrument validation and calculation.
		qty = fill.FilledQuantity
		if link.MaxQtyPerOrder > 0 && qty > link.MaxQtyPerOrder {
			qty = link.MaxQtyPerOrder
			reason = domain.ReasonCapped
		} else {
			reason = domain.ReasonOK
		}
	} else {
		// Scaled ratio path (ratio != 1): requires exchange lot size to round down to integer lot multiples.
		var err error
		lotSize, err = e.store.InstrumentLotSize(ctx, fill.Exchange, fill.Tradingsymbol)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("lookup instrument lot size: %w", err)
		}
		qty, reason = domain.SizeOrder(fill.FilledQuantity, link.CapitalRatio, lotSize, link.MaxQtyPerOrder)
	}

	tag := domain.IdempotencyTag(fill.ID, link.FollowerID)

	e.log().Debug("engine: sized follower order",
		"master_id", fill.MasterID,
		"follower_id", link.FollowerID,
		"intended_qty", qty,
		"lot_size", lotSize,
		"sizing_reason", reason,
		"fast_path", link.CapitalRatio.Equal(one),
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
		if err := e.store.UpdateFollowerOrderFailed(ctx, orderID, "SKIPPED", reason.String()); err != nil {
			e.log().Warn("engine: failed to mark zero-qty follower order as skipped",
				"order_id", orderID,
				"error", err,
			)
		}
		return nil
	}

	price, _ := fill.Price.Float64()
	triggerPrice, _ := fill.TriggerPrice.Float64()

	// If price or trigger price not set, inspect RawPayload if present
	if (price <= 0 || triggerPrice <= 0) && len(fill.RawPayload) > 0 {
		var raw struct {
			Price        float64 `json:"price"`
			TriggerPrice float64 `json:"trigger_price"`
		}
		if json.Unmarshal(fill.RawPayload, &raw) == nil {
			if price <= 0 && raw.Price > 0 {
				price = raw.Price
			}
			if triggerPrice <= 0 && raw.TriggerPrice > 0 {
				triggerPrice = raw.TriggerPrice
			}
		}
	}

	// Fallback to market execution price (AveragePrice) if LIMIT order still has no price
	if fill.OrderType == "LIMIT" && price <= 0 {
		price, _ = fill.AveragePrice.Float64()
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
		Price:           price,
		TriggerPrice:    triggerPrice,
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
