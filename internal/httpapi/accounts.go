package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// AccountsStore is what the Accounts page needs: the CopyToggle/Stop
// Copy field (enabled), the master Stop/Start field (active), full
// account listing/creation/deletion, and the follow-link terms edit
// (capitalRatio, maxQtyPerOrder, status).
type AccountsStore interface {
	SetFollowLinkEnabled(ctx context.Context, followerID uuid.UUID, enabled bool) error
	SetAccountActive(ctx context.Context, id uuid.UUID, active bool) error
	Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error)
	AccountRole(ctx context.Context, id uuid.UUID) (string, error)
	CreateAccount(ctx context.Context, id uuid.UUID, role, broker, brokerUserID, apiSecret string) error
	CreateFollowLink(ctx context.Context, link domain.FollowLink) error
	UpdateFollowLinkTerms(ctx context.Context, followerID uuid.UUID, capitalRatio decimal.Decimal, maxQtyPerOrder *int) error
	SetAccountStatus(ctx context.Context, id uuid.UUID, status string) error
	DeleteAccount(ctx context.Context, id uuid.UUID) error
	DeleteFollowLink(ctx context.Context, followerID uuid.UUID) error
}

type accountResponse struct {
	ID              string  `json:"id"`
	Role            string  `json:"role"`
	Broker          string  `json:"broker"`
	BrokerAccountID string  `json:"brokerAccountId"`
	MasterID        *string `json:"masterId"`
	CapitalRatio    *string `json:"capitalRatio"`
	MaxQtyPerOrder  *int    `json:"maxQtyPerOrder"`
	Enabled         bool    `json:"enabled"`
	Active          bool    `json:"active"`
	Status          string  `json:"status"`
}

func toAccountResponse(a domain.Account) accountResponse {
	resp := accountResponse{
		ID:              a.ID.String(),
		Role:            a.Role,
		Broker:          a.Broker,
		BrokerAccountID: a.BrokerAccountID,
		MaxQtyPerOrder:  a.MaxQtyPerOrder,
		Enabled:         a.Enabled,
		Active:          a.Active,
		Status:          a.Status,
	}
	if a.MasterID != nil {
		s := a.MasterID.String()
		resp.MasterID = &s
	}
	if a.CapitalRatio != nil {
		s := a.CapitalRatio.String()
		resp.CapitalRatio = &s
	}
	return resp
}

// getAccounts lists every account, for the Accounts page's flat list. An
// optional ?ids=uuid,uuid filters to a specific set — how the
// group-management page hydrates full fields for a group's members
// (whose IDs it already has from GET /groups/{masterId}).
func (h *handlers) getAccounts(w http.ResponseWriter, r *http.Request) {
	var ids []uuid.UUID
	if raw := r.URL.Query().Get("ids"); raw != "" {
		for _, s := range strings.Split(raw, ",") {
			id, err := uuid.Parse(strings.TrimSpace(s))
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid id in ids filter")
				return
			}
			ids = append(ids, id)
		}
	}

	accounts, err := h.store.Accounts(r.Context(), ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list accounts")
		return
	}
	resp := make([]accountResponse, len(accounts))
	for i, a := range accounts {
		resp[i] = toAccountResponse(a)
	}
	writeJSON(w, http.StatusOK, resp)
}

type postAccountRequest struct {
	Role            string  `json:"role"`
	Broker          string  `json:"broker"`
	BrokerAccountID string  `json:"brokerAccountId"`
	ApiSecret       string  `json:"apiSecret"`
	CapitalRatio    *string `json:"capitalRatio"`
	MaxQtyPerOrder  *int    `json:"maxQtyPerOrder"`
	MasterID        *string `json:"masterId"`
	// ApiKey is accepted and ignored — accounts has no api_key column yet
	// (docs/APIs/accounts.md's documented backend gap).
}

