package httpapi_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"envoytrade/internal/httpapi"
)

type dummyPostbackHandler struct {
	called bool
}

func (d *dummyPostbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.called = true
	w.WriteHeader(http.StatusOK)
}

func TestRouter_WithPostbackHandler_MountsBrokerCallback(t *testing.T) {
	postback := &dummyPostbackHandler{}
	router := httpapi.NewRouter(&stubStore{}, &stubActionEngine{}, httpapi.WithPostbackHandler(postback))

	req := httptest.NewRequest(http.MethodPost, "/broker-callback", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if !postback.called {
		t.Fatal("postback handler was not called")
	}
}

func TestRouter_WithLogger_EmitsAccessLogs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	router := httpapi.NewRouter(&stubStore{}, &stubActionEngine{}, httpapi.WithLogger(logger))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	logged := buf.String()
	if !strings.Contains(logged, "http request completed") {
		t.Errorf("expected access log, got: %s", logged)
	}
	if !strings.Contains(logged, `"/api/v1/groups"`) && !strings.Contains(logged, `"path":"/api/v1/groups"`) {
		t.Errorf("expected path in log, got: %s", logged)
	}
	if !strings.Contains(logged, `"status":200`) {
		t.Errorf("expected status 200 in log, got: %s", logged)
	}
}
