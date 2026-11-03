//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"envoytrade/internal/auth"
	"envoytrade/internal/domain"
	"envoytrade/internal/httpapi"
	"envoytrade/internal/store/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

type testRegistrar struct {
	registered   []uuid.UUID
	unregistered []uuid.UUID
}

func (r *testRegistrar) RegisterFollower(_ context.Context, id uuid.UUID) error {
	r.registered = append(r.registered, id)
	return nil
}

func (r *testRegistrar) UnregisterFollower(id uuid.UUID) {
	r.unregistered = append(r.unregistered, id)
}

func setupTestStore(t *testing.T) (*postgres.Store, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("envoytrade_test"),
		tcpostgres.WithUsername("envoytrade"),
		tcpostgres.WithPassword("envoytrade"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("terminate postgres container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	pool, err := postgres.NewPool(ctx, connStr)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)

	store := postgres.New(pool)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return store, pool
}

func seedAccount(t *testing.T, s *postgres.Store, role string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	ip := fmt.Sprintf("10.0.%d.%d", id[0], id[1])
	if err := s.CreateAccount(context.Background(), id, "Account "+id.String()[:8], role, "zerodha", id.String(), "key", "secret", ip); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if role == "master" {
		if err := s.CreateGroup(context.Background(), id, "Group "+id.String()[:8], id); err != nil {
			t.Fatalf("CreateGroup: %v", err)
		}
	}
	return id
}

type fakeActionEngine struct{}

func (f *fakeActionEngine) HandleMasterFill(_ context.Context, _ domain.MasterFill) error {
	return nil
}

func (f *fakeActionEngine) SetKillSwitch(_ bool, _ string, _ string) {}

func (f *fakeActionEngine) KillSwitchStatus() domain.KillSwitchStatus {
	return domain.KillSwitchStatus{}
}

func TestHTTP_PostGroupFollowers_Integration(t *testing.T) {
	store, pool := setupTestStore(t)
	ctx := context.Background()

	// Seed tenant user and session
	userID := uuid.New()
	pwHash, err := auth.HashPassword("TestPass123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateUser(ctx, userID, "trader@envoytrade.com", "trader", "Trader", pwHash, "admin"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionToken, tokenHash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken: %v", err)
	}
	if err := store.CreateSession(ctx, tokenHash, userID, "127.0.0.1", "test-agent", time.Now().Add(24*time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	reg := &testRegistrar{}
	router := httpapi.NewRouter(store, &fakeActionEngine{}, httpapi.WithFollowerRegistrar(reg))

	newAuthRequest := func(method, target string, body []byte) *http.Request {
		var reader *bytes.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		} else {
			reader = bytes.NewReader([]byte{})
		}
		req := httptest.NewRequest(method, target, reader)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: sessionToken})
		return req
	}

	t.Run("batch array with numeric and string clone factors", func(t *testing.T) {
		masterID := seedAccount(t, store, "master")
		f1 := seedAccount(t, store, "follower")
		f2 := seedAccount(t, store, "follower")

		// Payload mixes numeric cloneFactor/maxQty and string cloneFactor
		body := []map[string]any{
			{"accountId": f1.String(), "cloneFactor": 1.5, "maxQtyPerOrder": 50},
			{"accountId": f2.String(), "cloneFactor": "0.75", "maxQtyPerOrder": 0},
		}
		bodyBytes, _ := json.Marshal(body)

		req := newAuthRequest(http.MethodPost, "/api/v1/groups/"+masterID.String()+"/followers", bodyBytes)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("got status %d, want 201; body: %s", w.Code, w.Body.String())
		}

		// Assert persisted state directly in PostgreSQL
		var cf1 decimal.Decimal
		var qty1 *int
		var enabled1 bool
		err := pool.QueryRow(ctx, "SELECT clone_factor, max_qty_per_order, enabled FROM follow_links WHERE follower_id = $1", f1).Scan(&cf1, &qty1, &enabled1)
		if err != nil {
			t.Fatalf("query f1 follow_link: %v", err)
		}
		if !cf1.Equal(decimal.RequireFromString("1.5")) || qty1 == nil || *qty1 != 50 || !enabled1 {
			t.Errorf("f1 DB mismatch: cf=%v, qty=%v, enabled=%v", cf1, qty1, enabled1)
		}

		var cf2 decimal.Decimal
		var qty2 *int
		var enabled2 bool
		err = pool.QueryRow(ctx, "SELECT clone_factor, max_qty_per_order, enabled FROM follow_links WHERE follower_id = $1", f2).Scan(&cf2, &qty2, &enabled2)
		if err != nil {
			t.Fatalf("query f2 follow_link: %v", err)
		}
		if !cf2.Equal(decimal.RequireFromString("0.75")) || (qty2 != nil && *qty2 != 0) || !enabled2 {
			t.Errorf("f2 DB mismatch: cf=%v, qty=%v, enabled=%v", cf2, qty2, enabled2)
		}

		// Verify registrar invocation
		foundF1, foundF2 := false, false
		for _, id := range reg.registered {
			if id == f1 {
				foundF1 = true
			}
			if id == f2 {
				foundF2 = true
			}
		}
		if !foundF1 || !foundF2 {
			t.Errorf("registered missing f1 (%v) or f2 (%v): %+v", foundF1, foundF2, reg.registered)
		}
	})

	t.Run("batch object wrapper with numeric clone factor", func(t *testing.T) {
		masterID := seedAccount(t, store, "master")
		f1 := seedAccount(t, store, "follower")

		body := map[string]any{
			"followers": []map[string]any{
				{"accountId": f1.String(), "cloneFactor": 2.25, "maxQtyPerOrder": 100},
			},
		}
		bodyBytes, _ := json.Marshal(body)

		req := newAuthRequest(http.MethodPost, "/api/v1/groups/"+masterID.String()+"/followers", bodyBytes)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("got status %d, want 201; body: %s", w.Code, w.Body.String())
		}

		var cf decimal.Decimal
		var qty int
		err := pool.QueryRow(ctx, "SELECT clone_factor, max_qty_per_order FROM follow_links WHERE follower_id = $1", f1).Scan(&cf, &qty)
		if err != nil {
			t.Fatalf("query f1 follow_link: %v", err)
		}
		if !cf.Equal(decimal.RequireFromString("2.25")) || qty != 100 {
			t.Errorf("f1 DB mismatch: cf=%v, qty=%d", cf, qty)
		}
	})

	t.Run("legacy single object with numeric clone factor", func(t *testing.T) {
		masterID := seedAccount(t, store, "master")
		f1 := seedAccount(t, store, "follower")

		body := map[string]any{
			"accountId":      f1.String(),
			"cloneFactor":    1,
			"maxQtyPerOrder": 20,
		}
		bodyBytes, _ := json.Marshal(body)

		req := newAuthRequest(http.MethodPost, "/api/v1/groups/"+masterID.String()+"/followers", bodyBytes)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("got status %d, want 201; body: %s", w.Code, w.Body.String())
		}

		var cf decimal.Decimal
		var qty int
		err := pool.QueryRow(ctx, "SELECT clone_factor, max_qty_per_order FROM follow_links WHERE follower_id = $1", f1).Scan(&cf, &qty)
		if err != nil {
			t.Fatalf("query f1 follow_link: %v", err)
		}
		if !cf.Equal(decimal.RequireFromString("1")) || qty != 20 {
			t.Errorf("f1 DB mismatch: cf=%v, qty=%d", cf, qty)
		}
	})

	t.Run("duplicate account in batch fails and rolls back all", func(t *testing.T) {
		masterID := seedAccount(t, store, "master")
		f1 := seedAccount(t, store, "follower")

		body := []map[string]any{
			{"accountId": f1.String(), "cloneFactor": 1.0},
			{"accountId": f1.String(), "cloneFactor": 1.0},
		}
		bodyBytes, _ := json.Marshal(body)

		req := newAuthRequest(http.MethodPost, "/api/v1/groups/"+masterID.String()+"/followers", bodyBytes)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("got status %d, want 400; body: %s", w.Code, w.Body.String())
		}

		// Verify 0 rows inserted in Postgres
		var count int
		err := pool.QueryRow(ctx, "SELECT count(*) FROM follow_links WHERE follower_id = $1", f1).Scan(&count)
		if err != nil {
			t.Fatalf("query follow_links count: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 links in DB after rollback, got %d", count)
		}
	})

	t.Run("already attached account returns 409 and rolls back all in batch", func(t *testing.T) {
		masterID := seedAccount(t, store, "master")
		f1 := seedAccount(t, store, "follower")
		f2 := seedAccount(t, store, "follower")

		// First attach f1 successfully
		singleBody, _ := json.Marshal(map[string]any{"accountId": f1.String(), "cloneFactor": 1.0})
		req1 := newAuthRequest(http.MethodPost, "/api/v1/groups/"+masterID.String()+"/followers", singleBody)
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, req1)
		if w1.Code != http.StatusCreated {
			t.Fatalf("setup f1 attach failed: %d", w1.Code)
		}

		// Now attempt batch containing f2 (new) and f1 (already attached)
		batchBody, _ := json.Marshal([]map[string]any{
			{"accountId": f2.String(), "cloneFactor": 1.5},
			{"accountId": f1.String(), "cloneFactor": 1.0},
		})
		req2 := newAuthRequest(http.MethodPost, "/api/v1/groups/"+masterID.String()+"/followers", batchBody)
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)

		if w2.Code != http.StatusConflict {
			t.Fatalf("got status %d, want 409; body: %s", w2.Code, w2.Body.String())
		}

		// Verify f2 was NOT inserted (atomic rollback)
		var f2Count int
		err := pool.QueryRow(ctx, "SELECT count(*) FROM follow_links WHERE follower_id = $1", f2).Scan(&f2Count)
		if err != nil {
			t.Fatalf("query f2 count: %v", err)
		}
		if f2Count != 0 {
			t.Errorf("f2 should have rolled back, got %d rows", f2Count)
		}
	})

	t.Run("invalid clone factor zero or negative returns 400", func(t *testing.T) {
		masterID := seedAccount(t, store, "master")
		f1 := seedAccount(t, store, "follower")

		for _, cf := range []any{0, -1, "-2.5", "invalid"} {
			body, _ := json.Marshal([]map[string]any{
				{"accountId": f1.String(), "cloneFactor": cf},
			})
			req := newAuthRequest(http.MethodPost, "/api/v1/groups/"+masterID.String()+"/followers", body)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("cloneFactor %v: got status %d, want 400", cf, w.Code)
			}
		}
	})

	t.Run("non-follower role returns 400", func(t *testing.T) {
		masterID := seedAccount(t, store, "master")
		otherMaster := seedAccount(t, store, "master")

		body, _ := json.Marshal([]map[string]any{
			{"accountId": otherMaster.String(), "cloneFactor": 1.0},
		})
		req := newAuthRequest(http.MethodPost, "/api/v1/groups/"+masterID.String()+"/followers", body)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("got status %d, want 400; body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("empty body returns 400", func(t *testing.T) {
		masterID := seedAccount(t, store, "master")
		req := newAuthRequest(http.MethodPost, "/api/v1/groups/"+masterID.String()+"/followers", []byte("   "))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("got status %d, want 400; body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("corrupted json syntax returns 400", func(t *testing.T) {
		masterID := seedAccount(t, store, "master")
		req := newAuthRequest(http.MethodPost, "/api/v1/groups/"+masterID.String()+"/followers", []byte("[{invalid json"))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("got status %d, want 400; body: %s", w.Code, w.Body.String())
		}
	})
}