// postAccount creates a master or follower account, for the Accounts
// page's "Add Account" drawer.
func (h *handlers) postAccount(w http.ResponseWriter, r *http.Request) {
	var req postAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON body")
		return
	}
	if req.Broker != "kite" {
		writeError(w, http.StatusBadRequest, `broker must be "kite"`)
		return
	}
	if req.Role != "master" && req.Role != "follower" {
		writeError(w, http.StatusBadRequest, `role must be "master" or "follower"`)
		return
	}
	if req.BrokerAccountID == "" {
		writeError(w, http.StatusBadRequest, "brokerAccountId is required")
		return
	}

	var capitalRatio decimal.Decimal
	var masterID uuid.UUID
	if req.Role == "follower" {
		if req.CapitalRatio == nil || req.MaxQtyPerOrder == nil || req.MasterID == nil {
			writeError(w, http.StatusBadRequest, "capitalRatio, maxQtyPerOrder, and masterId are required for a follower account")
			return
		}
		var err error
		capitalRatio, err = decimal.NewFromString(*req.CapitalRatio)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid capitalRatio")
			return
		}
		masterID, err = uuid.Parse(*req.MasterID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid masterId")
			return
		}
		role, err := h.store.AccountRole(r.Context(), masterID)
		if errors.Is(err, domain.ErrNotFound) || (err == nil && role != "master") {
			writeError(w, http.StatusBadRequest, "invalid masterId")
			return
		}
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, "failed to validate masterId")
			return
		}
	}

	id := uuid.New()
	err := h.store.CreateAccount(r.Context(), id, req.Role, req.Broker, req.BrokerAccountID, req.ApiSecret)
	if errors.Is(err, domain.ErrDuplicate) {
		writeError(w, http.StatusConflict, "an account with this brokerAccountId already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}

	resp := accountResponse{
		ID:              id.String(),
		Role:            req.Role,
		Broker:          req.Broker,
		BrokerAccountID: req.BrokerAccountID,
		Enabled:         true,
		Active:          true,
		Status:          "ok",
	}
	if req.Role == "follower" {
		maxQty := 0
		if req.MaxQtyPerOrder != nil {
			maxQty = *req.MaxQtyPerOrder
		}
		if err := h.store.CreateFollowLink(r.Context(), domain.FollowLink{
			FollowerID: id, MasterID: masterID, CapitalRatio: capitalRatio,
			MaxQtyPerOrder: maxQty, Enabled: true,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "account created but failed to attach to group")
			return
		}
		masterIDStr := masterID.String()
		capitalRatioStr := capitalRatio.String()
		resp.MasterID = &masterIDStr
		resp.CapitalRatio = &capitalRatioStr
		resp.MaxQtyPerOrder = req.MaxQtyPerOrder
	}
	writeJSON(w, http.StatusCreated, resp)
}

type patchAccountRequest struct {
	Enabled        *bool   `json:"enabled"`
	Active         *bool   `json:"active"`
	CapitalRatio   *string `json:"capitalRatio"`
	MaxQtyPerOrder *int    `json:"maxQtyPerOrder"`
	Status         *string `json:"status"`
}

// writePatchStoreError writes the appropriate error response for a store
// error from a PATCH/DELETE mutation and reports whether it did so.
func writePatchStoreError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "account not found")
	default:
		writeError(w, http.StatusInternalServerError, "failed to update account")
	}
	return true
}

func (h *handlers) patchAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	var req patchAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON body")
		return
	}

	switch {
	case req.Active != nil:
		if writePatchStoreError(w, h.store.SetAccountActive(r.Context(), id, *req.Active)) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"active": *req.Active})
	case req.Enabled != nil:
		if writePatchStoreError(w, h.store.SetFollowLinkEnabled(r.Context(), id, *req.Enabled)) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"enabled": *req.Enabled})
	case req.CapitalRatio != nil || req.MaxQtyPerOrder != nil:
		h.patchFollowLinkTerms(w, r, id, req)
	case req.Status != nil:
		if writePatchStoreError(w, h.store.SetAccountStatus(r.Context(), id, *req.Status)) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": *req.Status})
	default:
		writeError(w, http.StatusBadRequest, "\"enabled\", \"active\", \"capitalRatio\", \"maxQtyPerOrder\", or \"status\" is required")
	}
}

// patchFollowLinkTerms handles the capitalRatio/maxQtyPerOrder half of
// PATCH /accounts/{id} — the Accounts page's edit form. It fetches the
// account's current follow-link fields first so a partial body (just
// one of the two) doesn't clobber the other.
func (h *handlers) patchFollowLinkTerms(w http.ResponseWriter, r *http.Request, id uuid.UUID, req patchAccountRequest) {
	accounts, err := h.store.Accounts(r.Context(), []uuid.UUID{id})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update account")
		return
	}
	if len(accounts) == 0 {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	current := accounts[0]
	if current.Role == "master" {
		writeError(w, http.StatusBadRequest, "capitalRatio/maxQtyPerOrder only apply to follower accounts")
		return
	}

	capitalRatio := current.CapitalRatio
	if req.CapitalRatio != nil {
		parsed, err := decimal.NewFromString(*req.CapitalRatio)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid capitalRatio")
			return
		}
		capitalRatio = &parsed
	}
	if capitalRatio == nil {
		writeError(w, http.StatusBadRequest, "account has no capitalRatio to update")
		return
	}
	maxQtyPerOrder := current.MaxQtyPerOrder
	if req.MaxQtyPerOrder != nil {
		maxQtyPerOrder = req.MaxQtyPerOrder
	}

	if writePatchStoreError(w, h.store.UpdateFollowLinkTerms(r.Context(), id, *capitalRatio, maxQtyPerOrder)) {
		return
	}

	resp := map[string]any{}
	if req.CapitalRatio != nil {
		resp["capitalRatio"] = *req.CapitalRatio
	}
	if req.MaxQtyPerOrder != nil {
		resp["maxQtyPerOrder"] = *req.MaxQtyPerOrder
	}
	writeJSON(w, http.StatusOK, resp)
}

// deleteAccount removes an account, for the Accounts page's "Delete"
// action. 409 if the account is still referenced (attached to a group,
// or has fill/order history) — the Accounts page surfaces this as
// "remove from group first" and offers deleteAccountGroup.
func (h *handlers) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	switch err := h.store.DeleteAccount(r.Context(), id); {
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusConflict, "account is still referenced by a group or order history; remove it from its group first")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "account not found")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to delete account")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// deleteAccountGroup detaches a follower from its group without
// deleting the account — the Accounts page's and group-management
// page's "Remove from group" action.
func (h *handlers) deleteAccountGroup(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	switch err := h.store.DeleteFollowLink(r.Context(), id); {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "account is not attached to a group")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to remove account from group")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
