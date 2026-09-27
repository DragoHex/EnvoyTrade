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
	"envoytrade/internal/httpapi"

	"github.com/google/uuid"
)

func TestUpdateUserProfile(t *testing.T) {
	store := newFullStubStore()
	handler := httpapi.NewRouter(store, &stubActionEngine{})

	hash, _ := auth.HashPassword("Password123!")
	user1ID := uuid.New()
	_ = store.CreateUser(context.Background(), user1ID, "user1@example.com", "user1", "User One", hash, "trader")

	user2ID := uuid.New()
	_ = store.CreateUser(context.Background(), user2ID, "user2@example.com", "user2", "User Two", hash, "trader")

	plainToken1, tokenHash1, _ := auth.GenerateSessionToken()
	_ = store.CreateSession(context.Background(), tokenHash1, user1ID, "127.0.0.1", "test-agent", time.Now().Add(24*time.Hour))

	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		body := map[string]string{
			"email":    "new@example.com",
			"username": "newuser",
			"name":     "New Name",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/user/profile", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("successful profile update with valid phone and GSTIN", func(t *testing.T) {
		body := map[string]string{
			"email":     "user1_updated@example.com",
			"username":  "user1_updated",
			"name":      "User One Updated",
			"phone":     "+91 98765 43210",
			"address":   "123 MG Road, Bengaluru, Karnataka",
			"gstNumber": "29abcde1234f1z5",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/user/profile", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken1})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		userMap, ok := resp["user"].(map[string]any)
		if !ok {
			t.Fatalf("expected 'user' in response, got %v", resp)
		}
		if userMap["email"] != "user1_updated@example.com" {
			t.Errorf("expected updated email, got %v", userMap["email"])
		}
		if userMap["username"] != "user1_updated" {
			t.Errorf("expected updated username, got %v", userMap["username"])
		}
		if userMap["name"] != "User One Updated" {
			t.Errorf("expected updated name, got %v", userMap["name"])
		}
		if userMap["phone"] != "+919876543210" {
			t.Errorf("expected normalized phone '+919876543210', got %v", userMap["phone"])
		}
		if userMap["address"] != "123 MG Road, Bengaluru, Karnataka" {
			t.Errorf("expected updated address, got %v", userMap["address"])
		}
		if userMap["gstNumber"] != "29ABCDE1234F1Z5" {
			t.Errorf("expected uppercase GSTIN '29ABCDE1234F1Z5', got %v", userMap["gstNumber"])
		}
	})

	t.Run("empty email returns 400", func(t *testing.T) {
		body := map[string]string{
			"email":    "",
			"username": "user1_updated",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/user/profile", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken1})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid email returns 400", func(t *testing.T) {
		body := map[string]string{
			"email":    "not-an-email",
			"username": "user1_updated",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/user/profile", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken1})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid phone number returns 400", func(t *testing.T) {
		body := map[string]string{
			"email":    "user1_updated@example.com",
			"username": "user1_updated",
			"phone":    "12345",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/user/profile", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken1})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid GSTIN returns 400", func(t *testing.T) {
		body := map[string]string{
			"email":     "user1_updated@example.com",
			"username":  "user1_updated",
			"gstNumber": "INVALID_GSTIN_123",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/user/profile", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken1})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("duplicate email returns 409", func(t *testing.T) {
		body := map[string]string{
			"email":    "user2@example.com", // taken by user2
			"username": "user1_updated",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/user/profile", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken1})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("duplicate username returns 409", func(t *testing.T) {
		body := map[string]string{
			"email":    "user1_updated@example.com",
			"username": "user2", // taken by user2
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/user/profile", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken1})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestUpdateUserPassword(t *testing.T) {
	store := newFullStubStore()
	handler := httpapi.NewRouter(store, &stubActionEngine{})

	hash, _ := auth.HashPassword("OldSecretPass123!")
	userID := uuid.New()
	_ = store.CreateUser(context.Background(), userID, "trader@example.com", "trader", "Trader", hash, "trader")

	plainToken1, tokenHash1, _ := auth.GenerateSessionToken()
	_ = store.CreateSession(context.Background(), tokenHash1, userID, "127.0.0.1", "device1", time.Now().Add(24*time.Hour))

	plainToken2, tokenHash2, _ := auth.GenerateSessionToken()
	_ = store.CreateSession(context.Background(), tokenHash2, userID, "127.0.0.2", "device2", time.Now().Add(24*time.Hour))

	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		body := map[string]string{
			"oldPassword": "OldSecretPass123!",
			"newPassword": "NewSecretPass456!",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("missing fields returns 400", func(t *testing.T) {
		body := map[string]string{
			"oldPassword": "OldSecretPass123!",
			"newPassword": "",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken1})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("short new password returns 400", func(t *testing.T) {
		body := map[string]string{
			"oldPassword": "OldSecretPass123!",
			"newPassword": "short",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken1})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("incorrect old password returns 401", func(t *testing.T) {
		body := map[string]string{
			"oldPassword": "WrongOldPassword!",
			"newPassword": "NewSecretPass456!",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken1})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("successful password reset invalidates other sessions and allows new password login", func(t *testing.T) {
		body := map[string]string{
			"oldPassword": "OldSecretPass123!",
			"newPassword": "NewSecretPass456!",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken1})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		// Current session 1 still valid
		reqMe := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		reqMe.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken1})
		recMe := httptest.NewRecorder()
		handler.ServeHTTP(recMe, reqMe)
		if recMe.Code != http.StatusOK {
			t.Fatalf("expected current session to remain valid (200), got %d", recMe.Code)
		}

		// Other session 2 invalidated
		reqOther := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		reqOther.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken2})
		recOther := httptest.NewRecorder()
		handler.ServeHTTP(recOther, reqOther)
		if recOther.Code != http.StatusUnauthorized {
			t.Fatalf("expected other session to be invalidated (401), got %d", recOther.Code)
		}

		// Login with old password fails
		loginOld := map[string]string{
			"email":    "trader@example.com",
			"password": "OldSecretPass123!",
		}
		jsonOld, _ := json.Marshal(loginOld)
		reqLoginOld := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(jsonOld))
		reqLoginOld.Header.Set("Content-Type", "application/json")
		recLoginOld := httptest.NewRecorder()
		handler.ServeHTTP(recLoginOld, reqLoginOld)
		if recLoginOld.Code != http.StatusUnauthorized {
			t.Fatalf("expected old password login to fail (401), got %d", recLoginOld.Code)
		}

		// Login with new password succeeds
		loginNew := map[string]string{
			"email":    "trader@example.com",
			"password": "NewSecretPass456!",
		}
		jsonNew, _ := json.Marshal(loginNew)
		reqLoginNew := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(jsonNew))
		reqLoginNew.Header.Set("Content-Type", "application/json")
		recLoginNew := httptest.NewRecorder()
		handler.ServeHTTP(recLoginNew, reqLoginNew)
		if recLoginNew.Code != http.StatusOK {
			t.Fatalf("expected new password login to succeed (200), got %d", recLoginNew.Code)
		}
	})
}
