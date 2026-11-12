package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"aego/asset"
	"aego/core/ids"
	"aego/format/aetext"
	"aego/project"
)

type guardedPreviewTestImporter struct {
	fakeImporter
	change func()
	calls  int
}

type blockingGuardedPreviewImporter struct {
	fakeImporter
	entered, release chan struct{}
}

func (*blockingGuardedPreviewImporter) GuardedMetadataCandidate(_ string, metadata []byte) (bool, error) {
	return bytes.Contains(metadata, []byte("version=2")), nil
}

func (i *blockingGuardedPreviewImporter) PreviewGuardedMetadata(_ context.Context, ctx *ImportContext, _ []byte) (*Result, error) {
	close(i.entered)
	if ctx.Meta != nil || ctx.Read != nil || ctx.Lookup != nil || ctx.SourcePath != nil {
		return nil, errors.New("unlocked renderer received mutable pipeline callbacks")
	}
	<-i.release
	return &Result{Kind: asset.ArtScene, Artifacts: []Produced{{Kind: asset.ArtScene, Data: bytes.Clone(ctx.Source)}}}, nil
}

func TestGuardedPreviewDoesNotBlockPipelineAndRejectsStaleResult(t *testing.T) {
	for _, change := range []string{"unchanged", "metadata", "metadata-aba", "closed", "cancel"} {
		t.Run(change, func(t *testing.T) {
			p := newProject(t)
			file := write(t, p, "assets/a.txt", "original")
			imp := &blockingGuardedPreviewImporter{fakeImporter: fakeImporter{version: 1}, entered: make(chan struct{}), release: make(chan struct{})}
			pl := openPipeline(t, p, imp)
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			id, ok := pl.Lookup("assets/a.txt")
			if !ok {
				t.Fatal("source was not indexed")
			}
			batch, err := pl.Import(id)
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v %v", batch, err)
			}
			meta, err := os.ReadFile(file + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			candidate := bytes.Replace(meta, []byte("version=1"), []byte("version=2"), 1)
			var once sync.Once
			release := func() { once.Do(func() { close(imp.release) }) }
			t.Cleanup(release)
			type previewResult struct {
				result *Result
				err    error
			}
			previewDone := make(chan previewResult, 1)
			request, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				result, err := pl.PreviewWithMetadataContext(request, "assets/a.txt", []byte("original"), candidate, nil)
				previewDone <- previewResult{result, err}
			}()
			select {
			case <-imp.entered:
			case got := <-previewDone:
				t.Fatalf("preview never entered the renderer: %+v %v", got.result, got.err)
			case <-time.After(2 * time.Second):
				release()
				t.Fatal("preview did not enter the renderer")
			}
			operationDone := make(chan error, 1)
			go func() {
				switch change {
				case "unchanged":
					batch, err := pl.Import(id)
					if err == nil && (batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0) {
						err = errors.New("unrelated import failed or requested retry")
					}
					operationDone <- err
				case "metadata", "metadata-aba":
					settings := imp.Defaults()
					settings["mode"] = aetext.Ident("new")
					err := pl.SetSettings(id, settings)
					if err == nil && change == "metadata-aba" {
						err = pl.SetSettings(id, imp.Defaults())
					}
					operationDone <- err
				case "closed":
					pl.Close()
					operationDone <- nil
				case "cancel":
					cancel()
					operationDone <- nil
				}
			}()
			select {
			case err := <-operationDone:
				if err != nil {
					release()
					<-previewDone
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				release()
				<-previewDone
				<-operationDone
				t.Fatal("private preview held the pipeline lock during rendering")
			}
			release()
			got := <-previewDone
			if change == "unchanged" {
				if got.err != nil || got.result == nil {
					t.Fatalf("unchanged preview was rejected: %+v %v", got.result, got.err)
				}
			} else if got.err == nil || got.result != nil {
				t.Fatalf("preview accepted %s during rendering: %+v %v", change, got.result, got.err)
			}
			if change == "cancel" && !errors.Is(got.err, context.Canceled) {
				t.Fatalf("preview lost cancellation: %v", got.err)
			}
			if got, err := os.ReadFile(file); err != nil || string(got) != "original" {
				t.Fatal("private preview changed source", err)
			}
		})
	}
}

