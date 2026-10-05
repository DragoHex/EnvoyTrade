package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/httpapi"

	"github.com/google/uuid"
)

func TestGetProxyIPs_ReturnsRegisteredIPs(t *testing.T) {
	assignedID := uuid.New()
	assignedName := "Trader Account"
	now := time.Now()

	store := &stubStore{
		proxyIPs: []domain.ProxyIP{
			{
				IPAddress:           "148.113.41.41",
				IPType:              "ipv4",
				Host:                "dc46-mum-01.algoip.in",
				Port:                443,
				ValidFrom:           now,
				ValidUntil:          now.Add(90 * 24 * time.Hour),
				Plan:                "QUARTERLY",
				IsAssigned:          true,
				AssignedAccountID:   &assignedID,
				AssignedAccountName: &assignedName,
			},
			{
				IPAddress:  "2402:1f00:8302:91e6:6d08:9249:eca8:8252",
				IPType:     "ipv6",
				Host:       "dc46-mum-01.algoip.in",
				Port:       443,
				ValidFrom:  now,
				ValidUntil: now.Add(90 * 24 * time.Hour),
				Plan:       "QUARTERLY",
				IsAssigned: false,
			},
		},
	}

	r := httpapi.NewRouter(store, &stubActionEngine{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy-ips", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp) != 2 {
		t.Fatalf("got %d items, want 2", len(resp))
	}

	// Verify credentials are NOT leaked
	for _, item := range resp {
		if _, ok := item["username"]; ok {
			t.Errorf("username should not be exposed in JSON response")
		}
		if _, ok := item["password"]; ok {
			t.Errorf("password should not be exposed in JSON response")
		}
	}

	if resp[0]["ipAddress"] != "148.113.41.41" || resp[0]["isAssigned"] != true {
		t.Errorf("unexpected first item: %+v", resp[0])
	}
	if resp[0]["assignedAccountId"] != assignedID.String() {
		t.Errorf("assignedAccountId = %v, want %v", resp[0]["assignedAccountId"], assignedID)
	}
	if resp[1]["ipAddress"] != "2402:1f00:8302:91e6:6d08:9249:eca8:8252" || resp[1]["isAssigned"] != false {
		t.Errorf("unexpected second item: %+v", resp[1])
	}
}

func TestGetAvailableProxyIPs_GroupsByIPv4AndIPv6(t *testing.T) {
	now := time.Now()
	store := &stubStore{
		availableIPs: []domain.ProxyIP{
			{
				IPAddress:  "148.113.41.42",
				IPType:     "ipv4",
				Host:       "dc46-mum-01.algoip.in",
				Port:       443,
				ValidFrom:  now,
				ValidUntil: now.Add(90 * 24 * time.Hour),
				Plan:       "QUARTERLY",
			},
			{
				IPAddress:  "2402:1f00:8302:91e6:6d08:9249:eca8:8252",
				IPType:     "ipv6",
				Host:       "dc46-mum-01.algoip.in",
				Port:       443,
				ValidFrom:  now,
				ValidUntil: now.Add(90 * 24 * time.Hour),
				Plan:       "QUARTERLY",
			},
		},
	}

	r := httpapi.NewRouter(store, &stubActionEngine{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy-ips/available", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		IPv4 []map[string]any `json:"ipv4"`
		IPv6 []map[string]any `json:"ipv6"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.IPv4) != 1 || resp.IPv4[0]["ipAddress"] != "148.113.41.42" {
		t.Errorf("unexpected IPv4 list: %+v", resp.IPv4)
	}
	if len(resp.IPv6) != 1 || resp.IPv6[0]["ipAddress"] != "2402:1f00:8302:91e6:6d08:9249:eca8:8252" {
		t.Errorf("unexpected IPv6 list: %+v", resp.IPv6)
	}
}

func TestPostAccount_Follower_RequiresIP(t *testing.T) {
	masterID := uuid.New()
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{masterID: "master"},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"role":            "follower",
		"broker":          "kite",
		"brokerAccountId": "FOLL01",
		"cloneFactor":    "0.5",
		"maxQtyPerOrder":  10,
		"masterId":        masterID.String(),
		"ip":              "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestPostAccount_UnregisteredIP_Returns400(t *testing.T) {
	masterID := uuid.New()
	store := &stubStore{
		accountRoles:     map[uuid.UUID]string{masterID: "master"},
		proxyIPByAddrErr: domain.ErrNotFound,
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"role":            "follower",
		"broker":          "kite",
		"brokerAccountId": "FOLL01",
		"cloneFactor":    "0.5",
		"maxQtyPerOrder":  10,
		"masterId":        masterID.String(),
		"ip":              "192.0.2.1",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}

	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "IP address is not registered in proxy pool" {
		t.Errorf("error = %q, want 'IP address is not registered in proxy pool'", resp["error"])
	}
}

func TestPostAccount_DuplicateIP_Returns409(t *testing.T) {
	masterID := uuid.New()
	store := &stubStore{
		accountRoles:     map[uuid.UUID]string{masterID: "master"},
		createAccountErr: domain.ErrIPAlreadyAssigned,
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"role":            "follower",
		"broker":          "kite",
		"brokerAccountId": "FOLL01",
		"cloneFactor":    "0.5",
		"maxQtyPerOrder":  10,
		"masterId":        masterID.String(),
		"ip":              "148.113.41.41",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}

	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "IP address 148.113.41.41 is already assigned to another account" {
		t.Errorf("error = %q, want IP collision error", resp["error"])
	}
}

func TestPatchAccount_DuplicateIP_Returns409(t *testing.T) {
	accID := uuid.New()
	store := &stubStore{
		accountRoles:    map[uuid.UUID]string{accID: "follower"},
		setIPAddressErr: domain.ErrIPAlreadyAssigned,
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"ip": "148.113.41.41",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+accID.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}

	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "IP address 148.113.41.41 is already assigned to another account" {
		t.Errorf("error = %q, want IP collision error", resp["error"])
	}
}
