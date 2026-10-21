package httpapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"envoytrade/internal/domain"
)

// AdminProxyStore defines persistence methods required for admin proxy IP management.
type AdminProxyStore interface {
	ListProxyIPs(ctx context.Context) ([]domain.ProxyIP, error)
	ProxyIPStats(ctx context.Context) (domain.ProxyIPStats, error)
	UpsertProxyIP(ctx context.Context, p domain.ProxyIP) error
	UpsertProxyIPsBatch(ctx context.Context, ips []domain.ProxyIP) (domain.BulkUploadResult, error)
	UpdateProxyIP(ctx context.Context, ipAddress string, validUntil time.Time, password, plan string) error
	UpdateProxyIPFull(ctx context.Context, p domain.ProxyIP) error
	DeleteProxyIP(ctx context.Context, ipAddress string) error
	ProxyIPByAddress(ctx context.Context, ipAddress string) (domain.ProxyIP, error)
}

// ProxyTester tests single proxy connectivity with an egress probe.
type ProxyTester interface {
	TestProxy(ctx context.Context, proxy domain.ProxyIP) domain.ProxyTestResult
}

type adminProxyIPsResponse struct {
	IPs   []domain.ProxyIP     `json:"ips"`
	Stats domain.ProxyIPStats `json:"stats"`
}

type patchProxyIPRequest struct {
	ValidUntil *string `json:"validUntil"`
	Password   string  `json:"password"`
	Plan       string  `json:"plan"`
}

// getAdminProxyIPs handles GET /api/v1/admin/proxy-ips.
func (h *handlers) getAdminProxyIPs(w http.ResponseWriter, r *http.Request) {
	ips, err := h.store.ListProxyIPs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list proxy IPs")
		return
	}

	stats := domain.ComputeProxyIPStats(ips)

	statusFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	typeFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))

	filtered := make([]domain.ProxyIP, 0, len(ips))
	for _, ip := range ips {
		if statusFilter != "" && strings.ToLower(string(ip.ExpiryStatus)) != statusFilter {
			continue
		}
		if typeFilter != "" && strings.ToLower(ip.IPType) != typeFilter {
			continue
		}
		filtered = append(filtered, ip)
	}

	writeJSON(w, http.StatusOK, adminProxyIPsResponse{
		IPs:   filtered,
		Stats: stats,
	})
}

// postAdminProxyIPsBulk handles POST /api/v1/admin/proxy-ips/bulk.
func (h *handlers) postAdminProxyIPsBulk(w http.ResponseWriter, r *http.Request) {
	var inputs []domain.ProxyIPInput
	if err := json.NewDecoder(r.Body).Decode(&inputs); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: expected array of proxy IP objects")
		return
	}

	result := domain.BulkUploadResult{
		Total:  len(inputs),
		Errors: make([]string, 0),
	}

	validIPs := make([]domain.ProxyIP, 0, len(inputs))
	now := time.Now()
	for idx, in := range inputs {
		p, err := domain.ParseAndValidateProxyIP(in, now)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("item %d (%s): %s", idx, in.IP, err.Error()))
			continue
		}
		validIPs = append(validIPs, p)
	}

	if len(validIPs) > 0 {
		batchRes, err := h.store.UpsertProxyIPsBatch(r.Context(), validIPs)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save batch proxy IPs: "+err.Error())
			return
		}
		result.Inserted = batchRes.Inserted
		result.Updated = batchRes.Updated
		if len(batchRes.Errors) > 0 {
			result.Errors = append(result.Errors, batchRes.Errors...)
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// postAdminProxyIP handles POST /api/v1/admin/proxy-ips.
func (h *handlers) postAdminProxyIP(w http.ResponseWriter, r *http.Request) {
	var input domain.ProxyIPInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: expected proxy IP object")
		return
	}

	p, err := domain.ParseAndValidateProxyIP(input, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.UpsertProxyIP(r.Context(), p); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save proxy IP: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status": "ok",
		"ip":     p,
	})
}

// patchAdminProxyIP handles PATCH /api/v1/admin/proxy-ips/{ip}.
func (h *handlers) patchAdminProxyIP(w http.ResponseWriter, r *http.Request) {
	ip := r.PathValue("ip")
	if ip == "" {
		writeError(w, http.StatusBadRequest, "missing ip parameter")
		return
	}

	var req patchProxyIPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var validUntilTime time.Time
	if req.ValidUntil != nil && *req.ValidUntil != "" {
		parsed, err := domain.ParseProxyTimestamp(*req.ValidUntil)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid validUntil date: "+err.Error())
			return
		}
		validUntilTime = parsed
	}

	if err := h.store.UpdateProxyIP(r.Context(), ip, validUntilTime, req.Password, req.Plan); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "proxy IP not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update proxy IP: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// putAdminProxyIP handles PUT /api/v1/admin/proxy-ips/{ip} (individual IP JSON update).
