package kite

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// InnerSyncer is what DebouncedSyncer wraps.
type InnerSyncer interface {
	SyncAccountPortfolio(ctx context.Context, accountID uuid.UUID) error
}

// DebouncedSyncer wraps an InnerSyncer and coalesces rapid sync requests
// for the same account, preventing concurrent or redundant upstream broker calls.
type DebouncedSyncer struct {
	inner    InnerSyncer
	cooldown time.Duration

	mu      sync.Mutex
	active  map[uuid.UUID]bool
	pending map[uuid.UUID]bool
}

// NewDebouncedSyncer constructs a debounced portfolio syncer.
func NewDebouncedSyncer(inner InnerSyncer, cooldown time.Duration) *DebouncedSyncer {
	if cooldown <= 0 {
		cooldown = 500 * time.Millisecond
	}
	return &DebouncedSyncer{
		inner:    inner,
		cooldown: cooldown,
		active:   make(map[uuid.UUID]bool),
		pending:  make(map[uuid.UUID]bool),
	}
}

// SyncAccountPortfolio schedules or runs a sync for accountID. If a sync is
// currently running for this account, it marks that another run is needed and returns immediately.
func (d *DebouncedSyncer) SyncAccountPortfolio(ctx context.Context, accountID uuid.UUID) error {
	d.mu.Lock()
	if d.active[accountID] {
		d.pending[accountID] = true
		d.mu.Unlock()
		return nil
	}
	d.active[accountID] = true
	d.mu.Unlock()

	go d.run(accountID)
	return nil
}

func (d *DebouncedSyncer) run(accountID uuid.UUID) {
	for {
		_ = d.inner.SyncAccountPortfolio(context.Background(), accountID)

		if d.cooldown > 0 {
			time.Sleep(d.cooldown)
		}

		d.mu.Lock()
		if !d.pending[accountID] {
			delete(d.active, accountID)
			d.mu.Unlock()
			return
		}
		d.pending[accountID] = false
		d.mu.Unlock()
	}
}
