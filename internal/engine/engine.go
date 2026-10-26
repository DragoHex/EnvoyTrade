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
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	InsertFollowerOrdersBatch(ctx context.Context, orders []domain.FollowerOrder) (map[string]int64, error)
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

// ExecutionRecorder records master fill fan-out events into telemetry or feed.
type ExecutionRecorder interface {
	Record(event domain.ExecutionEvent)
}

// Engine fans a master fill out to every enabled follower.
type Engine struct {
	store             Store
	dispatcher        Dispatcher
	Logger            *slog.Logger
	ExecutionRecorder ExecutionRecorder
	lotSizeCache      sync.Map
	killSwitch        atomic.Bool
	killSwitchStatus  atomic.Pointer[domain.KillSwitchStatus]
}

// New wires an Engine to its store and dispatcher.
func New(store Store, dispatcher Dispatcher) *Engine {
	return &Engine{store: store, dispatcher: dispatcher}
}

// IsKillSwitchEngaged returns whether the emergency fan-out kill switch is active.
func (e *Engine) IsKillSwitchEngaged() bool {
	return e.killSwitch.Load()
}

// KillSwitchStatus returns the current kill switch state.
func (e *Engine) KillSwitchStatus() domain.KillSwitchStatus {
	ptr := e.killSwitchStatus.Load()
	if ptr == nil {
		return domain.KillSwitchStatus{Engaged: false}
	}
	return *ptr
}

// SetKillSwitch engages or disengages the emergency fan-out kill switch.
func (e *Engine) SetKillSwitch(engaged bool, reason string, engagedBy string) {
	e.killSwitch.Store(engaged)
	now := time.Now().UTC()
	st := &domain.KillSwitchStatus{
		Engaged: engaged,
	}
	if engaged {
		st.EngagedAt = &now
		st.EngagedBy = engagedBy
		st.Reason = reason
		e.log().Warn("engine: EMERGENCY KILL SWITCH ENGAGED", "reason", reason, "by", engagedBy)
	} else {
		e.log().Info("engine: emergency kill switch disengaged", "by", engagedBy)
	}
	e.killSwitchStatus.Store(st)
}

// ClearLotSizeCache clears the in-memory instrument lot size cache.
func (e *Engine) ClearLotSizeCache() {
	e.lotSizeCache.Range(func(key, _ any) bool {
		e.lotSizeCache.Delete(key)
		return true
	})
	e.log().Info("engine: lot size cache cleared")
}

