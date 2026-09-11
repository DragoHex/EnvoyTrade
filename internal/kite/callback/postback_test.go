package callback_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"envoytrade/internal/domain"
	"envoytrade/internal/kite/callback"

	"github.com/google/uuid"
)

type fakeAccounts struct {
	id        uuid.UUID
	role      string
	apiSecret string
	err       error
}

func (f fakeAccounts) AccountByBrokerUserID(ctx context.Context, brokerUserID string) (uuid.UUID, string, string, error) {
	if f.err != nil {
		return uuid.UUID{}, "", "", f.err
	}
	return f.id, f.role, f.apiSecret, nil
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

func checksumFor(orderID, ts, secret string) string {
	sum := sha256.Sum256([]byte(orderID + ts + secret))
	return hex.EncodeToString(sum[:])
}

func postbackBody(t *testing.T, orderID, status, userID, apiSecret string) []byte {
	t.Helper()
	const ts = "2026-09-05 10:30:00"
	body := map[string]any{
		"order_id":         orderID,
		"order_timestamp":  ts,
		"status":           status,
		"user_id":          userID,
		"checksum":         checksumFor(orderID, ts, apiSecret),
		"exchange":         "NSE",
		"tradingsymbol":    "INFY",
		"instrument_token": 408065,
		"transaction_type": "BUY",
		"product":          "MIS",
		"order_type":       "MARKET",
		"filled_quantity":  100,
		"average_price":    1500.25,
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return b
}

func TestHandler_ValidMasterPostback_PublishesMasterFill(t *testing.T) {
	masterID := uuid.New()
	masters := &fakePublisher[domain.MasterFill]{}
	followers := &fakePublisher[domain.OrderUpdate]{}
	h := &callback.Handler{
		Accounts:        fakeAccounts{id: masterID, role: callback.RoleMaster, apiSecret: "secret"},
		MasterFills:     masters,
		FollowerUpdates: followers,
	}

	body := postbackBody(t, "1", domain.TerminalComplete, "MASTER1", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/postback", bytes.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(masters.published) != 1 {
		t.Fatalf("published %d master fills, want 1", len(masters.published))
	}
	if masters.published[0].MasterID != masterID {
		t.Errorf("MasterID = %v, want %v", masters.published[0].MasterID, masterID)
	}
	if len(followers.published) != 0 {
		t.Errorf("published %d follower updates, want 0", len(followers.published))
	}
}

func TestHandler_ValidFollowerPostback_PublishesOrderUpdate(t *testing.T) {
	masters := &fakePublisher[domain.MasterFill]{}
	followers := &fakePublisher[domain.OrderUpdate]{}
	h := &callback.Handler{
		Accounts:        fakeAccounts{id: uuid.New(), role: callback.RoleFollower, apiSecret: "secret"},
		MasterFills:     masters,
		FollowerUpdates: followers,
	}

	body := postbackBody(t, "2", domain.TerminalComplete, "FOLLOWER1", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/postback", bytes.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(followers.published) != 1 {
		t.Fatalf("published %d follower updates, want 1", len(followers.published))
	}
	if followers.published[0].BrokerOrderID != "2" {
		t.Errorf("BrokerOrderID = %q, want %q", followers.published[0].BrokerOrderID, "2")
	}
	if len(masters.published) != 0 {
		t.Errorf("published %d master fills, want 0", len(masters.published))
	}
}

func TestHandler_BadChecksum_DropsAndAcks(t *testing.T) {
	masters := &fakePublisher[domain.MasterFill]{}
	h := &callback.Handler{
		Accounts:        fakeAccounts{id: uuid.New(), role: callback.RoleMaster, apiSecret: "secret"},
		MasterFills:     masters,
		FollowerUpdates: &fakePublisher[domain.OrderUpdate]{},
	}

	body := postbackBody(t, "3", domain.TerminalComplete, "MASTER1", "wrong-secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/postback", bytes.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (ack, drop)", rec.Code)
	}
	if len(masters.published) != 0 {
		t.Fatalf("published %d master fills, want 0", len(masters.published))
	}
}

func TestHandler_UnknownAccount_DropsAndAcks(t *testing.T) {
	masters := &fakePublisher[domain.MasterFill]{}
	h := &callback.Handler{
		Accounts:        fakeAccounts{err: domain.ErrNotFound},
		MasterFills:     masters,
		FollowerUpdates: &fakePublisher[domain.OrderUpdate]{},
	}

	body := postbackBody(t, "4", domain.TerminalComplete, "GHOST", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/postback", bytes.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (ack, drop)", rec.Code)
	}
	if len(masters.published) != 0 {
		t.Fatalf("published %d master fills, want 0", len(masters.published))
	}
}

func TestHandler_NonTerminalStatus_DropsAndAcks(t *testing.T) {
	masters := &fakePublisher[domain.MasterFill]{}
	h := &callback.Handler{
		Accounts:        fakeAccounts{id: uuid.New(), role: callback.RoleMaster, apiSecret: "secret"},
		MasterFills:     masters,
		FollowerUpdates: &fakePublisher[domain.OrderUpdate]{},
	}

	body := postbackBody(t, "5", "OPEN", "MASTER1", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/postback", bytes.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(masters.published) != 0 {
		t.Fatalf("published %d master fills, want 0", len(masters.published))
	}
}

func TestHandler_MalformedBody_Returns400(t *testing.T) {
	h := &callback.Handler{
		Accounts:        fakeAccounts{},
		MasterFills:     &fakePublisher[domain.MasterFill]{},
		FollowerUpdates: &fakePublisher[domain.OrderUpdate]{},
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/postback", bytes.NewReader([]byte("not json"))))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandler_QueueFull_StillAcks(t *testing.T) {
	masters := &fakePublisher[domain.MasterFill]{err: errFull}
	h := &callback.Handler{
		Accounts:        fakeAccounts{id: uuid.New(), role: callback.RoleMaster, apiSecret: "secret"},
		MasterFills:     masters,
		FollowerUpdates: &fakePublisher[domain.OrderUpdate]{},
	}

	body := postbackBody(t, "6", domain.TerminalComplete, "MASTER1", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/postback", bytes.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (acked even though publish failed)", rec.Code)
	}
	if len(masters.published) != 0 {
		t.Fatalf("published %d master fills, want 0", len(masters.published))
	}
}

var errFull = &queueFullError{}

type queueFullError struct{}

func (*queueFullError) Error() string { return "queue full" }
