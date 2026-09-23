package kite_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"envoytrade/internal/kite"
)

func TestHeadlessLogin_SuccessFlow(t *testing.T) {
	totpSecret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	expectedUserID := "TEST01"
	expectedPassword := "securePass123"
	expectedAPIKey := "test_api_key"
	expectedAPISecret := "test_api_secret"
	mockRequestID := "mock_req_12345"
	mockRequestToken := "mock_req_token_67890"
	mockAccessToken := "mock_access_token_abcde"

	// Mock server handling all steps
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/login":
			if r.Method != http.MethodPost {
				http.Error(w, "bad method", http.StatusMethodNotAllowed)
				return
			}
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if r.FormValue("user_id") != expectedUserID || r.FormValue("password") != expectedPassword {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status":  "error",
					"message": "Invalid user_id or password",
				})
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"request_id": mockRequestID,
				},
			})

		case "/api/twofa":
			if r.Method != http.MethodPost {
				http.Error(w, "bad method", http.StatusMethodNotAllowed)
				return
			}
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if r.FormValue("request_id") != mockRequestID || r.FormValue("user_id") != expectedUserID {
				http.Error(w, "invalid request_id or user_id", http.StatusBadRequest)
				return
			}
			// Verify TOTP
			totp := r.FormValue("twofa_value")
			expectedTOTP, _ := kite.GenerateTOTP(totpSecret, time.Now())
			if totp != expectedTOTP {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status":  "error",
					"message": "Invalid 2FA code",
				})
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "enctoken", Value: "valid_session_token", Path: "/"})
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"user_id": expectedUserID,
				},
			})

		case "/connect/login":
			if r.Method != http.MethodGet {
				http.Error(w, "bad method", http.StatusMethodNotAllowed)
				return
			}
			cookie, err := r.Cookie("enctoken")
			if err != nil || cookie.Value != "valid_session_token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			apiKey := r.URL.Query().Get("api_key")
			if apiKey != expectedAPIKey {
				http.Error(w, "invalid api_key", http.StatusBadRequest)
				return
			}
			// Redirect with request_token
			redirectURL := "http://127.0.0.1:8080/redirect?action=login&status=success&request_token=" + mockRequestToken
			http.Redirect(w, r, redirectURL, http.StatusFound)

		case "/session/token":
			if r.Method != http.MethodPost {
				http.Error(w, "bad method", http.StatusMethodNotAllowed)
				return
			}
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if r.FormValue("api_key") != expectedAPIKey || r.FormValue("request_token") != mockRequestToken {
				http.Error(w, "invalid api_key or request_token", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"access_token": mockAccessToken,
					"user_id":      expectedUserID,
				},
			})

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := kite.LoginConfig{
		KiteWebBaseURL: server.URL,
		KiteAPIBaseURL: server.URL,
	}

	token, err := kite.HeadlessLoginWithConfig(
		context.Background(),
		nil,
		expectedUserID,
		expectedPassword,
		totpSecret,
		expectedAPIKey,
		expectedAPISecret,
		cfg,
	)
	if err != nil {
		t.Fatalf("HeadlessLogin failed: %v", err)
	}
	if token != mockAccessToken {
		t.Errorf("got access_token %q, want %q", token, mockAccessToken)
	}
}

func TestHeadlessLogin_BadCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "error",
			"message": "Invalid password",
		})
	}))
	defer server.Close()

	cfg := kite.LoginConfig{
		KiteWebBaseURL: server.URL,
		KiteAPIBaseURL: server.URL,
	}

	_, err := kite.HeadlessLoginWithConfig(
		context.Background(),
		nil,
		"user",
		"wrongpass",
		"GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
		"k",
		"s",
		cfg,
	)
	if err == nil {
		t.Fatalf("expected error on bad password, got nil")
	}
}

