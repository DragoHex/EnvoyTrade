package httpapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"envoytrade/internal/crypto"
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

func TestPatchAccount_Follower_ActiveFalse_DisablesCopyLinkAndReturnsBoth(t *testing.T) {
	follower := uuid.New()
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{follower: "follower"},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"active": false})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+follower.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(store.setActiveArgs) != 1 || store.setActiveArgs[0].Active != false {
		t.Fatalf("SetAccountActive not called with false: %+v", store.setActiveArgs)
	}
	if len(store.setEnabledArgs) != 1 || store.setEnabledArgs[0].Enabled != false {
		t.Fatalf("SetFollowLinkEnabled not called with false: %+v", store.setEnabledArgs)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["active"] != false {
		t.Errorf("resp[active] = %v, want false", resp["active"])
	}
	if resp["enabled"] != false {
		t.Errorf("resp[enabled] = %v, want false", resp["enabled"])
	}
}

func TestPatchAccount_Follower_ActiveTrue_KeepsEnabledUntouched(t *testing.T) {
	follower := uuid.New()
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{follower: "follower"},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"active": true})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+follower.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(store.setActiveArgs) != 1 || store.setActiveArgs[0].Active != true {
		t.Fatalf("SetAccountActive not called with true: %+v", store.setActiveArgs)
	}
	if len(store.setEnabledArgs) != 0 {
		t.Fatalf("SetFollowLinkEnabled should NOT be called when activating, got: %+v", store.setEnabledArgs)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["active"] != true {
		t.Errorf("resp[active] = %v, want true", resp["active"])
	}
	if _, ok := resp["enabled"]; ok {
		t.Errorf("resp[enabled] should not be present in response when only active is patched, got %v", resp["enabled"])
	}
}

func TestPatchAccount_Follower_EnabledTrue_ActivatesAccountAndReturnsBoth(t *testing.T) {
	follower := uuid.New()
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{follower: "follower"},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"enabled": true})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+follower.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(store.setEnabledArgs) != 1 || store.setEnabledArgs[0].Enabled != true {
		t.Fatalf("SetFollowLinkEnabled not called with true: %+v", store.setEnabledArgs)
	}
	if len(store.setActiveArgs) != 1 || store.setActiveArgs[0].Active != true {
		t.Fatalf("SetAccountActive not called with true when enabling copy: %+v", store.setActiveArgs)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["enabled"] != true {
		t.Errorf("resp[enabled] = %v, want true", resp["enabled"])
	}
	if resp["active"] != true {
		t.Errorf("resp[active] = %v, want true", resp["active"])
	}
}

func TestPatchAccount_Follower_EnabledFalse_LeavesAccountActiveUntouched(t *testing.T) {
	follower := uuid.New()
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{follower: "follower"},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"enabled": false})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+follower.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(store.setEnabledArgs) != 1 || store.setEnabledArgs[0].Enabled != false {
		t.Fatalf("SetFollowLinkEnabled not called with false: %+v", store.setEnabledArgs)
	}
	if len(store.setActiveArgs) != 0 {
		t.Fatalf("SetAccountActive should NOT be called when disabling copy, got: %+v", store.setActiveArgs)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["enabled"] != false {
		t.Errorf("resp[enabled] = %v, want false", resp["enabled"])
	}
	if _, ok := resp["active"]; ok {
		t.Errorf("resp[active] should not be present in response when only enabled is patched to false, got %v", resp["active"])
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

func TestPatchAccount_CloneFactor_UpdatesFollowLinkTerms(t *testing.T) {
	follower := uuid.New()
	maxQty := 10
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{follower: "follower"},
		accounts: []domain.Account{{
			ID: follower, Role: "follower",
			CloneFactor: decimalPtr("0.5"), MaxQtyPerOrder: &maxQty,
		}},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"cloneFactor": "0.75"})
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
	if call.FollowerID != follower || call.CloneFactor.String() != "0.75" || *call.MaxQtyPerOrder != maxQty {
		t.Errorf("call = %+v, want follower=%v cloneFactor=0.75 maxQty=%d preserved", call, follower, maxQty)
	}
}

