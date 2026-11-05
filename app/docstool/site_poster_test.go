package docstool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDemoPosterPrefersCurrentGameAndFallsBackToShots(t *testing.T) {
	root := t.TempDir()
	shots := filepath.Join(root, "docs", "shots", "demo.png")
	poster := filepath.Join(root, "docs", "demo-poster.png")
	if err := os.MkdirAll(filepath.Dir(shots), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := demoPoster(root); got != "" {
		t.Fatalf("missing poster: %q", got)
	}
	if err := os.WriteFile(shots, []byte("historical"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := demoPoster(root); got != shots {
		t.Fatalf("fallback: %q", got)
	}
	if err := os.WriteFile(poster, []byte("current"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := demoPoster(root); got != poster {
		t.Fatalf("current poster: %q", got)
	}
}
