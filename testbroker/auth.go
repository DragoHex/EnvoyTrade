package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type contextKey string

const userIDKey contextKey = "user_id"

// UserIDFromContext retrieves the user ID from the request context.
func UserIDFromContext(ctx context.Context) string {
	if v := ctx.Value(userIDKey); v != nil {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// parseKiteAuth parses the Kite Connect Authorization header.
func parseKiteAuth(header string) (apiKey, accessToken string, ok bool) {
	if !strings.HasPrefix(header, "token ") {
		return "", "", false
	}
	
	parts := strings.SplitN(strings.TrimPrefix(header, "token "), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// AuthMiddleware creates a middleware that validates kite authorization headers.
func AuthMiddleware(cfg *Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if strings.HasPrefix(path, "/ui") || strings.HasPrefix(path, "/api") || path == "/session/token" || path == "/" || r.Header.Get("Upgrade") == "websocket" {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			apiKey, accessToken, ok := parseKiteAuth(authHeader)
			if !ok {
				writeTokenError(w)
				return
			}

			userID, ok := cfg.UserByToken(apiKey, accessToken)
			if !ok {
				writeTokenError(w)
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeTokenError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "error",
		"error_type": "TokenException",
		"message":    "Invalid or missing token",
	})
}
