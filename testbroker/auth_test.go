package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseKiteAuthHeader(t *testing.T) {
	tests := []struct {
		header      string
		apiKey      string
		accessToken string
		ok          bool
	}{
		{"token api_key1:access_token1", "api_key1", "access_token1", true},
		{"token api_key2:access_token2", "api_key2", "access_token2", true},
		{"token invalid_format", "", "", false},
		{"invalid api_key:access_token", "", "", false},
		{"", "", "", false},
		{"token :access_token", "", "access_token", true}, 
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			apiKey, accessToken, ok := parseKiteAuth(tt.header)
			if apiKey != tt.apiKey || accessToken != tt.accessToken || ok != tt.ok {
				t.Errorf("parseKiteAuth(%q) = %v, %v, %v; want %v, %v, %v", tt.header, apiKey, accessToken, ok, tt.apiKey, tt.accessToken, tt.ok)
			}
		})
	}
}

func TestAuthMiddleware(t *testing.T) {
	cfg := &Config{
		tokenIndex: map[string]string{
			"valid_key:valid_token": "USER1",
		},
	}

	handler := AuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		if userID != "USER1" {
			t.Errorf("Expected userID USER1, got %s", userID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Valid Token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/some/path", nil)
		req.Header.Set("Authorization", "token valid_key:valid_token")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rr.Code)
		}
	})

	t.Run("Invalid Token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/some/path", nil)
		req.Header.Set("Authorization", "token invalid_key:invalid_token")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", rr.Code)
		}

		var resp map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response JSON: %v", err)
		}
		if resp["status"] != "error" || resp["error_type"] != "TokenException" {
			t.Errorf("Unexpected response: %v", resp)
		}
	})

	t.Run("Missing Header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/some/path", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", rr.Code)
		}
	})
}

func TestAuthMiddleware_SkipsUIPath(t *testing.T) {
	cfg := &Config{}
	handler := AuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	paths := []string{"/ui", "/ui/", "/ui/dashboard", "/api", "/api/", "/api/users"}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest("GET", p, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected path %s to skip auth and return 200, got %d", p, rr.Code)
			}
		})
	}
}
