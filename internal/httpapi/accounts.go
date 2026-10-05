package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"strings"

	"envoytrade/internal/crypto"
	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// AccountsStore is what the Accounts page needs: the CopyToggle/Stop
// Copy field (enabled), the master Stop/Start field (active), full
// account listing/creation/deletion, and the follow-link terms edit
// (cloneFactor, maxQtyPerOrder, status).
type AccountsStore interface {
	SetFollowLinkEnabled(ctx context.Context, followerID uuid.UUID, enabled bool) error
	SetAccountActive(ctx context.Context, id uuid.UUID, active bool) error
	Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error)
	AccountRole(ctx context.Context, id uuid.UUID) (string, error)
	CreateAccount(ctx context.Context, id uuid.UUID, name, role, broker, brokerUserID, apiKey, apiSecret, ipAddress string) error
	CreateAccountWithCredentials(ctx context.Context, id uuid.UUID, name, role, broker, brokerUserID, apiKey, apiSecret, ipAddress, encPassword, encTotpSecret string) error
	SetAccountName(ctx context.Context, id uuid.UUID, name string) error
	SetAccountIPAddress(ctx context.Context, id uuid.UUID, ip string) error
	SetAccountAPIKey(ctx context.Context, id uuid.UUID, apiKey string) error
	SetAccountAPISecret(ctx context.Context, id uuid.UUID, apiSecret string) error
	SetAccountEncryptedCredentials(ctx context.Context, id uuid.UUID, encPassword, encTotpSecret string) error
	CreateFollowLink(ctx context.Context, link domain.FollowLink) error
	UpdateFollowLinkTerms(ctx context.Context, followerID uuid.UUID, cloneFactor decimal.Decimal, maxQtyPerOrder *int) error
	SetAccountStatus(ctx context.Context, id uuid.UUID, status domain.AccountStatus) error
	DeleteAccount(ctx context.Context, id uuid.UUID) error
	DeleteFollowLink(ctx context.Context, followerID uuid.UUID) error
	ProxyIPByAddress(ctx context.Context, ipAddress string) (domain.ProxyIP, error)
}

type accountResponse struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Role            string  `json:"role"`
	Broker          string  `json:"broker"`
	BrokerAccountID string  `json:"brokerAccountId"`
	ApiKey          *string `json:"apiKey,omitempty"`
	ApiSecret       *string `json:"apiSecret,omitempty"`
	IP              *string `json:"ip"`
	GroupID         *string `json:"groupId"`
	GroupName       *string `json:"groupName"`
	MasterID        *string `json:"masterId"`
	CloneFactor     *string `json:"cloneFactor"`
	MaxQtyPerOrder  *int    `json:"maxQtyPerOrder"`
	Enabled         bool    `json:"enabled"`
	Active          bool    `json:"active"`
	Status          string  `json:"status"`
	AuthStatus      *string `json:"authStatus,omitempty"`
	AuthError       *string `json:"authError,omitempty"`
}

