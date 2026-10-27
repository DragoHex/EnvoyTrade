package httpapi_test

import (
	"bytes"
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

type mockAdminTelemetry struct {
	status domain.PipelineStatus
}

func (m *mockAdminTelemetry) PipelineStatus(_ context.Context) domain.PipelineStatus {
	return m.status
}

type mockProxyTester struct {
	result domain.ProxyTestResult
}

func (m *mockProxyTester) TestProxy(_ context.Context, proxy domain.ProxyIP) domain.ProxyTestResult {
	res := m.result
	res.IPAddress = proxy.IPAddress
	return res
}

func setupAdminTestServer(t *testing.T) (*stubStore, *stubActionEngine, http.Handler) {
	t.Helper()
	store := &stubStore{}
	engine := &stubActionEngine{}

	telemetry := &mockAdminTelemetry{
		status: domain.PipelineStatus{
			KillSwitch: domain.KillSwitchStatus{Engaged: false},
			MasterTickers: []domain.TickerStatus{
				{MasterID: uuid.New(), Active: true},
			},
			Queues: domain.QueueStatus{
				MasterFillDepth:  3,
				MasterFillCap:    1024,
				OrderUpdateDepth: 1,
				OrderUpdateCap:   512,
			},
			WorkerCount: 5,
			DBPool: domain.DBPoolStatus{
				AcquiredConns: 2,
				IdleConns:     8,
				TotalConns:    10,
				MaxConns:      20,
			},
			SystemResources: domain.SystemResources{
				CPUPercent:     15.4,
				HeapAllocMB:    32.5,
				SysMemoryMB:    64.0,
				GoroutineCount: 42,
				GCCycles:       8,
			},
		},
	}

	proxyTester := &mockProxyTester{
		result: domain.ProxyTestResult{
			Success:   true,
			LatencyMs: 42,
			EgressIP:  "2401:c080:2400:16b8:9828:f266:cc87:9162",
		},
	}

	handler := httpapi.NewRouter(store, engine,
		httpapi.WithTelemetry(telemetry),
		httpapi.WithProxyTester(proxyTester),
	)

	return store, engine, handler
}

func makeAdminSession(role string, active bool) (*domain.SessionWithUser, string) {
	token := "admintoken_12345678901234567890"
	uID := uuid.New()
	user := domain.User{
		ID:        uID,
		Email:     role + "@envoytrade.com",
		Username:  role + "user",
		Name:      "Test " + role,
		Role:      role,
		Active:    active,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	sess := domain.Session{
		ID:         1,
		TokenHash:  auth.HashSessionToken(token),
		UserID:     uID,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
		CreatedAt:  time.Now(),
		LastSeenAt: time.Now(),
	}
	return &domain.SessionWithUser{Session: sess, User: user}, token
}

func TestAdminAuthMiddleware(t *testing.T) {
	memStore := newMemAuthStore()
	store := &fullStubStore{
		stubStore:    &stubStore{},
		memAuthStore: memStore,
	}
	eng := &stubActionEngine{}
	router := httpapi.NewRouter(store, eng)

	// 1. Unauthenticated request to /api/v1/admin/status -> 401
	req := httptest.NewRequest("GET", "/api/v1/admin/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated admin route, got %d", rec.Code)
	}

	// 2. Regular user (role = "user") -> 403 Forbidden
	userSess, userToken := makeAdminSession(domain.RoleUser, true)
	memStore.sessions[userSess.Session.TokenHash] = userSess

	req = httptest.NewRequest("GET", "/api/v1/admin/status", nil)
	req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: userToken})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin user, got %d", rec.Code)
	}

	// 3. Deactivated admin user (active = false) -> 401 Unauthorized
	deactSess, deactToken := makeAdminSession(domain.RoleAdmin, false)
	memStore.sessions[deactSess.Session.TokenHash] = deactSess

	req = httptest.NewRequest("GET", "/api/v1/admin/status", nil)
	req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: deactToken})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for deactivated admin user, got %d", rec.Code)
	}

	// 4. Active admin user (role = "admin", active = true) -> 200 OK
	adminSess, adminToken := makeAdminSession(domain.RoleAdmin, true)
	memStore.sessions[adminSess.Session.TokenHash] = adminSess

	req = httptest.NewRequest("GET", "/api/v1/admin/status", nil)
	req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: adminToken})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for active admin user, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminServiceStatusAndKillSwitch(t *testing.T) {
	_, engine, handler := setupAdminTestServer(t)

	// GET /api/v1/admin/status
	req := httptest.NewRequest("GET", "/api/v1/admin/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var status domain.PipelineStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.WorkerCount != 5 || status.Queues.MasterFillDepth != 3 || status.DBPool.TotalConns != 10 {
		t.Fatalf("unexpected pipeline status: %+v", status)
	}
	if status.SystemResources.CPUPercent != 15.4 || status.SystemResources.HeapAllocMB != 32.5 || status.SystemResources.GoroutineCount != 42 {
		t.Fatalf("unexpected system resources: %+v", status.SystemResources)
	}

	// POST /api/v1/admin/killswitch -> Engage
	body, _ := json.Marshal(map[string]any{
		"engaged": true,
		"reason":  "Extreme volatility spike",
	})
	req = httptest.NewRequest("POST", "/api/v1/admin/killswitch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on kill switch engage, got %d", rec.Code)
	}
	if !engine.killSwitchStatus.Engaged || engine.killSwitchStatus.Reason != "Extreme volatility spike" {
		t.Fatalf("kill switch not engaged in engine: %+v", engine.killSwitchStatus)
	}

	// POST /api/v1/admin/killswitch -> Disengage
	body, _ = json.Marshal(map[string]any{
		"engaged": false,
		"reason":  "",
	})
	req = httptest.NewRequest("POST", "/api/v1/admin/killswitch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on kill switch disengage, got %d", rec.Code)
	}
	if engine.killSwitchStatus.Engaged {
		t.Fatal("kill switch should be disengaged")
	}
}

