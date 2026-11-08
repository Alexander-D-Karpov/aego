package pipeline

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/vfs"
)

// Simulate an external edit after publication but before the import completes.
type rollbackConflictImporter struct {
	fakeImporter
	mutate  func() error
	trigger string
	called  bool
	success bool
}

func (imp *rollbackConflictImporter) Import(ctx *ImportContext) (*Result, error) {
	trigger := imp.trigger
	if trigger == "" {
		trigger = "candidate"
	}
	if string(ctx.Source) == trigger && imp.mutate != nil && !imp.called {
		imp.called = true
		if err := imp.mutate(); err != nil {
			return nil, err
		}
		if imp.success {
			return imp.fakeImporter.Import(ctx)
		}
		return nil, errors.New("injected import failure")
	}
	return imp.fakeImporter.Import(ctx)
}

func TestApplySourcesReverseRollbackPreservesExternalEdit(t *testing.T) {
	p := newProject(t)
	a := write(t, p, "assets/a.txt", "old-a")
	b := write(t, p, "assets/b.txt", "old-b")
	imp := &rollbackConflictImporter{fakeImporter: fakeImporter{version: 1}}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("initial import: %+v, %v", batch, err)
	}
	edits := []SourceEdit{
		{Path: "assets/a.txt", Before: []byte("old-a"), After: []byte("candidate")},
		{Path: "assets/b.txt", Before: []byte("old-b"), After: []byte("new-b")},
	}
	batch, err = pl.ApplySources(edits, false)
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("forward transaction: %+v, %v", batch, err)
	}
	index := asset.WriteIndex(pl.Index())
	imp.trigger = "old-a"
	imp.mutate = func() error { return os.WriteFile(a, []byte("outside-after-undo"), 0644) }
	_, err = pl.ApplySources(edits, true)
	if !imp.called || err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("expected undo conflict, called=%v, err=%v", imp.called, err)
	}
	if got, err := os.ReadFile(a); err != nil || string(got) != "outside-after-undo" {
		t.Fatalf("Undo rollback overwrote external edit: %q, %v", got, err)
	}
	if got, err := os.ReadFile(b); err != nil || string(got) != "new-b" {
		t.Fatalf("uncontested Undo rollback failed: %q, %v", got, err)
	}
	if got := asset.WriteIndex(pl.Index()); !bytes.Equal(got, index) {
		t.Fatal("failed Undo left a partially imported index")
	}
	for _, rel := range []string{"assets/a.txt", "assets/b.txt"} {
		id, ok := pl.Lookup(rel)
		if !ok || pl.VerifyCurrent(id) == nil {
			t.Errorf("%s was not marked stale after the failed Undo", rel)
		}
	}
}