func (h *handlers) putAdminProxyIP(w http.ResponseWriter, r *http.Request) {
	routeIPStr := r.PathValue("ip")
	if routeIPStr == "" {
		writeError(w, http.StatusBadRequest, "missing ip parameter")
		return
	}

	parsedRouteIP := net.ParseIP(strings.TrimSpace(routeIPStr))
	if parsedRouteIP == nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid route IP address format: %q", routeIPStr))
		return
	}

	var input domain.ProxyIPInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: expected proxy IP object")
		return
	}

	cleanInputIP := strings.TrimSpace(input.IP)
	if cleanInputIP == "" {
		writeError(w, http.StatusBadRequest, "IP address is required in JSON")
		return
	}

	parsedInputIP := net.ParseIP(cleanInputIP)
	if parsedInputIP == nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid IP address format in JSON: %q", cleanInputIP))
		return
	}

	// Strictly verify IP match using net.ParseIP().Equal() without string matching fallback
	if !parsedInputIP.Equal(parsedRouteIP) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("IP in JSON (%s) does not match IP being updated (%s)", input.IP, routeIPStr))
		return
	}

	p, err := domain.ParseAndValidateProxyIP(input, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Target the exact primary key IP in the database from the URL route
	p.IPAddress = routeIPStr

	// Single database call: in-place update of all IP attributes
	if err := h.store.UpdateProxyIPFull(r.Context(), p); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "proxy IP not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update proxy IP: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"ip":     p,
	})
}

// deleteAdminProxyIP handles DELETE /api/v1/admin/proxy-ips/{ip}.
func (h *handlers) deleteAdminProxyIP(w http.ResponseWriter, r *http.Request) {
	ip := r.PathValue("ip")
	if ip == "" {
		writeError(w, http.StatusBadRequest, "missing ip parameter")
		return
	}

	if err := h.store.DeleteProxyIP(r.Context(), ip); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			writeError(w, http.StatusConflict, "cannot delete proxy IP: assigned to an active account")
			return
		}
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "proxy IP not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete proxy IP: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// postAdminProxyIPTest handles POST /api/v1/admin/proxy-ips/{ip}/test.
func (h *handlers) postAdminProxyIPTest(w http.ResponseWriter, r *http.Request) {
	ip := r.PathValue("ip")
	if ip == "" {
		writeError(w, http.StatusBadRequest, "missing ip parameter")
		return
	}

	p, err := h.store.ProxyIPByAddress(r.Context(), ip)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "proxy IP not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load proxy IP: "+err.Error())
		return
	}

	// Hard 3-second timeout for connectivity probe
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if h.proxyTester != nil {
		res := h.proxyTester.TestProxy(ctx, p)
		writeJSON(w, http.StatusOK, res)
		return
	}

	res := defaultTestProxy(ctx, p)
	writeJSON(w, http.StatusOK, res)
}

// postAdminProxyIPsTestAll handles POST /api/v1/admin/proxy-ips/test-all.
func (h *handlers) postAdminProxyIPsTestAll(w http.ResponseWriter, r *http.Request) {
	ips, err := h.store.ListProxyIPs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load proxy IPs: "+err.Error())
		return
	}

	var targets []domain.ProxyIP
	for _, ip := range ips {
		if ip.IsAssigned {
			targets = append(targets, ip)
		}
	}
	if len(targets) == 0 {
		targets = ips
	}

	if len(targets) == 0 {
		writeJSON(w, http.StatusOK, domain.BulkProxyTestSummary{
			Results: []domain.ProxyTestResult{},
		})
		return
	}

	results := make([]domain.ProxyTestResult, len(targets))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	for i, target := range targets {
		wg.Add(1)
		go func(idx int, p domain.ProxyIP) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			probeCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()

			if h.proxyTester != nil {
				results[idx] = h.proxyTester.TestProxy(probeCtx, p)
			} else {
				results[idx] = defaultTestProxy(probeCtx, p)
			}
		}(i, target)
	}
	wg.Wait()

	successCount := 0
	var totalLatency int64
	for _, res := range results {
		if res.Success {
			successCount++
		}
		totalLatency += res.LatencyMs
	}

	avgLatency := 0
	if len(results) > 0 {
		avgLatency = int(totalLatency / int64(len(results)))
	}

	writeJSON(w, http.StatusOK, domain.BulkProxyTestSummary{
		TotalTested:  len(results),
		SuccessCount: successCount,
		FailureCount: len(results) - successCount,
		AvgLatencyMs: avgLatency,
		Results:      results,
	})
}

func defaultTestProxy(ctx context.Context, p domain.ProxyIP) domain.ProxyTestResult {
	start := time.Now()
	res := domain.ProxyTestResult{
		IPAddress: p.IPAddress,
	}

	scheme := "https"
	if p.Port == 80 {
		scheme = "http"
	}

	u := &url.URL{
		Scheme: scheme,
		User:   url.UserPassword(p.Username, p.Password),
		Host:   fmt.Sprintf("%s:%d", p.Host, p.Port),
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(u),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   3 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "HEAD", "https://api.kite.trade", nil)
	if err != nil {
		res.LatencyMs = time.Since(start).Milliseconds()
		res.Error = err.Error()
		return res
	}

	resp, err := client.Do(req)
	res.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()

	res.Success = true
	res.EgressIP = p.IPAddress
	return res
}
