package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"envoytrade/internal/domain"
	"envoytrade/internal/httpapi"

	"github.com/google/uuid"
)

func TestPostAction_Rebalance_CallsEngineWithLatestMasterFill(t *testing.T) {
	master := uuid.New()
	follower := uuid.New()
	fill := domain.MasterFill{ID: 42, MasterID: master, BrokerOrderID: "1"}
	store := &stubStore{resolveMasterID: master, latestFill: fill}
	engine := &stubActionEngine{}
	r := httpapi.NewRouter(store, engine)

	body, _ := json.Marshal(map[string]string{"type": "rebalance"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/"+follower.String()+"/actions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", w.Code, w.Body.String())
	}
	if len(engine.calls) != 1 || engine.calls[0].ID != fill.ID {
		t.Fatalf("engine calls = %+v, want one call with fill %+v", engine.calls, fill)
	}

	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["type"] != "rebalance" || got["status"] != "accepted" {
		t.Errorf("body = %+v, want {type:rebalance status:accepted}", got)
	}
}

func TestPostAction_Rebalance_FromMasterRowRebalancesGroup(t *testing.T) {
	master := uuid.New()
	fill := domain.MasterFill{ID: 42, MasterID: master, BrokerOrderID: "1"}
	store := &stubStore{resolveMasterID: master, latestFill: fill}
	engine := &stubActionEngine{}
	r := httpapi.NewRouter(store, engine)

	body, _ := json.Marshal(map[string]string{"type": "rebalance"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/"+master.String()+"/actions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", w.Code, w.Body.String())
	}
	if len(engine.calls) != 1 || engine.calls[0].ID != fill.ID {
		t.Fatalf("engine calls = %+v, want one call with fill %+v", engine.calls, fill)
	}
}

func TestPostAction_Rebalance_NoFollowLinkReturns404(t *testing.T) {
	store := &stubStore{resolveMasterIDErr: domain.ErrNotFound}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]string{"type": "rebalance"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/"+uuid.New().String()+"/actions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestPostAction_Rebalance_NoMasterFillYetReturns404(t *testing.T) {
	store := &stubStore{resolveMasterID: uuid.New(), latestFillErr: domain.ErrNotFound}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]string{"type": "rebalance"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/"+uuid.New().String()+"/actions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestPostAction_SquareOff_Returns501(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]string{"type": "square_off"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/"+uuid.New().String()+"/actions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501; body=%s", w.Code, w.Body.String())
	}
}

func TestPostAction_ExitOpenOrders_Returns501(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]string{"type": "exit_open_orders"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/"+uuid.New().String()+"/actions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501; body=%s", w.Code, w.Body.String())
	}
}

func TestPostAction_UnknownType_Returns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]string{"type": "cancel_everything"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/"+uuid.New().String()+"/actions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestPostAction_InvalidUUID_Returns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]string{"type": "rebalance"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/not-a-uuid/actions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}
