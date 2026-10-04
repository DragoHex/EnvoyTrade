package httpapi

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

func recoveryMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rErr := recover(); rErr != nil {
				if logger != nil {
					logger.Error("panic recovered in http request",
						"error", rErr,
						"method", r.Method,
						"path", r.URL.Path,
						"stack", string(debug.Stack()),
					)
				}
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
