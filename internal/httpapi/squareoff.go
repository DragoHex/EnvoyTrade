package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

// SquareOffService declares what the square-off endpoints need.
type SquareOffService interface {
	SquareOffGroup(ctx context.Context, groupID uuid.UUID, symbols []string) (domain.SquareOffResult, error)
	SquareOffAccount(ctx context.Context, accountID uuid.UUID, symbols []string) (domain.SquareOffResult, error)
}

func (h *handlers) postGroupSquareOff(w http.ResponseWriter, r *http.Request) {
	if h.squareOffSvc == nil {
		writeError(w, http.StatusNotImplemented, "square off service not configured")
		return
	}

	groupID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}

	var req domain.SquareOffRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "malformed JSON body")
			return
		}
	}

	result, err := h.squareOffSvc.SquareOffGroup(r.Context(), groupID, req.Symbols)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *handlers) postAccountSquareOff(w http.ResponseWriter, r *http.Request) {
	if h.squareOffSvc == nil {
		writeError(w, http.StatusNotImplemented, "square off service not configured")
		return
	}

	accountID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	var req domain.SquareOffRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "malformed JSON body")
			return
		}
	}

	result, err := h.squareOffSvc.SquareOffAccount(r.Context(), accountID, req.Symbols)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "account not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}