func TestApplySourcesRollbackPreservesExternalChanges(t *testing.T) {
	for _, scenario := range []string{"overwrite", "delete", "sidecar", "symlink", "overwrite-success", "delete-success", "sidecar-success", "sidecar-delete-success", "guard-success", "guard-failure", "guard-meta-failure"} {
		t.Run(scenario, func(t *testing.T) {
			p := newProject(t)
			a := write(t, p, "assets/a.txt", "old-a")
			b := write(t, p, "assets/b.txt", "old-b")
			imp := &rollbackConflictImporter{fakeImporter: fakeImporter{version: 1}}
			pl := openPipeline(t, p, imp)
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			batch, err := pl.ImportAll()
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v, %v", batch, err)
			}
			meta, err := os.ReadFile(a + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			edits := []SourceEdit{
				{Path: "assets/a.txt", Before: []byte("old-a"), After: []byte("candidate")},
				{Path: "assets/b.txt", Before: []byte("old-b"), After: []byte("new-b")},
			}
			originalIndex := asset.WriteIndex(pl.Index())
			conflict := a
			var outside string
			switch scenario {
			case "overwrite", "overwrite-success":
				imp.mutate = func() error { return os.WriteFile(a, []byte("outside-a"), 0644) }
			case "delete", "delete-success":
				imp.mutate = func() error { return os.Remove(a) }
			case "sidecar", "sidecar-success", "sidecar-delete-success":
				conflict = a + MetaSuffix
				imp.mutate = func() error { return os.WriteFile(conflict, []byte("outside-meta"), 0644) }
				edits = append(edits, SourceEdit{Path: "assets/a.txt.meta", Before: meta, After: append(append([]byte(nil), meta...), '\n')})
				if scenario == "sidecar-delete-success" {
					imp.mutate = func() error { return os.Remove(conflict) }
				}
			case "symlink":
				outside = filepath.Join(t.TempDir(), "external.txt")
				if err := os.WriteFile(outside, []byte("outside-target"), 0644); err != nil {
					t.Fatal(err)
				}
				linkProbe := filepath.Join(t.TempDir(), "probe")
				if err := os.Symlink(outside, linkProbe); err != nil {
					t.Skipf("symlink is unavailable: %v", err)
				}
				imp.mutate = func() error {
					if err := os.Remove(a); err != nil {
						return err
					}
					return os.Symlink(outside, a)
				}
			case "guard-success", "guard-failure":
				conflict = write(t, p, "assets/guard.txt", "old-guard")
				imp.mutate = func() error { return os.WriteFile(conflict, []byte("outside-guard"), 0644) }
				edits = append(edits, SourceEdit{Path: "assets/guard.txt", Before: []byte("old-guard"), After: []byte("old-guard")})
			case "guard-meta-failure":
				conflict = b + MetaSuffix
				guard, err := os.ReadFile(conflict)
				if err != nil {
					t.Fatal(err)
				}
				imp.mutate = func() error { return os.WriteFile(conflict, []byte("outside-meta"), 0644) }
				edits = append(edits, SourceEdit{Path: "assets/b.txt.meta", Before: guard, After: guard})
			}
			imp.success = strings.HasSuffix(scenario, "-success")
			_, err = pl.ApplySources(edits, false)
			if !imp.called || err == nil {
				t.Fatalf("expected external-change conflict, called=%v err=%v", imp.called, err)
			}
			if !strings.Contains(err.Error(), "changed") {
				t.Errorf("expected explicit external-change conflict, got %v", err)
			}
			got, readErr := os.ReadFile(conflict)
			if scenario == "delete" || scenario == "delete-success" || scenario == "sidecar-delete-success" {
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Errorf("externally deleted file was recreated: %q, %v", got, readErr)
				}
			} else if scenario == "symlink" {
				info, err := os.Lstat(conflict)
				if err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Errorf("external symlink was replaced: %v", err)
				}
				if got, err := os.ReadFile(outside); err != nil || string(got) != "outside-target" {
					t.Errorf("symlink target changed: %q, %v", got, err)
				}
			} else {
				want := "outside-a"
				if scenario == "sidecar" || scenario == "sidecar-success" || scenario == "guard-meta-failure" {
					want = "outside-meta"
				} else if scenario == "guard-success" || scenario == "guard-failure" {
					want = "outside-guard"
				}
				if readErr != nil || string(got) != want {
					t.Errorf("external bytes lost: %q, %v; want %q", got, readErr, want)
				}
			}
			if got, err := os.ReadFile(b); err != nil || string(got) != "old-b" {
				t.Errorf("uncontested source not rolled back: %q, %v", got, err)
			}
			if strings.HasPrefix(scenario, "sidecar") {
				if got, err := os.ReadFile(filepath.Join(p.Root, "assets/a.txt")); err != nil || string(got) != "old-a" {
					t.Errorf("uncontested source not rolled back: %q, %v", got, err)
				}
			}
			if got := asset.WriteIndex(pl.Index()); !bytes.Equal(got, originalIndex) {
				t.Error("failed transaction left a partially imported index generation")
			}
			for _, rel := range []string{"assets/a.txt", "assets/b.txt"} {
				id, ok := pl.Lookup(rel)
				if !ok || pl.VerifyCurrent(id) == nil {
					t.Errorf("%s was not marked stale after the conflict", rel)
				}
			}
		})
	}
}

