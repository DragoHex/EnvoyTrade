package kite_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"envoytrade/internal/kite"

	"github.com/google/uuid"
)

type mockSyncer struct {
	calls atomic.Int32
}

func (m *mockSyncer) SyncAccountPortfolio(ctx context.Context, accountID uuid.UUID) error {
	m.calls.Add(1)
	time.Sleep(50 * time.Millisecond)
	return nil
}

func TestDebouncedSyncer_CoalescesRapidCalls(t *testing.T) {
	inner := &mockSyncer{}
	debouncer := kite.NewDebouncedSyncer(inner, 50*time.Millisecond)

	accountID := uuid.New()

	// Fire 10 rapid sync calls
	for i := 0; i < 10; i++ {
		_ = debouncer.SyncAccountPortfolio(context.Background(), accountID)
	}

	// Wait for execution to settle
	time.Sleep(250 * time.Millisecond)

	// Calls should have coalesced to at most 2 runs (first one + 1 pending rerun)
	calls := inner.calls.Load()
	if calls > 2 {
		t.Fatalf("expected <= 2 calls due to coalescing, got %d", calls)
	}
	if calls < 1 {
		t.Fatalf("expected at least 1 call, got %d", calls)
	}
}
