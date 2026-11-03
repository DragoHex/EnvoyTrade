package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

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
	CreateFollowLinksBatch(ctx context.Context, groupID uuid.UUID, links []domain.FollowLink) error
	CreateGroup(ctx context.Context, id uuid.UUID, name string, masterID uuid.UUID) error
	UpdateGroup(ctx context.Context, id uuid.UUID, name *string, masterID *uuid.UUID) error
	DeleteGroup(ctx context.Context, id uuid.UUID, deleteContainedAccounts bool) ([]uuid.UUID, error)
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
	HasImbalance    bool   `json:"hasImbalance"`
}

type groupFollowerResponse struct {
	AccountID            string                     `json:"accountId"`
	Name                 string                     `json:"name"`
	BrokerAccountID      string                     `json:"brokerAccountId"`
	Enabled              bool                       `json:"enabled"`
	Status               string                     `json:"status"`
	NetQty               int                        `json:"netQty"`
	OpenPositionsCount   int                        `json:"openPositionsCount"`
	ClosedPositionsCount int                        `json:"closedPositionsCount"`
	OpenOrdersCount      int                        `json:"openOrdersCount"`
	TotalMtm             decimal.Decimal            `json:"totalMtm"`
	AvailableCash        *decimal.Decimal           `json:"availableCash,omitempty"`
	AvailableMargin      *decimal.Decimal           `json:"availableMargin,omitempty"`
	MtmBreakdown         map[string]decimal.Decimal `json:"mtmBreakdown,omitempty"`
	MarginBreakdown      *domain.MarginBreakdown    `json:"marginBreakdown,omitempty"`
}