type sourceSubassetImporter struct {
	fakeImporter
	sub ids.AssetID
}

func (imp *sourceSubassetImporter) Import(ctx *ImportContext) (*Result, error) {
	result, err := imp.fakeImporter.Import(ctx)
	if err != nil {
		return nil, err
	}
	result.Subs = []SubResult{{ID: imp.sub, Name: "frame", Kind: asset.ArtScene,
		Artifacts: []Produced{{Kind: asset.ArtScene, Name: "source-transaction-frame.aescene", Data: ctx.Source}}}}
	return result, nil
}

type failCandidateSubasset struct{ vfs.FS }

func (f failCandidateSubasset) WriteFile(name string, data []byte) error {
	if strings.HasSuffix(name, "/source-transaction-frame.aescene") && string(data) == "candidate" {
		return errors.New("injected subasset artifact failure")
	}
	return f.FS.WriteFile(name, data)
}

func TestApplySourcesRejectsFailedSubasset(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	imp := &sourceSubassetImporter{fakeImporter: fakeImporter{version: 1}, sub: ids.NewAssetID()}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("initial import: %+v, %v", batch, err)
	}
	index := asset.WriteIndex(pl.Index())
	pl.artifacts = failCandidateSubasset{pl.artifacts}
	batch, err = pl.ApplySources([]SourceEdit{{Path: "assets/a.txt", Before: []byte("old"), After: []byte("candidate")}}, false)
	if err == nil || !strings.Contains(err.Error(), "subasset artifact failure") {
		t.Fatalf("failed subasset was accepted: batch=%+v err=%v", batch, err)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "old" {
		t.Fatalf("failed publication did not restore the source: %q, %v", got, err)
	}
	if got := asset.WriteIndex(pl.Index()); !bytes.Equal(got, index) {
		t.Fatal("failed publication did not restore the original index")
	}
}

type sourceDependentImporter struct{ fakeImporter }

func (imp *sourceDependentImporter) Import(ctx *ImportContext) (*Result, error) {
	result, err := imp.fakeImporter.Import(ctx)
	if err == nil && ctx.Path == "assets/b.txt" {
		id, ok := ctx.Lookup("assets/a.txt")
		if !ok {
			return nil, errors.New("missing dependency")
		}
		result.Deps = []ids.AssetID{id}
	}
	return result, err
}

func TestApplySourcesRejectsUnstableDependent(t *testing.T) {
	p := newProject(t)
	a := write(t, p, "assets/a.txt", "old")
	b := write(t, p, "assets/b.txt", "dependent")
	pl := openPipeline(t, p, &sourceDependentImporter{fakeImporter{version: 1}})
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("initial import: %+v, %v", batch, err)
	}
	if err := os.WriteFile(b, nil, 0644); err != nil {
		t.Fatal(err)
	}
	batch, err = pl.ApplySources([]SourceEdit{{Path: "assets/a.txt", Before: []byte("old"), After: []byte("candidate")}}, false)
	if err == nil || !strings.Contains(err.Error(), "retry") {
		t.Fatalf("unstable dependent was accepted: batch=%+v err=%v", batch, err)
	}
	if got, err := os.ReadFile(a); err != nil || string(got) != "old" {
		t.Fatalf("failed publication did not restore the source: %q, %v", got, err)
	}
	if got, err := os.ReadFile(b); err != nil || len(got) != 0 {
		t.Fatalf("external dependent was overwritten: %q, %v", got, err)
	}
}

