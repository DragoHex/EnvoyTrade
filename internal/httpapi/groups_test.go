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
	store := &stubStore{
		detail: domain.GroupDetail{
			MasterID:        master,
			MasterAccountID: "ZX1234",
			Followers: []domain.GroupFollower{
				{AccountID: follower, BrokerAccountID: "ZY5678", Enabled: true, Status: "ok"},
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
