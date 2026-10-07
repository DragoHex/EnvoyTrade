// Package recon implements periodic reconciliation between broker REST
// state and local database state, serving as the backstop for WS disconnects
// and missed postbacks (Design Doc §5).
package recon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/kite/callback"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

// MasterOrderReader fetches recent orders for a master account via REST.
type MasterOrderReader interface {
	GetMasterOrders(ctx context.Context, masterID uuid.UUID) ([]kiteconnect.Order, error)
}

// FollowerOrderReader fetches transition history and orders for a follower's order via REST.
type FollowerOrderReader interface {
	GetFollowerOrderHistory(ctx context.Context, followerID uuid.UUID, brokerOrderID string) ([]kiteconnect.Order, error)
	GetFollowerOrders(ctx context.Context, followerID uuid.UUID) ([]kiteconnect.Order, error)
}

// MasterFillConsumer handles a master fill (idempotently inserting and fanning out).
type MasterFillConsumer interface {
	Handle(ctx context.Context, fill domain.MasterFill) error
}

// FollowerStatusConsumer handles follower order updates (backfilling or status updating).
type FollowerStatusConsumer interface {
	Handle(ctx context.Context, upd domain.OrderUpdate) error
}

// Store is everything the reconciler needs from persistence.
type Store interface {
	Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error)
	PendingFollowerOrders(ctx context.Context, cutoff time.Time) ([]domain.FollowerOrder, error)
	UpdateFollowerOrderStatus(ctx context.Context, brokerOrderID, status string, filledQty int, averagePrice decimal.Decimal) (int64, error)
	AppendOrderEvent(ctx context.Context, ev domain.OrderEvent) error
	SweepPendingOrderUpdates(ctx context.Context, cutoff time.Time) ([]string, error)
	MasterFillExists(ctx context.Context, masterID uuid.UUID, brokerOrderID string, filledQty int, status string) (bool, error)
	FollowerOrderTerminalExists(ctx context.Context, followerID uuid.UUID, brokerOrderID string) (bool, error)
	UpdateFollowerOrderFailed(ctx context.Context, id int64, terminalStatus string, errMsg string) error
}

// Alerter receives notifications about drift and unresolvable states.
type Alerter interface {
	Alert(ctx context.Context, msg string, details map[string]any)
}

// LogAlerter logs alerts at Error level.
type LogAlerter struct {
	Logger *slog.Logger
}

// Alert logs msg and details at Error level.
func (l *LogAlerter) Alert(ctx context.Context, msg string, details map[string]any) {
	logger := l.Logger
	if logger == nil {
		logger = slog.Default()
	}
	attrs := make([]any, 0, len(details)*2)
	for k, v := range details {
		attrs = append(attrs, k, v)
	}
	logger.Error("RECON_ALERT: "+msg, attrs...)
}

// Config configures the reconciler polling frequencies.
type Config struct {
	PollInterval     time.Duration
	PendingThreshold time.Duration
	PollingWindow    time.Duration
}

// DefaultConfig provides standard operational intervals.
func DefaultConfig() Config {
	return Config{
		PollInterval:     10 * time.Second,
		PendingThreshold: 15 * time.Second,
		PollingWindow:    15 * time.Minute,
	}
}

// Poller runs periodic reconciliation cycles.
type Poller struct {
	store            Store
	masterReader     MasterOrderReader
	followerReader   FollowerOrderReader
	masterConsumer   MasterFillConsumer
	FollowerConsumer FollowerStatusConsumer
	alerter          Alerter
	cfg              Config
	logger           *slog.Logger
}

// NewPoller creates a new reconciliation poller.
func NewPoller(
	store Store,
	masterReader MasterOrderReader,
	followerReader FollowerOrderReader,
	masterConsumer MasterFillConsumer,
	alerter Alerter,
	cfg Config,
	logger *slog.Logger,
) *Poller {
	if alerter == nil {
		alerter = &LogAlerter{Logger: logger}
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Second
	}
	if cfg.PendingThreshold <= 0 {
		cfg.PendingThreshold = 2 * time.Minute
	}
	if cfg.PollingWindow <= 0 {
		cfg.PollingWindow = 15 * time.Minute
	}
	return &Poller{
		store:          store,
		masterReader:   masterReader,
		followerReader: followerReader,
		masterConsumer: masterConsumer,
		alerter:        alerter,
		cfg:            cfg,
		logger:         logger,
	}
}

