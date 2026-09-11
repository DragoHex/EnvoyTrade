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

// GroupsStore is what GET /groups, POST /groups, GET /groups/{id},
// PATCH /groups/{id}, DELETE /groups/{id}, and POST /groups/{id}/followers need.
type GroupsStore interface {
	Groups(ctx context.Context) ([]domain.GroupSummary, error)
	GroupDetail(ctx context.Context, id uuid.UUID) (domain.GroupDetail, error)
	AccountRole(ctx context.Context, id uuid.UUID) (string, error)
	CreateFollowLink(ctx context.Context, link domain.FollowLink) error
	CreateGroup(ctx context.Context, id uuid.UUID, name string, masterID uuid.UUID) error
	UpdateGroup(ctx context.Context, id uuid.UUID, name *string, masterID *uuid.UUID) error
	DeleteGroup(ctx context.Context, id uuid.UUID) error
}

type groupSummaryResponse struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	MasterID        string `json:"masterId"`
	MasterAccountID string `json:"masterAccountId"`
	MasterName      string `json:"masterName"`
	Broker          string `json:"broker"`
	FollowerCount   int    `json:"followerCount"`
	Status          string `json:"status"`
	Active          bool   `json:"active"`
}

type groupFollowerResponse struct {
	AccountID       string `json:"accountId"`
	Name            string `json:"name"`
	BrokerAccountID string `json:"brokerAccountId"`
	Enabled         bool   `json:"enabled"`
	Status          string `json:"status"`
}

type groupDetailResponse struct {
	ID              string                  `json:"id"`
	Name            string                  `json:"name"`
	MasterID        string                  `json:"masterId"`
	MasterAccountID string                  `json:"masterAccountId"`
	MasterName      string                  `json:"masterName"`
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
			ID:              g.ID.String(),
			Name:            g.Name,
			MasterID:        g.MasterID.String(),
			MasterAccountID: g.MasterAccountID,
			MasterName:      g.MasterName,
			Broker:          g.Broker,
			FollowerCount:   g.FollowerCount,
			Status:          g.Status,
			Active:          g.Active,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type postGroupRequest struct {
	Name     string `json:"name"`
	MasterID string `json:"masterId"`
}

func (h *handlers) postGroup(w http.ResponseWriter, r *http.Request) {
	var req postGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	masterID, err := uuid.Parse(req.MasterID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid masterId")
		return
	}
	role, err := h.store.AccountRole(r.Context(), masterID)
	if err != nil || role != "master" {
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, "failed to validate masterId")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid masterId: account must have master role")
		return
	}

	id := uuid.New()
	if err := h.store.CreateGroup(r.Context(), id, req.Name, masterID); err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			writeError(w, http.StatusConflict, "group already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create group")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":       id.String(),
		"name":     req.Name,
		"masterId": req.MasterID,
	})
}

func (h *handlers) getGroupDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.PathValue("masterId")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}

	detail, err := h.store.GroupDetail(r.Context(), id)
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
			Name:            f.Name,
			BrokerAccountID: f.BrokerAccountID,
			Enabled:         f.Enabled,
			Status:          f.Status,
		}
	}
	writeJSON(w, http.StatusOK, groupDetailResponse{
		ID:              detail.GroupID.String(),
		Name:            detail.GroupName,
		MasterID:        detail.MasterID.String(),
		MasterAccountID: detail.MasterAccountID,
		MasterName:      detail.MasterName,
		MasterActive:    detail.MasterActive,
		Followers:       followers,
	})
}

type patchGroupRequest struct {
	Name     *string `json:"name"`
	MasterID *string `json:"masterId"`
}

func (h *handlers) patchGroup(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.PathValue("masterId")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}

	var req patchGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON body")
		return
	}

	var masterIDPtr *uuid.UUID
	if req.MasterID != nil {
		parsed, err := uuid.Parse(*req.MasterID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid masterId")
			return
		}
		role, err := h.store.AccountRole(r.Context(), parsed)
		if err != nil || role != "master" {
			if err != nil && !errors.Is(err, domain.ErrNotFound) {
				writeError(w, http.StatusInternalServerError, "failed to validate masterId")
				return
			}
			writeError(w, http.StatusBadRequest, "invalid masterId: account must have master role")
			return
		}
		masterIDPtr = &parsed
	}

	if err := h.store.UpdateGroup(r.Context(), id, req.Name, masterIDPtr); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update group")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) deleteGroup(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.PathValue("masterId")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}

	if err := h.store.DeleteGroup(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			writeError(w, http.StatusConflict, "group has follower accounts attached; detach them first")
			return
		}
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete group")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type postGroupFollowerRequest struct {
	AccountID      string `json:"accountId"`
	CapitalRatio   string `json:"capitalRatio"`
	MaxQtyPerOrder *int   `json:"maxQtyPerOrder"`
}

// postGroupFollower attaches an existing follower-role account to a group.
func (h *handlers) postGroupFollower(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.PathValue("masterId")
	}
	targetID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group/master id")
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

	targetRole, targetRoleErr := h.store.AccountRole(r.Context(), targetID)
	if targetRoleErr == nil && targetRole != "master" {
		writeError(w, http.StatusBadRequest, "invalid masterId: account is not a master")
		return
	}

	detail, detailErr := h.store.GroupDetail(r.Context(), targetID)
	if detailErr != nil && !errors.Is(detailErr, domain.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "failed to resolve group")
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

	linkMasterID := detail.MasterID
	if linkMasterID == uuid.Nil && targetRole == "master" {
		linkMasterID = targetID
	}
	linkGroupID := detail.GroupID
	if linkGroupID == uuid.Nil {
		linkGroupID = targetID
	}

	err = h.store.CreateFollowLink(r.Context(), domain.FollowLink{
		FollowerID:     accountID,
		GroupID:        linkGroupID,
		MasterID:       linkMasterID,
		CapitalRatio:   capitalRatio,
		MaxQtyPerOrder: maxQty,
		Enabled:        true,
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