func TestApplySourcesRejectsIndexSaveFailure(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	pl := openPipeline(t, p, &fakeImporter{version: 1})
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("initial import: %+v, %v", batch, err)
	}
	// This is a private test project: replacing its cache index with a directory
	// makes saveIndex fail without depending on root/user permission semantics.
	if err := os.Remove(pl.indexPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(pl.indexPath(), 0755); err != nil {
		t.Fatal(err)
	}
	batch, err = pl.ApplySources([]SourceEdit{{Path: "assets/a.txt", Before: []byte("old"), After: []byte("candidate")}}, false)
	if err == nil || !strings.Contains(err.Error(), "index") {
		t.Fatalf("index save failure was accepted: batch=%+v err=%v", batch, err)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "old" {
		t.Fatalf("failed publication did not restore the source: %q, %v", got, err)
	}
}

func TestApplySourcesRejectsOversizeBeforeWritingAnySource(t *testing.T) {
	large := bytes.Repeat([]byte{'x'}, maxSourceBytes+1)
	for _, back := range []bool{false, true} {
		name := "forward"
		if back {
			name = "undo"
		}
		t.Run(name, func(t *testing.T) {
			p := newProject(t)
			a := write(t, p, "assets/a.txt", "old-a")
			b := write(t, p, "assets/b.txt", "old-b")
			pl := openPipeline(t, p, &fakeImporter{version: 1})
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			batch, err := pl.ImportAll()
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v, %v", batch, err)
			}
			edits := []SourceEdit{
				{Path: "assets/b.txt", Before: []byte("old-b"), After: []byte("new-b")},
				{Path: "assets/a.txt", Before: []byte("old-a"), After: large},
			}
			if back {
				for i := range edits {
					edits[i].Before, edits[i].After = edits[i].After, edits[i].Before
				}
			}
			if _, err := pl.ApplySources(edits, back); err == nil || !strings.Contains(err.Error(), "exceeds") {
				t.Fatalf("oversize transaction was not rejected: %v", err)
			}
			for _, check := range []struct{ file, want string }{{a, "old-a"}, {b, "old-b"}} {
				got, err := os.ReadFile(check.file)
				if err != nil || string(got) != check.want {
					t.Fatalf("%s changed on rejected transaction: size=%d, err=%v", check.file, len(got), err)
				}
			}
		})
	}
}

func TestApplySourcesDoesNotCreateUnrelatedMetadataDuringScan(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "successful import"
		if fail {
			name = "failed import"
		}
		t.Run(name, func(t *testing.T) {
			p := newProject(t)
			a := write(t, p, "assets/a.txt", "old")
			imp := &rollbackConflictImporter{fakeImporter: fakeImporter{version: 1}}
			pl := openPipeline(t, p, imp)
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			batch, err := pl.ImportAll()
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v, %v", batch, err)
			}
			index := asset.WriteIndex(pl.Index())
			// Simulate a source added by another process after the last scan.
			other := write(t, p, "assets/z-external.txt", "external")
			if fail {
				imp.mutate = func() error { return nil }
			}
			batch, err = pl.ApplySources([]SourceEdit{{Path: "assets/a.txt", Before: []byte("old"), After: []byte("candidate")}}, false)
			if fail {
				if err == nil || !imp.called {
					t.Fatalf("injected import failure was not reported: %+v, %v", batch, err)
				}
				if got := asset.WriteIndex(pl.Index()); !bytes.Equal(got, index) {
					t.Error("failed transaction changed the published asset index")
				}
			} else if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("unrelated source blocked the edit: %+v, %v", batch, err)
			}
			want := "candidate"
			if fail {
				want = "old"
			}
			if got, err := os.ReadFile(a); err != nil || string(got) != want {
				t.Errorf("edited source = %q, %v; want %q", got, err, want)
			}
			if got, err := os.ReadFile(other); err != nil || string(got) != "external" {
				t.Errorf("external source changed: %q, %v", got, err)
			}
			if _, err := os.Lstat(other + MetaSuffix); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("transaction created an unrelated sidecar: %v", err)
			}
			if _, ok := pl.Lookup("assets/z-external.txt"); ok {
				t.Error("unrelated source was indexed without its persistent identity")
			}
			// The next ordinary Scan still owns sidecar creation and indexing.
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(other + MetaSuffix); err != nil {
				t.Fatalf("ordinary scan did not create the sidecar: %v", err)
			}
			if _, ok := pl.Lookup("assets/z-external.txt"); !ok {
				t.Fatal("ordinary scan did not index the deferred source")
			}
			batch, err = pl.ImportAll()
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("ordinary import after scan: %+v, %v", batch, err)
			}
		})
	}
}