func (p *Poller) log() *slog.Logger {
	if p.logger != nil {
		return p.logger
	}
	return slog.Default()
}

// RunOnce performs a single reconciliation pass across master fills and follower orders.
func (p *Poller) RunOnce(ctx context.Context) error {
	// 1. List active accounts
	accounts, err := p.store.Accounts(ctx, nil)
	if err != nil {
		return fmt.Errorf("recon: list accounts: %w", err)
	}

	var masters []domain.Account
	var followers []domain.Account
	for _, a := range accounts {
		if a.Active {
			if a.Role == "master" {
				masters = append(masters, a)
			} else if a.Role == "follower" {
				followers = append(followers, a)
			}
		}
	}

	now := time.Now()

	// 2. Poll master orders & backfill any missed fills (WS gap fallback)
	for _, m := range masters {
		_ = p.ReconcileMaster(ctx, m.ID)
	}

	// 2b. Poll follower orders & backfill any missed callbacks
	for _, f := range followers {
		_ = p.ReconcileFollower(ctx, f.ID)
	}

	// 3. Inspect stuck follower orders older than threshold
	cutoff := now.Add(-p.cfg.PendingThreshold)
	pending, err := p.store.PendingFollowerOrders(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("recon: fetch pending follower orders: %w", err)
	}

	for _, fo := range pending {
		if fo.BrokerOrderID == "" {
			// Unplaced order stuck without a broker order ID past timeout
			p.alerter.Alert(ctx, "unresolved follower order: missing broker_order_id past threshold", map[string]any{
				"follower_order_id": fo.ID,
				"follower_id":       fo.FollowerID,
				"master_fill_id":    fo.MasterFillID,
				"created_at":        fo.CreatedAt.In(domain.IST),
			})
			if time.Since(fo.CreatedAt) > 5*time.Minute {
				_ = p.store.UpdateFollowerOrderFailed(ctx, fo.ID, "FAILED", "order unplaced without broker_order_id past 5m")
			}
			continue
		}

		history, err := p.followerReader.GetFollowerOrderHistory(ctx, fo.FollowerID, fo.BrokerOrderID)
		if err != nil {
			p.alerter.Alert(ctx, "failed to read follower order history", map[string]any{
				"follower_order_id": fo.ID,
				"follower_id":       fo.FollowerID,
				"broker_order_id":   fo.BrokerOrderID,
				"error":             err.Error(),
			})
			if time.Since(fo.CreatedAt) > 5*time.Minute {
				_ = p.store.UpdateFollowerOrderFailed(ctx, fo.ID, "FAILED", fmt.Sprintf("order unresolvable at broker past 5m: %v", err))
			}
			continue
		}

		if len(history) == 0 {
			p.alerter.Alert(ctx, "follower order not found at broker", map[string]any{
				"follower_order_id": fo.ID,
				"follower_id":       fo.FollowerID,
				"broker_order_id":   fo.BrokerOrderID,
			})
			if time.Since(fo.CreatedAt) > 5*time.Minute {
				_ = p.store.UpdateFollowerOrderFailed(ctx, fo.ID, "FAILED", "order not found at broker past 5m")
			}
			continue
		}

		latest := history[len(history)-1]
		if callback.IsTerminal(latest.Status) {
			avgPrice := decimal.NewFromFloat(latest.AveragePrice)
			_, err := p.store.UpdateFollowerOrderStatus(ctx, fo.BrokerOrderID, latest.Status, int(latest.FilledQuantity), avgPrice)
			if err != nil {
				p.alerter.Alert(ctx, "failed to update resolved follower order status", map[string]any{
					"follower_order_id": fo.ID,
					"broker_order_id":   fo.BrokerOrderID,
					"status":            latest.Status,
					"error":             err.Error(),
				})
			} else {
				raw, _ := json.Marshal(latest)
				_ = p.store.AppendOrderEvent(ctx, domain.OrderEvent{
					FollowerOrderID: &fo.ID,
					AccountID:       fo.FollowerID,
					EventType:       "reconciled",
					Payload:         raw,
				})
			}

			// Check drift
			if latest.Status != domain.TerminalComplete || int(latest.FilledQuantity) != fo.IntendedQty {
				p.alerter.Alert(ctx, "drift detected on follower order", map[string]any{
					"follower_order_id": fo.ID,
					"follower_id":       fo.FollowerID,
					"status":            latest.Status,
					"intended_qty":      fo.IntendedQty,
					"filled_qty":        latest.FilledQuantity,
				})
			}
		} else {
			// Order is still non-terminal after threshold
			p.alerter.Alert(ctx, "follower order remains non-terminal past timeout", map[string]any{
				"follower_order_id": fo.ID,
				"follower_id":       fo.FollowerID,
				"broker_order_id":   fo.BrokerOrderID,
				"status":            latest.Status,
			})
			if time.Since(fo.CreatedAt) > 5*time.Minute {
				_ = p.store.UpdateFollowerOrderFailed(ctx, fo.ID, "FAILED", fmt.Sprintf("order remains non-terminal past 5m: %s", latest.Status))
			}
		}
	}

	// 4. Sweep stale pending order updates (>24h old)
	staleCutoff := now.Add(-24 * time.Hour)
	swept, err := p.store.SweepPendingOrderUpdates(ctx, staleCutoff)
	if err != nil {
		p.log().Warn("recon: sweep pending order updates failed", "error", err)
	} else if len(swept) > 0 {
		p.alerter.Alert(ctx, "stale unmatched pending order updates swept", map[string]any{
			"count":     len(swept),
			"order_ids": swept,
		})
	}

	return nil
}

