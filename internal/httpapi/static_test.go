package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"envoytrade/internal/httpapi"
)

func TestRouter_WithStaticFS_ServesFilesAndSPAFallback(t *testing.T) {
	mockFS := fstest.MapFS{
		"index.html":       {Data: []byte("<!DOCTYPE html><html><body>Root App</body></html>")},
		"assets/style.css": {Data: []byte("body { background: #000; }")},
	}

	router := httpapi.NewRouter(
		&stubStore{},
		&stubActionEngine{},
		httpapi.WithStaticFS(mockFS),
	)

	// 1. Root path "/" should serve index.html
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / returned status %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "<!DOCTYPE html><html><body>Root App</body></html>" {
		t.Fatalf("GET / returned %q, want index.html content", rec.Body.String())
	}

	// 2. Exact asset path "/assets/style.css" should serve asset
	req = httptest.NewRequest(http.MethodGet, "/assets/style.css", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /assets/style.css returned status %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "body { background: #000; }" {
		t.Fatalf("GET /assets/style.css returned %q", rec.Body.String())
	}

	// 3. SPA route "/groups" should fall back to index.html with 200 OK
	req = httptest.NewRequest(http.MethodGet, "/groups", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /groups returned status %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "<!DOCTYPE html><html><body>Root App</body></html>" {
		t.Fatalf("GET /groups returned %q, want index.html fallback", rec.Body.String())
	}

	// 4. API routes should NOT be intercepted by SPA fallback
	req = httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	// API route returns 200 JSON list, not html
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected application/json for API route, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestRouter_WithoutStaticFS_DoesNotMountFrontend(t *testing.T) {
	router := httpapi.NewRouter(
		&stubStore{},
		&stubActionEngine{},
	)

	req := httptest.NewRequest(http.MethodGet, "/some-spa-page", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown route without static FS, got %d", rec.Code)
	}
}
