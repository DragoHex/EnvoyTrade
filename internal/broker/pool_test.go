package broker_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"envoytrade/internal/broker"

	"github.com/google/uuid"
)

type dummyPoolBroker struct {
	id string
}

func (d *dummyPoolBroker) PlaceOrder(ctx context.Context, variety string, params broker.OrderParams) (broker.OrderResponse, error) {
	return broker.OrderResponse{OrderID: d.id}, nil
}
func (d *dummyPoolBroker) GetPositions(ctx context.Context) ([]broker.Position, error) { return nil, nil }
func (d *dummyPoolBroker) GetOpenOrders(ctx context.Context) ([]broker.Order, error)    { return nil, nil }
func (d *dummyPoolBroker) CancelOrder(ctx context.Context, variety, orderID string) (broker.OrderResponse, error) {
	return broker.OrderResponse{OrderID: orderID}, nil
}

func TestAccountPool_CacheHitAndReuse(t *testing.T) {
	var resolveCalls int64
	accID := uuid.New()

	pool := broker.NewAccountPool(broker.ResolverFunc(func(ctx context.Context, id uuid.UUID) (broker.Broker, error) {
		atomic.AddInt64(&resolveCalls, 1)
		return &dummyPoolBroker{id: fmt.Sprintf("broker-%s-%d", id, atomic.LoadInt64(&resolveCalls))}, nil
	}))

	ctx := context.Background()

	b1, err := pool.ResolveBroker(ctx, accID)
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}

	b2, err := pool.ResolveBroker(ctx, accID)
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}

	if b1 != b2 {
		t.Errorf("expected exact same broker pointer on cache hit, got %p and %p", b1, b2)
	}

	if atomic.LoadInt64(&resolveCalls) != 1 {
		t.Errorf("expected resolver called once, got %d", atomic.LoadInt64(&resolveCalls))
	}
	if pool.Count() != 1 {
		t.Errorf("expected count 1, got %d", pool.Count())
	}
}

func TestAccountPool_Evict(t *testing.T) {
	var resolveCalls int64
	accID := uuid.New()

	pool := broker.NewAccountPool(broker.ResolverFunc(func(ctx context.Context, id uuid.UUID) (broker.Broker, error) {
		count := atomic.AddInt64(&resolveCalls, 1)
		return &dummyPoolBroker{id: fmt.Sprintf("broker-%s-%d", id, count)}, nil
	}))

	ctx := context.Background()

	b1, _ := pool.ResolveBroker(ctx, accID)
	pool.Evict(accID)

	if pool.Count() != 0 {
		t.Errorf("expected count 0 after evict, got %d", pool.Count())
	}

	b2, _ := pool.ResolveBroker(ctx, accID)

	if b1 == b2 {
		t.Errorf("expected different broker pointer after eviction, got identical %p", b1)
	}

	if atomic.LoadInt64(&resolveCalls) != 2 {
		t.Errorf("expected resolver called twice after evict, got %d", atomic.LoadInt64(&resolveCalls))
	}
}

func TestAccountPool_EvictAll(t *testing.T) {
	acc1 := uuid.New()
	acc2 := uuid.New()

	pool := broker.NewAccountPool(broker.ResolverFunc(func(ctx context.Context, id uuid.UUID) (broker.Broker, error) {
		return &dummyPoolBroker{id: id.String()}, nil
	}))

	ctx := context.Background()
	_, _ = pool.ResolveBroker(ctx, acc1)
	_, _ = pool.ResolveBroker(ctx, acc2)

	if pool.Count() != 2 {
		t.Fatalf("expected count 2, got %d", pool.Count())
	}

	pool.EvictAll()

	if pool.Count() != 0 {
		t.Fatalf("expected count 0 after EvictAll, got %d", pool.Count())
	}
}

func TestAccountPool_ResolverErrorNotCached(t *testing.T) {
	var resolveCalls int64
	errExpected := errors.New("broker unavailable")
	accID := uuid.New()

	pool := broker.NewAccountPool(broker.ResolverFunc(func(ctx context.Context, id uuid.UUID) (broker.Broker, error) {
		count := atomic.AddInt64(&resolveCalls, 1)
		if count == 1 {
			return nil, errExpected
		}
		return &dummyPoolBroker{id: "recovered"}, nil
	}))

	ctx := context.Background()

	_, err := pool.ResolveBroker(ctx, accID)
	if !errors.Is(err, errExpected) {
		t.Fatalf("expected errExpected, got %v", err)
	}
	if pool.Count() != 0 {
		t.Errorf("error should not be cached in pool, count=%d", pool.Count())
	}

	// Next call should retry resolver and succeed
	b, err := pool.ResolveBroker(ctx, accID)
	if err != nil {
		t.Fatalf("second resolve should succeed, got %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil broker")
	}
	if pool.Count() != 1 {
		t.Errorf("expected count 1 after success, got %d", pool.Count())
	}
}

func TestAccountPool_ConcurrentAccess(t *testing.T) {
	accs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	pool := broker.NewAccountPool(broker.ResolverFunc(func(ctx context.Context, id uuid.UUID) (broker.Broker, error) {
		return &dummyPoolBroker{id: id.String()}, nil
	}))

	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			targetID := accs[idx%len(accs)]
			if idx%10 == 0 {
				pool.Evict(targetID)
			} else {
				b, err := pool.ResolveBroker(ctx, targetID)
				if err != nil || b == nil {
					t.Errorf("concurrent resolve error: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()
}
