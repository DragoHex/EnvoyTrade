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
	"github.com/shopspring/decimal"
)

func TestPatchAccount_EnabledFalse_TogglesFollowLink(t *testing.T) {
	follower := uuid.New()
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"enabled": false})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+follower.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(store.setEnabledArgs) != 1 {
		t.Fatalf("SetFollowLinkEnabled called %d times, want 1", len(store.setEnabledArgs))
	}
	call := store.setEnabledArgs[0]
	if call.FollowerID != follower || call.Enabled != false {
		t.Errorf("call = %+v, want {%v false}", call, follower)
	}
}

func TestPatchAccount_ActiveFalse_TogglesMasterActive(t *testing.T) {
	master := uuid.New()
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"active": false})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+master.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(store.setActiveArgs) != 1 {
		t.Fatalf("SetAccountActive called %d times, want 1", len(store.setActiveArgs))
	}
	call := store.setActiveArgs[0]
	if call.AccountID != master || call.Active != false {
		t.Errorf("call = %+v, want {%v false}", call, master)
	}
	if len(store.setEnabledArgs) != 0 {
		t.Errorf("SetFollowLinkEnabled should not be called, got %d calls", len(store.setEnabledArgs))
	}
}

func TestPatchAccount_UnknownAccount_Returns404(t *testing.T) {
	store := &stubStore{setEnabledErr: domain.ErrNotFound}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"enabled": true})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+uuid.New().String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestPatchAccount_InvalidUUID_Returns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"enabled": true})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/not-a-uuid", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestPatchAccount_NoRecognizedField_Returns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+uuid.New().String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if len(store.setEnabledArgs) != 0 {
		t.Errorf("SetFollowLinkEnabled should not be called, got %d calls", len(store.setEnabledArgs))
	}
}

func TestPatchAccount_CapitalRatio_UpdatesFollowLinkTerms(t *testing.T) {
	follower := uuid.New()
	maxQty := 10
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{follower: "follower"},
		accounts: []domain.Account{{
			ID: follower, Role: "follower",
			CapitalRatio: decimalPtr("0.5"), MaxQtyPerOrder: &maxQty,
		}},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"capitalRatio": "0.75"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+follower.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(store.updateFollowLinkTermsArgs) != 1 {
		t.Fatalf("UpdateFollowLinkTerms called %d times, want 1", len(store.updateFollowLinkTermsArgs))
	}
	call := store.updateFollowLinkTermsArgs[0]
	if call.FollowerID != follower || call.CapitalRatio.String() != "0.75" || *call.MaxQtyPerOrder != maxQty {
		t.Errorf("call = %+v, want follower=%v capitalRatio=0.75 maxQty=%d preserved", call, follower, maxQty)
	}
}

func TestPatchAccount_CapitalRatioOnMaster_Returns400(t *testing.T) {
	master := uuid.New()
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{master: "master"},
		accounts:     []domain.Account{{ID: master, Role: "master"}},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"capitalRatio": "0.75"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+master.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestPatchAccount_Status_UpdatesAccountStatus(t *testing.T) {
	id := uuid.New()
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"status": "error"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+id.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(store.setStatusArgs) != 1 || store.setStatusArgs[0] != (setStatusCall{id, "error"}) {
		t.Errorf("setStatusArgs = %+v, want [{%v error}]", store.setStatusArgs, id)
	}
}

func decimalPtr(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

func TestGetAccounts_ReturnsAllAsJSON(t *testing.T) {
	master := uuid.New()
	follower := uuid.New()
	maxQty := 5
	store := &stubStore{accounts: []domain.Account{
		{ID: master, Role: "master", Broker: "kite", BrokerAccountID: "M1", Active: true, Enabled: true, Status: "ok"},
		{ID: follower, Role: "follower", Broker: "kite", BrokerAccountID: "F1", Active: true, Status: "ok",
			MasterID: &master, CapitalRatio: decimalPtr("0.5"), MaxQtyPerOrder: &maxQty, Enabled: true},
	}}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d accounts, want 2", len(got))
	}
	if got[1]["masterId"] != master.String() || got[1]["capitalRatio"] != "0.5" {
		t.Errorf("follower entry = %+v", got[1])
	}
	if got[0]["masterId"] != nil || got[0]["capitalRatio"] != nil {
		t.Errorf("master entry should have nil group fields, got %+v", got[0])
	}
}

func TestGetAccounts_IDsFilter_PassesParsedUUIDs(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts?ids="+a.String()+","+b.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(store.accountsIDs) != 2 || store.accountsIDs[0] != a || store.accountsIDs[1] != b {
		t.Errorf("accountsIDs = %v, want [%v %v]", store.accountsIDs, a, b)
	}
}

