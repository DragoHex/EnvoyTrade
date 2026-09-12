package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"envoytrade/internal/domain"
	"envoytrade/internal/httpapi"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestGetAccountOrders_EmptyData(t *testing.T) {
	accID := uuid.New()
	store := &stubStore{
		ordersDetail: domain.AccountOrdersDetail{
			AccountID:       accID,
			Role:            "master",
			BrokerAccountID: "MASTER01",
			Summary: domain.AccountSummaryMetrics{
				NetQty:               0,
				OpenPositionsCount:   0,
				ClosedPositionsCount: 0,
				PendingOrdersCount:   0,
				TotalMtm:             decimal.Zero,
				RealizedPnl:          decimal.Zero,
				AccountValue:         decimal.Zero,
				Status:               "offline",
			},
			Counts: domain.TabCounts{},
			Pagination: domain.PaginationInfo{
				Tab:        "open_positions",
				Page:       1,
				Limit:      10,
				TotalCount: 0,
				TotalPages: 0,
			},
			OpenPositions:   []domain.PositionItem{},
			ClosedPositions: []domain.PositionItem{},
			Holdings:        []domain.HoldingItem{},
			OpenOrders:      []domain.OrderDetailItem{},
			ClosedOrders:    []domain.OrderDetailItem{},
			RejectedOrders:  []domain.OrderDetailItem{},
		},
	}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+accID.String()+"/orders", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var detail domain.AccountOrdersDetail
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if detail.AccountID != accID {
		t.Errorf("accountID = %v, want %v", detail.AccountID, accID)
	}
	if len(detail.OpenPositions) != 0 {
		t.Errorf("openPositions length = %d, want 0", len(detail.OpenPositions))
	}
	if len(detail.ClosedPositions) != 0 {
		t.Errorf("closedPositions length = %d, want 0", len(detail.ClosedPositions))
	}
	if len(detail.Holdings) != 0 {
		t.Errorf("holdings length = %d, want 0", len(detail.Holdings))
	}
	if len(detail.OpenOrders) != 0 {
		t.Errorf("openOrders length = %d, want 0", len(detail.OpenOrders))
	}
	if detail.Summary.NetQty != 0 {
		t.Errorf("summary.NetQty = %d, want 0", detail.Summary.NetQty)
	}
	if !detail.Summary.TotalMtm.IsZero() {
		t.Errorf("summary.TotalMtm = %v, want 0", detail.Summary.TotalMtm)
	}
	if !detail.Summary.AccountValue.IsZero() {
		t.Errorf("summary.AccountValue = %v, want 0", detail.Summary.AccountValue)
	}
}

func TestGetAccountOrders_PassesQueryParams(t *testing.T) {
	accID := uuid.New()
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+accID.String()+"/orders?tab=holdings&page=2&limit=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if store.ordersDetailTab != "holdings" {
		t.Errorf("tab = %q, want 'holdings'", store.ordersDetailTab)
	}
	if store.ordersDetailPage != 2 {
		t.Errorf("page = %d, want 2", store.ordersDetailPage)
	}
	if store.ordersDetailLimit != 5 {
		t.Errorf("limit = %d, want 5", store.ordersDetailLimit)
	}
}

func TestGetAccountOrders_NotFound(t *testing.T) {
	accID := uuid.New()
	store := &stubStore{ordersDetailErr: domain.ErrNotFound}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+accID.String()+"/orders", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestGetAccountOrders_InvalidUUID(t *testing.T) {
	store := &stubStore{}
	r := httpapi.NewRouter(store, &stubActionEngine{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/not-a-uuid/orders", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}
