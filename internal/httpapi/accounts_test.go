package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestPostAccount_FollowerMissingMasterID_Returns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"role": "follower", "broker": "kite", "brokerAccountId": "ZY5678",
		"cloneFactor": "0.5", "maxQtyPerOrder": 10,
		"ip": "192.168.1.100",
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