func TestGetAccounts_InvalidIDInFilter_Returns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts?ids=not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestPostAccount_Master_Succeeds(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"role": "master", "broker": "kite", "brokerAccountId": "ZX1234", "apiSecret": "s",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(store.createAccountArgs) != 1 {
		t.Fatalf("CreateAccount called %d times, want 1", len(store.createAccountArgs))
	}
	call := store.createAccountArgs[0]
	if call.Role != "master" || call.Broker != "kite" || call.BrokerUserID != "ZX1234" {
		t.Errorf("call = %+v", call)
	}
	if len(store.createFollowLinkArgs) != 0 {
		t.Errorf("CreateFollowLink should not be called for a master")
	}
}

func TestPostAccount_FollowerWithMasterID_Succeeds(t *testing.T) {
	master := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{master: "master"}}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	maxQty := 25
	body, _ := json.Marshal(map[string]any{
		"role": "follower", "broker": "kite", "brokerAccountId": "ZY5678", "apiSecret": "s",
		"capitalRatio": "0.5", "maxQtyPerOrder": maxQty, "masterId": master.String(),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(store.createFollowLinkArgs) != 1 {
		t.Fatalf("CreateFollowLink called %d times, want 1", len(store.createFollowLinkArgs))
	}
	link := store.createFollowLinkArgs[0]
	if link.MasterID != master || link.MaxQtyPerOrder != maxQty || !link.CapitalRatio.Equal(decimal.RequireFromString("0.5")) {
		t.Errorf("link = %+v", link)
	}
}

func TestPostAccount_FollowerMissingMasterID_Returns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"role": "follower", "broker": "kite", "brokerAccountId": "ZY5678",
		"capitalRatio": "0.5", "maxQtyPerOrder": 10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestPostAccount_InvalidMasterID_Returns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"role": "follower", "broker": "kite", "brokerAccountId": "ZY5678",
		"capitalRatio": "0.5", "maxQtyPerOrder": 10, "masterId": uuid.New().String(),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestPostAccount_BadBroker_Returns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"role": "master", "broker": "zerodha", "brokerAccountId": "ZX1234"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestPostAccount_BadRole_Returns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"role": "admin", "broker": "kite", "brokerAccountId": "ZX1234"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestPostAccount_DuplicateBrokerAccountID_Returns409(t *testing.T) {
	store := &stubStore{createAccountErr: domain.ErrDuplicate}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"role": "master", "broker": "kite", "brokerAccountId": "ZX1234"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteAccount_Succeeds(t *testing.T) {
	id := uuid.New()
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", w.Code, w.Body.String())
	}
	if len(store.deleteAccountArgs) != 1 || store.deleteAccountArgs[0] != id {
		t.Errorf("deleteAccountArgs = %v, want [%v]", store.deleteAccountArgs, id)
	}
}

func TestDeleteAccount_Conflict_Returns409(t *testing.T) {
	store := &stubStore{deleteAccountErr: domain.ErrConflict}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteAccount_NotFound_Returns404(t *testing.T) {
	store := &stubStore{deleteAccountErr: domain.ErrNotFound}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteAccountGroup_Succeeds(t *testing.T) {
	id := uuid.New()
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/"+id.String()+"/group", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", w.Code, w.Body.String())
	}
	if len(store.deleteFollowLinkArgs) != 1 || store.deleteFollowLinkArgs[0] != id {
		t.Errorf("deleteFollowLinkArgs = %v, want [%v]", store.deleteFollowLinkArgs, id)
	}
}

func TestDeleteAccountGroup_NotFound_Returns404(t *testing.T) {
	store := &stubStore{deleteFollowLinkErr: domain.ErrNotFound}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/"+uuid.New().String()+"/group", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestPatchAccount_MalformedJSON_Returns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+uuid.New().String(), bytes.NewReader([]byte("{not json")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestPatchAccount_Name_UpdatesAccountName(t *testing.T) {
	id := uuid.New()
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"name": "New Name"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+id.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}