func TestGuardedPreviewReleasesLockAfterRendererPanic(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "original")
	imp := &guardedPreviewTestImporter{fakeImporter: fakeImporter{version: 1}}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	metadata, err := os.ReadFile(file + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	candidate := bytes.Replace(metadata, []byte("version=1"), []byte("version=2"), 1)
	imp.change = func() { panic("renderer fixture") }
	var caught any
	func() {
		defer func() { caught = recover() }()
		_, _ = pl.PreviewWithMetadata("assets/a.txt", []byte("original"), candidate, nil)
	}()
	if caught != "renderer fixture" {
		t.Fatalf("unexpected renderer panic: %v", caught)
	}
	if !pl.mu.TryLock() {
		t.Fatal("panicking renderer left the pipeline locked")
	}
	pl.mu.Unlock()
}

func (*guardedPreviewTestImporter) GuardedMetadataCandidate(_ string, metadata []byte) (bool, error) {
	return bytes.Contains(metadata, []byte("version=2")), nil
}

func (i *guardedPreviewTestImporter) PreviewGuardedMetadata(_ context.Context, ctx *ImportContext, _ []byte) (*Result, error) {
	i.calls++
	if i.change != nil {
		i.change()
	}
	return &Result{Kind: asset.ArtScene, Artifacts: []Produced{{Kind: asset.ArtScene, Data: bytes.Clone(ctx.Source)}}}, nil
}

func TestGuardedMetadataPreviewRechecksInputsBeforeReturning(t *testing.T) {
	for _, change := range []string{"none", "source", "metadata", "recovery", "cancel"} {
		t.Run(change, func(t *testing.T) {
			p := newProject(t)
			file := write(t, p, "assets/a.txt", "original")
			imp := &guardedPreviewTestImporter{fakeImporter: fakeImporter{version: 1}}
			pl := openPipeline(t, p, imp)
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			id, ok := pl.Lookup("assets/a.txt")
			if !ok {
				t.Fatal("source was not indexed")
			}
			batch, err := pl.Import(id)
			if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v %v", batch, err)
			}
			before, err := os.ReadFile(file + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			candidate := bytes.Replace(before, []byte("version=1"), []byte("version=2"), 1)
			if bytes.Equal(candidate, before) {
				t.Fatal("missing version marker")
			}
			wantSource, wantMeta := []byte("original"), bytes.Clone(before)
			request, cancel := context.WithCancel(context.Background())
			defer cancel()
			imp.change = func() {
				switch change {
				case "source":
					wantSource = []byte("external") // same length as original
					if err := os.WriteFile(file, wantSource, 0o644); err != nil {
						t.Fatal(err)
					}
				case "metadata":
					wantMeta = append(bytes.Clone(before), '\n')
					if err := os.WriteFile(file+MetaSuffix, wantMeta, 0o644); err != nil {
						t.Fatal(err)
					}
				case "recovery":
					if err := os.MkdirAll(filepath.Join(p.Root, filepath.FromSlash(sourceJournalDir)), 0o700); err != nil {
						t.Fatal(err)
					}
				case "cancel":
					cancel()
				}
			}
			result, err := pl.PreviewWithMetadataContext(request, "assets/a.txt", []byte("original"), candidate, nil)
			if imp.calls != 1 {
				t.Fatal("fixture did not enter guarded preview")
			}
			if change == "none" {
				if err != nil || result == nil || len(result.Artifacts) != 1 {
					t.Fatalf("valid guarded preview: %+v %v", result, err)
				}
			} else if err == nil || result != nil {
				t.Fatalf("preview published despite %s change: %+v %v", change, result, err)
			}
			if change == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("preview lost cancellation: %v", err)
			}
			if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, wantSource) {
				t.Fatal("preview overwrote source", err)
			}
			if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, wantMeta) {
				t.Fatal("preview overwrote metadata", err)
			}
		})
	}
}

