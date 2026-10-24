package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"envoytrade/internal/auth"
	"envoytrade/internal/domain"
)

type authBypasser interface {
	BypassAuth() bool
}

func authMiddleware(next http.Handler, store AuthStore, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Public endpoints that never require session authentication
		if path == "/healthz" ||
			path == "/broker-callback" ||
			strings.HasPrefix(path, "/broker-callback/") ||
			path == "/api/v1/auth/login" ||
			path == "/api/v1/auth/register" {
			next.ServeHTTP(w, r)
			return
		}

		// Non-API routes (frontend pages, static assets) load freely
		if !strings.HasPrefix(path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Support test stubs configured to bypass auth
		if bp, ok := store.(authBypasser); ok && bp.BypassAuth() {
			next.ServeHTTP(w, r)
			return
		}

		plainToken := extractToken(r)
		if plainToken == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		tokenHash := auth.HashSessionToken(plainToken)
		sessWithUser, err := store.GetSessionWithUser(r.Context(), tokenHash)
		if err != nil || sessWithUser == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		now := time.Now()

		// 1. Sliding Idle Expiration: session expires_at must be in the future
		if sessWithUser.Session.ExpiresAt.Before(now) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// 2. Absolute Hard Max Expiration: session cannot exceed maxSessionDuration from CreatedAt
		if !sessWithUser.Session.CreatedAt.IsZero() && now.Sub(sessWithUser.Session.CreatedAt) >= maxSessionDuration {
			writeError(w, http.StatusUnauthorized, "session expired")
			return
		}

		if !sessWithUser.User.Active {
			writeError(w, http.StatusUnauthorized, "account deactivated")
			return
		}

		// 3. Rolling session touch if older than sessionTouchWindow (1 hour)
		if now.Sub(sessWithUser.Session.LastSeenAt) > sessionTouchWindow {
			newExpiry := now.Add(sessionDuration)
			if !sessWithUser.Session.CreatedAt.IsZero() {
				maxExpiry := sessWithUser.Session.CreatedAt.Add(maxSessionDuration)
				if newExpiry.After(maxExpiry) {
					newExpiry = maxExpiry
				}
			}
			_ = store.TouchSession(r.Context(), tokenHash, newExpiry)
		}

		ctx := domain.ContextWithUser(r.Context(), &sessWithUser.User)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requireAdminMiddleware(next http.Handler, store AuthStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bp, ok := store.(authBypasser); ok && bp.BypassAuth() {
			next.ServeHTTP(w, r)
			return
		}
		user, ok := domain.UserFromContext(r.Context())
		if !ok || user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if user.Role != domain.RoleAdmin {
			writeError(w, http.StatusForbidden, "forbidden: admin privileges required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

