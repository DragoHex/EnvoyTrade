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

func TestGetGroups_ReturnsGroupsAsJSON(t *testing.T) {
	master := uuid.New()
	store := &stubStore{
		groups: []domain.GroupSummary{
			{MasterID: master, MasterAccountID: "ZX1234", Broker: "zerodha", FollowerCount: 2, Status: "ok", HasImbalance: true},
		},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
	}
	if len(got) != 1 {
		t.Fatalf("got %d groups, want 1", len(got))
	}
	if got[0]["masterAccountId"] != "ZX1234" {
		t.Errorf("masterAccountId = %v, want ZX1234", got[0]["masterAccountId"])
	}
	if got[0]["followerCount"] != float64(2) {
		t.Errorf("followerCount = %v, want 2", got[0]["followerCount"])
	}
	if got[0]["status"] != "ok" {
		t.Errorf("status = %v, want ok", got[0]["status"])
	}
	if got[0]["hasImbalance"] != true {
		t.Errorf("hasImbalance = %v, want true", got[0]["hasImbalance"])
	}
}

func TestGetGroupDetail_ReturnsDetailAsJSON(t *testing.T) {
	master := uuid.New()
	follower := uuid.New()
	cash := decimal.NewFromInt(50000)
	margin := decimal.NewFromInt(120000)
	store := &stubStore{
		detail: domain.GroupDetail{
			MasterID:                   master,
			MasterAccountID:            "ZX1234",
			HasImbalance:               true,
			MasterNetQty:               100,
			MasterOpenPositionsCount:   2,
			MasterClosedPositionsCount: 1,
			MasterOpenOrdersCount:      3,
			MasterTotalMtm:             decimal.NewFromFloat(1500.50),
			MasterAvailableCash:        &cash,
			MasterAvailableMargin:      &margin,
			MasterMtmBreakdown: map[string]decimal.Decimal{
				"NRML": decimal.NewFromFloat(1000.0),
				"MIS":  decimal.NewFromFloat(500.50),
			},
			Followers: []domain.GroupFollower{
				{
					AccountID:            follower,
					BrokerAccountID:      "ZY5678",
					Enabled:              true,
					Status:               "ok",
					NetQty:               50,
					OpenPositionsCount:   1,
					ClosedPositionsCount: 0,
					OpenOrdersCount:      1,
					TotalMtm:             decimal.NewFromFloat(750.25),
					AvailableCash:        &cash,
					AvailableMargin:      &margin,
					MtmBreakdown: map[string]decimal.Decimal{
						"MIS": decimal.NewFromFloat(750.25),
					},
					MarginBreakdown: &domain.MarginBreakdown{
						Net:         margin,
						Cash:        cash,
						OpeningCash: cash,
						Collateral:  decimal.NewFromInt(100000),
						Utilised:    decimal.Zero,
					},
				},
			},
		},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+master.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
	}
	if got["masterNetQty"] != float64(100) {
		t.Errorf("masterNetQty = %v, want 100", got["masterNetQty"])
	}
	if got["hasImbalance"] != true {
		t.Errorf("hasImbalance = %v, want true", got["hasImbalance"])
	}
	if got["masterOpenPositionsCount"] != float64(2) {
		t.Errorf("masterOpenPositionsCount = %v, want 2", got["masterOpenPositionsCount"])
	}
	masterBreakdown, ok := got["masterMtmBreakdown"].(map[string]any)
	if !ok || masterBreakdown["NRML"] != "1000" {
		t.Errorf("masterMtmBreakdown = %v, want NRML=1000", got["masterMtmBreakdown"])
	}
	followers, ok := got["followers"].([]any)
	if !ok || len(followers) != 1 {
		t.Fatalf("followers = %v, want 1 entry", got["followers"])
	}
	f0 := followers[0].(map[string]any)
	if f0["brokerAccountId"] != "ZY5678" {
		t.Errorf("brokerAccountId = %v, want ZY5678", f0["brokerAccountId"])
	}
	if f0["enabled"] != true {
		t.Errorf("enabled = %v, want true", f0["enabled"])
	}
	if f0["netQty"] != float64(50) {
		t.Errorf("netQty = %v, want 50", f0["netQty"])
	}
	if f0["openPositionsCount"] != float64(1) {
		t.Errorf("openPositionsCount = %v, want 1", f0["openPositionsCount"])
	}
	fBreakdown, ok := f0["mtmBreakdown"].(map[string]any)
	if !ok || fBreakdown["MIS"] != "750.25" {
		t.Errorf("follower mtmBreakdown = %v, want MIS=750.25", f0["mtmBreakdown"])
	}
	mb, ok := f0["marginBreakdown"].(map[string]any)
	if !ok || mb["net"] != "120000" {
		t.Errorf("follower marginBreakdown = %v, want net=120000", f0["marginBreakdown"])
	}
}