func TestGuardedMetadataPreviewRejectsCancelledContext(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "original")
	imp := &guardedPreviewTestImporter{fakeImporter: fakeImporter{version: 1}}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	metadata, err := os.ReadFile(file + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	candidate := bytes.Replace(metadata, []byte("version=1"), []byte("version=2"), 1)
	request, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := pl.PreviewWithMetadataContext(request, "assets/a.txt", []byte("original"), candidate, nil); result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled preview: %+v %v", result, err)
	}
	if result, err := pl.PreviewWithMetadataContext(nil, "assets/a.txt", []byte("original"), candidate, nil); result != nil || err == nil {
		t.Fatalf("nil context was accepted: %+v %v", result, err)
	}
	if imp.calls != 0 {
		t.Fatal("cancelled preview reached the importer")
	}
}

func TestGuardedPreviewWithExistingReadOnlyMetadata(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "original")
	meta := NewMeta("fake", 1, Settings{"mode": aetext.Ident("plain")})
	original := WriteMeta(meta)
	if err := os.WriteFile(file+MetaSuffix, original, 0o644); err != nil {
		t.Fatal(err)
	}
	imp := &guardedPreviewTestImporter{fakeImporter: fakeImporter{version: 1}}
	pl := openPipeline(t, p, imp)
	pl.SetMetaMode(MetaReadOnly)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	candidate := bytes.Replace(original, []byte("version=1"), []byte("version=2"), 1)
	result, err := pl.PreviewWithMetadata("assets/a.txt", []byte("original"), candidate, nil)
	if err != nil || result == nil || len(result.Artifacts) != 1 || imp.calls != 1 {
		t.Fatalf("read-only guarded preview failed: %+v %v", result, err)
	}
	fingerprint := SourceFingerprint{Exists: true, Size: int64(len("original")), SHA256: sha256.Sum256([]byte("original"))}
	if err := pl.VerifyGuardedPreviewSource(context.Background(), meta.ID, "assets/a.txt", fingerprint); err != nil {
		t.Fatal("read-only preview recheck failed", err)
	}
	if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, original) {
		t.Fatal("read-only preview changed metadata", err)
	}
}