func TestPatchAccount_AssignGroupID_Succeeds(t *testing.T) {
	follower := uuid.New()
	groupID := uuid.New()
	masterID := uuid.New()
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{follower: "follower"},
		accounts:     []domain.Account{{ID: follower, Role: "follower"}},
		groupDetails: map[uuid.UUID]domain.GroupDetail{
			groupID: {
				GroupID:   groupID,
				GroupName: "Target Group",
				MasterID:  masterID,
			},
		},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"groupId":     groupID.String(),
		"cloneFactor": "2.0",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+follower.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(store.createFollowLinkArgs) != 1 {
		t.Fatalf("expected 1 CreateFollowLink call, got %d", len(store.createFollowLinkArgs))
	}
	link := store.createFollowLinkArgs[0]
	if link.FollowerID != follower || link.GroupID != groupID || link.MasterID != masterID || link.CloneFactor.String() != "2" {
		t.Errorf("unexpected link: %+v", link)
	}
}

func TestPatchAccount_DetachGroupID_Succeeds(t *testing.T) {
	follower := uuid.New()
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{follower: "follower"},
		accounts:     []domain.Account{{ID: follower, Role: "follower"}},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"groupId": "",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+follower.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["groupId"] != nil {
		t.Errorf("expected groupId null in response, got %v", resp["groupId"])
	}
}

func TestPatchAccount_CloneFactorOnMaster_Returns400(t *testing.T) {
	master := uuid.New()
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{master: "master"},
		accounts:     []domain.Account{{ID: master, Role: "master"}},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"cloneFactor": "0.75"})
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
	if len(store.setStatusArgs) != 1 || store.setStatusArgs[0] != (setStatusCall{id, domain.AccountStatusError}) {
		t.Errorf("setStatusArgs = %+v, want [{%v error}]", store.setStatusArgs, id)
	}
}

func TestPatchAccount_InvalidStatus_Returns400(t *testing.T) {
	id := uuid.New()
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"status": "invalid_status_xyz"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+id.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if len(store.setStatusArgs) != 0 {
		t.Errorf("expected no SetAccountStatus calls on invalid input, got %d", len(store.setStatusArgs))
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
			MasterID: &master, CloneFactor: decimalPtr("0.5"), MaxQtyPerOrder: &maxQty, Enabled: true},
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
	if got[1]["masterId"] != master.String() || got[1]["cloneFactor"] != "0.5" {
		t.Errorf("follower entry = %+v", got[1])
	}
	if got[0]["masterId"] != nil || got[0]["cloneFactor"] != nil {
		t.Errorf("master entry should have nil group fields, got %+v", got[0])
	}
}

