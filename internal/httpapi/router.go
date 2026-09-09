// Package httpapi is the stdlib net/http admin API backing the Dashboard
// page (docs/UI-PLAN.md). Interfaces are declared here, the consumer,
// satisfied structurally by *postgres.Store and *engine.Engine — same
// seam rule as the rest of the repo (AGENTS.md "The seam rule").
package httpapi

import (
	"encoding/json"
	"net/http"
)

// NewRouter wires every Dashboard-page route onto a stdlib ServeMux using
// Go 1.22+ method+path pattern routing.
func NewRouter(store Store, actionEngine Engine) *http.ServeMux {
	mux := http.NewServeMux()
	h := &handlers{store: store, engine: actionEngine}

	mux.HandleFunc("GET /api/v1/groups", h.getGroups)
	mux.HandleFunc("GET /api/v1/groups/{masterId}", h.getGroupDetail)
	mux.HandleFunc("POST /api/v1/groups/{masterId}/followers", h.postGroupFollower)
	mux.HandleFunc("GET /api/v1/accounts", h.getAccounts)
	mux.HandleFunc("POST /api/v1/accounts", h.postAccount)
	mux.HandleFunc("PATCH /api/v1/accounts/{id}", h.patchAccount)
	mux.HandleFunc("DELETE /api/v1/accounts/{id}", h.deleteAccount)
	mux.HandleFunc("DELETE /api/v1/accounts/{id}/group", h.deleteAccountGroup)
	mux.HandleFunc("POST /api/v1/accounts/{id}/actions", h.postAction)

	return mux
}

// Store is everything the handlers need from persistence — the union of
// GroupsStore/AccountsStore/ActionsStore, kept as one interface since one
// concrete *postgres.Store always satisfies all three.
type Store interface {
	GroupsStore
	AccountsStore
	ActionsStore
}

type handlers struct {
	store  Store
	engine Engine
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
