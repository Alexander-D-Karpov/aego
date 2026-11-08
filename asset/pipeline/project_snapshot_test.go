package pipeline

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"aego/project"
)

// Project Settings replaces the project value while preview workers resolve
// paths. A pipeline belongs to the root at which it was opened, even if the
// caller subsequently replaces or changes its Project value.
func TestPipelinePathsAreIndependentOfProjectReplacement(t *testing.T) {
	root := t.TempDir()
	p := &project.Project{Root: root, Name: "before"}
	pl, err := Open(p, NewImporterRegistry(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	want := filepath.Join(root, "assets", "sprite.png")
	if err := os.MkdirAll(filepath.Dir(want), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("original source"), 0644); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	bad := make(chan string, 1)
	wg.Go(func() {
		<-start
		for range 10000 {
			if got := pl.Abs("assets/sprite.png"); got != want {
				select {
				case bad <- got:
				default:
				}
				return
			}
		}
	})
	next := *p
	next.Name = "after"
	close(start)
	for range 10000 {
		*p = next
	}
	wg.Wait()
	select {
	case got := <-bad:
		t.Fatalf("path during project replacement = %s, want %s", got, want)
	default:
	}
	p.Root = t.TempDir()
	if got := pl.Abs("assets/sprite.png"); got != want {
		t.Fatalf("pipeline unexpectedly moved to the caller's new root: %s", got)
	}
	if got := pl.indexPath(); got != filepath.Join(root, ".aego", "cache", "index.aetext") {
		t.Fatalf("pipeline index unexpectedly moved: %s", got)
	}
	if data, err := pl.SourceBytes("assets/sprite.png"); err != nil || string(data) != "original source" {
		t.Fatalf("pipeline source root changed: %q, %v", data, err)
	}
}
