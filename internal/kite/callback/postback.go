package callback

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"envoytrade/internal/domain"
	"envoytrade/internal/queue"

	"github.com/google/uuid"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

// RoleMaster and RoleFollower are the account roles AccountLookup can
// return — matching the "master"/"follower" strings already used in
// accounts.role (PLAN.md §2).
const (
	RoleMaster   = "master"
	RoleFollower = "follower"
)

// AccountLookup resolves the account a postback belongs to. Defined here
// (the consumer), not in the store package, per PLAN.md §1's seam rule.
type AccountLookup interface {
	// AccountByBrokerUserID returns the account's id, role, and Kite
	// Connect api_secret (needed to verify this postback's checksum —
	// every account has its own app). Returns domain.ErrNotFound if no
	// account has this broker_user_id.
	AccountByBrokerUserID(ctx context.Context, brokerUserID string) (id uuid.UUID, role string, apiSecret string, err error)
}

// postbackPayload is the subset of Kite's postback JSON body (same shape
// as kiteconnect.Order plus user_id/checksum, which Order doesn't carry)
// needed to route and verify it.
type postbackPayload struct {
	kiteconnect.Order
	UserID   string `json:"user_id"`
	Checksum string `json:"checksum"`
}

// Handler is an http.Handler for Kite's postback webhook. It always
// responds 200 unless the body itself couldn't be parsed — Kite retries
// on non-200, and an unknown account, bad checksum, non-terminal status,
// or full queue are all "acknowledge and drop", not "please resend".
type Handler struct {
	Accounts        AccountLookup
	MasterFills     queue.Publisher[domain.MasterFill]
	FollowerUpdates queue.Publisher[domain.OrderUpdate]
	Logger          *slog.Logger
}

func (h *Handler) log() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var p postbackPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "malformed body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	id, role, apiSecret, err := h.Accounts.AccountByBrokerUserID(ctx, p.UserID)
	if err != nil {
		h.log().Warn("postback: unknown account, dropping", "user_id", p.UserID, "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	if !VerifyChecksum(apiSecret, p.OrderID, p.OrderTimestamp.Format("2006-01-02 15:04:05"), p.Checksum) {
		h.log().Warn("postback: checksum mismatch, dropping", "order_id", p.OrderID, "user_id", p.UserID)
		w.WriteHeader(http.StatusOK)
		return
	}

	if !IsTerminal(p.Status) {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch role {
	case RoleMaster:
		if err := h.MasterFills.Publish(ctx, ToMasterFill(p.Order, id)); err != nil {
			h.log().Warn("postback: master fill queue full, dropping", "order_id", p.OrderID, "error", err)
		}
	case RoleFollower:
		if err := h.FollowerUpdates.Publish(ctx, ToOrderUpdate(p.Order)); err != nil {
			h.log().Warn("postback: order update queue full, dropping", "order_id", p.OrderID, "error", err)
		}
	default:
		h.log().Warn("postback: unknown account role, dropping", "user_id", p.UserID, "role", role)
	}

	w.WriteHeader(http.StatusOK)
}
