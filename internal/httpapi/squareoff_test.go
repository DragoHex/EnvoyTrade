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
)

type fakeSquareOffService struct {
	groupResult   domain.SquareOffResult
	groupErr      error
	accountResult domain.SquareOffResult
	accountErr    error
	lastSymbols   []string
	lastGroupID   uuid.UUID
	lastAccountID uuid.UUID
}

func (f *fakeSquareOffService) SquareOffGroup(ctx context.Context, groupID uuid.UUID, symbols []string) (domain.SquareOffResult, error) {
	f.lastGroupID = groupID
	f.lastSymbols = symbols
	return f.groupResult, f.groupErr
}

func (f *fakeSquareOffService) SquareOffAccount(ctx context.Context, accountID uuid.UUID, symbols []string) (domain.SquareOffResult, error) {
	f.lastAccountID = accountID
	f.lastSymbols = symbols
	return f.accountResult, f.accountErr
}

func TestPostGroupSquareOff_Success(t *testing.T) {
	groupID := uuid.New()
	masterID := uuid.New()

	sqSvc := &fakeSquareOffService{
		groupResult: domain.SquareOffResult{
			Action:              "square_off",
			Status:              "completed",
			GroupID:             &groupID,
			AccountID:           &masterID,
			Role:                "master",
			FollowersAffected:   2,
			PositionsSquaredOff: 3,
		},
	}

	store := &stubStore{}
	engine := &stubActionEngine{}
	router := httpapi.NewRouter(store, engine, httpapi.WithSquareOffService(sqSvc))

	body := []byte(`{"symbols":["CRUDEOIL17SEP26C10600"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+groupID.String()+"/positions/square-off", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var res domain.SquareOffResult
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Status != "completed" || res.PositionsSquaredOff != 3 {
		t.Errorf("unexpected result: %+v", res)
	}
	if len(sqSvc.lastSymbols) != 1 || sqSvc.lastSymbols[0] != "CRUDEOIL17SEP26C10600" {
		t.Errorf("expected symbol filter passed through, got %v", sqSvc.lastSymbols)
	}
}

func TestPostAccountSquareOff_Success(t *testing.T) {
	accID := uuid.New()

	sqSvc := &fakeSquareOffService{
		accountResult: domain.SquareOffResult{
			Action:              "square_off",
			Status:              "completed",
			AccountID:           &accID,
			Role:                "follower",
			FollowersAffected:   1,
			PositionsSquaredOff: 1,
		},
	}

	store := &stubStore{}
	engine := &stubActionEngine{}
	router := httpapi.NewRouter(store, engine, httpapi.WithSquareOffService(sqSvc))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/"+accID.String()+"/positions/square-off", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var res domain.SquareOffResult
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Status != "completed" || res.PositionsSquaredOff != 1 {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestPostGroupSquareOff_NotFound(t *testing.T) {
	groupID := uuid.New()
	sqSvc := &fakeSquareOffService{
		groupErr: domain.ErrNotFound,
	}

	store := &stubStore{}
	engine := &stubActionEngine{}
	router := httpapi.NewRouter(store, engine, httpapi.WithSquareOffService(sqSvc))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+groupID.String()+"/positions/square-off", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", rec.Code)
	}
}

func TestPostAccountSquareOff_InvalidUUID(t *testing.T) {
	sqSvc := &fakeSquareOffService{}
	store := &stubStore{}
	engine := &stubActionEngine{}
	router := httpapi.NewRouter(store, engine, httpapi.WithSquareOffService(sqSvc))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/not-a-uuid/positions/square-off", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
	}
}

func TestPostAccountSquareOff_DisabledAccount(t *testing.T) {
	accID := uuid.New()
	sqSvc := &fakeSquareOffService{
		accountErr: domain.ErrAccountDisabled,
	}

	store := &stubStore{}
	engine := &stubActionEngine{}
	router := httpapi.NewRouter(store, engine, httpapi.WithSquareOffService(sqSvc))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/"+accID.String()+"/positions/square-off", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
	}
}

