package fake_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"envoytrade/internal/broker"
	"envoytrade/internal/kite/fake"
)

var _ broker.Broker = (*fake.Broker)(nil)

func TestBroker_SuccessReturnsConfiguredOrderID(t *testing.T) {
	b := &fake.Broker{OrderID: "231000000123456"}
	resp, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if resp.OrderID != "231000000123456" {
		t.Fatalf("OrderID = %q, want configured value", resp.OrderID)
	}
}

func TestBroker_ErrorInjection(t *testing.T) {
	wantErr := errors.New("boom")
	b := &fake.Broker{Err: wantErr}
	_, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestBroker_LatencyIsHonored(t *testing.T) {
	b := &fake.Broker{Latency: 50 * time.Millisecond}
	start := time.Now()
	if _, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{}); err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Fatalf("PlaceOrder returned after %v, want >= 50ms", elapsed)
	}
}

func TestBroker_ContextCancelledDuringLatency(t *testing.T) {
	b := &fake.Broker{Latency: time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := b.PlaceOrder(ctx, "regular", broker.OrderParams{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestBroker_RecordsCalls(t *testing.T) {
	b := &fake.Broker{}
	params := broker.OrderParams{Tag: "abc123"}
	if _, err := b.PlaceOrder(context.Background(), "regular", params); err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if len(b.Calls) != 1 {
		t.Fatalf("Calls = %d, want 1", len(b.Calls))
	}
	if b.Calls[0].Tag != "abc123" {
		t.Fatalf("recorded Tag = %q, want %q", b.Calls[0].Tag, "abc123")
	}
}