func TestHeadlessLogin_MultiStepRedirect_ExtractsRequestToken(t *testing.T) {
	totpSecret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	expectedUserID := "TEST01"
	expectedPassword := "securePass123"
	expectedAPIKey := "test_api_key"
	expectedAPISecret := "test_api_secret"
	mockRequestID := "mock_req_12345"
	mockRequestToken := "mock_req_token_multi"
	mockAccessToken := "mock_access_token_multi"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"request_id": mockRequestID},
			})
		case "/api/twofa":
			http.SetCookie(w, &http.Cookie{Name: "enctoken", Value: "valid_session", Path: "/"})
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"user_id": expectedUserID},
			})
		case "/connect/login":
			// First redirect to /connect/finish
			http.Redirect(w, r, "/connect/finish?api_key="+expectedAPIKey, http.StatusFound)
		case "/connect/finish":
			// Second redirect to external callback with request_token
			http.Redirect(w, r, "http://127.0.0.1:8080/callback?action=login&status=success&request_token="+mockRequestToken, http.StatusFound)
		case "/session/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"access_token": mockAccessToken, "user_id": expectedUserID},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := kite.LoginConfig{
		KiteWebBaseURL: server.URL,
		KiteAPIBaseURL: server.URL,
	}

	token, err := kite.HeadlessLoginWithConfig(
		context.Background(),
		nil,
		expectedUserID,
		expectedPassword,
		totpSecret,
		expectedAPIKey,
		expectedAPISecret,
		cfg,
	)
	if err != nil {
		t.Fatalf("HeadlessLogin multi-step redirect failed: %v", err)
	}
	if token != mockAccessToken {
		t.Errorf("got access_token %q, want %q", token, mockAccessToken)
	}
}

func TestHeadlessLogin_ConnectLoginInvalidKey_ReturnsCleanError(t *testing.T) {
	totpSecret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"request_id": "req_1"},
			})
		case "/api/twofa":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"user_id": "user1"},
			})
		case "/connect/login":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "error",
				"message": "Invalid api_key.",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := kite.LoginConfig{
		KiteWebBaseURL: server.URL,
		KiteAPIBaseURL: server.URL,
	}

	_, err := kite.HeadlessLoginWithConfig(
		context.Background(),
		nil,
		"user1",
		"pass",
		totpSecret,
		"invalid_api_key",
		"secret",
		cfg,
	)
	if err == nil {
		t.Fatalf("expected error on invalid api key, got nil")
	}
	if !strings.Contains(err.Error(), "Invalid api_key.") {
		t.Errorf("expected error to contain 'Invalid api_key.', got: %v", err)
	}
}

func TestHeadlessLogin_ConnectLoginHTMLBody_ExtractsRequestToken(t *testing.T) {
	totpSecret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	expectedUserID := "TEST01"
	expectedPassword := "securePass123"
	expectedAPIKey := "test_api_key"
	expectedAPISecret := "test_api_secret"
	mockRequestID := "mock_req_12345"
	mockRequestToken := "mock_req_token_html_body"
	mockAccessToken := "mock_access_token_html_body"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"request_id": mockRequestID},
			})
		case "/api/twofa":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"user_id": expectedUserID},
			})
		case "/connect/login":
			// HTTP 200 with HTML/script redirect
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><script>window.location="http://127.0.0.1:8080/callback?action=login&status=success&request_token=` + mockRequestToken + `";</script></body></html>`))
		case "/session/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"access_token": mockAccessToken, "user_id": expectedUserID},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := kite.LoginConfig{
		KiteWebBaseURL: server.URL,
		KiteAPIBaseURL: server.URL,
	}

	token, err := kite.HeadlessLoginWithConfig(
		context.Background(),
		nil,
		expectedUserID,
		expectedPassword,
		totpSecret,
		expectedAPIKey,
		expectedAPISecret,
		cfg,
	)
	if err != nil {
		t.Fatalf("HeadlessLogin HTML body extraction failed: %v", err)
	}
	if token != mockAccessToken {
		t.Errorf("got access_token %q, want %q", token, mockAccessToken)
	}
}