func TestGetGroupDetail_NotFoundReturns404(t *testing.T) {
	store := &stubStore{detailErr: domain.ErrNotFound}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestGetGroupDetail_InvalidUUIDReturns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestPostGroupFollower_Succeeds(t *testing.T) {
	master := uuid.New()
	follower := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{master: "master", follower: "follower"}}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"accountId": follower.String(), "cloneFactor": "0.5", "maxQtyPerOrder": 10})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+master.String()+"/followers", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(store.createFollowLinkArgs) != 1 {
		t.Fatalf("CreateFollowLink called %d times, want 1", len(store.createFollowLinkArgs))
	}
	link := store.createFollowLinkArgs[0]
	if link.MasterID != master || link.FollowerID != follower || link.MaxQtyPerOrder != 10 {
		t.Errorf("link = %+v", link)
	}
}

func TestPostGroupFollower_MasterIDNotAMaster_Returns400(t *testing.T) {
	notMaster := uuid.New()
	follower := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{notMaster: "follower", follower: "follower"}}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"accountId": follower.String(), "cloneFactor": "0.5"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+notMaster.String()+"/followers", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestPostGroupFollower_AccountIDNotAFollower_Returns400(t *testing.T) {
	master := uuid.New()
	otherMaster := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{master: "master", otherMaster: "master"}}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"accountId": otherMaster.String(), "cloneFactor": "0.5"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+master.String()+"/followers", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestPostGroupFollower_AlreadyAttached_Returns409(t *testing.T) {
	master := uuid.New()
	follower := uuid.New()
	store := &stubStore{
		accountRoles:        map[uuid.UUID]string{master: "master", follower: "follower"},
		createFollowLinkErr: domain.ErrDuplicate,
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"accountId": follower.String(), "cloneFactor": "0.5"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+master.String()+"/followers", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
}

func TestPostGroupFollowers_BatchArraySuccess(t *testing.T) {
	master := uuid.New()
	f1 := uuid.New()
	f2 := uuid.New()
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{
			master: "master",
			f1:     "follower",
			f2:     "follower",
		},
	}
	reg := &stubFollowerRegistrar{}
	r := httpapi.NewRouter(store, &stubActionEngine{}, httpapi.WithFollowerRegistrar(reg))

	body, _ := json.Marshal([]map[string]any{
		{"accountId": f1.String(), "cloneFactor": 1.5, "maxQtyPerOrder": 25},
		{"accountId": f2.String(), "cloneFactor": "0.75"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+master.String()+"/followers", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(store.createFollowLinksBatchArgs) != 1 {
		t.Fatalf("CreateFollowLinksBatch calls = %d, want 1", len(store.createFollowLinksBatchArgs))
	}
	batch := store.createFollowLinksBatchArgs[0]
	if len(batch) != 2 {
		t.Fatalf("batch length = %d, want 2", len(batch))
	}
	if batch[0].FollowerID != f1 || !batch[0].CloneFactor.Equal(decimal.RequireFromString("1.5")) || batch[0].MaxQtyPerOrder != 25 {
		t.Errorf("batch[0] unexpected: %+v", batch[0])
	}
	if batch[1].FollowerID != f2 || !batch[1].CloneFactor.Equal(decimal.RequireFromString("0.75")) || batch[1].MaxQtyPerOrder != 0 {
		t.Errorf("batch[1] unexpected: %+v", batch[1])
	}
	// Verify registrar was invoked for both followers
	if len(reg.registered) != 2 {
		t.Fatalf("registered followers count = %d, want 2", len(reg.registered))
	}
	if reg.registered[0] != f1 || reg.registered[1] != f2 {
		t.Errorf("registered order mismatch: %+v", reg.registered)
	}
}

func TestPostGroupFollowers_BatchObjectSuccess(t *testing.T) {
	master := uuid.New()
	f1 := uuid.New()
	f2 := uuid.New()
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{
			master: "master",
			f1:     "follower",
			f2:     "follower",
		},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{
		"followers": []map[string]any{
			{"accountId": f1.String(), "cloneFactor": "1.0"},
			{"accountId": f2.String(), "cloneFactor": "2.0"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+master.String()+"/followers", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(store.createFollowLinksBatchArgs) != 1 || len(store.createFollowLinksBatchArgs[0]) != 2 {
		t.Fatalf("expected 1 batch call with 2 followers, got %+v", store.createFollowLinksBatchArgs)
	}
}

func TestPostGroupFollowers_BatchValidationErrors(t *testing.T) {
	master := uuid.New()
	f1 := uuid.New()
	f2 := uuid.New()
	nonFollower := uuid.New()
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{
			master:      "master",
			f1:          "follower",
			f2:          "follower",
			nonFollower: "master",
		},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	tests := []struct {
		name       string
		body       any
		wantStatus int
	}{
		{
			name: "duplicate in batch",
			body: []map[string]any{
				{"accountId": f1.String(), "cloneFactor": "1.0"},
				{"accountId": f1.String(), "cloneFactor": "1.0"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "invalid cloneFactor zero",
			body: []map[string]any{
				{"accountId": f1.String(), "cloneFactor": "0"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "invalid cloneFactor negative",
			body: []map[string]any{
				{"accountId": f1.String(), "cloneFactor": "-1.5"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "non-follower account",
			body: []map[string]any{
				{"accountId": f1.String(), "cloneFactor": "1.0"},
				{"accountId": nonFollower.String(), "cloneFactor": "1.0"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty batch",
			body:       map[string]any{"followers": []any{}},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+master.String()+"/followers", bytes.NewReader(body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestPostGroupFollowers_BatchConflict_Returns409(t *testing.T) {
	master := uuid.New()
	f1 := uuid.New()
	f2 := uuid.New()
	store := &stubStore{
		accountRoles: map[uuid.UUID]string{
			master: "master",
			f1:     "follower",
			f2:     "follower",
		},
		createFollowLinksBatchErr: domain.ErrDuplicate,
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal([]map[string]any{
		{"accountId": f1.String(), "cloneFactor": "1.0"},
		{"accountId": f2.String(), "cloneFactor": "1.0"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+master.String()+"/followers", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
}

func TestPostGroup_Succeeds(t *testing.T) {
	master := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{master: "master"}}
	tickerMgr := &stubTickerManager{}
	r := httpapi.NewRouter(store, &stubActionEngine{}, httpapi.WithTickerManager(tickerMgr))

	body, _ := json.Marshal(map[string]any{"name": "Alpha Group", "masterId": master.String()})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["name"] != "Alpha Group" || got["masterId"] != master.String() {
		t.Errorf("got = %+v", got)
	}
	if len(tickerMgr.started) != 1 || tickerMgr.started[0] != master {
		t.Errorf("expected tickerMgr.started with %v, got %v", master, tickerMgr.started)
	}
}

func TestPatchGroup_SwapMaster_Succeeds(t *testing.T) {
	newMaster := uuid.New()
	groupID := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{newMaster: "master"}}
	tickerMgr := &stubTickerManager{}
	r := httpapi.NewRouter(store, &stubActionEngine{}, httpapi.WithTickerManager(tickerMgr))

	body, _ := json.Marshal(map[string]any{"masterId": newMaster.String(), "name": "Renamed Group"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/groups/"+groupID.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(tickerMgr.started) != 1 || tickerMgr.started[0] != newMaster {
		t.Errorf("expected tickerMgr.started with %v, got %v", newMaster, tickerMgr.started)
	}
}

func TestPostGroup_MasterAlreadyAssigned_Returns409(t *testing.T) {
	master := uuid.New()
	store := &stubStore{
		accountRoles:   map[uuid.UUID]string{master: "master"},
		createGroupErr: domain.ErrDuplicate,
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"name": "Duplicate Group", "masterId": master.String()})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
	var got map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["error"] != "master account is already assigned to a group" {
		t.Errorf("error = %q, want %q", got["error"], "master account is already assigned to a group")
	}
}

func TestPatchGroup_SwapMaster_CurrentMasterHasOpenPositions_Returns409(t *testing.T) {
	newMaster := uuid.New()
	groupID := uuid.New()
	store := &stubStore{
		accountRoles:   map[uuid.UUID]string{newMaster: "master"},
		updateGroupErr: domain.ErrMasterHasOpenPositions,
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"masterId": newMaster.String()})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/groups/"+groupID.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
	var got map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["error"] != "cannot swap master: master has open positions" {
		t.Errorf("error = %q, want %q", got["error"], "cannot swap master: master has open positions")
	}
}

func TestPatchGroup_SwapMaster_NewMasterAlreadyAssigned_Returns409(t *testing.T) {
	newMaster := uuid.New()
	groupID := uuid.New()
	store := &stubStore{
		accountRoles:   map[uuid.UUID]string{newMaster: "master"},
		updateGroupErr: domain.ErrDuplicate,
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"masterId": newMaster.String()})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/groups/"+groupID.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
	var got map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["error"] != "new master account is already assigned to another group" {
		t.Errorf("error = %q, want %q", got["error"], "new master account is already assigned to another group")
	}
}

func TestDeleteGroup_Default_ReleasesAccounts(t *testing.T) {
	groupID := uuid.New()
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/"+groupID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", w.Code, w.Body.String())
	}
	if len(store.deleteGroupCalls) != 1 {
		t.Fatalf("deleteGroupCalls = %d, want 1", len(store.deleteGroupCalls))
	}
	if store.deleteGroupCalls[0].id != groupID {
		t.Errorf("deleteGroupCalls[0].id = %v, want %v", store.deleteGroupCalls[0].id, groupID)
	}
	if store.deleteGroupCalls[0].deleteAccounts != false {
		t.Errorf("deleteGroupCalls[0].deleteAccounts = %v, want false", store.deleteGroupCalls[0].deleteAccounts)
	}
}

func TestDeleteGroup_WithContainedAccounts_True(t *testing.T) {
	groupID := uuid.New()
	acc1 := uuid.New()
	acc2 := uuid.New()
	store := &stubStore{
		deleteGroupDeletedIDs: []uuid.UUID{acc1, acc2},
	}
	brokerEvicter := &stubBrokerEvicter{}
	tickerMgr := &stubTickerManager{}
	followerReg := &stubFollowerRegistrar{}

	r := httpapi.NewRouter(store, &stubActionEngine{},
		httpapi.WithBrokerEvicter(brokerEvicter),
		httpapi.WithTickerManager(tickerMgr),
		httpapi.WithFollowerRegistrar(followerReg),
	)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/"+groupID.String()+"?delete_accounts=true", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", w.Code, w.Body.String())
	}
	if len(store.deleteGroupCalls) != 1 {
		t.Fatalf("deleteGroupCalls = %d, want 1", len(store.deleteGroupCalls))
	}
	if store.deleteGroupCalls[0].id != groupID {
		t.Errorf("deleteGroupCalls[0].id = %v, want %v", store.deleteGroupCalls[0].id, groupID)
	}
	if store.deleteGroupCalls[0].deleteAccounts != true {
		t.Errorf("deleteGroupCalls[0].deleteAccounts = %v, want true", store.deleteGroupCalls[0].deleteAccounts)
	}
	// Check broker eviction, ticker stopping, and worker unregistration
	if len(brokerEvicter.evicted) != 2 {
		t.Errorf("brokerEvicter.evicted = %v, want 2 accounts", brokerEvicter.evicted)
	}
	if len(tickerMgr.stopped) != 2 {
		t.Errorf("tickerMgr.stopped = %v, want 2 accounts", tickerMgr.stopped)
	}
	if len(followerReg.unregistered) != 2 {
		t.Errorf("followerReg.unregistered = %v, want 2 accounts", followerReg.unregistered)
	}
}

func TestDeleteGroup_NotFound_Returns404(t *testing.T) {
	groupID := uuid.New()
	store := &stubStore{
		deleteGroupErr: domain.ErrNotFound,
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/"+groupID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteGroup_InvalidUUID_Returns400(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/not-a-valid-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}
