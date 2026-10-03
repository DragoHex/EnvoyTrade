package httpapi

import (
	"context"
	"net/http"
	"time"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

// ProxyStore declares the persistence methods needed for proxy IP endpoints.
type ProxyStore interface {
	ListProxyIPs(ctx context.Context) ([]domain.ProxyIP, error)
	AvailableProxyIPs(ctx context.Context, ipType string, excludeAccountID *uuid.UUID) ([]domain.ProxyIP, error)
	ProxyIPByAddress(ctx context.Context, ipAddress string) (domain.ProxyIP, error)
}

type proxyIPResponse struct {
	IPAddress           string    `json:"ipAddress"`
	IPType              string    `json:"ipType"`
	Host                string    `json:"host"`
	Port                int       `json:"port"`
	ValidFrom           time.Time `json:"validFrom"`
	ValidUntil          time.Time `json:"validUntil"`
	Plan                string    `json:"plan"`
	IsAssigned          bool      `json:"isAssigned"`
	AssignedAccountID   *string   `json:"assignedAccountId"`
	AssignedAccountName *string   `json:"assignedAccountName"`
}

type availableIPResponse struct {
	IPAddress  string    `json:"ipAddress"`
	IPType     string    `json:"ipType"`
	Host       string    `json:"host"`
	Port       int       `json:"port"`
	ValidFrom  time.Time `json:"validFrom"`
	ValidUntil time.Time `json:"validUntil"`
	Plan       string    `json:"plan"`
}

type availableIPsGroupedResponse struct {
	IPv4 []availableIPResponse `json:"ipv4"`
	IPv6 []availableIPResponse `json:"ipv6"`
}

// getProxyIPs handles GET /api/v1/proxy-ips.
func (h *handlers) getProxyIPs(w http.ResponseWriter, r *http.Request) {
	ips, err := h.store.ListProxyIPs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list proxy IPs")
		return
	}

	resp := make([]proxyIPResponse, 0, len(ips))
	for _, ip := range ips {
		var accountIDStr *string
		if ip.AssignedAccountID != nil {
			s := ip.AssignedAccountID.String()
			accountIDStr = &s
		}
		resp = append(resp, proxyIPResponse{
			IPAddress:           ip.IPAddress,
			IPType:              ip.IPType,
			Host:                ip.Host,
			Port:                ip.Port,
			ValidFrom:           ip.ValidFrom,
			ValidUntil:          ip.ValidUntil,
			Plan:                ip.Plan,
			IsAssigned:          ip.IsAssigned,
			AssignedAccountID:   accountIDStr,
			AssignedAccountName: ip.AssignedAccountName,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// getAvailableProxyIPs handles GET /api/v1/proxy-ips/available.
func (h *handlers) getAvailableProxyIPs(w http.ResponseWriter, r *http.Request) {
	var excludeID *uuid.UUID
	if rawID := r.URL.Query().Get("accountId"); rawID != "" {
		id, err := uuid.Parse(rawID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid accountId query parameter")
			return
		}
		excludeID = &id
	}

	ips, err := h.store.AvailableProxyIPs(r.Context(), "", excludeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list available proxy IPs")
		return
	}

	result := availableIPsGroupedResponse{
		IPv4: make([]availableIPResponse, 0),
		IPv6: make([]availableIPResponse, 0),
	}

	for _, ip := range ips {
		item := availableIPResponse{
			IPAddress:  ip.IPAddress,
			IPType:     ip.IPType,
			Host:       ip.Host,
			Port:       ip.Port,
			ValidFrom:  ip.ValidFrom,
			ValidUntil: ip.ValidUntil,
			Plan:       ip.Plan,
		}
		if ip.IPType == "ipv4" {
			result.IPv4 = append(result.IPv4, item)
		} else if ip.IPType == "ipv6" {
			result.IPv6 = append(result.IPv6, item)
		}
	}

	writeJSON(w, http.StatusOK, result)
}