func toAccountResponse(a domain.Account) accountResponse {
	resp := accountResponse{
		ID:              a.ID.String(),
		Name:            a.Name,
		Role:            a.Role,
		Broker:          a.Broker,
		BrokerAccountID: a.BrokerAccountID,
		MaxQtyPerOrder:  a.MaxQtyPerOrder,
		Enabled:         a.Enabled,
		Active:          a.Active,
		Status:          a.Status,
		GroupName:       a.GroupName,
	}
	if a.AuthStatus != "" {
		st := a.AuthStatus
		resp.AuthStatus = &st
	}
	if a.AuthError != "" {
		ae := a.AuthError
		resp.AuthError = &ae
	}
	if a.ApiKey != "" {
		k := a.ApiKey
		resp.ApiKey = &k
	}
	if a.ApiSecret != "" {
		s := a.ApiSecret
		resp.ApiSecret = &s
	}
	if a.IPAddress != "" {
		ip := a.IPAddress
		resp.IP = &ip
	}
	if a.GroupID != nil {
		s := a.GroupID.String()
		resp.GroupID = &s
	}
	if a.MasterID != nil {
		s := a.MasterID.String()
		resp.MasterID = &s
	}
	if a.CloneFactor != nil {
		s := a.CloneFactor.String()
		resp.CloneFactor = &s
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

func validateIP(raw *string, role string) (string, error) {
	val := ""
	if raw != nil {
		val = strings.TrimSpace(*raw)
	}
	if role == "follower" {
		if val == "" {
			return "", errors.New("ip is required for follower accounts")
		}
	}
	if val == "" {
		return "", nil
	}
	addr, err := netip.ParseAddr(val)
	if err != nil || (!addr.Is4() && !addr.Is6()) {
		return "", errors.New("invalid IP address: must be a valid IPv4 or IPv6 address")
	}
	return val, nil
}

type postAccountRequest struct {
	Name            string  `json:"name"`
	Role            string  `json:"role"`
	Broker          string  `json:"broker"`
	BrokerAccountID string  `json:"brokerAccountId"`
	ApiKey          string  `json:"apiKey"`
	ApiSecret       string  `json:"apiSecret"`
	Password        *string `json:"password"`
	TotpSecret      *string `json:"totpSecret"`
	IP              *string `json:"ip"`
	IPAddress       *string `json:"ipAddress"`
	CloneFactor     *string `json:"cloneFactor"`
	CapitalRatio    *string `json:"capitalRatio"`
	MaxQtyPerOrder  *int    `json:"maxQtyPerOrder"`
	GroupID         *string `json:"groupId"`
	MasterID        *string `json:"masterId"`
}

// postAccount creates a master or follower account, for the Accounts
// page's "Add Account" drawer.
func (h *handlers) postAccount(w http.ResponseWriter, r *http.Request) {
	var req postAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON body")
		return
	}
	broker := strings.ToLower(strings.TrimSpace(req.Broker))
	if broker != "kite" && broker != "testbroker" {
		writeError(w, http.StatusBadRequest, `broker must be "kite" or "testbroker"`)
		return
	}
	req.Broker = broker
	if req.Role != "master" && req.Role != "follower" {
		writeError(w, http.StatusBadRequest, `role must be "master" or "follower"`)
		return
	}
	if req.BrokerAccountID == "" {
		writeError(w, http.StatusBadRequest, "brokerAccountId is required")
		return
	}
	if req.Name == "" {
		req.Name = req.BrokerAccountID
	}

	var rawIP *string
	if req.IP != nil {
		rawIP = req.IP
	} else if req.IPAddress != nil {
		rawIP = req.IPAddress
	}
	validIP, err := validateIP(rawIP, req.Role)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if validIP != "" {
		if _, err := h.store.ProxyIPByAddress(r.Context(), validIP); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				writeError(w, http.StatusBadRequest, "IP address is not registered in proxy pool")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to check proxy IP")
			return
		}
	}

	var cloneFactor decimal.Decimal = decimal.NewFromInt(1)
	var masterID uuid.UUID
	var groupID uuid.UUID
	if req.Role == "follower" {
		if req.MaxQtyPerOrder == nil || (req.MasterID == nil && req.GroupID == nil) {
			writeError(w, http.StatusBadRequest, "maxQtyPerOrder, and masterId or groupId are required for a follower account")
			return
		}
		rawCF := req.CloneFactor
		if rawCF == nil {
			rawCF = req.CapitalRatio
		}
		if rawCF != nil && strings.TrimSpace(*rawCF) != "" {
			var err error
			cloneFactor, err = decimal.NewFromString(strings.TrimSpace(*rawCF))
			if err != nil || cloneFactor.Sign() <= 0 {
				writeError(w, http.StatusBadRequest, "invalid cloneFactor")
				return
			}
		}
		if req.GroupID != nil && *req.GroupID != "" {
			groupID, err = uuid.Parse(*req.GroupID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid groupId")
				return
			}
		}
		if req.MasterID != nil && *req.MasterID != "" {
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
	}

	id := uuid.New()
	var encPass, encTotp string
	if req.Password != nil && strings.TrimSpace(*req.Password) != "" {
		enc, err := crypto.Encrypt(strings.TrimSpace(*req.Password))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt password")
			return
		}
		encPass = enc
	}
	if req.TotpSecret != nil && strings.TrimSpace(*req.TotpSecret) != "" {
		enc, err := crypto.Encrypt(strings.TrimSpace(*req.TotpSecret))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt totp secret")
			return
		}
		encTotp = enc
	}

	err = h.store.CreateAccountWithCredentials(r.Context(), id, req.Name, req.Role, req.Broker, req.BrokerAccountID, req.ApiKey, req.ApiSecret, validIP, encPass, encTotp)
	if errors.Is(err, domain.ErrIPAlreadyAssigned) {
		writeError(w, http.StatusConflict, "IP address "+validIP+" is already assigned to another account")
		return
	}
	if errors.Is(err, domain.ErrDuplicate) {
		writeError(w, http.StatusConflict, "an account with this brokerAccountId already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}

	if h.syncer != nil && encPass != "" && encTotp != "" {
		go func() {
			_ = h.syncer.SyncAccountPortfolio(context.Background(), id)
		}()
	}

	resp := accountResponse{
		ID:              id.String(),
		Name:            req.Name,
		Role:            req.Role,
		Broker:          req.Broker,
		BrokerAccountID: req.BrokerAccountID,
		Enabled:         true,
		Active:          true,
		Status:          "ok",
	}
	if req.ApiKey != "" {
		resp.ApiKey = &req.ApiKey
	}
	if req.ApiSecret != "" {
		resp.ApiSecret = &req.ApiSecret
	}
	if validIP != "" {
		resp.IP = &validIP
	}
	if req.Role == "follower" {
		maxQty := 0
		if req.MaxQtyPerOrder != nil {
			maxQty = *req.MaxQtyPerOrder
		}
		link := domain.FollowLink{
			FollowerID:     id,
			GroupID:        groupID,
			MasterID:       masterID,
			CloneFactor:    cloneFactor,
			MaxQtyPerOrder: maxQty,
			Enabled:        true,
		}
		if err := h.store.CreateFollowLink(r.Context(), link); err != nil {
			writeError(w, http.StatusInternalServerError, "account created but failed to attach to group")
			return
		}
		if masterID != uuid.Nil {
			masterIDStr := masterID.String()
			resp.MasterID = &masterIDStr
		}
		if groupID != uuid.Nil {
			groupIDStr := groupID.String()
			resp.GroupID = &groupIDStr
		}
		cloneFactorStr := cloneFactor.String()
		resp.CloneFactor = &cloneFactorStr
		resp.MaxQtyPerOrder = req.MaxQtyPerOrder
	}
	writeJSON(w, http.StatusCreated, resp)
}

type patchAccountRequest struct {
	Name           *string `json:"name"`
	Enabled        *bool   `json:"enabled"`
	Active         *bool   `json:"active"`
	CloneFactor    *string `json:"cloneFactor"`
	CapitalRatio   *string `json:"capitalRatio"`
	MaxQtyPerOrder *int    `json:"maxQtyPerOrder"`
	Status         *string `json:"status"`
	IP             *string `json:"ip"`
	IPAddress      *string `json:"ipAddress"`
	ApiKey         *string `json:"apiKey"`
	ApiSecret      *string `json:"apiSecret"`
	Password       *string `json:"password"`
	TotpSecret     *string `json:"totpSecret"`
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

	updated := false
	resp := map[string]any{}

	if req.Name != nil {
		if writePatchStoreError(w, h.store.SetAccountName(r.Context(), id, *req.Name)) {
			return
		}
		resp["name"] = *req.Name
		updated = true
	}
	if req.Active != nil {
		if writePatchStoreError(w, h.store.SetAccountActive(r.Context(), id, *req.Active)) {
			return
		}
		if h.tickerMgr != nil {
			role, _ := h.store.AccountRole(r.Context(), id)
			if role == "master" {
				if *req.Active {
					_ = h.tickerMgr.StartMaster(r.Context(), id)
				} else {
					_ = h.tickerMgr.StopMaster(id)
				}
			}
		}
		resp["active"] = *req.Active
		updated = true
	}
	if req.Enabled != nil {
		if writePatchStoreError(w, h.store.SetFollowLinkEnabled(r.Context(), id, *req.Enabled)) {
			return
		}
		resp["enabled"] = *req.Enabled
		updated = true
	}
	if req.CloneFactor != nil || req.CapitalRatio != nil || req.MaxQtyPerOrder != nil {
		linkResp, ok := h.applyFollowLinkTerms(w, r, id, req)
		if !ok {
			return
		}
		for k, v := range linkResp {
			resp[k] = v
		}
		updated = true
	}
	if req.Status != nil {
		status, err := domain.ParseAccountStatus(*req.Status)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if writePatchStoreError(w, h.store.SetAccountStatus(r.Context(), id, status)) {
			return
		}
		resp["status"] = status.ToAPI()
		updated = true
	}
	if req.IP != nil || req.IPAddress != nil {
		rawIP := req.IP
		if rawIP == nil {
			rawIP = req.IPAddress
		}
		role, err := h.store.AccountRole(r.Context(), id)
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "account not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get account role")
			return
		}
		validIP, err := validateIP(rawIP, role)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if validIP != "" {
			if _, err := h.store.ProxyIPByAddress(r.Context(), validIP); err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					writeError(w, http.StatusBadRequest, "IP address is not registered in proxy pool")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to check proxy IP")
				return
			}
		}
		setErr := h.store.SetAccountIPAddress(r.Context(), id, validIP)
		if errors.Is(setErr, domain.ErrIPAlreadyAssigned) {
			writeError(w, http.StatusConflict, "IP address "+validIP+" is already assigned to another account")
			return
		}
		if writePatchStoreError(w, setErr) {
			return
		}
		resp["ip"] = validIP
		updated = true
	}
	if req.ApiKey != nil {
		if writePatchStoreError(w, h.store.SetAccountAPIKey(r.Context(), id, *req.ApiKey)) {
			return
		}
		resp["apiKey"] = *req.ApiKey
		updated = true
	}
	if req.ApiSecret != nil {
		if writePatchStoreError(w, h.store.SetAccountAPISecret(r.Context(), id, *req.ApiSecret)) {
			return
		}
		resp["apiSecret"] = *req.ApiSecret
		updated = true
	}
	if req.Password != nil || req.TotpSecret != nil {
		var encPass, encTotp string
		if req.Password != nil && strings.TrimSpace(*req.Password) != "" {
			enc, err := crypto.Encrypt(strings.TrimSpace(*req.Password))
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to encrypt password")
				return
			}
			encPass = enc
		}
		if req.TotpSecret != nil && strings.TrimSpace(*req.TotpSecret) != "" {
			enc, err := crypto.Encrypt(strings.TrimSpace(*req.TotpSecret))
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to encrypt totp secret")
				return
			}
			encTotp = enc
		}
		if writePatchStoreError(w, h.store.SetAccountEncryptedCredentials(r.Context(), id, encPass, encTotp)) {
			return
		}
		if h.syncer != nil && (encPass != "" || encTotp != "") {
			go func() {
				_ = h.syncer.SyncAccountPortfolio(context.Background(), id)
			}()
		}
		resp["credentials"] = "updated"
		updated = true
	}

	if !updated {
		writeError(w, http.StatusBadRequest, "\"name\", \"enabled\", \"active\", \"cloneFactor\", \"maxQtyPerOrder\", \"status\", \"ip\", \"apiKey\", \"apiSecret\", \"password\", or \"totpSecret\" is required")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// applyFollowLinkTerms handles the cloneFactor/maxQtyPerOrder half of
// PATCH /accounts/{id} — the Accounts page's edit form. It fetches the
// account's current follow-link fields first so a partial body (just
// one of the two) doesn't clobber the other.
func (h *handlers) applyFollowLinkTerms(w http.ResponseWriter, r *http.Request, id uuid.UUID, req patchAccountRequest) (map[string]any, bool) {
	accounts, err := h.store.Accounts(r.Context(), []uuid.UUID{id})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update account")
		return nil, false
	}
	if len(accounts) == 0 {
		writeError(w, http.StatusNotFound, "account not found")
		return nil, false
	}
	current := accounts[0]
	if current.Role == "master" {
		writeError(w, http.StatusBadRequest, "cloneFactor/maxQtyPerOrder only apply to follower accounts")
		return nil, false
	}

	cloneFactor := current.CloneFactor
	cfInput := req.CloneFactor
	if cfInput == nil {
		cfInput = req.CapitalRatio
	}
	if cfInput != nil {
		parsed, err := decimal.NewFromString(*cfInput)
		if err != nil || parsed.Sign() <= 0 {
			writeError(w, http.StatusBadRequest, "invalid cloneFactor")
			return nil, false
		}
		cloneFactor = &parsed
	}
	if cloneFactor == nil {
		writeError(w, http.StatusBadRequest, "account has no cloneFactor to update")
		return nil, false
	}
	maxQtyPerOrder := current.MaxQtyPerOrder
	if req.MaxQtyPerOrder != nil {
		maxQtyPerOrder = req.MaxQtyPerOrder
	}

	if writePatchStoreError(w, h.store.UpdateFollowLinkTerms(r.Context(), id, *cloneFactor, maxQtyPerOrder)) {
		return nil, false
	}

	resp := map[string]any{}
	if cfInput != nil {
		resp["cloneFactor"] = *cfInput
	}
	if req.MaxQtyPerOrder != nil {
		resp["maxQtyPerOrder"] = *req.MaxQtyPerOrder
	}
	return resp, true
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
		if h.tickerMgr != nil {
			_ = h.tickerMgr.StopMaster(id)
		}
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
