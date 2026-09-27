package httpapi

import (
	"log/slog"
	"net/http"
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
		if path == "/broker-callback" ||
			path == "/api/v1/auth/login" ||
			path == "/api/v1/auth/register" {
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

		if sessWithUser.Session.ExpiresAt.Before(time.Now()) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// Rolling session touch if older than 15 minutes
		if time.Since(sessWithUser.Session.LastSeenAt) > 15*time.Minute {
			_ = store.TouchSession(r.Context(), tokenHash, time.Now().Add(sessionDuration))
		}

		ctx := domain.ContextWithUser(r.Context(), &sessWithUser.User)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
