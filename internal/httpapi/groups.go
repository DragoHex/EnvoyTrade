package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// GroupsStore is what GET /groups, GET /groups/{masterId}, and POST
// /groups/{masterId}/followers (the group-management page's "add
// account to group" action) need.
type GroupsStore interface {
	Groups(ctx context.Context) ([]domain.GroupSummary, error)
	GroupDetail(ctx context.Context, masterID uuid.UUID) (domain.GroupDetail, error)
	AccountRole(ctx context.Context, id uuid.UUID) (string, error)
	CreateFollowLink(ctx context.Context, link domain.FollowLink) error
}

type groupSummaryResponse struct {
	MasterID        string `json:"masterId"`
	MasterAccountID string `json:"masterAccountId"`
	Broker          string `json:"broker"`
	FollowerCount   int    `json:"followerCount"`
	Status          string `json:"status"`
	Active          bool   `json:"active"`
}

type groupFollowerResponse struct {
	AccountID       string `json:"accountId"`
	BrokerAccountID string `json:"brokerAccountId"`
	Enabled         bool   `json:"enabled"`
	Status          string `json:"status"`
}

type groupDetailResponse struct {
	MasterID        string                  `json:"masterId"`
	MasterAccountID string                  `json:"masterAccountId"`
	MasterActive    bool                    `json:"masterActive"`
	Followers       []groupFollowerResponse `json:"followers"`
}

func (h *handlers) getGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.store.Groups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list groups")
		return
	}

	resp := make([]groupSummaryResponse, len(groups))
	for i, g := range groups {
		resp[i] = groupSummaryResponse{
			MasterID:        g.MasterID.String(),
			MasterAccountID: g.MasterAccountID,
			Broker:          g.Broker,
			FollowerCount:   g.FollowerCount,
			Status:          g.Status,
			Active:          g.Active,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *handlers) getGroupDetail(w http.ResponseWriter, r *http.Request) {
	masterID, err := uuid.Parse(r.PathValue("masterId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid masterId")
		return
	}

	detail, err := h.store.GroupDetail(r.Context(), masterID)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load group")
		return
	}

	followers := make([]groupFollowerResponse, len(detail.Followers))
	for i, f := range detail.Followers {
		followers[i] = groupFollowerResponse{
			AccountID:       f.AccountID.String(),
			BrokerAccountID: f.BrokerAccountID,
			Enabled:         f.Enabled,
			Status:          f.Status,
		}
	}
	writeJSON(w, http.StatusOK, groupDetailResponse{
		MasterID:        detail.MasterID.String(),
		MasterAccountID: detail.MasterAccountID,
		MasterActive:    detail.MasterActive,
		Followers:       followers,
	})
}

type postGroupFollowerRequest struct {
	AccountID      string `json:"accountId"`
	CapitalRatio   string `json:"capitalRatio"`
	MaxQtyPerOrder *int   `json:"maxQtyPerOrder"`
}

// postGroupFollower attaches an existing follower-role account to a
// group — the group-management page's "Add account to group" action.
func (h *handlers) postGroupFollower(w http.ResponseWriter, r *http.Request) {
	masterID, err := uuid.Parse(r.PathValue("masterId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid masterId")
		return
	}

	var req postGroupFollowerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON body")
		return
	}
	accountID, err := uuid.Parse(req.AccountID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid accountId")
		return
	}
	capitalRatio, err := decimal.NewFromString(req.CapitalRatio)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid capitalRatio")
		return
	}

	if role, err := h.store.AccountRole(r.Context(), masterID); err != nil || role != "master" {
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, "failed to validate masterId")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid masterId")
		return
	}
	if role, err := h.store.AccountRole(r.Context(), accountID); err != nil || role != "follower" {
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, "failed to validate accountId")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid accountId")
		return
	}

	maxQty := 0
	if req.MaxQtyPerOrder != nil {
		maxQty = *req.MaxQtyPerOrder
	}
	err = h.store.CreateFollowLink(r.Context(), domain.FollowLink{
		FollowerID: accountID, MasterID: masterID, CapitalRatio: capitalRatio,
		MaxQtyPerOrder: maxQty, Enabled: true,
	})
	if errors.Is(err, domain.ErrDuplicate) {
		writeError(w, http.StatusConflict, "account is already attached to a group")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to attach account to group")
		return
	}
	w.WriteHeader(http.StatusCreated)
}
