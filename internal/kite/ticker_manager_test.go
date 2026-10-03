package kite_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"envoytrade/internal/domain"
	"envoytrade/internal/kite"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/goleak"
)

type fakeTickerStore struct {
	mu       sync.Mutex
	groups   []domain.GroupSummary
	authInfo map[uuid.UUID]domain.AccountAuthInfo
	active   map[uuid.UUID]bool
	err      error
}

func (s *fakeTickerStore) Groups(ctx context.Context) ([]domain.GroupSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	return s.groups, nil
}

func (s *fakeTickerStore) AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return domain.AccountAuthInfo{}, s.err
	}
	info, ok := s.authInfo[id]
	if !ok {
		return domain.AccountAuthInfo{}, domain.ErrNotFound
	}
	return info, nil
}

func (s *fakeTickerStore) MasterActive(ctx context.Context, masterID uuid.UUID) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return false, s.err
	}
	return s.active[masterID], nil
}

type fakeQueue struct {
	mu        sync.Mutex
	published []domain.MasterFill
}

func (q *fakeQueue) Publish(ctx context.Context, fill domain.MasterFill) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.published = append(q.published, fill)
	return nil
}

type fakeReconciler struct {
	mu         sync.Mutex
	reconciled []uuid.UUID
}

func (r *fakeReconciler) ReconcileMaster(ctx context.Context, masterID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reconciled = append(r.reconciled, masterID)
	return nil
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestTickerManager_Lifecycle(t *testing.T) {
	defer goleak.VerifyNone(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	master1 := uuid.New()
	master2 := uuid.New()

	store := &fakeTickerStore{
		authInfo: map[uuid.UUID]domain.AccountAuthInfo{
			master1: {BrokerAccountID: "M1", ApiKey: "k1", AccessToken: "tok1"},
			master2: {BrokerAccountID: "M2", ApiKey: "k2", AccessToken: "tok2"},
		},
		active: map[uuid.UUID]bool{
			master1: true,
			master2: false, // inactive master
		},
		groups: []domain.GroupSummary{
			{ID: uuid.New(), Name: "G1", MasterID: master1},
			{ID: uuid.New(), Name: "G2", MasterID: master2},
		},
	}

	queue := &fakeQueue{}
	reconciler := &fakeReconciler{}

	mgr := kite.NewTickerManager(store, queue, reconciler, nil)
	mgr.SetWSRootURL(wsURL)

	ctx := context.Background()

	// 1. Start active master1
	if err := mgr.StartMaster(ctx, master1); err != nil {
		t.Fatalf("StartMaster(master1): %v", err)
	}

	// Idempotent start: calling again should not start a duplicate or error
	if err := mgr.StartMaster(ctx, master1); err != nil {
		t.Fatalf("StartMaster duplicate: %v", err)
	}

	if count := mgr.ActiveCount(); count != 1 {
		t.Fatalf("ActiveCount = %d, want 1", count)
	}

	// 2. Start inactive master2: should be skipped
	if err := mgr.StartMaster(ctx, master2); err != nil {
		t.Fatalf("StartMaster(master2): %v", err)
	}
	if count := mgr.ActiveCount(); count != 1 {
		t.Fatalf("ActiveCount after inactive master = %d, want 1", count)
	}

	// 3. Restart master1 with refreshed token
	store.mu.Lock()
	store.authInfo[master1] = domain.AccountAuthInfo{BrokerAccountID: "M1", ApiKey: "k1", AccessToken: "tok1_new"}
	store.mu.Unlock()

	if err := mgr.RestartMaster(ctx, master1); err != nil {
		t.Fatalf("RestartMaster(master1): %v", err)
	}
	if count := mgr.ActiveCount(); count != 1 {
		t.Fatalf("ActiveCount after restart = %d, want 1", count)
	}

	// 4. Stop master1
	if err := mgr.StopMaster(master1); err != nil {
		t.Fatalf("StopMaster(master1): %v", err)
	}
	if count := mgr.ActiveCount(); count != 0 {
		t.Fatalf("ActiveCount after stop = %d, want 0", count)
	}

	// 5. SyncActiveMasters: should start master1 only
	if err := mgr.SyncActiveMasters(ctx); err != nil {
		t.Fatalf("SyncActiveMasters: %v", err)
	}
	if count := mgr.ActiveCount(); count != 1 {
		t.Fatalf("ActiveCount after SyncActiveMasters = %d, want 1", count)
	}

	// 6. Shutdown
	mgr.Shutdown()
	if count := mgr.ActiveCount(); count != 0 {
		t.Fatalf("ActiveCount after Shutdown = %d, want 0", count)
	}
}
