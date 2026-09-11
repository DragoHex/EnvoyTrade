package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

// OrdersStore provides order and portfolio detail queries for an account.
type OrdersStore interface {
	AccountOrders(ctx context.Context, id uuid.UUID, tab string, page int, limit int) (domain.AccountOrdersDetail, error)
}

func (h *handlers) getAccountOrders(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "open_positions"
	}

	page := 1
	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 10
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	detail, err := h.store.AccountOrders(r.Context(), id, tab, page, limit)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, detail)
}
