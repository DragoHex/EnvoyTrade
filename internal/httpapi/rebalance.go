package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

// RebalanceService declares what the rebalance endpoints need.
type RebalanceService interface {
	ComputeGroupDiff(ctx context.Context, groupID uuid.UUID) (domain.GroupRebalanceDiff, error)
	ComputeAccountDiff(ctx context.Context, accountID uuid.UUID) (domain.FollowerDrift, error)
	RebalanceGroup(ctx context.Context, groupID uuid.UUID, followerIDs []uuid.UUID) (domain.RebalanceResult, error)
	RebalanceAccount(ctx context.Context, accountID uuid.UUID) (domain.RebalanceResult, error)
}

func (h *handlers) getGroupRebalanceDiff(w http.ResponseWriter, r *http.Request) {
	if h.rebalanceSvc == nil {
		writeError(w, http.StatusNotImplemented, "rebalance service not configured")
		return
	}

	groupID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}

	diff, err := h.rebalanceSvc.ComputeGroupDiff(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, diff)
}

func (h *handlers) postGroupRebalance(w http.ResponseWriter, r *http.Request) {
	if h.rebalanceSvc == nil {
		writeError(w, http.StatusNotImplemented, "rebalance service not configured")
		return
	}

	groupID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}

	var req domain.RebalanceRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "malformed JSON body")
			return
		}
	}

	result, err := h.rebalanceSvc.RebalanceGroup(r.Context(), groupID, req.FollowerIDs)
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

func (h *handlers) getAccountRebalanceDiff(w http.ResponseWriter, r *http.Request) {
	if h.rebalanceSvc == nil {
		writeError(w, http.StatusNotImplemented, "rebalance service not configured")
		return
	}

	accountID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	diff, err := h.rebalanceSvc.ComputeAccountDiff(r.Context(), accountID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "account not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, diff)
}

func (h *handlers) postAccountRebalance(w http.ResponseWriter, r *http.Request) {
	if h.rebalanceSvc == nil {
		writeError(w, http.StatusNotImplemented, "rebalance service not configured")
		return
	}

	accountID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	result, err := h.rebalanceSvc.RebalanceAccount(r.Context(), accountID)
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
