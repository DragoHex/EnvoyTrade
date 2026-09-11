package memchan_test

import (
	"context"
	"testing"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/queue"
	"envoytrade/internal/queue/memchan"
)

func fill(id int64) domain.MasterFill {
	return domain.MasterFill{ID: id, BrokerOrderID: "order"}
}

func TestPublishConsume_ReturnsSameEvent(t *testing.T) {
	q := memchan.New(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := q.Publish(ctx, fill(1)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	got := make(chan domain.MasterFill, 1)
	go func() {
		_ = q.Consume(ctx, func(_ context.Context, ev domain.MasterFill) error {
			got <- ev
			cancel()
			return nil
		})
	}()

	select {
	case ev := <-got:
		if ev.ID != 1 {
			t.Fatalf("got fill ID %d, want 1", ev.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for consumed event")
	}
}

func TestConsume_OrderPreserved(t *testing.T) {
	q := memchan.New(3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, id := range []int64{1, 2, 3} {
		if err := q.Publish(ctx, fill(id)); err != nil {
			t.Fatalf("Publish(%d): %v", id, err)
		}
	}

	var got []int64
	done := make(chan struct{})
	go func() {
		_ = q.Consume(ctx, func(_ context.Context, ev domain.MasterFill) error {
			got = append(got, ev.ID)
			if len(got) == 3 {
				close(done)
			}
			return nil
		})
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for all events")
	}
	cancel()

	want := []int64{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestPublish_NonBlockingWhenFull(t *testing.T) {
	q := memchan.New(1)
	ctx := context.Background()

	if err := q.Publish(ctx, fill(1)); err != nil {
		t.Fatalf("first Publish: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- q.Publish(ctx, fill(2)) }()

	select {
	case err := <-done:
		if err != memchan.ErrFull {
			t.Fatalf("second Publish error = %v, want ErrFull", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Publish blocked instead of failing fast when full")
	}
}

func TestPublish_BufferedBeforeConsumerStarts(t *testing.T) {
	q := memchan.New(2)
	ctx := t.Context()

	if err := q.Publish(ctx, fill(1)); err != nil {
		t.Fatalf("Publish(1): %v", err)
	}
	if err := q.Publish(ctx, fill(2)); err != nil {
		t.Fatalf("Publish(2): %v", err)
	}

	var got []int64
	done := make(chan struct{})
	go func() {
		_ = q.Consume(ctx, func(_ context.Context, ev domain.MasterFill) error {
			got = append(got, ev.ID)
			if len(got) == 2 {
				close(done)
			}
			return nil
		})
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for buffered events")
	}

	want := []int64{1, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}

	var _ queue.Publisher = q
	var _ queue.Consumer = q
}
