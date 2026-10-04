//go:build !embed_ui

package frontend

import "io/fs"

// Dist returns nil when compiled without embed_ui.
func Dist() (fs.FS, error) {
	return nil, nil
}

// HasEmbeddedUI reports whether the binary includes the compiled frontend.
func HasEmbeddedUI() bool {
	return false
}
