package pipeline

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuxiliarySourceCommitRollbackAndHistory(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "commit-undo-redo"
		if fail {
			name = "import-failure"
		}
		t.Run(name, func(t *testing.T) {
			p := newProject(t)
			assetPath := write(t, p, "assets/animation.txt", "old")
			companion := filepath.Join(p.Root, "assets/animation.frames.json")
			imp := &rollbackConflictImporter{fakeImporter: fakeImporter{version: 1}}
			if fail {
				imp.mutate = func() error { return nil }
			}
			pl := openPipeline(t, p, imp)
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			initial, err := pl.ImportAll()
			if err != nil || initial == nil || len(initial.Failed) != 0 || len(initial.Retry) != 0 {
				t.Fatalf("initial import: %+v, %v", initial, err)
			}
			edits := []SourceEdit{
				{Path: "assets/animation.txt", Before: []byte("old"), After: []byte("candidate")},
				{Path: "assets/animation.frames.json", After: []byte(`{"frames":[1,2]}`), Auxiliary: true},
			}
			batch, err := pl.ApplySources(edits, false)
			if fail {
				if err == nil {
					t.Fatal("injected import failure was accepted")
				}
				if data, err := os.ReadFile(assetPath); err != nil || string(data) != "old" {
					t.Fatalf("source did not roll back: %q, %v", data, err)
				}
				if _, err := os.Stat(companion); !os.IsNotExist(err) {
					t.Fatalf("companion survived rollback: %v", err)
				}
				if status, err := SourceRecoveryStatus(p); err != nil || status != nil {
					t.Fatalf("successful rollback left recovery pending: %+v, %v", status, err)
				}
				return
			}
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("commit: %+v, %v", batch, err)
			}
			if _, ok := pl.Lookup(edits[1].Path); ok {
				t.Fatal("auxiliary companion unexpectedly obtained an AssetID")
			}
			if _, err := os.Stat(companion + MetaSuffix); !os.IsNotExist(err) {
				t.Fatalf("auxiliary companion obtained metadata: %v", err)
			}
			for _, back := range []bool{true, false} {
				batch, err := pl.ApplySources(edits, back)
				if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
					t.Fatalf("history back=%v: %+v, %v", back, batch, err)
				}
				data, err := os.ReadFile(companion)
				if back && !os.IsNotExist(err) {
					t.Fatalf("Undo retained companion: %v", err)
				}
				if !back && (err != nil || !bytes.Equal(data, edits[1].After)) {
					t.Fatalf("Redo lost companion: %q, %v", data, err)
				}
			}
			if err := os.WriteFile(companion, []byte("external"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := pl.ApplySources(edits, true); err == nil || !strings.Contains(err.Error(), "changed") {
				t.Fatalf("external change was accepted: %v", err)
			}
			if data, _ := os.ReadFile(assetPath); string(data) != "candidate" {
				t.Fatal("preflight conflict changed the imported source")
			}
		})
	}
}

func TestAuxiliarySourceCannotBypassImporterOrMetadata(t *testing.T) {
	p := newProject(t)
	pl := openPipeline(t, p, &fakeImporter{version: 1})
	for _, rel := range []string{"assets/image.txt", "assets/image.txt.meta", "assets/image.txt.META"} {
		if _, err := pl.ApplySources([]SourceEdit{{Path: rel, After: []byte("invalid"), Auxiliary: true}}, false); err == nil {
			t.Errorf("auxiliary bypass accepted %s", rel)
		}
		if _, err := os.Stat(filepath.Join(p.Root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("rejected edit wrote %s: %v", rel, err)
		}
	}
}

func TestVerifyCurrentRejectsPendingSourceJournal(t *testing.T) {
	p := newProject(t)
	write(t, p, "assets/a.txt", "old")
	pl := openPipeline(t, p, &fakeImporter{version: 1})
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("initial import: %+v, %v", batch, err)
	}
	id, ok := pl.Lookup("assets/a.txt")
	if !ok || pl.VerifyCurrent(id) != nil {
		t.Fatal("initial asset was not current")
	}
	root, err := os.OpenRoot(p.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	journal, err := startSourceJournal(root, []SourceEdit{{Path: "assets/a.txt", Before: []byte("old"), After: []byte("candidate")}}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := journal.finish("restored"); err != nil {
			t.Errorf("fixture journal cleanup: %v", err)
		}
	}()
	if err := pl.VerifyCurrent(id); err == nil {
		t.Fatal("strict export accepted an asset while a source journal was pending")
	}
	if data, err := os.ReadFile(filepath.Join(p.Root, "assets", "a.txt")); err != nil || string(data) != "old" {
		t.Fatalf("checking pending recovery modified the source: %q, %v", data, err)
	}
}
