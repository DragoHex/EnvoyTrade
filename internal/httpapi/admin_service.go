package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"envoytrade/internal/domain"
)

// TelemetryCollector supplies real-time in-memory system metrics without querying Postgres.
type TelemetryCollector interface {
	PipelineStatus(ctx context.Context) domain.PipelineStatus
}

type killSwitchRequest struct {
	Engaged bool   `json:"engaged"`
	Reason  string `json:"reason"`
}

// getAdminServiceStatus handles GET /api/v1/admin/status (100% in-memory telemetry).
func (h *handlers) getAdminServiceStatus(w http.ResponseWriter, r *http.Request) {
	if h.telemetry != nil {
		status := h.telemetry.PipelineStatus(r.Context())
		writeJSON(w, http.StatusOK, status)
		return
	}

	// Fallback when telemetry collector is not explicitly configured
	var ks domain.KillSwitchStatus
	if h.engine != nil {
		ks = h.engine.KillSwitchStatus()
	}
	writeJSON(w, http.StatusOK, domain.PipelineStatus{
		KillSwitch: ks,
	})
}

// postAdminKillSwitch handles POST /api/v1/admin/killswitch.
func (h *handlers) postAdminKillSwitch(w http.ResponseWriter, r *http.Request) {
	var req killSwitchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	engagedBy := "system"
	if user, ok := domain.UserFromContext(r.Context()); ok && user != nil {
		engagedBy = user.Email
	}

	if h.engine != nil {
		h.engine.SetKillSwitch(req.Engaged, req.Reason, engagedBy)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "ok",
			"killSwitch": h.engine.KillSwitchStatus(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// getKillSwitchStatus handles GET /api/v1/killswitch (accessible to all authenticated users).
func (h *handlers) getKillSwitchStatus(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		writeJSON(w, http.StatusOK, domain.KillSwitchStatus{Engaged: false})
		return
	}
	writeJSON(w, http.StatusOK, h.engine.KillSwitchStatus())
}

// getAdminBrokerReadiness handles GET /api/v1/admin/broker-readiness.
func (h *handlers) getAdminBrokerReadiness(w http.ResponseWriter, r *http.Request) {
	readiness, err := h.store.BrokerReadiness(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load broker readiness: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, readiness)
}

type adminOrderIssuesResponse struct {
	Issues []domain.OrderIssue `json:"issues"`
	Total  int                 `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

// getAdminOrderIssues handles GET /api/v1/admin/order-issues.
func (h *handlers) getAdminOrderIssues(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > 100 {
		limit = 100
	}

	offset := 0
	if oStr := r.URL.Query().Get("offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}

	issues, total, err := h.store.ListRecentOrderIssues(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list order issues: "+err.Error())
		return
	}
	if issues == nil {
		issues = []domain.OrderIssue{}
	}
	writeJSON(w, http.StatusOK, adminOrderIssuesResponse{
		Issues: issues,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// getAdminExecutionFeed handles GET /api/v1/admin/execution-feed.
func (h *handlers) getAdminExecutionFeed(w http.ResponseWriter, r *http.Request) {
	if h.executionFeed == nil {
		writeJSON(w, http.StatusOK, []domain.ExecutionEvent{})
		return
	}
	events := h.executionFeed.Recent()
	if events == nil {
		events = []domain.ExecutionEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

// getAdminInstrumentStatus handles GET /api/v1/admin/instruments/status.
func (h *handlers) getAdminInstrumentStatus(w http.ResponseWriter, r *http.Request) {
	brokers, err := h.store.BrokerScripDetails(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load broker scrip details: "+err.Error())
		return
	}
	if brokers == nil {
		brokers = make(map[string]domain.BrokerScripStatus)
	}

	var cacheSummary domain.ScripCacheSummary
	if h.instrumentCache != nil {
		lastAutoLoad := h.instrumentCache.LastAutoLoad()
		var lastAutoLoadPtr *time.Time
		if !lastAutoLoad.IsZero() {
			lastAutoLoadPtr = &lastAutoLoad
		}
		cacheSummary = domain.ScripCacheSummary{
			ActiveCount:  h.instrumentCache.Size(),
			Capacity:     h.instrumentCache.Capacity(),
			LastAutoLoad: lastAutoLoadPtr,
		}
	}

	overallStatus := "healthy"
	var alerts []string

	for name, b := range brokers {
		if b.Status == "empty" {
			overallStatus = "error"
			alerts = append(alerts, name+": 0 instruments in master database")
		} else if b.Status == "stale" && overallStatus != "error" {
			overallStatus = "warning"
			alerts = append(alerts, name+": instrument master is older than 24h")
		}
	}

	if h.instrumentCache != nil && h.instrumentCache.Size() == 0 {
		if overallStatus != "error" {
			overallStatus = "warning"
		}
		alerts = append(alerts, "in-memory instrument cache is empty (0 active F&O loaded)")
	}

	if alerts == nil {
		alerts = []string{}
	}

	resp := domain.ScripMasterSummary{
		Status:  overallStatus,
		Cache:   cacheSummary,
		Brokers: brokers,
		Alerts:  alerts,
	}
	writeJSON(w, http.StatusOK, resp)
}

type adminInstrumentSyncRequest struct {
	Broker string `json:"broker"`
}

// postAdminInstrumentSync handles POST /api/v1/admin/instruments/sync.
func (h *handlers) postAdminInstrumentSync(w http.ResponseWriter, r *http.Request) {
	if h.instrumentSyncer == nil {
		writeError(w, http.StatusServiceUnavailable, "instrument syncer not configured")
		return
	}

	var req adminInstrumentSyncRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	broker := strings.ToLower(strings.TrimSpace(req.Broker))
	if broker == "" {
		broker = "all"
	}

	var onComplete func(context.Context) error
	if h.instrumentCache != nil {
		onComplete = func(c context.Context) error {
			return h.instrumentCache.AutoLoadActiveFO(c)
		}
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if err := h.instrumentSyncer.SyncBroker(bgCtx, broker, onComplete); err != nil {
			if h.logger != nil {
				h.logger.Error("on-demand instrument sync failed", "broker", broker, "error", err)
			}
		} else {
			if h.logger != nil {
				h.logger.Info("on-demand instrument sync completed successfully", "broker", broker)
			}
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "sync_started",
		"broker":  broker,
		"message": "Instrument sync triggered in background",
	})
}
