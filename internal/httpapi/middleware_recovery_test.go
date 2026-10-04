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

type panickingPostbackHandler struct{}

func (p *panickingPostbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	panic("unexpected nil pointer or exception")
}

func TestRouter_RecoveryMiddleware_RecoversPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	router := httpapi.NewRouter(
		&stubStore{},
		&stubActionEngine{},
		httpapi.WithLogger(logger),
		httpapi.WithPostbackHandler(&panickingPostbackHandler{}),
	)

	req := httptest.NewRequest(http.MethodPost, "/broker-callback", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	// Must not crash or panic
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "internal server error") {
		t.Fatalf("expected error json, got %s", rec.Body.String())
	}

	logs := buf.String()
	if !strings.Contains(logs, "panic recovered") {
		t.Fatalf("expected panic recovered log, got: %s", logs)
	}
}
