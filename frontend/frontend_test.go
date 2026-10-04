package frontend_test

import (
	"testing"

	"envoytrade/frontend"
)

func TestFrontendAssets(t *testing.T) {
	dist, err := frontend.Dist()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if frontend.HasEmbeddedUI() {
		if dist == nil {
			t.Fatal("expected dist FS to be non-nil when HasEmbeddedUI() is true")
		}
		// Verify index.html exists in embedded assets
		if _, err := dist.Open("index.html"); err != nil {
			t.Fatalf("expected index.html in embedded assets, got error: %v", err)
		}
	} else {
		if dist != nil {
			t.Fatal("expected dist FS to be nil when HasEmbeddedUI() is false")
		}
	}
}
