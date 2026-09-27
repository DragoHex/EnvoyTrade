package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"envoytrade/internal/auth"
	"envoytrade/internal/domain"
	"envoytrade/internal/httpapi"

	"github.com/google/uuid"
)

// scopingStubStore supports user-scoped account and group lists
type scopingStubStore struct {
	*fullStubStore
	accountsByUser map[uuid.UUID][]domain.Account
	groupsByUser   map[uuid.UUID][]domain.GroupSummary
}

func newScopingStubStore() *scopingStubStore {
	return &scopingStubStore{
		fullStubStore:  newFullStubStore(),
		accountsByUser: make(map[uuid.UUID][]domain.Account),
		groupsByUser:   make(map[uuid.UUID][]domain.GroupSummary),
	}
}

func (s *scopingStubStore) Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error) {
	user, ok := domain.UserFromContext(ctx)
	if !ok {
		return s.fullStubStore.stubStore.Accounts(ctx, ids)
	}
	return s.accountsByUser[user.ID], nil
}

func (s *scopingStubStore) Groups(ctx context.Context) ([]domain.GroupSummary, error) {
	user, ok := domain.UserFromContext(ctx)
	if !ok {
		return s.fullStubStore.stubStore.Groups(ctx)
	}
	return s.groupsByUser[user.ID], nil
}

func (s *scopingStubStore) GroupDetail(ctx context.Context, id uuid.UUID) (domain.GroupDetail, error) {
	user, ok := domain.UserFromContext(ctx)
	if ok {
		// Verify group belongs to user
		var found bool
		for _, g := range s.groupsByUser[user.ID] {
			if g.ID == id || g.MasterID == id {
				found = true
				break
			}
		}
		if !found {
			return domain.GroupDetail{}, domain.ErrNotFound
		}
	}
	return s.fullStubStore.stubStore.GroupDetail(ctx, id)
}

func TestUserDataScoping(t *testing.T) {
	store := newScopingStubStore()
	handler := httpapi.NewRouter(store, &stubActionEngine{})

	// 1. Create User Alice
	aliceID := uuid.New()
	hashA, _ := auth.HashPassword("PassAlice123")
	_ = store.CreateUser(context.Background(), aliceID, "alice@example.com", "alice", "Alice", hashA, "user")
	tokenAlice, tokenHashA, _ := auth.GenerateSessionToken()
	_ = store.CreateSession(context.Background(), tokenHashA, aliceID, "127.0.0.1", "agent", time.Now().Add(24*time.Hour))

	// 2. Create User Bob
	bobID := uuid.New()
	hashB, _ := auth.HashPassword("PassBob123")
	_ = store.CreateUser(context.Background(), bobID, "bob@example.com", "bob", "Bob", hashB, "user")
	tokenBob, tokenHashB, _ := auth.GenerateSessionToken()
	_ = store.CreateSession(context.Background(), tokenHashB, bobID, "127.0.0.1", "agent", time.Now().Add(24*time.Hour))

	// Seed Alice's data
	aliceGroupID := uuid.New()
	aliceMasterID := uuid.New()
	store.groupsByUser[aliceID] = []domain.GroupSummary{
		{
			ID:              aliceGroupID,
			Name:            "Alice Group",
			MasterID:        aliceMasterID,
			MasterAccountID: "ALICE1",
		},
	}
	store.accountsByUser[aliceID] = []domain.Account{
		{
			ID:              aliceMasterID,
			Name:            "Alice Master",
			Role:            "master",
			BrokerAccountID: "ALICE1",
		},
	}

	t.Run("Alice sees her own accounts and groups", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: tokenAlice})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var accounts []domain.Account
		_ = json.Unmarshal(rec.Body.Bytes(), &accounts)
		if len(accounts) != 1 || accounts[0].Name != "Alice Master" {
			t.Fatalf("expected Alice to see her account, got %+v", accounts)
		}
	})

	t.Run("Bob does NOT see Alice's accounts", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: tokenBob})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var accounts []domain.Account
		_ = json.Unmarshal(rec.Body.Bytes(), &accounts)
		if len(accounts) != 0 {
			t.Fatalf("expected Bob to see 0 accounts, got %d", len(accounts))
		}
	})

	t.Run("Bob does NOT see Alice's groups", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: tokenBob})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var groups []domain.GroupSummary
		_ = json.Unmarshal(rec.Body.Bytes(), &groups)
		if len(groups) != 0 {
			t.Fatalf("expected Bob to see 0 groups, got %d", len(groups))
		}
	})

	t.Run("Bob attempting to get Alice's group detail returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+aliceGroupID.String(), nil)
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: tokenBob})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 when Bob requests Alice's group, got %d", rec.Code)
		}
	})
}
