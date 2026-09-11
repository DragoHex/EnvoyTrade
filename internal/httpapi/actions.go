package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

const (
	actionRebalance      = "rebalance"
	actionSquareOff      = "square_off"
	actionExitOpenOrders = "exit_open_orders"
)

// ActionsStore is what POST /accounts/{id}/actions needs to run
// "rebalance" — resolve the account's master (works from a follower row
// or the master row itself), then its latest fill.
type ActionsStore interface {
	ResolveMasterID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	LatestMasterFill(ctx context.Context, masterID uuid.UUID) (domain.MasterFill, error)
}

// Engine re-runs fan-out for a master fill — already idempotent
// (domain.ErrDuplicate no-ops a redelivery), which is what makes
// "rebalance" safe to click more than once (docs/APIs/actions.md).
type Engine interface {
	HandleMasterFill(ctx context.Context, fill domain.MasterFill) error
}

type postActionRequest struct {
	Type string `json:"type"`
}

func (h *handlers) postAction(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	var req postActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON body")
		return
	}

	switch req.Type {
	case actionRebalance:
		h.rebalance(w, r, id)
	case actionSquareOff:
		writeError(w, http.StatusNotImplemented, "square_off is not implemented yet")
	case actionExitOpenOrders:
		writeError(w, http.StatusNotImplemented, "exit_open_orders is not implemented yet")
	default:
		writeError(w, http.StatusBadRequest, "unknown action type")
	}
}

func (h *handlers) rebalance(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	ctx := r.Context()

	masterID, err := h.store.ResolveMasterID(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve master")
		return
	}

	fill, err := h.store.LatestMasterFill(ctx, masterID)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no master fill to rebalance from")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load latest master fill")
		return
	}

	if err := h.engine.HandleMasterFill(ctx, fill); err != nil {
		writeError(w, http.StatusInternalServerError, "rebalance failed")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"type": actionRebalance, "status": "accepted"})
}
