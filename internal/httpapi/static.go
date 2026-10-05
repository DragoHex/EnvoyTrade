package httpapi

import (
	"io/fs"
	"net/http"
	"strings"
)

// WithStaticFS configures the router to serve embedded static files and handle SPA fallback.
func WithStaticFS(staticFS fs.FS) Option {
	return func(c *routerConfig) {
		c.staticFS = staticFS
	}
}

// spaHandler serves static files from staticFS, falling back to index.html for client routes.
func spaHandler(staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if f, err := staticFS.Open(path); err == nil {
				stat, err := f.Stat()
				_ = f.Close()
				if err == nil && !stat.IsDir() {
					if strings.HasPrefix(path, "assets/") {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					} else {
						w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
						w.Header().Set("Pragma", "no-cache")
						w.Header().Set("Expires", "0")
					}
					fileServer.ServeHTTP(w, r)
					return
				}
			}
		}

		// Fallback to index.html for client-side routing
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