// ReconcileMaster checks and backfills recent fills for a single master account (Design Doc §4, §6).
func (p *Poller) ReconcileMaster(ctx context.Context, masterID uuid.UUID) error {
	now := time.Now()
	windowStart := now.Add(-p.cfg.PollingWindow)

	orders, err := p.masterReader.GetMasterOrders(ctx, masterID)
	if err != nil {
		p.alerter.Alert(ctx, "failed to read master orders", map[string]any{
			"master_id": masterID,
			"error":     err.Error(),
		})
		return err
	}

	for _, o := range orders {
		if !callback.IsTerminal(o.Status) {
			continue
		}
		if !o.OrderTimestamp.Time.IsZero() && o.OrderTimestamp.Time.Before(windowStart) {
			continue
		}
		exists, err := p.store.MasterFillExists(ctx, masterID, o.OrderID, int(o.FilledQuantity), o.Status)
		if err == nil && exists {
			continue
		}
		fill := callback.ToMasterFill(o, masterID)
		if err := p.masterConsumer.Handle(ctx, fill); err != nil {
			p.alerter.Alert(ctx, "failed to handle master fill from recon", map[string]any{
				"master_id":       masterID,
				"broker_order_id": o.OrderID,
				"error":           err.Error(),
			})
		}
	}
	return nil
}

// ReconcileFollower checks and backfills recent orders for a single follower account.
func (p *Poller) ReconcileFollower(ctx context.Context, followerID uuid.UUID) error {
	if p.FollowerConsumer == nil {
		return nil
	}
	now := time.Now()
	windowStart := now.Add(-p.cfg.PollingWindow)

	orders, err := p.followerReader.GetFollowerOrders(ctx, followerID)
	if err != nil {
		p.alerter.Alert(ctx, "failed to read follower orders", map[string]any{
			"follower_id": followerID,
			"error":       err.Error(),
		})
		return err
	}

	for _, o := range orders {
		if !callback.IsTerminal(o.Status) {
			continue
		}
		if !o.OrderTimestamp.Time.IsZero() && o.OrderTimestamp.Time.Before(windowStart) {
			continue
		}
		exists, err := p.store.FollowerOrderTerminalExists(ctx, followerID, o.OrderID)
		if err == nil && exists {
			continue
		}
		upd := callback.ToOrderUpdate(o, followerID)
		if err := p.FollowerConsumer.Handle(ctx, upd); err != nil {
			p.alerter.Alert(ctx, "failed to handle follower order from recon", map[string]any{
				"follower_id":     followerID,
				"broker_order_id": o.OrderID,
				"error":           err.Error(),
			})
		}
	}
	return nil
}

// Run executes reconciliation cycles continuously until ctx is cancelled.
func (p *Poller) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := p.RunOnce(ctx); err != nil {
				p.log().Error("recon: poll cycle failed", "error", err)
			}
		}
	}
}
