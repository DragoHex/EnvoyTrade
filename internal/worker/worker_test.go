package worker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"
	"envoytrade/internal/kite/fake"
	"envoytrade/internal/worker"

	"github.com/google/uuid"
	"go.uber.org/goleak"
)

// fakeStore is an in-memory recorder of everything worker writes back —
// enough to assert worker behavior without a real Postgres.
type fakeStore struct {
	mu     sync.Mutex
	placed map[int64]placedCall
	failed map[int64]failedCall
	events []domain.OrderEvent
}

type placedCall struct {
	brokerOrderID string
	placedQty     int
}

type failedCall struct {
	terminalStatus string
	errMsg         string
}

func newFakeStore() *fakeStore {
	return &fakeStore{placed: map[int64]placedCall{}, failed: map[int64]failedCall{}}
}

func (s *fakeStore) UpdateFollowerOrderPlaced(ctx context.Context, id int64, brokerOrderID string, placedQty int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.placed[id] = placedCall{brokerOrderID, placedQty}
	return nil
}

func (s *fakeStore) UpdateFollowerOrderFailed(ctx context.Context, id int64, terminalStatus string, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed[id] = failedCall{terminalStatus, errMsg}
	return nil
}

func (s *fakeStore) AppendOrderEvent(ctx context.Context, ev domain.OrderEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	return nil
}

func (s *fakeStore) eventCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func (s *fakeStore) placedFor(id int64) (placedCall, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.placed[id]
	return c, ok
}

func (s *fakeStore) failedFor(id int64) (failedCall, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.failed[id]
	return c, ok
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func TestPool_SuccessfulPlaceOrderUpdatesStore(t *testing.T) {
	store := newFakeStore()
	broker := &fake.Broker{OrderID: "ORDER1"}
	pool := worker.NewPool()
	defer pool.Shutdown()

	followerID := uuid.New()
	pool.Register(context.Background(), followerID, broker, store, 4)

	job := domain.Job{FollowerOrderID: 1, FollowerID: followerID, Quantity: 50, IdempotencyTag: "tag1"}
	if !pool.Dispatch(followerID, job) {
		t.Fatal("Dispatch returned false, want true")
	}

	waitFor(t, func() bool { _, ok := store.placedFor(1); return ok })
	call, _ := store.placedFor(1)
	if call.brokerOrderID != "ORDER1" || call.placedQty != 50 {
		t.Fatalf("placed call = %+v, want OrderID=ORDER1 Qty=50", call)
	}
	waitFor(t, func() bool { return store.eventCount() >= 1 })
}

func TestPool_BrokerErrorIsTerminalNoRetry(t *testing.T) {
	store := newFakeStore()
	wantErr := errors.New("insufficient margin")
	broker := &fake.Broker{Err: wantErr}
	pool := worker.NewPool()
	defer pool.Shutdown()

	followerID := uuid.New()
	pool.Register(context.Background(), followerID, broker, store, 4)

	job := domain.Job{FollowerOrderID: 2, FollowerID: followerID, Quantity: 10, IdempotencyTag: "tag2"}
	pool.Dispatch(followerID, job)

	waitFor(t, func() bool { _, ok := store.failedFor(2); return ok })
	call, _ := store.failedFor(2)
	if call.errMsg != wantErr.Error() {
		t.Fatalf("failed errMsg = %q, want %q", call.errMsg, wantErr.Error())
	}

	// No retry: broker should have been called exactly once.
	time.Sleep(50 * time.Millisecond)
	if len(broker.Calls) != 1 {
		t.Fatalf("broker was called %d times, want 1 (no retry)", len(broker.Calls))
	}
	waitFor(t, func() bool { return store.eventCount() >= 1 })
}

func TestPool_IsolatesSlowFollowerFromOthers(t *testing.T) {
	defer goleak.VerifyNone(t)

	store := newFakeStore()
	slowBroker := &fake.Broker{Latency: 300 * time.Millisecond, OrderID: "SLOW"}
	fastBroker := &fake.Broker{OrderID: "FAST"}

	pool := worker.NewPool()
	slowFollower := uuid.New()
	fastFollower := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())
	pool.Register(ctx, slowFollower, slowBroker, store, 4)
	pool.Register(ctx, fastFollower, fastBroker, store, 4)

	pool.Dispatch(slowFollower, domain.Job{FollowerOrderID: 10, FollowerID: slowFollower, Quantity: 1})
	pool.Dispatch(fastFollower, domain.Job{FollowerOrderID: 11, FollowerID: fastFollower, Quantity: 1})

	// The fast follower must complete well before the slow one, proving
	// one follower's latency doesn't block another's goroutine.
	waitFor(t, func() bool { _, ok := store.placedFor(11); return ok })
	if _, ok := store.placedFor(10); ok {
		t.Fatal("slow follower's order completed too early — dispatch isn't isolated")
	}

	waitFor(t, func() bool { _, ok := store.placedFor(10); return ok })
	cancel()
	pool.Shutdown()
}

