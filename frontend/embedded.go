//go:build embed_ui

package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist returns the sub filesystem containing the built frontend.
func Dist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// HasEmbeddedUI reports whether the binary includes the compiled frontend.
func HasEmbeddedUI() bool {
	return true
}