func TestGuardedMetadataPreviewRejectsReplacedProjectRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	p, err := project.Create(root, "PreviewRoot", false)
	if err != nil {
		t.Fatal(err)
	}
	file := write(t, p, "assets/a.txt", "original")
	imp := &guardedPreviewTestImporter{fakeImporter: fakeImporter{version: 1}}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	metadata, err := os.ReadFile(file + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	candidate := bytes.Replace(metadata, []byte("version=1"), []byte("version=2"), 1)
	oldRoot := filepath.Join(parent, "project-original")
	imp.change = func() {
		if err := os.Rename(root, oldRoot); err != nil {
			t.Skipf("OS refused renaming a project with an open root: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("original"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file+MetaSuffix, metadata, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	result, err := pl.PreviewWithMetadata("assets/a.txt", []byte("original"), candidate, nil)
	if imp.calls != 1 || err == nil || result != nil {
		t.Fatalf("preview accepted a replaced project root: calls=%d result=%+v err=%v", imp.calls, result, err)
	}
	for _, base := range []string{root, oldRoot} {
		if got, err := os.ReadFile(filepath.Join(base, "assets", "a.txt")); err != nil || string(got) != "original" {
			t.Fatal("preview changed a project's source", err)
		}
		if got, err := os.ReadFile(filepath.Join(base, "assets", "a.txt.meta")); err != nil || !bytes.Equal(got, metadata) {
			t.Fatal("preview changed a project's metadata", err)
		}
	}
}

func TestGuardedMetadataPreviewRejectsProjectRootReplacedBeforeStart(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	p, err := project.Create(root, "PreviewRoot", false)
	if err != nil {
		t.Fatal(err)
	}
	file := write(t, p, "assets/a.txt", "original")
	imp := &guardedPreviewTestImporter{fakeImporter: fakeImporter{version: 1}}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	metadata, err := os.ReadFile(file + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	candidate := bytes.Replace(metadata, []byte("version=1"), []byte("version=2"), 1)
	oldRoot := filepath.Join(parent, "project-original")
	if err := os.Rename(root, oldRoot); err != nil {
		t.Skipf("OS refused renaming the open project: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file+MetaSuffix, metadata, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := pl.PreviewWithMetadata("assets/a.txt", []byte("original"), candidate, nil)
	if imp.calls != 0 || err == nil || result != nil {
		t.Fatalf("preview accepted a replaced project root: calls=%d result=%+v err=%v", imp.calls, result, err)
	}
	for _, base := range []string{root, oldRoot} {
		if got, err := os.ReadFile(filepath.Join(base, "assets", "a.txt")); err != nil || string(got) != "original" {
			t.Fatal("preview changed a project's source", err)
		}
		if got, err := os.ReadFile(filepath.Join(base, "assets", "a.txt.meta")); err != nil || !bytes.Equal(got, metadata) {
			t.Fatal("preview changed a project's metadata", err)
		}
	}
}

func TestVerifyGuardedPreviewSourceRejectsStaleInputs(t *testing.T) {
	for _, reason := range []string{"none", "source", "metadata", "recovery", "other-id", "other-path", "root", "cancel", "closed"} {
		t.Run(reason, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "project")
			p, err := project.Create(root, "VerifyPreview", false)
			if err != nil {
				t.Fatal(err)
			}
			file := write(t, p, "assets/a.txt", "original")
			pl := openPipeline(t, p, &fakeImporter{version: 1})
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			id, ok := pl.Lookup("assets/a.txt")
			if !ok {
				t.Fatal("source was not indexed")
			}
			metadata, err := os.ReadFile(file + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			wantSource, wantMeta := []byte("original"), bytes.Clone(metadata)
			fingerprint := SourceFingerprint{Exists: true, Size: int64(len(wantSource)), SHA256: sha256.Sum256(wantSource)}
			path := "assets/a.txt"
			request, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch reason {
			case "source":
				wantSource = []byte("external")
				if err := os.WriteFile(file, wantSource, 0o644); err != nil {
					t.Fatal(err)
				}
			case "metadata":
				wantMeta = append(wantMeta, '\n')
				if err := os.WriteFile(file+MetaSuffix, wantMeta, 0o644); err != nil {
					t.Fatal(err)
				}
			case "recovery":
				if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(sourceJournalDir)), 0o700); err != nil {
					t.Fatal(err)
				}
			case "other-id":
				id = ids.NewAssetID()
			case "other-path":
				path = "assets/other.txt"
			case "root":
				if err := os.Rename(root, filepath.Join(parent, "old")); err != nil {
					t.Skipf("OS refused renaming the open project: %v", err)
				}
				if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, wantSource, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file+MetaSuffix, wantMeta, 0o644); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				cancel()
			case "closed":
				pl.Close()
			}
			err = pl.VerifyGuardedPreviewSource(request, id, path, fingerprint)
			if reason == "none" && err != nil || reason != "none" && err == nil {
				t.Fatalf("verification for %s: %v", reason, err)
			}
			if reason == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("verification lost cancellation: %v", err)
			}
			if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, wantSource) {
				t.Fatal("verification changed source", err)
			}
			if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, wantMeta) {
				t.Fatal("verification changed metadata", err)
			}
		})
	}
}