func TestAdminProxyIPsCRUD(t *testing.T) {
	store, _, handler := setupAdminTestServer(t)

	// Seed store with proxy IPs
	now := time.Now()
	store.proxyIPs = []domain.ProxyIP{
		{
			IPAddress:    "10.0.0.1",
			IPType:       "ipv4",
			Host:         "dc1.algoip.in",
			Port:         443,
			Username:     "proxy_user_1",
			ValidFrom:    now.AddDate(0, -1, 0),
			ValidUntil:   now.AddDate(0, 2, 0),
			Plan:         "QUARTERLY",
			ExpiryStatus: domain.ExpiryActive,
		},
		{
			IPAddress:    "2401:c080::1",
			IPType:       "ipv6",
			Host:         "dc6.algoip.in",
			Port:         443,
			Username:     "proxy_user_2",
			ValidFrom:    now.AddDate(0, -3, 0),
			ValidUntil:   now.AddDate(0, 0, 5),
			Plan:         "QUARTERLY",
			ExpiryStatus: domain.ExpiryExpiringSoon,
		},
	}
	store.proxyStats = domain.ProxyIPStats{
		TotalCount:        2,
		AvailableCount:    2,
		IPv4Count:         1,
		IPv6Count:         1,
		ExpiringSoonCount: 1,
	}

	// 1. GET /api/v1/admin/proxy-ips
	req := httptest.NewRequest("GET", "/api/v1/admin/proxy-ips", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		IPs   []domain.ProxyIP     `json:"ips"`
		Stats domain.ProxyIPStats `json:"stats"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(listResp.IPs) != 2 || listResp.Stats.TotalCount != 2 {
		t.Fatalf("unexpected list response: %+v", listResp)
	}
	if listResp.IPs[0].Username != "proxy_user_1" {
		t.Errorf("expected username proxy_user_1, got %s", listResp.IPs[0].Username)
	}

	// 2. GET /api/v1/admin/proxy-ips?type=ipv6 filter
	req = httptest.NewRequest("GET", "/api/v1/admin/proxy-ips?type=ipv6", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	_ = json.NewDecoder(rec.Body).Decode(&listResp)
	if len(listResp.IPs) != 1 || listResp.IPs[0].IPType != "ipv6" {
		t.Fatalf("expected 1 ipv6 IP, got %+v", listResp.IPs)
	}

	// 3. POST /api/v1/admin/proxy-ips/bulk
	bulkPayload := []domain.ProxyIPInput{
		{
			IP:                 "2401:c080::2",
			Host:               "dc6.algoip.in",
			Port:               443,
			UserID:             "user1",
			Password:           "pass1",
			ValidFrom:          "2026-10-06 08:57:22 (IST)",
			ValidUntil:         "2027-01-06 08:57:22 (IST)",
			Plan:               "QUARTERLY",
			Type:               "IPV6",
			WhitelistedBrokers: "TestBroker",
		},
		{
			IP:   "invalid-ip",
			Host: "dc6.algoip.in",
		},
	}
	bulkBody, _ := json.Marshal(bulkPayload)
	req = httptest.NewRequest("POST", "/api/v1/admin/proxy-ips/bulk", bytes.NewReader(bulkBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var bulkResult domain.BulkUploadResult
	if err := json.NewDecoder(rec.Body).Decode(&bulkResult); err != nil {
		t.Fatalf("decode bulk result: %v", err)
	}
	if bulkResult.Total != 2 || bulkResult.Inserted != 1 || len(bulkResult.Errors) != 1 {
		t.Fatalf("unexpected bulk result: %+v", bulkResult)
	}
	if store.proxyIPs[len(store.proxyIPs)-1].WhitelistedBrokers != "TestBroker" {
		t.Errorf("expected WhitelistedBrokers TestBroker from bulk, got %q", store.proxyIPs[len(store.proxyIPs)-1].WhitelistedBrokers)
	}

	// 4. POST /api/v1/admin/proxy-ips (single IP with custom plan & whitelisted brokers)
	singlePayload := domain.ProxyIPInput{
		IP:                 "10.0.0.5",
		Host:               "dc1.algoip.in",
		Port:               443,
		UserID:             "single_user",
		Password:           "single_pass",
		ValidFrom:          "2026-10-06 08:57:22 (IST)",
		ValidUntil:         "2027-01-06 08:57:22 (IST)",
		Plan:               "YEARLY",
		Type:               "IPV4",
		WhitelistedBrokers: "Zerodha",
	}
	singleBody, _ := json.Marshal(singlePayload)
	req = httptest.NewRequest("POST", "/api/v1/admin/proxy-ips", bytes.NewReader(singleBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if last := store.proxyIPs[len(store.proxyIPs)-1]; last.Plan != "YEARLY" || last.WhitelistedBrokers != "Zerodha" {
		t.Errorf("expected plan YEARLY and WhitelistedBrokers Zerodha, got %+v", last)
	}

	// 4b. PUT /api/v1/admin/proxy-ips/{ip}
	putPayload := domain.ProxyIPInput{
		IP:                 "10.0.0.5",
		Host:               "dc1.algoip.in",
		Port:               443,
		UserID:             "single_user",
		Password:           "new_secret_password",
		ValidFrom:          "2026-10-06 08:57:22 (IST)",
		ValidUntil:         "2027-01-06 08:57:22 (IST)",
		Plan:               "YEARLY",
		Type:               "IPV4",
		WhitelistedBrokers: "Zerodha, TestBroker",
	}
	putBody, _ := json.Marshal(putPayload)
	req = httptest.NewRequest("PUT", "/api/v1/admin/proxy-ips/10.0.0.5", bytes.NewReader(putBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on put, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.lastUpdatedProxyIP.WhitelistedBrokers != "Zerodha, TestBroker" {
		t.Errorf("expected WhitelistedBrokers 'Zerodha, TestBroker', got %q", store.lastUpdatedProxyIP.WhitelistedBrokers)
	}

	// 5. PATCH /api/v1/admin/proxy-ips/{ip}
	patchPayload := map[string]any{
		"plan":     "YEARLY",
		"password": "new_secret_password",
	}
	patchBody, _ := json.Marshal(patchPayload)
	req = httptest.NewRequest("PATCH", "/api/v1/admin/proxy-ips/10.0.0.5", bytes.NewReader(patchBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on patch, got %d: %s", rec.Code, rec.Body.String())
	}

	// 6. DELETE /api/v1/admin/proxy-ips/{ip} - Conflict when assigned
	store.deleteProxyErr = domain.ErrConflict
	req = httptest.NewRequest("DELETE", "/api/v1/admin/proxy-ips/10.0.0.5", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when deleting assigned IP, got %d", rec.Code)
	}

	// 7. DELETE /api/v1/admin/proxy-ips/{ip} - Success
	store.deleteProxyErr = nil
	req = httptest.NewRequest("DELETE", "/api/v1/admin/proxy-ips/10.0.0.5", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on delete, got %d", rec.Code)
	}

	// 8. POST /api/v1/admin/proxy-ips/{ip}/test
	req = httptest.NewRequest("POST", "/api/v1/admin/proxy-ips/10.0.0.1/test", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on proxy test, got %d: %s", rec.Code, rec.Body.String())
	}
	var testRes domain.ProxyTestResult
	if err := json.NewDecoder(rec.Body).Decode(&testRes); err != nil {
		t.Fatalf("decode test result: %v", err)
	}
	if !testRes.Success || testRes.LatencyMs != 42 {
		t.Fatalf("unexpected test result: %+v", testRes)
	}
}

func TestAdminUsersCRUDAndSessions(t *testing.T) {
	store, _, handler := setupAdminTestServer(t)

	// Seed users
	u1 := uuid.New()
	u2 := uuid.New()
	store.adminUsers = []domain.UserSummary{
		{
			ID:           u1,
			Email:        "admin@envoytrade.com",
			Username:     "admin",
			Name:         "Super Admin",
			Role:         domain.RoleAdmin,
			Active:       true,
			AccountCount: 2,
			SessionCount: 1,
		},
		{
			ID:           u2,
			Email:        "trader@envoytrade.com",
			Username:     "trader",
			Name:         "Lead Trader",
			Role:         domain.RoleUser,
			Active:       true,
			AccountCount: 5,
			SessionCount: 2,
		},
	}
	store.adminUsersTotal = 2

	// 1. GET /api/v1/admin/users
	req := httptest.NewRequest("GET", "/api/v1/admin/users?limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var usersResp struct {
		Users []domain.UserSummary `json:"users"`
		Total int                  `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&usersResp); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	if len(usersResp.Users) != 2 || usersResp.Total != 2 {
		t.Fatalf("unexpected users response: %+v", usersResp)
	}

	// 2. POST /api/v1/admin/users - validation failure (short password)
	invalidUserBody, _ := json.Marshal(map[string]any{
		"email":    "new@envoytrade.com",
		"username": "newuser",
		"password": "123", // too short
	})
	req = httptest.NewRequest("POST", "/api/v1/admin/users", bytes.NewReader(invalidUserBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short password, got %d", rec.Code)
	}

	// 3. POST /api/v1/admin/users - success
	validUserBody, _ := json.Marshal(map[string]any{
		"email":    "ops@envoytrade.com",
		"username": "opsguy",
		"name":     "Ops Guy",
		"password": "SecurePassword123!",
		"role":     "admin",
	})
	req = httptest.NewRequest("POST", "/api/v1/admin/users", bytes.NewReader(validUserBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 on user create, got %d: %s", rec.Code, rec.Body.String())
	}

	store.usersByID = map[uuid.UUID]*domain.User{
		u1: {
			ID:       u1,
			Email:    "admin@envoytrade.com",
			Username: "admin",
			Name:     "Super Admin",
			Role:     domain.RoleAdmin,
			Active:   true,
		},
		u2: {
			ID:       u2,
			Email:    "trader@envoytrade.com",
			Username: "trader",
			Name:     "Lead Trader",
			Role:     domain.RoleUser,
			Active:   true,
		},
	}

	// 4a. PATCH /api/v1/admin/users/{id} - attempt to suspend admin account returns 400
	activeFalse := false
	patchAdminBody, _ := json.Marshal(map[string]any{
		"active": &activeFalse,
	})
	req = httptest.NewRequest("PATCH", "/api/v1/admin/users/"+u1.String(), bytes.NewReader(patchAdminBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when attempting to suspend admin user, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4b. PATCH /api/v1/admin/users/{id} - regular user suspension succeeds
	patchUserBody, _ := json.Marshal(map[string]any{
		"active": &activeFalse,
	})
	req = httptest.NewRequest("PATCH", "/api/v1/admin/users/"+u2.String(), bytes.NewReader(patchUserBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on patch user, got %d", rec.Code)
	}

	// 4c. PATCH /api/v1/admin/users/{id} - negative case: broker sum mismatch
	mismatchMax := 5
	mismatchBody, _ := json.Marshal(map[string]any{
		"maxIPs": &mismatchMax,
		"brokerIPLimits": map[string]int{
			"kite": 2,
			"dhan": 1, // sum 3 != 5
		},
	})
	req = httptest.NewRequest("PATCH", "/api/v1/admin/users/"+u2.String(), bytes.NewReader(mismatchBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for broker limits mismatch, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4d. PATCH /api/v1/admin/users/{id} - negative case: negative quota less than -1
	negMax := -2
	negBody, _ := json.Marshal(map[string]any{
		"maxIPs": &negMax,
	})
	req = httptest.NewRequest("PATCH", "/api/v1/admin/users/"+u2.String(), bytes.NewReader(negBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for maxIPs < -1, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4d-2. PATCH /api/v1/admin/users/{id} - positive case: -1 for unlimited
	unlimitedMax := -1
	unlimitedBody, _ := json.Marshal(map[string]any{
		"maxIPs": &unlimitedMax,
	})
	req = httptest.NewRequest("PATCH", "/api/v1/admin/users/"+u2.String(), bytes.NewReader(unlimitedBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for maxIPs -1 (unlimited), got %d: %s", rec.Code, rec.Body.String())
	}
	if store.lastUpdatedMaxIPs == nil || *store.lastUpdatedMaxIPs != -1 {
		t.Fatalf("expected maxIPs -1, got %v", store.lastUpdatedMaxIPs)
	}

	// 4d-3. PATCH /api/v1/admin/users/{id} - negative case: 0 total with non-zero broker limit
	zeroMax := 0
	zeroMismatchBody, _ := json.Marshal(map[string]any{
		"maxIPs": &zeroMax,
		"brokerIPLimits": map[string]int{
			"kite": 1,
		},
	})
	req = httptest.NewRequest("PATCH", "/api/v1/admin/users/"+u2.String(), bytes.NewReader(zeroMismatchBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for maxIPs 0 with broker limit sum 1, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4d-4. PATCH /api/v1/admin/users/{id} - positive case: 0 total with empty/zero broker limits
	zeroValidBody, _ := json.Marshal(map[string]any{
		"maxIPs": &zeroMax,
		"brokerIPLimits": map[string]int{},
	})
	req = httptest.NewRequest("PATCH", "/api/v1/admin/users/"+u2.String(), bytes.NewReader(zeroValidBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for maxIPs 0 with 0 broker limits, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.lastUpdatedMaxIPs == nil || *store.lastUpdatedMaxIPs != 0 {
		t.Fatalf("expected maxIPs 0, got %v", store.lastUpdatedMaxIPs)
	}

	// 4e. PATCH /api/v1/admin/users/{id} - positive case: matching broker sum
	validMax := 5
	validQuotaBody, _ := json.Marshal(map[string]any{
		"maxIPs": &validMax,
		"brokerIPLimits": map[string]int{
			"kite": 3,
			"dhan": 2, // sum 5 == 5
		},
	})
	req = httptest.NewRequest("PATCH", "/api/v1/admin/users/"+u2.String(), bytes.NewReader(validQuotaBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on valid quota patch, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.lastUpdatedMaxIPs == nil || *store.lastUpdatedMaxIPs != 5 {
		t.Fatalf("expected maxIPs 5, got %v", store.lastUpdatedMaxIPs)
	}
	if store.lastUpdatedBrokerLimits["kite"] != 3 || store.lastUpdatedBrokerLimits["dhan"] != 2 {
		t.Fatalf("unexpected broker limits: %+v", store.lastUpdatedBrokerLimits)
	}

	// 4f. PATCH /api/v1/admin/users/{id} - positive case: matching broker sum with general and testbroker
	genTestMax := 6
	genTestQuotaBody, _ := json.Marshal(map[string]any{
		"maxIPs": &genTestMax,
		"brokerIPLimits": map[string]int{
			"general":    2,
			"kite":       2,
			"dhan":       1,
			"testbroker": 1, // sum 6 == 6
		},
	})
	req = httptest.NewRequest("PATCH", "/api/v1/admin/users/"+u2.String(), bytes.NewReader(genTestQuotaBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on valid quota patch with general and testbroker, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.lastUpdatedBrokerLimits["general"] != 2 || store.lastUpdatedBrokerLimits["testbroker"] != 1 {
		t.Fatalf("unexpected broker limits: %+v", store.lastUpdatedBrokerLimits)
	}

	// 5. GET /api/v1/admin/users/{id}/accounts
	acctID := uuid.New()
	store.accountsByUserID = map[uuid.UUID][]domain.Account{
		u2: {
			{
				ID:              acctID,
				Name:            "Zerodha Alpha",
				Role:            "master",
				Broker:          "kite",
				BrokerAccountID: "AB1234",
				Active:          true,
				Status:          "active",
				AuthStatus:      "authenticated",
				IPAddress:       "148.113.41.42",
			},
		},
	}
	req = httptest.NewRequest("GET", "/api/v1/admin/users/"+u2.String()+"/accounts", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on get user accounts, got %d: %s", rec.Code, rec.Body.String())
	}
	var userAccountsResp struct {
		User     domain.User `json:"user"`
		Accounts []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"accounts"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&userAccountsResp); err != nil {
		t.Fatalf("decode user accounts: %v", err)
	}
	if len(userAccountsResp.Accounts) != 1 || userAccountsResp.Accounts[0].Name != "Zerodha Alpha" {
		t.Fatalf("unexpected user accounts payload: %+v", userAccountsResp)
	}

	// 6. GET /api/v1/admin/users/{id}/sessions
	testIP := "192.168.1.50"
	testUA := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)"
	store.userSessions = []domain.Session{
		{
			ID:         10,
			TokenHash:  "super-secret-token-hash-must-not-leak",
			UserID:     u2,
			IPAddress:  &testIP,
			UserAgent:  &testUA,
			ExpiresAt:  time.Now().Add(12 * time.Hour),
			CreatedAt:  time.Now().Add(-1 * time.Hour),
			LastSeenAt: time.Now(),
		},
	}
	req = httptest.NewRequest("GET", "/api/v1/admin/users/"+u2.String()+"/sessions", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on list sessions, got %d", rec.Code)
	}

	var rawSessionsResp struct {
		Sessions []map[string]any `json:"sessions"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&rawSessionsResp); err != nil {
		t.Fatalf("failed to decode sessions response: %v", err)
	}
	if len(rawSessionsResp.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(rawSessionsResp.Sessions))
	}
	s0 := rawSessionsResp.Sessions[0]
	if _, ok := s0["TokenHash"]; ok {
		t.Errorf("expected TokenHash to be omitted from JSON, but found it")
	}
	if _, ok := s0["tokenHash"]; ok {
		t.Errorf("expected tokenHash to be omitted from JSON, but found it")
	}
	if ip, ok := s0["ipAddress"].(string); !ok || ip != testIP {
		t.Errorf("expected ipAddress %q, got %v", testIP, s0["ipAddress"])
	}
	if ua, ok := s0["userAgent"].(string); !ok || ua != testUA {
		t.Errorf("expected userAgent %q, got %v", testUA, s0["userAgent"])
	}
	if _, ok := s0["expiresAt"].(string); !ok {
		t.Errorf("expected expiresAt string, got %v", s0["expiresAt"])
	}
	if _, ok := s0["lastSeenAt"].(string); !ok {
		t.Errorf("expected lastSeenAt string, got %v", s0["lastSeenAt"])
	}
	if _, ok := s0["createdAt"].(string); !ok {
		t.Errorf("expected createdAt string, got %v", s0["createdAt"])
	}

	// 7. DELETE /api/v1/admin/users/{id}/sessions
	store.revokeSessionsN = 2
	req = httptest.NewRequest("DELETE", "/api/v1/admin/users/"+u2.String()+"/sessions", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on revoke sessions, got %d", rec.Code)
	}
	var revokeResp struct {
		Status       string `json:"status"`
		RevokedCount int    `json:"revokedCount"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&revokeResp)
	if revokeResp.RevokedCount != 2 {
		t.Fatalf("expected revokedCount 2, got %d", revokeResp.RevokedCount)
	}
}

func TestKillSwitchStatusEndpoint(t *testing.T) {
	_, engine, handler := setupAdminTestServer(t)

	// Disengaged
	engine.SetKillSwitch(false, "", "")
	req := httptest.NewRequest("GET", "/api/v1/killswitch", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on get killswitch, got %d", rec.Code)
	}
	var ks domain.KillSwitchStatus
	_ = json.NewDecoder(rec.Body).Decode(&ks)
	if ks.Engaged {
		t.Fatalf("expected killswitch disengaged")
	}

	// Engaged with audit reason
	engine.SetKillSwitch(true, "Market circuit breaker", "trader@envoytrade.com")
	req = httptest.NewRequest("GET", "/api/v1/killswitch", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on get killswitch, got %d", rec.Code)
	}
	_ = json.NewDecoder(rec.Body).Decode(&ks)
	if !ks.Engaged || ks.Reason != "Market circuit breaker" || ks.EngagedBy != "trader@envoytrade.com" {
		t.Fatalf("unexpected killswitch status: %+v", ks)
	}
}

type mockExecutionFeed struct {
	events []domain.ExecutionEvent
}

func (m *mockExecutionFeed) Recent() []domain.ExecutionEvent {
	return m.events
}

func TestAdmin_BrokerReadiness(t *testing.T) {
	store, _, handler := setupAdminTestServer(t)

	store.brokerReadiness = domain.BrokerReadinessSummary{
		MasterReady:    true,
		TotalFollowers: 3,
		FollowersReady: 2,
		AttentionAccounts: []domain.AttentionAccount{
			{
				ID:              uuid.New(),
				Name:            "Follower 3",
				BrokerAccountID: "FOL003",
				Role:            "follower",
				AuthStatus:      "error",
				AuthError:       "Token expired",
			},
		},
	}

	req := httptest.NewRequest("GET", "/api/v1/admin/broker-readiness", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res domain.BrokerReadinessSummary
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !res.MasterReady || res.TotalFollowers != 3 || res.FollowersReady != 2 {
		t.Errorf("unexpected readiness summary: %+v", res)
	}
	if len(res.AttentionAccounts) != 1 || res.AttentionAccounts[0].BrokerAccountID != "FOL003" {
		t.Errorf("unexpected attention accounts: %+v", res.AttentionAccounts)
	}
}

func TestAdmin_OrderIssues(t *testing.T) {
	store, _, handler := setupAdminTestServer(t)

	store.orderIssues = []domain.OrderIssue{
		{
			ID:              1,
			CreatedAt:       time.Now().Add(-1 * time.Minute),
			FollowerName:    "Alpha Follower",
			BrokerAccountID: "FOL001",
			Tradingsymbol:   "NIFTY26OCTFUT",
			Exchange:        "NFO",
			IntendedQty:     25,
			PlacedQty:       0,
			SizingReason:    "below_one_lot",
			TerminalStatus:  "SKIPPED",
			LastError:       "below_one_lot",
		},
	}

	req := httptest.NewRequest("GET", "/api/v1/admin/order-issues?limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Issues []domain.OrderIssue `json:"issues"`
		Total  int                 `json:"total"`
		Limit  int                 `json:"limit"`
		Offset int                 `json:"offset"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 || len(resp.Issues) != 1 || resp.Issues[0].SizingReason != "below_one_lot" {
		t.Errorf("unexpected order issues response: %+v", resp)
	}
	if resp.Limit != 10 || resp.Offset != 0 {
		t.Errorf("expected limit=10, offset=0; got limit=%d, offset=%d", resp.Limit, resp.Offset)
	}

	// Test offset beyond total -> returns empty slice, total preserved
	req = httptest.NewRequest("GET", "/api/v1/admin/order-issues?limit=10&offset=10", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var emptyResp struct {
		Issues []domain.OrderIssue `json:"issues"`
		Total  int                 `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&emptyResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if emptyResp.Total != 1 || len(emptyResp.Issues) != 0 {
		t.Errorf("expected 0 issues with total=1, got: %+v", emptyResp)
	}
}

func TestAdmin_ExecutionFeed(t *testing.T) {
	store := &stubStore{}
	engine := &stubActionEngine{}
	feed := &mockExecutionFeed{
		events: []domain.ExecutionEvent{
			{
				Timestamp:       time.Now(),
				Tradingsymbol:   "RELIANCE",
				Exchange:        "NSE",
				TransactionType: "BUY",
				MasterQty:       50,
				Price:           2900.5,
				FollowersCount:  4,
				SkippedCount:    1,
				LatencyMs:       12,
			},
		},
	}

	handler := httpapi.NewRouter(store, engine,
		httpapi.WithExecutionFeed(feed),
	)

	req := httptest.NewRequest("GET", "/api/v1/admin/execution-feed", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var events []domain.ExecutionEvent
	if err := json.NewDecoder(rec.Body).Decode(&events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(events) != 1 || events[0].Tradingsymbol != "RELIANCE" || events[0].LatencyMs != 12 {
		t.Errorf("unexpected execution events: %+v", events)
	}
}

func TestAdmin_ProxyIPsTestAll(t *testing.T) {
	store, _, handler := setupAdminTestServer(t)

	now := time.Now()
	accID := uuid.New()
	store.proxyIPs = []domain.ProxyIP{
		{
			IPAddress:         "10.0.0.1",
			Host:              "dc1.proxy.internal",
			Port:              8080,
			IsAssigned:        true,
			AssignedAccountID: &accID,
			ValidFrom:         now.Add(-24 * time.Hour),
			ValidUntil:        now.Add(30 * 24 * time.Hour),
		},
		{
			IPAddress:         "10.0.0.2",
			Host:              "dc2.proxy.internal",
			Port:              8080,
			IsAssigned:        true,
			AssignedAccountID: &accID,
			ValidFrom:         now.Add(-24 * time.Hour),
			ValidUntil:        now.Add(30 * 24 * time.Hour),
		},
		{
			IPAddress:  "10.0.0.3",
			Host:       "dc3.proxy.internal",
			Port:       8080,
			IsAssigned: false, // Unassigned, should not be tested when assigned ones exist
			ValidFrom:  now.Add(-24 * time.Hour),
			ValidUntil: now.Add(30 * 24 * time.Hour),
		},
	}

	req := httptest.NewRequest("POST", "/api/v1/admin/proxy-ips/test-all", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var summary domain.BulkProxyTestSummary
	if err := json.NewDecoder(rec.Body).Decode(&summary); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if summary.TotalTested != 2 {
		t.Errorf("expected 2 tested assigned IPs, got %d", summary.TotalTested)
	}
	if summary.SuccessCount != 2 {
		t.Errorf("expected 2 successful probes, got %d", summary.SuccessCount)
	}
	if summary.AvgLatencyMs <= 0 {
		t.Errorf("expected positive avg latency, got %d", summary.AvgLatencyMs)
	}
	if len(summary.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(summary.Results))
	}
}

func TestAdminProxyIP_PUT(t *testing.T) {
	store, _, handler := setupAdminTestServer(t)

	// 1. Success: Valid IPv4 matching route
	payload := domain.ProxyIPInput{
		IP:         "10.0.0.5",
		Host:       "dc46-mum-02.algoip.in",
		Port:       8443,
		UserID:     "user_updated",
		Password:   "pass_1",
		ValidFrom:  "2026-10-06 08:57:22 (IST)",
		ValidUntil: "2027-01-06 08:57:22 (IST)",
		Plan:       "YEARLY",
		Type:       "IPV4",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("PUT", "/api/v1/admin/proxy-ips/10.0.0.5", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.lastUpdatedProxyIP.IPAddress != "10.0.0.5" {
		t.Errorf("expected IPAddress 10.0.0.5, got %s", store.lastUpdatedProxyIP.IPAddress)
	}
	if store.lastUpdatedProxyIP.Host != "dc46-mum-02.algoip.in" {
		t.Errorf("expected Host dc46-mum-02.algoip.in, got %s", store.lastUpdatedProxyIP.Host)
	}
	if store.lastUpdatedProxyIP.Port != 8443 {
		t.Errorf("expected Port 8443, got %d", store.lastUpdatedProxyIP.Port)
	}
	if store.lastUpdatedProxyIP.Username != "user_updated" {
		t.Errorf("expected Username user_updated, got %s", store.lastUpdatedProxyIP.Username)
	}
	if store.lastUpdatedProxyIP.Plan != "YEARLY" {
		t.Errorf("expected Plan YEARLY, got %s", store.lastUpdatedProxyIP.Plan)
	}

	// 2. Success: Valid IPv6 matching route via net.ParseIP().Equal()
	ipv6Payload := domain.ProxyIPInput{
		IP:         "2401:C080:0:0:0:0:0:1", // expanded/uppercase
		Host:       "dc6.algoip.in",
		Port:       443,
		UserID:     "user_6",
		Password:   "pass_6",
		ValidFrom:  "2026-10-06 08:57:22 (IST)",
		ValidUntil: "2027-01-06 08:57:22 (IST)",
		Plan:       "QUARTERLY",
		Type:       "IPV6",
	}
	body, _ = json.Marshal(ipv6Payload)
	req = httptest.NewRequest("PUT", "/api/v1/admin/proxy-ips/2401:c080::1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on IPv6 match, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Negative: Mismatched IP address
	mismatchPayload := domain.ProxyIPInput{
		IP:   "10.0.0.99",
		Host: "dc1.algoip.in",
	}
	body, _ = json.Marshal(mismatchPayload)
	req = httptest.NewRequest("PUT", "/api/v1/admin/proxy-ips/10.0.0.5", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on mismatched IP, got %d", rec.Code)
	}

	// 4. Negative: Missing IP in JSON
	missingIPPayload := map[string]any{
		"host": "dc1.algoip.in",
		"port": 443,
	}
	body, _ = json.Marshal(missingIPPayload)
	req = httptest.NewRequest("PUT", "/api/v1/admin/proxy-ips/10.0.0.5", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on missing IP, got %d", rec.Code)
	}

	// 5. Negative: Unparseable IP in JSON
	unparseableJSONIP := map[string]any{
		"ip":   "invalid-ip-string",
		"host": "dc1.algoip.in",
	}
	body, _ = json.Marshal(unparseableJSONIP)
	req = httptest.NewRequest("PUT", "/api/v1/admin/proxy-ips/10.0.0.5", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on unparseable JSON IP, got %d", rec.Code)
	}

	// 6. Negative: Unparseable IP in URL path
	req = httptest.NewRequest("PUT", "/api/v1/admin/proxy-ips/not-an-ip", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on unparseable route IP, got %d", rec.Code)
	}

	// 7. Negative: Target IP does not exist (404)
	store.updateProxyFullErr = domain.ErrNotFound
	body, _ = json.Marshal(payload)
	req = httptest.NewRequest("PUT", "/api/v1/admin/proxy-ips/10.0.0.5", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", rec.Code)
	}
}