func TestApplySourcesFailedCreateRemovesItsGeneratedSidecar(t *testing.T) {
	p := newProject(t)
	imp := &canonicalRollbackImporter{fakeImporter: fakeImporter{version: 1}, fail: true}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	if _, err := pl.ApplySources([]SourceEdit{{Path: "assets/new.txt", After: []byte("candidate")}}, false); err == nil {
		t.Fatal("failed import was accepted")
	}
	for _, rel := range []string{"assets/new.txt", "assets/new.txt.meta"} {
		if _, err := os.Lstat(filepath.Join(p.Root, filepath.FromSlash(rel))); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("failed create left %s behind: %v", rel, err)
		}
	}
	if id, ok := pl.Lookup("assets/new.txt"); ok {
		t.Errorf("failed create left an indexed asset: %s", id)
	}
}

func TestApplySourcesCreateUndoRedoRetainsAssetID(t *testing.T) {
	p := newProject(t)
	pl := openPipeline(t, p, &fakeImporter{version: 1})
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	edits := []SourceEdit{{Path: "assets/new.txt", After: []byte("created")}}
	batch, err := pl.ApplySources(edits, false)
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("create: %+v, %v", batch, err)
	}
	id, ok := pl.Lookup("assets/new.txt")
	if !ok {
		t.Fatal("created source was not indexed")
	}
	for _, back := range []bool{true, false, true, false} {
		batch, err = pl.ApplySources(edits, back)
		if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
			t.Fatalf("back=%v: %+v, %v", back, batch, err)
		}
		now, exists := pl.Lookup("assets/new.txt")
		if back && exists || !back && (!exists || now != id) {
			t.Fatalf("back=%v: source identity %s, exists=%v; want %s", back, now, exists, id)
		}
	}
}

func TestApplySourcesFailedCreatePreservesExternalSidecar(t *testing.T) {
	p := newProject(t)
	metaPath := filepath.Join(p.Root, "assets", "new.txt.meta")
	imp := &canonicalRollbackImporter{fakeImporter: fakeImporter{version: 1}, fail: true}
	imp.afterResolve = func() error { return os.WriteFile(metaPath, []byte("outside metadata"), 0644) }
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	if _, err := pl.ApplySources([]SourceEdit{{Path: "assets/new.txt", After: []byte("candidate")}}, false); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("external sidecar conflict not reported: %v", err)
	}
	if got, err := os.ReadFile(metaPath); err != nil || string(got) != "outside metadata" {
		t.Fatalf("external sidecar was overwritten: %q, %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(p.Root, "assets", "new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uncontested created source was not removed: %v", err)
	}
	if id, ok := pl.Lookup("assets/new.txt"); ok && pl.VerifyCurrent(id) == nil {
		t.Fatal("source with conflicted metadata was marked current")
	}
}

func TestApplySourcesRejectsNewSourceWithMissingMetadataGuard(t *testing.T) {
	p := newProject(t)
	pl := openPipeline(t, p, &fakeImporter{version: 1})
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	edits := []SourceEdit{
		{Path: "assets/new.txt", After: []byte("created")},
		{Path: "assets/new.txt.meta"}, // Explicitly require the sidecar to stay absent.
	}
	if _, err := pl.ApplySources(edits, false); err == nil {
		t.Fatal("an imported source without its required sidecar was accepted")
	}
	for _, rel := range []string{"assets/new.txt", "assets/new.txt.meta"} {
		if _, err := os.Lstat(filepath.Join(p.Root, filepath.FromSlash(rel))); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("incompatible source/sidecar request created %s: %v", rel, err)
		}
	}
}
