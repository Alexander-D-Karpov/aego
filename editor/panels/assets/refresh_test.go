package assets

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"aego/asset/pipeline"
	"aego/project"
)

func TestDirectoryRefreshFindsExternalEmptyFolders(t *testing.T) {
	proj, err := project.Create(t.TempDir(), "External directories", false)
	if err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(proj, pipeline.NewImporterRegistry(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	p := New(Deps{Pipeline: pl})
	p.now = 10
	p.refresh()
	if p.DirectoryRefreshes() != 1 || !slices.Contains(p.dirs, "assets") {
		t.Fatalf("initial directory refresh failed: count=%d dirs=%v", p.DirectoryRefreshes(), p.dirs)
	}
	first := filepath.Join(proj.AssetsDir(), "external-empty")
	if err := os.Mkdir(first, 0o755); err != nil {
		t.Fatal(err)
	}
	p.now = 11
	p.refresh()
	if p.DirectoryRefreshes() != 1 || slices.Contains(p.dirs, "assets/external-empty") {
		t.Fatal("directory list refreshed before its normal interval")
	}
	p.now = 12.01
	p.refresh()
	if p.DirectoryRefreshes() != 2 || !slices.Contains(p.dirs, "assets/external-empty") {
		t.Fatalf("periodic refresh missed an external empty folder: count=%d dirs=%v", p.DirectoryRefreshes(), p.dirs)
	}
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(proj.AssetsDir(), "another-empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	p.now = 14.02
	p.refresh()
	if p.DirectoryRefreshes() != 3 || slices.Contains(p.dirs, "assets/external-empty") || !slices.Contains(p.dirs, "assets/another-empty") {
		t.Fatalf("later refresh missed external directory changes: count=%d dirs=%v", p.DirectoryRefreshes(), p.dirs)
	}
}