type groupDetailResponse struct {
	ID                         string                     `json:"id"`
	Name                       string                     `json:"name"`
	MasterID                   string                     `json:"masterId"`
	MasterAccountID            string                     `json:"masterAccountId"`
	MasterName                 string                     `json:"masterName"`
	MasterActive               bool                       `json:"masterActive"`
	HasImbalance               bool                       `json:"hasImbalance"`
	MasterNetQty               int                        `json:"masterNetQty"`
	MasterOpenPositionsCount   int                        `json:"masterOpenPositionsCount"`
	MasterClosedPositionsCount int                        `json:"masterClosedPositionsCount"`
	MasterOpenOrdersCount      int                        `json:"masterOpenOrdersCount"`
	MasterTotalMtm             decimal.Decimal            `json:"masterTotalMtm"`
	MasterAvailableCash        *decimal.Decimal           `json:"masterAvailableCash,omitempty"`
	MasterAvailableMargin      *decimal.Decimal           `json:"masterAvailableMargin,omitempty"`
	MasterMtmBreakdown         map[string]decimal.Decimal `json:"masterMtmBreakdown,omitempty"`
	MasterMarginBreakdown      *domain.MarginBreakdown    `json:"masterMarginBreakdown,omitempty"`
	Followers                  []groupFollowerResponse    `json:"followers"`
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
			HasImbalance:    g.HasImbalance,
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
			writeError(w, http.StatusConflict, "master account is already assigned to a group")
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
			AccountID:            f.AccountID.String(),
			Name:                 f.Name,
			BrokerAccountID:      f.BrokerAccountID,
			Enabled:              f.Enabled,
			Status:               f.Status,
			NetQty:               f.NetQty,
			OpenPositionsCount:   f.OpenPositionsCount,
			ClosedPositionsCount: f.ClosedPositionsCount,
			OpenOrdersCount:      f.OpenOrdersCount,
			TotalMtm:             f.TotalMtm,
			AvailableCash:        f.AvailableCash,
			AvailableMargin:      f.AvailableMargin,
			MtmBreakdown:         f.MtmBreakdown,
			MarginBreakdown:      f.MarginBreakdown,
		}
	}
	writeJSON(w, http.StatusOK, groupDetailResponse{
		ID:                         detail.GroupID.String(),
		Name:                       detail.GroupName,
		MasterID:                   detail.MasterID.String(),
		MasterAccountID:            detail.MasterAccountID,
		MasterName:                 detail.MasterName,
		MasterActive:               detail.MasterActive,
		HasImbalance:               detail.HasImbalance,
		MasterNetQty:               detail.MasterNetQty,
		MasterOpenPositionsCount:   detail.MasterOpenPositionsCount,
		MasterClosedPositionsCount: detail.MasterClosedPositionsCount,
		MasterOpenOrdersCount:      detail.MasterOpenOrdersCount,
		MasterTotalMtm:             detail.MasterTotalMtm,
		MasterAvailableCash:        detail.MasterAvailableCash,
		MasterAvailableMargin:      detail.MasterAvailableMargin,
		MasterMtmBreakdown:         detail.MasterMtmBreakdown,
		MasterMarginBreakdown:      detail.MasterMarginBreakdown,
		Followers:                  followers,
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
		if errors.Is(err, domain.ErrMasterHasOpenPositions) {
			writeError(w, http.StatusConflict, "cannot swap master: master has open positions")
			return
		}
		if errors.Is(err, domain.ErrDuplicate) {
			writeError(w, http.StatusConflict, "new master account is already assigned to another group")
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

	deleteAccounts := r.URL.Query().Get("delete_accounts") == "true"
	deletedAccountIDs, err := h.store.DeleteGroup(r.Context(), id, deleteAccounts)
	if err != nil {
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

	if deleteAccounts {
		for _, accID := range deletedAccountIDs {
			if h.brokerEvicter != nil {
				h.brokerEvicter.Evict(accID)
			}
			if h.tickerMgr != nil {
				_ = h.tickerMgr.StopMaster(accID)
			}
			if h.followerRegistrar != nil {
				h.followerRegistrar.UnregisterFollower(accID)
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

type postGroupFollowerRequest struct {
	AccountID      string `json:"accountId"`
	CloneFactor    any    `json:"cloneFactor"`
	CapitalRatio   any    `json:"capitalRatio"`
	MaxQtyPerOrder any    `json:"maxQtyPerOrder"`
}

type postGroupFollowersBatchRequest struct {
	Followers []postGroupFollowerRequest `json:"followers"`
}

// postGroupFollower attaches one or more existing follower-role accounts to a group.
// Supports both a single account payload and a batch payload (top-level array or {"followers": [...]}).
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

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	trimmedBody := bytes.TrimSpace(bodyBytes)
	if len(trimmedBody) == 0 {
		writeError(w, http.StatusBadRequest, "empty request body")
		return
	}

	var items []postGroupFollowerRequest

	if trimmedBody[0] == '[' {
		// 1. Top-level array: [ {...}, {...} ]
		if err := json.Unmarshal(trimmedBody, &items); err != nil {
			writeError(w, http.StatusBadRequest, "malformed JSON array")
			return
		}
	} else if trimmedBody[0] == '{' {
		// 2. Object: could be batch wrapper {"followers": [...]} or legacy single object {"accountId": ...}
		var batchReq postGroupFollowersBatchRequest
		if err := json.Unmarshal(trimmedBody, &batchReq); err == nil && len(batchReq.Followers) > 0 {
			items = batchReq.Followers
		} else {
			var singleReq postGroupFollowerRequest
			if err := json.Unmarshal(trimmedBody, &singleReq); err != nil {
				writeError(w, http.StatusBadRequest, "malformed JSON body")
				return
			}
			if strings.TrimSpace(singleReq.AccountID) == "" {
				writeError(w, http.StatusBadRequest, "missing accountId or non-empty followers list")
				return
			}
			items = []postGroupFollowerRequest{singleReq}
		}
	} else {
		writeError(w, http.StatusBadRequest, "malformed JSON body")
		return
	}

	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, "followers list cannot be empty")
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

	linkMasterID := detail.MasterID
	if linkMasterID == uuid.Nil && targetRole == "master" {
		linkMasterID = targetID
	}
	linkGroupID := detail.GroupID
	if linkGroupID == uuid.Nil {
		linkGroupID = targetID
	}

	seenInBatch := make(map[uuid.UUID]struct{}, len(items))
	domainLinks := make([]domain.FollowLink, len(items))

	for i, item := range items {
		accountID, err := uuid.Parse(strings.TrimSpace(item.AccountID))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid accountId at index %d", i))
			return
		}
		if _, seen := seenInBatch[accountID]; seen {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("duplicate accountId %s in request", item.AccountID))
			return
		}
		seenInBatch[accountID] = struct{}{}

		cloneFactor, hasCF, err := parseFlexibleDecimal(item.CloneFactor)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid cloneFactor for account %s: %v", item.AccountID, err))
			return
		}
		if !hasCF {
			var hasCR bool
			cloneFactor, hasCR, err = parseFlexibleDecimal(item.CapitalRatio)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid capitalRatio for account %s: %v", item.AccountID, err))
				return
			}
			if !hasCR {
				cloneFactor = decimal.NewFromInt(1)
			}
		}
		if cloneFactor.Sign() <= 0 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid cloneFactor for account %s: must be greater than 0", item.AccountID))
			return
		}

		role, err := h.store.AccountRole(r.Context(), accountID)
		if err != nil || role != "follower" {
			if err != nil && !errors.Is(err, domain.ErrNotFound) {
				writeError(w, http.StatusInternalServerError, "failed to validate accountId")
				return
			}
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid accountId %s: account must exist and have follower role", item.AccountID))
			return
		}

		parsedMaxQty, err := parseFlexibleInt(item.MaxQtyPerOrder)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid maxQtyPerOrder for account %s: %v", item.AccountID, err))
			return
		}
		maxQty := 0
		if parsedMaxQty != nil {
			if *parsedMaxQty < 0 {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid maxQtyPerOrder for account %s: cannot be negative", item.AccountID))
				return
			}
			maxQty = *parsedMaxQty
		}

		domainLinks[i] = domain.FollowLink{
			FollowerID:     accountID,
			GroupID:        linkGroupID,
			MasterID:       linkMasterID,
			CloneFactor:    cloneFactor,
			MaxQtyPerOrder: maxQty,
			Enabled:        true,
		}
	}

	err = h.store.CreateFollowLinksBatch(r.Context(), linkGroupID, domainLinks)
	if errors.Is(err, domain.ErrDuplicate) {
		writeError(w, http.StatusConflict, "one or more accounts are already attached to a group")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to attach accounts to group")
		return
	}

	if h.followerRegistrar != nil {
		for _, l := range domainLinks {
			_ = h.followerRegistrar.RegisterFollower(r.Context(), l.FollowerID)
		}
	}
	w.WriteHeader(http.StatusCreated)
}
