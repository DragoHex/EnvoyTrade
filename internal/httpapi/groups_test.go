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
			{MasterID: master, MasterAccountID: "ZX1234", Broker: "zerodha", FollowerCount: 2, Status: "ok"},
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
			MasterNetQty:               100,
			MasterOpenPositionsCount:   2,
			MasterClosedPositionsCount: 1,
			MasterOpenOrdersCount:      3,
			MasterTotalMtm:             decimal.NewFromFloat(1500.50),
			MasterAvailableCash:        &cash,
			MasterAvailableMargin:      &margin,
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
	if got["masterOpenPositionsCount"] != float64(2) {
		t.Errorf("masterOpenPositionsCount = %v, want 2", got["masterOpenPositionsCount"])
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

	body, _ := json.Marshal(map[string]any{"accountId": follower.String(), "capitalRatio": "0.5", "maxQtyPerOrder": 10})
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

	body, _ := json.Marshal(map[string]any{"accountId": follower.String(), "capitalRatio": "0.5"})
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

	body, _ := json.Marshal(map[string]any{"accountId": otherMaster.String(), "capitalRatio": "0.5"})
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

	body, _ := json.Marshal(map[string]any{"accountId": follower.String(), "capitalRatio": "0.5"})
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
	r := httpapi.NewRouter(store, &stubActionEngine{})

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
}

func TestPatchGroup_SwapMaster_Succeeds(t *testing.T) {
	newMaster := uuid.New()
	groupID := uuid.New()
	store := &stubStore{accountRoles: map[uuid.UUID]string{newMaster: "master"}}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	body, _ := json.Marshal(map[string]any{"masterId": newMaster.String(), "name": "Renamed Group"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/groups/"+groupID.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
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
