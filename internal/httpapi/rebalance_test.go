package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"envoytrade/internal/domain"
	"envoytrade/internal/httpapi"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type fakeRebalanceService struct {
	groupDiff          domain.GroupRebalanceDiff
	groupDiffErr       error
	accountDiff        domain.FollowerDrift
	accountDiffErr     error
	rebalanceGroupRes  domain.RebalanceResult
	rebalanceGroupErr  error
	rebalanceAccRes    domain.RebalanceResult
	rebalanceAccErr    error
	lastGroupID        uuid.UUID
	lastAccountID      uuid.UUID
	lastFollowerIDs    []uuid.UUID
}

func (f *fakeRebalanceService) ComputeGroupDiff(ctx context.Context, groupID uuid.UUID) (domain.GroupRebalanceDiff, error) {
	f.lastGroupID = groupID
	return f.groupDiff, f.groupDiffErr
}

func (f *fakeRebalanceService) ComputeAccountDiff(ctx context.Context, accountID uuid.UUID) (domain.FollowerDrift, error) {
	f.lastAccountID = accountID
	return f.accountDiff, f.accountDiffErr
}

func (f *fakeRebalanceService) RebalanceGroup(ctx context.Context, groupID uuid.UUID, followerIDs []uuid.UUID) (domain.RebalanceResult, error) {
	f.lastGroupID = groupID
	f.lastFollowerIDs = followerIDs
	return f.rebalanceGroupRes, f.rebalanceGroupErr
}

func (f *fakeRebalanceService) RebalanceAccount(ctx context.Context, accountID uuid.UUID) (domain.RebalanceResult, error) {
	f.lastAccountID = accountID
	return f.rebalanceAccRes, f.rebalanceAccErr
}

func TestGetGroupRebalanceDiff_Success(t *testing.T) {
	groupID := uuid.New()
	masterID := uuid.New()
	fID := uuid.New()

	rebSvc := &fakeRebalanceService{
		groupDiff: domain.GroupRebalanceDiff{
			GroupID:            groupID,
			MasterID:           masterID,
			FollowersEvaluated: 1,
			FollowersWithDrift: 1,
			Drifts: []domain.FollowerDrift{
				{
					AccountID:       fID,
					AccountName:     "Follower 1",
					BrokerAccountID: "BRK_F1",
					Enabled:         true,
					CloneFactor:     decimal.NewFromFloat(1.0),
					Symbols: []domain.SymbolDrift{
						{
							Exchange:      "NFO",
							Tradingsymbol: "NIFTY26OCTFUT",
							Product:       "NRML",
							LotSize:       75,
							MasterQty:     75,
							TargetQty:     75,
							FollowerQty:   0,
							DriftQty:      75,
							Action:        "BUY",
						},
					},
				},
			},
		},
	}

	store := &stubStore{}
	engine := &stubActionEngine{}
	router := httpapi.NewRouter(store, engine, httpapi.WithRebalanceService(rebSvc))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+groupID.String()+"/positions/rebalance/diff", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var res domain.GroupRebalanceDiff
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if res.FollowersWithDrift != 1 || len(res.Drifts) != 1 {
		t.Errorf("unexpected diff response: %+v", res)
	}
	if rebSvc.lastGroupID != groupID {
		t.Errorf("expected groupID %s, got %s", groupID, rebSvc.lastGroupID)
	}
}