func (e *Engine) resolveLotSize(ctx context.Context, exchange, symbol string) (int, error) {
	key := exchange + ":" + symbol
	if val, ok := e.lotSizeCache.Load(key); ok {
		return val.(int), nil
	}
	lotSize, err := e.store.InstrumentLotSize(ctx, exchange, symbol)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			e.lotSizeCache.Store(key, 0)
			return 0, nil
		}
		return 0, err
	}
	e.lotSizeCache.Store(key, lotSize)
	return lotSize, nil
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
	startTime := time.Now()
	e.log().Info("engine: processing master fill",
		"master_id", fill.MasterID,
		"broker_order_id", fill.BrokerOrderID,
		"symbol", fill.Tradingsymbol,
		"qty", fill.FilledQuantity,
	)

	if e.IsKillSwitchEngaged() {
		e.log().Warn("engine: kill switch engaged, dropping master fill fan-out",
			"master_id", fill.MasterID,
			"broker_order_id", fill.BrokerOrderID,
			"symbol", fill.Tradingsymbol,
		)
		if fill.ID > 0 {
			_ = e.store.SetMasterFillDispatchState(ctx, fill.ID, domain.DispatchDispatched)
		}
		return nil
	}

	if fill.Status != "" && fill.Status != domain.TerminalComplete {
		e.log().Info("engine: master fill is not complete, skipping fan-out",
			"master_id", fill.MasterID,
			"broker_order_id", fill.BrokerOrderID,
			"status", fill.Status,
		)
		return nil
	}

	if strings.HasPrefix(fill.Tag, "sqoff") {
		e.log().Info("engine: skipping fan-out for square-off master fill",
			"master_id", fill.MasterID,
			"broker_order_id", fill.BrokerOrderID,
			"tag", fill.Tag,
		)
		if fill.ID > 0 {
			_ = e.store.SetMasterFillDispatchState(ctx, fill.ID, domain.DispatchDispatched)
		}
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
	if len(links) == 0 {
		if e.ExecutionRecorder != nil {
			execPrice, _ := fill.Price.Float64()
			if execPrice <= 0 {
				execPrice, _ = fill.AveragePrice.Float64()
			}
			e.ExecutionRecorder.Record(domain.ExecutionEvent{
				Timestamp:       time.Now().UTC(),
				Tradingsymbol:   fill.Tradingsymbol,
				Exchange:        fill.Exchange,
				TransactionType: fill.TransactionType,
				MasterQty:       fill.FilledQuantity,
				Price:           execPrice,
				FollowersCount:  0,
				SkippedCount:    0,
				LatencyMs:       time.Since(startTime).Milliseconds(),
			})
		}
		return e.store.SetMasterFillDispatchState(ctx, fill.ID, domain.DispatchDispatched)
	}

	// 1. Resolve master execution prices ONCE before processing followers
	price, triggerPrice := resolveExecutionPrices(fill)

	// 2. Resolve lot size ONCE via cached resolver (only if scaled ratios exist)
	one := decimal.NewFromInt(1)
	hasScaledRatios := false
	for _, link := range links {
		if !link.CloneFactor.Equal(one) {
			hasScaledRatios = true
			break
		}
	}

	var lotSize int
	if hasScaledRatios {
		var err error
		lotSize, err = e.resolveLotSize(ctx, fill.Exchange, fill.Tradingsymbol)
		if err != nil {
			return fmt.Errorf("lookup instrument lot size: %w", err)
		}
	}

	// 3. Pre-size all follower orders in memory
	type candidateJob struct {
		followerID uuid.UUID
		tag        string
		qty        int
		reason     domain.SizingReason
	}
	ordersToInsert := make([]domain.FollowerOrder, len(links))
	candidates := make([]candidateJob, len(links))

	for i, link := range links {
		var (
			qty    int
			reason domain.SizingReason
		)
		if link.Status == "error" || link.AuthStatus == "error" {
			qty = 0
			reason = domain.ReasonFollowerInError
		} else if link.Status == "pending" || link.AuthStatus == "pending" {
			qty = 0
			reason = domain.ReasonFollowerPending
		} else if link.CloneFactor.Equal(one) {
			qty = fill.FilledQuantity
			if link.MaxQtyPerOrder > 0 && qty > link.MaxQtyPerOrder {
				qty = link.MaxQtyPerOrder
				reason = domain.ReasonCapped
			} else {
				reason = domain.ReasonOK
			}
		} else {
			qty, reason = domain.SizeOrder(fill.FilledQuantity, link.CloneFactor, lotSize, link.MaxQtyPerOrder)
		}

		tag := domain.IdempotencyTag(fill.ID, link.FollowerID)
		candidates[i] = candidateJob{
			followerID: link.FollowerID,
			tag:        tag,
			qty:        qty,
			reason:     reason,
		}

		ordersToInsert[i] = domain.FollowerOrder{
			MasterFillID:    fill.ID,
			FollowerID:      link.FollowerID,
			IdempotencyTag:  tag,
			IntendedQty:     qty,
			LotSize:         lotSize,
			SizingReason:    reason,
			Origin:          "copy_trade",
			Tradingsymbol:   fill.Tradingsymbol,
			Exchange:        fill.Exchange,
			Product:         fill.Product,
			TransactionType: fill.TransactionType,
			OrderType:       fill.OrderType,
		}
	}

	// 4. Single Bulk SQL Insert (Atomic DB preparation)
	insertedMap, err := e.store.InsertFollowerOrdersBatch(ctx, ordersToInsert)
	if err != nil {
		return fmt.Errorf("bulk insert follower orders: %w", err)
	}

	// 5. In-Memory Dispatch Loop
	dispatchedCount := 0
	skippedCount := 0

	for _, c := range candidates {
		orderID, ok := insertedMap[c.tag]
		if !ok {
			// Already fanned out for this (fill, follower) pair — a
			// redelivery, not a new signal. Skip silently.
			e.log().Debug("engine: duplicate follower order, skipping redelivery",
				"follower_id", c.followerID,
				"fill_id", fill.ID,
			)
			continue
		}

		if c.qty == 0 {
			// Below one lot (or a bad instrument/ratio): visible on the
			// follower_order row, but there is nothing to place.
			skippedCount++
			e.log().Info("engine: zero quantity follower order, skipping dispatch",
				"follower_id", c.followerID,
				"order_id", orderID,
				"reason", c.reason,
			)
			if err := e.store.UpdateFollowerOrderFailed(ctx, orderID, "SKIPPED", c.reason.String()); err != nil {
				e.log().Warn("engine: failed to mark zero-qty follower order as skipped",
					"order_id", orderID,
					"error", err,
				)
			}
			continue
		}

		job := domain.Job{
			FollowerOrderID: orderID,
			FollowerID:      c.followerID,
			MasterFillID:    fill.ID,
			IdempotencyTag:  c.tag,
			Exchange:        fill.Exchange,
			Tradingsymbol:   fill.Tradingsymbol,
			TransactionType: fill.TransactionType,
			Product:         fill.Product,
			OrderType:       fill.OrderType,
			Quantity:        c.qty,
			Price:           price,
			TriggerPrice:    triggerPrice,
		}

		if !e.dispatcher.Dispatch(c.followerID, job) {
			e.log().Error("engine: follower worker channel full, dead-lettered",
				"follower_id", c.followerID,
				"order_id", orderID,
			)
			if err := e.store.UpdateFollowerOrderFailed(ctx, orderID, domain.TerminalDeadLettered,
				"worker channel full: fan-out could not hand off in time"); err != nil {
				e.log().Warn("engine: failed to mark dead-lettered follower order",
					"order_id", orderID,
					"error", err,
				)
			}
			continue
		}

		dispatchedCount++
		e.log().Info("engine: follower order dispatched",
			"follower_id", c.followerID,
			"order_id", orderID,
			"qty", c.qty,
		)
	}

	if e.ExecutionRecorder != nil {
		execPrice := price
		if execPrice <= 0 {
			execPrice, _ = fill.AveragePrice.Float64()
		}
		e.ExecutionRecorder.Record(domain.ExecutionEvent{
			Timestamp:       time.Now().UTC(),
			Tradingsymbol:   fill.Tradingsymbol,
			Exchange:        fill.Exchange,
			TransactionType: fill.TransactionType,
			MasterQty:       fill.FilledQuantity,
			Price:           execPrice,
			FollowersCount:  dispatchedCount,
			SkippedCount:    skippedCount,
			LatencyMs:       time.Since(startTime).Milliseconds(),
		})
	}

	return e.store.SetMasterFillDispatchState(ctx, fill.ID, domain.DispatchDispatched)
}

func resolveExecutionPrices(fill domain.MasterFill) (float64, float64) {
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
	return price, triggerPrice
}