func TestGetAccounts_SanitizesAuthError_HidesBackendAPICalls(t *testing.T) {
	accID := uuid.New()
	rawError := "dhan sync failed: get dhan holdings: dhan api error [DH-1111] (HTTP 500): No holdings available"
	store := &stubStore{accounts: []domain.Account{
		{
			ID:              accID,
			Role:            "master",
			Broker:          "dhan",
			BrokerAccountID: "1114425611",
			Active:          true,
			Enabled:         true,
			Status:          "error",
			AuthStatus:      "error",
			AuthError:       rawError,
		},
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
	if len(got) != 1 {
		t.Fatalf("got %d accounts, want 1", len(got))
	}
	authErrorVal, ok := got[0]["authError"].(string)
	if !ok || authErrorVal == "" {
		t.Fatalf("expected authError string, got %+v", got[0]["authError"])
	}
	if authErrorVal != "Broker server temporarily unavailable" {
		t.Errorf("authError = %q, want %q", authErrorVal, "Broker server temporarily unavailable")
	}
	lower := strings.ToLower(authErrorVal)
	if strings.Contains(lower, "dhan") || strings.Contains(lower, "500") || strings.Contains(lower, "holdings") || strings.Contains(lower, "api") {
		t.Errorf("authError leaked backend API calls: %q", authErrorVal)
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
		"cloneFactor": "0.5", "maxQtyPerOrder": maxQty, "masterId": master.String(),
		"ip": "192.168.1.100",
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
	if link.MasterID != master || link.MaxQtyPerOrder != maxQty || !link.CloneFactor.Equal(decimal.RequireFromString("0.5")) {
		t.Errorf("link = %+v", link)
	}
	if len(store.createAccountArgs) != 1 || store.createAccountArgs[0].IPAddress != "192.168.1.100" {
		t.Errorf("createAccountArgs = %+v, want ip 192.168.1.100", store.createAccountArgs)
	}
}

func TestPostAccount_Follower_RegistersWithWorkerPool(t *testing.T) {
	master := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{master: "master"}}
	registrar := &stubFollowerRegistrar{}
	r := httpapi.NewRouter(store, &stubActionEngine{}, httpapi.WithFollowerRegistrar(registrar))

	body, _ := json.Marshal(map[string]any{
		"role": "follower", "broker": "kite", "brokerAccountId": "REG001", "apiSecret": "s",
		"maxQtyPerOrder": 10, "masterId": master.String(), "ip": "192.168.1.100",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(registrar.registered) != 1 {
		t.Fatalf("RegisterFollower called %d times, want 1", len(registrar.registered))
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["id"] != registrar.registered[0].String() {
		t.Errorf("registered follower ID = %v, want %v", registrar.registered[0], resp["id"])
	}
}

func TestPostAccount_Master_DoesNotRegisterWithWorkerPool(t *testing.T) {
	store := &stubStore{}
	registrar := &stubFollowerRegistrar{}
	r := httpapi.NewRouter(store, &stubActionEngine{}, httpapi.WithFollowerRegistrar(registrar))

	body, _ := json.Marshal(map[string]any{
		"role": "master", "broker": "kite", "brokerAccountId": "MSTR01", "apiKey": "k", "apiSecret": "s",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(registrar.registered) != 0 {
		t.Fatalf("RegisterFollower should not be called for master, got %d calls", len(registrar.registered))
	}
}

func TestPostAccount_Follower_CreationFails_DoesNotRegister(t *testing.T) {
	master := uuid.New()
	store := &stubStore{
		accountRoles:     map[uuid.UUID]string{master: "master"},
		createAccountErr: errors.New("db insert failed"),
	}
	registrar := &stubFollowerRegistrar{}
	r := httpapi.NewRouter(store, &stubActionEngine{}, httpapi.WithFollowerRegistrar(registrar))

	body, _ := json.Marshal(map[string]any{
		"role": "follower", "broker": "kite", "brokerAccountId": "FAIL01", "apiSecret": "s",
		"maxQtyPerOrder": 10, "masterId": master.String(), "ip": "192.168.1.100",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if len(registrar.registered) != 0 {
		t.Fatalf("RegisterFollower should not be called when creation fails, got %d calls", len(registrar.registered))
	}
}

func TestPostAccount_Follower_CreateFollowLinkFails_DoesNotRegister(t *testing.T) {
	master := uuid.New()
	store := &stubStore{
		accountRoles:        map[uuid.UUID]string{master: "master"},
		createFollowLinkErr: errors.New("link failed"),
	}
	registrar := &stubFollowerRegistrar{}
	r := httpapi.NewRouter(store, &stubActionEngine{}, httpapi.WithFollowerRegistrar(registrar))

	body, _ := json.Marshal(map[string]any{
		"role": "follower", "broker": "kite", "brokerAccountId": "FAIL02", "apiSecret": "s",
		"maxQtyPerOrder": 10, "masterId": master.String(), "ip": "192.168.1.100",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if len(registrar.registered) != 0 {
		t.Fatalf("RegisterFollower should not be called when link fails, got %d calls", len(registrar.registered))
	}
}

func TestPostAccount_Follower_RegistrarError_StillReturns201(t *testing.T) {
	master := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{master: "master"}}
	registrar := &stubFollowerRegistrar{registerErr: errors.New("broker connection refused")}
	r := httpapi.NewRouter(store, &stubActionEngine{}, httpapi.WithFollowerRegistrar(registrar))

	body, _ := json.Marshal(map[string]any{
		"role": "follower", "broker": "kite", "brokerAccountId": "REGERR", "apiSecret": "s",
		"maxQtyPerOrder": 10, "masterId": master.String(), "ip": "192.168.1.100",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(registrar.registered) != 1 {
		t.Fatalf("RegisterFollower should still have been attempted, got %d calls", len(registrar.registered))
	}
}

func TestPostAccount_FollowerDefaultCloneFactor_DefaultsToOne(t *testing.T) {
	master := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{master: "master"}}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	maxQty := 25
	body, _ := json.Marshal(map[string]any{
		"role": "follower", "broker": "kite", "brokerAccountId": "ZY5679", "apiSecret": "s",
		"maxQtyPerOrder": maxQty, "masterId": master.String(),
		"ip": "192.168.1.100",
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
	if !link.CloneFactor.Equal(decimal.NewFromInt(1)) {
		t.Errorf("link.CloneFactor = %v, want 1", link.CloneFactor)
	}
}

func TestPostAccount_FollowerWithoutGroup_Succeeds(t *testing.T) {
	store := &stubStore{}
	registrar := &stubFollowerRegistrar{}
	r := httpapi.NewRouter(store, &stubActionEngine{}, httpapi.WithFollowerRegistrar(registrar))

	body, _ := json.Marshal(map[string]any{
		"role": "follower", "broker": "kite", "brokerAccountId": "ZY5678",
		"ip": "192.168.1.100",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(store.createFollowLinkArgs) != 0 {
		t.Errorf("expected 0 CreateFollowLink calls for unassigned follower, got %d", len(store.createFollowLinkArgs))
	}
	if len(registrar.registered) != 1 {
		t.Errorf("expected follower to register with pool, got %d", len(registrar.registered))
	}
}

func TestPostAccount_FollowerWithGroupID_Succeeds(t *testing.T) {
	groupID := uuid.New()
	masterID := uuid.New()
	store := &stubStore{
		groupDetails: map[uuid.UUID]domain.GroupDetail{
			groupID: {
				GroupID:         groupID,
				GroupName:       "Alpha Group",
				MasterID:        masterID,
				MasterAccountID: "ZX1234",
				MasterActive:    true,
			},
		},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"role": "follower", "broker": "kite", "brokerAccountId": "ZY5678",
		"ip": "192.168.1.100", "groupId": groupID.String(),
		"cloneFactor": "1.5",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(store.createFollowLinkArgs) != 1 {
		t.Fatalf("expected 1 CreateFollowLink call, got %d", len(store.createFollowLinkArgs))
	}
	link := store.createFollowLinkArgs[0]
	if link.GroupID != groupID || link.MasterID != masterID || link.CloneFactor.String() != "1.5" {
		t.Errorf("unexpected link args: %+v", link)
	}

	var resp struct {
		GroupID   *string `json:"groupId"`
		GroupName *string `json:"groupName"`
		MasterID  *string `json:"masterId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if resp.GroupName == nil || *resp.GroupName != "Alpha Group" {
		t.Errorf("expected groupName 'Alpha Group', got %v", resp.GroupName)
	}
	if resp.GroupID == nil || *resp.GroupID != groupID.String() {
		t.Errorf("expected groupId %s, got %v", groupID, resp.GroupID)
	}
}

func TestPostAccount_FollowerWithMasterID_ReturnsGroupName(t *testing.T) {
	groupID := uuid.New()
	masterID := uuid.New()
	store := &stubStore{
		groupDetails: map[uuid.UUID]domain.GroupDetail{
			masterID: {
				GroupID:         groupID,
				GroupName:       "Beta Group",
				MasterID:        masterID,
				MasterAccountID: "ZX1234",
				MasterActive:    true,
			},
		},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"role": "follower", "broker": "kite", "brokerAccountId": "ZY5678",
		"ip": "192.168.1.100", "masterId": masterID.String(),
		"cloneFactor": "1.0",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		GroupID   *string `json:"groupId"`
		GroupName *string `json:"groupName"`
		MasterID  *string `json:"masterId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if resp.GroupName == nil || *resp.GroupName != "Beta Group" {
		t.Errorf("expected groupName 'Beta Group', got %v", resp.GroupName)
	}
	if resp.GroupID == nil || *resp.GroupID != groupID.String() {
		t.Errorf("expected groupId %s, got %v", groupID, resp.GroupID)
	}
}

func TestPostAccount_InvalidMasterID_Returns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"role": "follower", "broker": "kite", "brokerAccountId": "ZY5678",
		"cloneFactor": "0.5", "maxQtyPerOrder": 10,
		"ip": "192.168.1.100", "masterId": uuid.New().String(),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestPostAccount_AllowsTestbroker(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"name":            "Test Master",
		"role":            "master",
		"broker":          "testbroker",
		"brokerAccountId": "MASTER01",
		"apiKey":          "key_master01",
		"apiSecret":       "sec_master01",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(store.createAccountArgs) != 1 {
		t.Fatalf("expected 1 createAccount call, got %d", len(store.createAccountArgs))
	}
	if store.createAccountArgs[0].Broker != "testbroker" {
		t.Errorf("broker = %s, want testbroker", store.createAccountArgs[0].Broker)
	}
}

func TestPostAccount_AllowsDhan(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"name":            "Dhan Master",
		"role":            "master",
		"broker":          "dhan",
		"brokerAccountId": "1000000001",
		"apiKey":          "dhan_key",
		"apiSecret":       "dhan_secret",
		"accessToken":     "jwt_token_sample",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(store.createAccountArgs) != 1 {
		t.Fatalf("expected 1 createAccount call, got %d", len(store.createAccountArgs))
	}
	if store.createAccountArgs[0].Broker != "dhan" {
		t.Errorf("broker = %s, want dhan", store.createAccountArgs[0].Broker)
	}
	if len(store.setAccessTokenArgs) != 1 {
		t.Fatalf("expected 1 setAccessToken call, got %d", len(store.setAccessTokenArgs))
	}
	if store.setAccessTokenArgs[0].Token != "jwt_token_sample" {
		t.Errorf("token = %s, want jwt_token_sample", store.setAccessTokenArgs[0].Token)
	}
	if store.setAccessTokenArgs[0].ExpiresAt == nil {
		t.Fatalf("expected non-nil ExpiresAt for Dhan access token")
	}
	if store.setAccessTokenArgs[0].ExpiresAt.Before(time.Now().Add(23 * time.Hour)) {
		t.Errorf("expected ExpiresAt around 24h in future, got %v", store.setAccessTokenArgs[0].ExpiresAt)
	}
	if store.setAccessTokenArgs[0].AuthStatus != "pending" {
		t.Errorf("setAccessToken AuthStatus = %s, want pending", store.setAccessTokenArgs[0].AuthStatus)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "pending" {
		t.Errorf("resp status = %v, want pending", resp["status"])
	}
}

func TestPostAccount_BadBroker_Returns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"role": "master", "broker": "invalid_broker", "brokerAccountId": "ZX1234"})
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

func TestPostAccount_ReactivatesExistingAccount_ReturnsOriginalUUID(t *testing.T) {
	origID := uuid.New()
	store := &stubStore{
		createAccountWithCredentialsID: origID,
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"role":            "master",
		"broker":          "kite",
		"brokerAccountId": "ZX1234",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if resp["id"] != origID.String() {
		t.Errorf("resp[id] = %v, want %v", resp["id"], origID)
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

func TestPostAccount_WithPasswordAndTOTPSecret_EncryptsCredentials(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"role":            "master",
		"broker":          "kite",
		"brokerAccountId": "TEST01",
		"apiKey":          "kite_key",
		"apiSecret":       "kite_secret",
		"password":        "secret_password_123",
		"totpSecret":      "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
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
	args := store.createAccountArgs[0]
	if args.EncryptedPassword == "" || args.EncryptedPassword == "secret_password_123" {
		t.Errorf("EncryptedPassword = %q, want encrypted ciphertext", args.EncryptedPassword)
	}
	if args.EncryptedTotpSecret == "" || args.EncryptedTotpSecret == "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" {
		t.Errorf("EncryptedTotpSecret = %q, want encrypted ciphertext", args.EncryptedTotpSecret)
	}

	decPass, err := crypto.Decrypt(args.EncryptedPassword)
	if err != nil || decPass != "secret_password_123" {
		t.Errorf("decrypted password = %q, err = %v", decPass, err)
	}
	decTotp, err := crypto.Decrypt(args.EncryptedTotpSecret)
	if err != nil || decTotp != "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" {
		t.Errorf("decrypted totp = %q, err = %v", decTotp, err)
	}
}

func TestPatchAccount_PasswordAndTOTPSecret_EncryptsCredentials(t *testing.T) {
	id := uuid.New()
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"password":   "new_secret_pass",
		"totpSecret": "NEWTOTPSECRET32CHARSXXXXXXXXXX",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+id.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(store.setEncryptedCredentialsArgs) != 1 {
		t.Fatalf("SetAccountEncryptedCredentials called %d times, want 1", len(store.setEncryptedCredentialsArgs))
	}
	args := store.setEncryptedCredentialsArgs[0]
	decPass, err := crypto.Decrypt(args.EncryptedPassword)
	if err != nil || decPass != "new_secret_pass" {
		t.Errorf("decrypted pass = %q, err = %v", decPass, err)
	}
	decTotp, err := crypto.Decrypt(args.EncryptedTotpSecret)
	if err != nil || decTotp != "NEWTOTPSECRET32CHARSXXXXXXXXXX" {
		t.Errorf("decrypted totp = %q, err = %v", decTotp, err)
	}
}

func TestPatchAccount_MultipleFields_UpdatesAllFields(t *testing.T) {
	id := uuid.New()
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"apiKey":     "multi-key",
		"apiSecret":  "multi-secret",
		"password":   "multi-pass",
		"totpSecret": "MULTITOTPSECRET32CHARSXXXXXXXX",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+id.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(store.setAPIKeyArgs) != 1 || store.setAPIKeyArgs[0].ApiKey != "multi-key" {
		t.Errorf("setAPIKeyArgs = %+v, want multi-key", store.setAPIKeyArgs)
	}
	if len(store.setAPISecretArgs) != 1 || store.setAPISecretArgs[0].ApiSecret != "multi-secret" {
		t.Errorf("setAPISecretArgs = %+v, want multi-secret", store.setAPISecretArgs)
	}
	if len(store.setEncryptedCredentialsArgs) != 1 {
		t.Fatalf("setEncryptedCredentialsArgs called %d times, want 1", len(store.setEncryptedCredentialsArgs))
	}
	decPass, err := crypto.Decrypt(store.setEncryptedCredentialsArgs[0].EncryptedPassword)
	if err != nil || decPass != "multi-pass" {
		t.Errorf("decrypted pass = %q, err = %v", decPass, err)
	}
	decTotp, err := crypto.Decrypt(store.setEncryptedCredentialsArgs[0].EncryptedTotpSecret)
	if err != nil || decTotp != "MULTITOTPSECRET32CHARSXXXXXXXX" {
		t.Errorf("decrypted totp = %q, err = %v", decTotp, err)
	}
}

func TestPostAccount_WithAccessToken_StoresTokenAndRegistersFollower(t *testing.T) {
	master := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{master: "master"}}
	registrar := &stubFollowerRegistrar{}
	r := httpapi.NewRouter(store, &stubActionEngine{}, httpapi.WithFollowerRegistrar(registrar))

	body, _ := json.Marshal(map[string]any{
		"role":            "follower",
		"broker":          "testbroker",
		"brokerAccountId": "NEW01",
		"apiKey":          "key1",
		"apiSecret":       "sec1",
		"accessToken":     "token123",
		"ip":              "192.168.1.100",
		"masterId":        master.String(),
		"maxQtyPerOrder":  100,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(store.setAccessTokenArgs) != 1 {
		t.Fatalf("setAccessTokenArgs called %d times, want 1", len(store.setAccessTokenArgs))
	}
	if store.setAccessTokenArgs[0].Token != "token123" {
		t.Errorf("got token %q, want %q", store.setAccessTokenArgs[0].Token, "token123")
	}
	if len(registrar.registered) != 1 {
		t.Fatalf("RegisterFollower called %d times, want 1", len(registrar.registered))
	}
}

func TestPatchAccount_WithAccessToken_UpdatesTokenAndRegistersFollower(t *testing.T) {
	followerID := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{followerID: "follower"}}
	registrar := &stubFollowerRegistrar{}
	r := httpapi.NewRouter(store, &stubActionEngine{}, httpapi.WithFollowerRegistrar(registrar))

	body, _ := json.Marshal(map[string]any{
		"accessToken": "new_refreshed_token",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+followerID.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(store.setAccessTokenArgs) != 1 {
		t.Fatalf("setAccessTokenArgs called %d times, want 1", len(store.setAccessTokenArgs))
	}
	if store.setAccessTokenArgs[0].Token != "new_refreshed_token" {
		t.Errorf("got token %q, want %q", store.setAccessTokenArgs[0].Token, "new_refreshed_token")
	}
	if len(registrar.registered) != 1 || registrar.registered[0] != followerID {
		t.Errorf("RegisterFollower called with %v, want [%v]", registrar.registered, followerID)
	}
}

type stubBrokerEvicter struct {
	evicted []uuid.UUID
}

func (s *stubBrokerEvicter) Evict(id uuid.UUID) {
	s.evicted = append(s.evicted, id)
}

func TestPatchAccount_CredentialsUpdate_EvictsBroker(t *testing.T) {
	accID := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{accID: "follower"}}
	evicter := &stubBrokerEvicter{}
	r := httpapi.NewRouter(store, &stubActionEngine{}, httpapi.WithBrokerEvicter(evicter))

	body, _ := json.Marshal(map[string]any{
		"accessToken": "new_token",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+accID.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(evicter.evicted) != 1 || evicter.evicted[0] != accID {
		t.Errorf("expected evicter called with %v, got %v", accID, evicter.evicted)
	}
}

func TestDeleteAccount_EvictsBroker(t *testing.T) {
	accID := uuid.New()
	store := &stubStore{}
	evicter := &stubBrokerEvicter{}
	r := httpapi.NewRouter(store, &stubActionEngine{}, httpapi.WithBrokerEvicter(evicter))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/"+accID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", w.Code, w.Body.String())
	}
	if len(evicter.evicted) != 1 || evicter.evicted[0] != accID {
		t.Errorf("expected evicter called with %v, got %v", accID, evicter.evicted)
	}
}

func TestPatchAccount_Deactivate_EvictsBrokerAndUnregistersFollower(t *testing.T) {
	accID := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{accID: "follower"}}
	evicter := &stubBrokerEvicter{}
	registrar := &stubFollowerRegistrar{}
	r := httpapi.NewRouter(store, &stubActionEngine{},
		httpapi.WithBrokerEvicter(evicter),
		httpapi.WithFollowerRegistrar(registrar),
	)

	body, _ := json.Marshal(map[string]any{
		"active": false,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+accID.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(evicter.evicted) != 1 || evicter.evicted[0] != accID {
		t.Errorf("expected evicter called with %v, got %v", accID, evicter.evicted)
	}
	if len(registrar.unregistered) != 1 || registrar.unregistered[0] != accID {
		t.Errorf("expected registrar.unregistered called with %v, got %v", accID, registrar.unregistered)
	}
}

func TestPatchAccount_Reactivate_RegistersFollower(t *testing.T) {
	accID := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{accID: "follower"}}
	registrar := &stubFollowerRegistrar{}
	r := httpapi.NewRouter(store, &stubActionEngine{},
		httpapi.WithFollowerRegistrar(registrar),
	)

	body, _ := json.Marshal(map[string]any{
		"active": true,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+accID.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(registrar.registered) != 1 || registrar.registered[0] != accID {
		t.Errorf("expected registrar.registered called with %v, got %v", accID, registrar.registered)
	}
}

func TestAccount_ProxyIP_WhitelistedBrokers_Validation(t *testing.T) {
	store := &stubStore{
		proxyIPs: []domain.ProxyIP{
			{IPAddress: "10.0.0.1", WhitelistedBrokers: "TestBroker"},
			{IPAddress: "10.0.0.2", WhitelistedBrokers: "Zerodha"},
			{IPAddress: "10.0.0.3", WhitelistedBrokers: "Zerodha, TestBroker"},
			{IPAddress: "10.0.0.4", WhitelistedBrokers: ""},
			{IPAddress: "10.0.0.5", WhitelistedBrokers: "Dhan"},
		},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	// Case 1: Create Kite account with IP whitelisted only for TestBroker -> 400 Bad Request
	{
		body, _ := json.Marshal(map[string]any{
			"broker":          "kite",
			"role":            "follower",
			"brokerAccountId": "FOLLOWER1",
			"apiKey":          "k",
			"apiSecret":       "s",
			"ip":              "10.0.0.1",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for broker kite on TestBroker IP, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Case 2: Create TestBroker account with IP whitelisted only for Zerodha -> 400 Bad Request
	{
		body, _ := json.Marshal(map[string]any{
			"broker":          "testbroker",
			"role":            "follower",
			"brokerAccountId": "FOLLOWER2",
			"apiKey":          "k",
			"apiSecret":       "s",
			"ip":              "10.0.0.2",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for broker testbroker on Zerodha IP, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Case 3: Create Kite account with IP whitelisted for Zerodha -> 201 Created
	{
		body, _ := json.Marshal(map[string]any{
			"broker":          "kite",
			"role":            "follower",
			"brokerAccountId": "FOLLOWER3",
			"apiKey":          "k",
			"apiSecret":       "s",
			"ip":              "10.0.0.2",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 for broker kite on Zerodha IP, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Case 4: Create Kite account with IP whitelisted for both Zerodha and TestBroker -> 201 Created
	{
		body, _ := json.Marshal(map[string]any{
			"broker":          "kite",
			"role":            "follower",
			"brokerAccountId": "FOLLOWER4",
			"apiKey":          "k",
			"apiSecret":       "s",
			"ip":              "10.0.0.3",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 for broker kite on multi-broker IP, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Case 5: Patch existing Kite account with IP whitelisted only for TestBroker -> 400 Bad Request
	accID := uuid.New()
	store.accounts = []domain.Account{
		{
			ID:              accID,
			Broker:          "kite",
			Role:            "follower",
			BrokerAccountID: "FOLLOWER_KITE",
		},
	}
	store.accountRoles = map[uuid.UUID]string{accID: "follower"}

	{
		body, _ := json.Marshal(map[string]any{
			"ip": "10.0.0.1",
		})
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+accID.String(), bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 patching kite account with TestBroker IP, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Case 6: Patch existing Kite account with IP whitelisted for Zerodha -> 200 OK
	{
		body, _ := json.Marshal(map[string]any{
			"ip": "10.0.0.2",
		})
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+accID.String(), bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 patching kite account with Zerodha IP, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Case 7: Create Dhan account with IP whitelisted for Dhan -> 201 Created
	{
		body, _ := json.Marshal(map[string]any{
			"broker":          "dhan",
			"role":            "follower",
			"brokerAccountId": "1000000005",
			"ip":              "10.0.0.5",
			"accessToken":     "token_dhan_valid",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 for broker dhan on Dhan IP, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Case 8: Create Dhan account with IP whitelisted only for Zerodha -> 400 Bad Request
	{
		body, _ := json.Marshal(map[string]any{
			"broker":          "dhan",
			"role":            "follower",
			"brokerAccountId": "1000000006",
			"ip":              "10.0.0.2",
			"accessToken":     "token_dhan_bad",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for broker dhan on Zerodha IP, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Case 9: Create Dhan account with IP whitelisted for All Brokers ("") -> 201 Created
	{
		body, _ := json.Marshal(map[string]any{
			"broker":          "dhan",
			"role":            "follower",
			"brokerAccountId": "1000000007",
			"ip":              "10.0.0.4",
			"accessToken":     "token_dhan_all",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 for broker dhan on All Brokers IP, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Case 10: Create Kite account with IP whitelisted for Dhan -> 400 Bad Request
	{
		body, _ := json.Marshal(map[string]any{
			"broker":          "kite",
			"role":            "follower",
			"brokerAccountId": "FOLLOWER_KITE_BAD",
			"apiKey":          "k",
			"apiSecret":       "s",
			"ip":              "10.0.0.5",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for broker kite on Dhan IP, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Case 11: Patch Dhan account with IP whitelisted for Zerodha -> 400 Bad Request
	dhanAccID := uuid.New()
	store.accounts = append(store.accounts, domain.Account{
		ID:              dhanAccID,
		Broker:          "dhan",
		Role:            "follower",
		BrokerAccountID: "1000000008",
	})
	store.accountRoles[dhanAccID] = "follower"

	{
		body, _ := json.Marshal(map[string]any{
			"ip": "10.0.0.2",
		})
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+dhanAccID.String(), bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 patching dhan account with Zerodha IP, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Case 12: Patch Dhan account with IP whitelisted for Dhan -> 200 OK
	{
		body, _ := json.Marshal(map[string]any{
			"ip": "10.0.0.5",
		})
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+dhanAccID.String(), bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 patching dhan account with Dhan IP, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Case 13: Patch Dhan account with accessToken -> sets ExpiresAt ~24h in future
	{
		before := len(store.setAccessTokenArgs)
		body, _ := json.Marshal(map[string]any{
			"accessToken": "patched_jwt_token",
		})
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+dhanAccID.String(), bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 patching dhan account accessToken, got %d: %s", w.Code, w.Body.String())
		}
		if len(store.setAccessTokenArgs) != before+1 {
			t.Fatalf("expected 1 setAccessToken call, got %d", len(store.setAccessTokenArgs)-before)
		}
		call := store.setAccessTokenArgs[len(store.setAccessTokenArgs)-1]
		if call.Token != "patched_jwt_token" {
			t.Errorf("token = %s, want patched_jwt_token", call.Token)
		}
		if call.ExpiresAt == nil || call.ExpiresAt.Before(time.Now().Add(23*time.Hour)) {
			t.Errorf("expected 24h ExpiresAt on patched token, got %v", call.ExpiresAt)
		}
	}
}