func TestPostGroupRebalance_Success(t *testing.T) {
	groupID := uuid.New()
	fID := uuid.New()

	rebSvc := &fakeRebalanceService{
		rebalanceGroupRes: domain.RebalanceResult{
			Action:            "rebalance",
			Status:            "completed",
			GroupID:           &groupID,
			FollowersAffected: 1,
			OrdersPlaced:      1,
		},
	}

	store := &stubStore{}
	engine := &stubActionEngine{}
	router := httpapi.NewRouter(store, engine, httpapi.WithRebalanceService(rebSvc))

	body := []byte(`{"follower_ids":["` + fID.String() + `"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+groupID.String()+"/positions/rebalance", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var res domain.RebalanceResult
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if res.Status != "completed" || res.OrdersPlaced != 1 {
		t.Errorf("unexpected rebalance response: %+v", res)
	}
	if len(rebSvc.lastFollowerIDs) != 1 || rebSvc.lastFollowerIDs[0] != fID {
		t.Errorf("expected follower filter passed through, got %v", rebSvc.lastFollowerIDs)
	}
}

func TestGetAccountRebalanceDiff_Success(t *testing.T) {
	fID := uuid.New()

	rebSvc := &fakeRebalanceService{
		accountDiff: domain.FollowerDrift{
			AccountID:       fID,
			AccountName:     "Follower 1",
			BrokerAccountID: "BRK_F1",
			Enabled:         true,
			CloneFactor:     decimal.NewFromFloat(1.0),
			Symbols: []domain.SymbolDrift{
				{
					Exchange:      "NSE",
					Tradingsymbol: "RELIANCE",
					Action:        "SELL",
					DriftQty:      -10,
				},
			},
		},
	}

	store := &stubStore{}
	engine := &stubActionEngine{}
	router := httpapi.NewRouter(store, engine, httpapi.WithRebalanceService(rebSvc))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+fID.String()+"/positions/rebalance/diff", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var res domain.FollowerDrift
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if res.AccountID != fID || len(res.Symbols) != 1 {
		t.Errorf("unexpected account diff: %+v", res)
	}
}

func TestPostAccountRebalance_Success(t *testing.T) {
	fID := uuid.New()

	rebSvc := &fakeRebalanceService{
		rebalanceAccRes: domain.RebalanceResult{
			Action:            "rebalance",
			Status:            "completed",
			AccountID:         &fID,
			FollowersAffected: 1,
			OrdersPlaced:      1,
		},
	}

	store := &stubStore{}
	engine := &stubActionEngine{}
	router := httpapi.NewRouter(store, engine, httpapi.WithRebalanceService(rebSvc))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/"+fID.String()+"/positions/rebalance", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var res domain.RebalanceResult
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if res.Status != "completed" || res.OrdersPlaced != 1 {
		t.Errorf("unexpected result: %+v", res)
	}
	if rebSvc.lastAccountID != fID {
		t.Errorf("expected accountID %s, got %s", fID, rebSvc.lastAccountID)
	}
}

func TestGetGroupRebalanceDiff_BrokerUnreachable(t *testing.T) {
	groupID := uuid.New()
	rebSvc := &fakeRebalanceService{
		groupDiffErr: domain.ErrBrokerUnreachable,
	}

	store := &stubStore{}
	engine := &stubActionEngine{}
	router := httpapi.NewRouter(store, engine, httpapi.WithRebalanceService(rebSvc))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+groupID.String()+"/positions/rebalance/diff", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 Bad Gateway, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetGroupRebalanceDiff_AuthExpired(t *testing.T) {
	groupID := uuid.New()
	rebSvc := &fakeRebalanceService{
		groupDiffErr: domain.ErrAuthExpired,
	}

	store := &stubStore{}
	engine := &stubActionEngine{}
	router := httpapi.NewRouter(store, engine, httpapi.WithRebalanceService(rebSvc))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+groupID.String()+"/positions/rebalance/diff", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostGroupRebalance_BrokerUnreachable(t *testing.T) {
	groupID := uuid.New()
	rebSvc := &fakeRebalanceService{
		rebalanceGroupErr: domain.ErrBrokerUnreachable,
	}

	store := &stubStore{}
	engine := &stubActionEngine{}
	router := httpapi.NewRouter(store, engine, httpapi.WithRebalanceService(rebSvc))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+groupID.String()+"/positions/rebalance", bytes.NewBufferString("{}"))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 Bad Gateway, got %d: %s", rec.Code, rec.Body.String())
	}
}
