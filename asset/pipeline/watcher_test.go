package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPollWatcherDetectsChanges(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("one"), 0o644)
	os.MkdirAll(filepath.Join(root, ".aego"), 0o755)

	got := make(chan []Change, 8)
	stop, err := PollWatcher{}.Start(root, []string{".aego"}, 30*time.Millisecond, func(c []Change) {
		got <- c
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	time.Sleep(50 * time.Millisecond)
	os.WriteFile(filepath.Join(root, "b.txt"), []byte("two"), 0o644)
	os.WriteFile(filepath.Join(root, ".aego", "ignored.txt"), []byte("x"), 0o644)

	select {
	case changes := <-got:
		found := false
		for _, c := range changes {
			if c.Path == ".aego/ignored.txt" {
				t.Fatal("skipped directory was watched")
			}
			if c.Path == "b.txt" && c.Kind == ChangeWrite {
				found = true
			}
		}
		if !found {
			t.Fatalf("new file not reported: %+v", changes)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no change reported")
	}

	os.Remove(filepath.Join(root, "a.txt"))
	deadline := time.After(2 * time.Second)
	for {
		select {
		case changes := <-got:
			for _, c := range changes {
				if c.Path == "a.txt" && c.Kind == ChangeRemove {
					return
				}
			}
		case <-deadline:
			t.Fatal("removal not reported")
		}
	}
}

func TestDiffTrees(t *testing.T) {
	before := map[string]fileState{
		"same.txt":    {size: 1, mtime: 100},
		"changed.txt": {size: 1, mtime: 100},
		"gone.txt":    {size: 1, mtime: 100},
	}
	after := map[string]fileState{
		"same.txt":    {size: 1, mtime: 100},
		"changed.txt": {size: 2, mtime: 200},
		"new.txt":     {size: 1, mtime: 300},
	}
	changes := diffTrees(before, after)
	kinds := map[string]ChangeKind{}
	for _, c := range changes {
		kinds[c.Path] = c.Kind
	}
	if len(changes) != 3 {
		t.Fatalf("changes = %+v", changes)
	}
	if kinds["changed.txt"] != ChangeWrite || kinds["new.txt"] != ChangeWrite || kinds["gone.txt"] != ChangeRemove {
		t.Fatalf("kinds = %+v", kinds)
	}
	if _, ok := kinds["same.txt"]; ok {
		t.Fatal("unchanged file reported")
	}
}
