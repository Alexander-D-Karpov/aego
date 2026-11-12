package pipeline

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
	"aego/project"
)

type guardedPreviewTestImporter struct {
	fakeImporter
	change func()
	calls  int
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
