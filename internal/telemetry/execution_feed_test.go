package telemetry_test

import (
	"sync"
	"testing"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/telemetry"
)

func TestExecutionFeed_CapacityAndOrder(t *testing.T) {
	feed := telemetry.NewExecutionFeed(5)

	if len(feed.Recent()) != 0 {
		t.Fatalf("expected empty feed initially, got %d", len(feed.Recent()))
	}

	// Add 3 events
	for i := 1; i <= 3; i++ {
		feed.Record(domain.ExecutionEvent{
			Timestamp:     time.Now().Add(time.Duration(i) * time.Second),
			Tradingsymbol: "SYMBOL_" + string(rune('A'+i-1)),
			MasterQty:     i * 25,
		})
	}

	recent := feed.Recent()
	if len(recent) != 3 {
		t.Fatalf("expected 3 events, got %d", len(recent))
	}
	// Newest should be first
	if recent[0].Tradingsymbol != "SYMBOL_C" {
		t.Errorf("expected newest first (SYMBOL_C), got %s", recent[0].Tradingsymbol)
	}
	if recent[2].Tradingsymbol != "SYMBOL_A" {
		t.Errorf("expected oldest last (SYMBOL_A), got %s", recent[2].Tradingsymbol)
	}

	// Add 4 more events to exceed capacity of 5 (total 7 added)
	for i := 4; i <= 7; i++ {
		feed.Record(domain.ExecutionEvent{
			Timestamp:     time.Now().Add(time.Duration(i) * time.Second),
			Tradingsymbol: "SYMBOL_" + string(rune('A'+i-1)),
			MasterQty:     i * 25,
		})
	}

	capped := feed.Recent()
	if len(capped) != 5 {
		t.Fatalf("expected capped at 5 events, got %d", len(capped))
	}
	// Newest should be SYMBOL_G (i=7)
	if capped[0].Tradingsymbol != "SYMBOL_G" {
		t.Errorf("expected newest SYMBOL_G, got %s", capped[0].Tradingsymbol)
	}
	// Oldest remaining should be SYMBOL_C (i=3)
	if capped[4].Tradingsymbol != "SYMBOL_C" {
		t.Errorf("expected oldest remaining SYMBOL_C, got %s", capped[4].Tradingsymbol)
	}
}

func TestExecutionFeed_ConcurrentSafe(t *testing.T) {
	feed := telemetry.NewExecutionFeed(25)
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				feed.Record(domain.ExecutionEvent{
					Tradingsymbol: "NIFTY",
					MasterQty:     50,
				})
				_ = feed.Recent()
			}
		}(i)
	}

	wg.Wait()
	recent := feed.Recent()
	if len(recent) != 25 {
		t.Errorf("expected exactly 25 items at full capacity, got %d", len(recent))
	}
}