func TestPool_PanicInOneWorkerDoesNotAffectAnother(t *testing.T) {
	defer goleak.VerifyNone(t)

	store := newFakeStore()
	panicky := &panickyBroker{}
	healthy := &fake.Broker{OrderID: "OK"}

	pool := worker.NewPool()
	panickyFollower := uuid.New()
	healthyFollower := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())
	pool.Register(ctx, panickyFollower, panicky, store, 4)
	pool.Register(ctx, healthyFollower, healthy, store, 4)

	pool.Dispatch(panickyFollower, domain.Job{FollowerOrderID: 20, FollowerID: panickyFollower, Quantity: 1})
	pool.Dispatch(healthyFollower, domain.Job{FollowerOrderID: 21, FollowerID: healthyFollower, Quantity: 1})

	waitFor(t, func() bool { _, ok := store.placedFor(21); return ok })
	waitFor(t, func() bool { _, ok := store.failedFor(20); return ok })

	cancel()
	pool.Shutdown()
}

func TestPool_OneFollowerFullChannelDoesNotBlockAnother(t *testing.T) {
	store := newFakeStore()
	// A broker that never returns until released, to keep the channel's
	// single in-flight slot occupied while we fill the buffer behind it.
	release := make(chan struct{})
	blocking := &blockingBroker{release: release}
	other := &fake.Broker{OrderID: "OTHER"}

	pool := worker.NewPool()
	defer pool.Shutdown()
	blockedFollower := uuid.New()
	otherFollower := uuid.New()
	pool.Register(context.Background(), blockedFollower, blocking, store, 1)
	pool.Register(context.Background(), otherFollower, other, store, 1)

	// Occupy the blocked follower's single worker goroutine.
	if !pool.Dispatch(blockedFollower, domain.Job{FollowerOrderID: 30, FollowerID: blockedFollower, Quantity: 1}) {
		t.Fatal("first dispatch to blockedFollower should succeed")
	}
	waitFor(t, func() bool { return blocking.started() })
	// Fill its buffer (capacity 1) so a further dispatch would be refused.
	if !pool.Dispatch(blockedFollower, domain.Job{FollowerOrderID: 31, FollowerID: blockedFollower, Quantity: 1}) {
		t.Fatal("second dispatch filling the buffer should still succeed")
	}
	if pool.Dispatch(blockedFollower, domain.Job{FollowerOrderID: 32, FollowerID: blockedFollower, Quantity: 1}) {
		t.Fatal("third dispatch should be refused: channel is full")
	}

	// A different follower's channel must still accept work immediately.
	if !pool.Dispatch(otherFollower, domain.Job{FollowerOrderID: 33, FollowerID: otherFollower, Quantity: 1}) {
		t.Fatal("dispatch to a different, idle follower should succeed")
	}
	waitFor(t, func() bool { _, ok := store.placedFor(33); return ok })

	close(release)
	waitFor(t, func() bool { _, ok := store.placedFor(30); return ok })
	waitFor(t, func() bool { _, ok := store.placedFor(31); return ok })
}

type panickyBroker struct{}

func (p *panickyBroker) PlaceOrder(ctx context.Context, variety string, params broker.OrderParams) (broker.OrderResponse, error) {
	panic("broker exploded")
}

type blockingBroker struct {
	mu       sync.Mutex
	release  chan struct{}
	hasStart bool
}

func (b *blockingBroker) started() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.hasStart
}

func (b *blockingBroker) PlaceOrder(ctx context.Context, variety string, params broker.OrderParams) (broker.OrderResponse, error) {
	b.mu.Lock()
	b.hasStart = true
	b.mu.Unlock()
	<-b.release
	return broker.OrderResponse{OrderID: "RELEASED"}, nil
}
