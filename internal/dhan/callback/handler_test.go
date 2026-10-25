package callback_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"envoytrade/internal/dhan"
	dhancallback "envoytrade/internal/dhan/callback"
	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

type fakeAccounts struct {
	id   uuid.UUID
	role string
	err  error
}

func (f fakeAccounts) AccountByBrokerAndUserID(ctx context.Context, brokerName, brokerUserID string) (uuid.UUID, string, string, error) {
	if f.err != nil {
		return uuid.UUID{}, "", "", f.err
	}
	return f.id, f.role, "", nil
}

func (f fakeAccounts) AccountByBrokerUserID(ctx context.Context, brokerUserID string) (uuid.UUID, string, string, error) {
	return f.AccountByBrokerAndUserID(ctx, "dhan", brokerUserID)
}

type fakePublisher[T any] struct {
	published []T
	err       error
}

func (f *fakePublisher[T]) Publish(ctx context.Context, ev T) error {
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, ev)
	return nil
}

func setupTestResolver(t *testing.T) *dhan.InstrumentResolver {
	t.Helper()
	resolver := dhan.NewInstrumentResolver(nil)
	csvData := `SEM_EXM_EXCH_ID,SEM_SEGMENT,SEM_SMST_SECURITY_ID,SEM_TRADING_SYMBOL,SEM_LOT_UNITS
NSE,E,11536,TATASTEEL,1
`
	if err := resolver.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("setupTestResolver failed: %v", err)
	}
	return resolver
}

func TestHandler_MasterFill_Positive(t *testing.T) {
	masterID := uuid.New()
	accounts := fakeAccounts{id: masterID, role: "master"}
	masterQueue := &fakePublisher[domain.MasterFill]{}
	followerQueue := &fakePublisher[domain.OrderUpdate]{}
	resolver := setupTestResolver(t)

	handler := &dhancallback.Handler{
		Accounts:        accounts,
		MasterFills:     masterQueue,
		FollowerUpdates: followerQueue,
		Resolver:        resolver,
	}

	payload := map[string]any{
		"dhanClientId":        "1000000001",
		"orderId":             "dhan-ord-999",
		"correlationId":       "tag-m1",
		"orderStatus":         "TRADED",
		"transactionType":     "BUY",
		"exchangeSegment":     "NSE_EQ",
		"productType":         "CNC",
		"orderType":           "LIMIT",
		"securityId":          "11536",
		"quantity":            100,
		"filledQty":           100,
		"price":               120.50,
		"averageTradedPrice":  120.50,
		"createTime":          "2026-10-24 15:00:00",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/broker-callback/dhan", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", rec.Code)
	}
	if len(masterQueue.published) != 1 {
		t.Fatalf("got %d master fills, want 1", len(masterQueue.published))
	}
	fill := masterQueue.published[0]
	if fill.MasterID != masterID || fill.BrokerOrderID != "dhan-ord-999" || fill.FilledQuantity != 100 {
		t.Errorf("master fill mismatch: %+v", fill)
	}
	if fill.Tradingsymbol != "TATASTEEL" || fill.Exchange != "NSE" {
		t.Errorf("got symbol=%s, ex=%s; want TATASTEEL, NSE", fill.Tradingsymbol, fill.Exchange)
	}
	if len(followerQueue.published) != 0 {
		t.Errorf("expected 0 follower updates, got %d", len(followerQueue.published))
	}
}

func TestHandler_FollowerUpdate_Positive(t *testing.T) {
	followerID := uuid.New()
	accounts := fakeAccounts{id: followerID, role: "follower"}
	masterQueue := &fakePublisher[domain.MasterFill]{}
	followerQueue := &fakePublisher[domain.OrderUpdate]{}
	resolver := setupTestResolver(t)

	handler := &dhancallback.Handler{
		Accounts:        accounts,
		MasterFills:     masterQueue,
		FollowerUpdates: followerQueue,
		Resolver:        resolver,
	}

	payload := map[string]any{
		"dhanClientId":        "1000000002",
		"orderId":             "dhan-fol-888",
		"correlationId":       "tag-f1",
		"orderStatus":         "TRADED",
		"transactionType":     "BUY",
		"exchangeSegment":     "NSE_EQ",
		"productType":         "CNC",
		"orderType":           "LIMIT",
		"securityId":          "11536",
		"quantity":            100,
		"filledQty":           100,
		"price":               120.50,
		"averageTradedPrice":  120.50,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/broker-callback/dhan", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", rec.Code)
	}
	if len(followerQueue.published) != 1 {
		t.Fatalf("got %d follower updates, want 1", len(followerQueue.published))
	}
	update := followerQueue.published[0]
	if update.FollowerID != followerID || update.BrokerOrderID != "dhan-fol-888" || update.FilledQuantity != 100 {
		t.Errorf("follower update mismatch: %+v", update)
	}
	if update.Status != domain.TerminalComplete {
		t.Errorf("got status=%s, want %s", update.Status, domain.TerminalComplete)
	}
}

func TestHandler_NegativeAndEdgeCases(t *testing.T) {
	accounts := fakeAccounts{id: uuid.New(), role: "master"}
	masterQueue := &fakePublisher[domain.MasterFill]{}
	followerQueue := &fakePublisher[domain.OrderUpdate]{}

	handler := &dhancallback.Handler{
		Accounts:        accounts,
		MasterFills:     masterQueue,
		FollowerUpdates: followerQueue,
	}

	// 1. Non-JSON body returns 400
	req := httptest.NewRequest(http.MethodPost, "/broker-callback/dhan", strings.NewReader("not-json"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400 for malformed json", rec.Code)
	}

	// 2. Unknown account returns 200 and drops
	accountsErr := fakeAccounts{err: domain.ErrNotFound}
	handlerUnknown := &dhancallback.Handler{
		Accounts:        accountsErr,
		MasterFills:     masterQueue,
		FollowerUpdates: followerQueue,
	}
	body, _ := json.Marshal(map[string]any{"dhanClientId": "UNKNOWN", "orderStatus": "TRADED"})
	req = httptest.NewRequest(http.MethodPost, "/broker-callback/dhan", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	handlerUnknown.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want 200 for unknown account", rec.Code)
	}
	if len(masterQueue.published) != 0 {
		t.Errorf("expected 0 master fills for unknown account, got %d", len(masterQueue.published))
	}

	// 3. Non-terminal status ("PENDING" / "TRANSIT") ignored
	body, _ = json.Marshal(map[string]any{"dhanClientId": "1000000001", "orderStatus": "PENDING"})
	req = httptest.NewRequest(http.MethodPost, "/broker-callback/dhan", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", rec.Code)
	}
	if len(masterQueue.published) != 0 {
		t.Errorf("expected non-terminal status to be dropped, got %d", len(masterQueue.published))
	}
}
