package pipeline

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/core/ids"
	"aego/format/aetext"
)

func TestRefreshMetadataSnapshotDoesNotPublishOrWrite(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "original")
	imp := &fakeImporter{version: 1}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	id, ok := pl.Lookup("assets/a.txt")
	if !ok {
		t.Fatal("fixture was not indexed")
	}
	batch, err := pl.Import(id)
	if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("initial import: %+v %v", batch, err)
	}
	prior, ok := pl.PrimaryArtifact(id)
	if !ok {
		t.Fatal("fixture has no original artifact")
	}
	artifact, err := pl.Artifacts().ReadFile(prior.Path)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(file + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ParseMeta(before, "a.txt.meta")
	if err != nil {
		t.Fatal(err)
	}
	meta.Settings["mode"] = aetext.Ident("external")
	external := WriteMeta(meta)
	if err := os.WriteFile(file+MetaSuffix, external, 0o644); err != nil {
		t.Fatal(err)
	}
	if result, err := pl.PreviewWithMetadata("assets/a.txt", []byte("original"), external, nil); err == nil || result != nil {
		t.Fatal("stale cache was accepted before explicit refresh")
	}
	if err := pl.RefreshMetadataSnapshot(id, before); err == nil {
		t.Fatal("accepted the obsolete disk snapshot")
	}
	if err := pl.RefreshMetadataSnapshot(id, external); err != nil {
		t.Fatal(err)
	}
	if imp.calls["assets/a.txt"] != 1 {
		t.Fatal("refresh unexpectedly ran the importer")
	}
	if current, ok := pl.MetaOf(id); !ok || current.Settings.String("mode", "") != "external" {
		t.Fatal("refresh did not adopt the explicitly loaded snapshot")
	}
	if err := pl.VerifyCurrent(id); err == nil {
		t.Fatal("old artifact was reported current after external metadata changed")
	}
	if got, err := pl.Artifacts().ReadFile(prior.Path); err != nil || !bytes.Equal(got, artifact) {
		t.Fatal("refresh changed the last saved artifact", err)
	}
	if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, external) {
		t.Fatal("refresh wrote over external metadata", err)
	}
	result, err := pl.PreviewWithMetadata("assets/a.txt", []byte("original"), external, nil)
	if err != nil || result == nil || len(result.Artifacts) != 1 {
		t.Fatalf("preview after refresh: %+v %v", result, err)
	}
	batch, err = pl.Import(id)
	if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("explicit import after refresh: %+v %v", batch, err)
	}
	if err := pl.VerifyCurrent(id); err != nil {
		t.Fatal("explicit import did not publish current metadata", err)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "original" {
		t.Fatal("refresh or import changed the source", err)
	}
}

func TestRefreshMetadataSnapshotRejectsUnsafeInput(t *testing.T) {
	for _, reason := range []string{"newer-version", "changed-id", "changed-importer", "different-disk", "pending-recovery", "symlink"} {
		t.Run(reason, func(t *testing.T) {
			p := newProject(t)
			file := write(t, p, "assets/a.txt", "original")
			imp := &fakeImporter{version: 1}
			pl := openPipeline(t, p, imp)
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			id, ok := pl.Lookup("assets/a.txt")
			if !ok {
				t.Fatal("fixture was not indexed")
			}
			batch, err := pl.Import(id)
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v %v", batch, err)
			}
			before, err := os.ReadFile(file + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			disk, expected := bytes.Clone(before), bytes.Clone(before)
			switch reason {
			case "newer-version":
				disk = bytes.Replace(before, []byte("version=1"), []byte("version=2"), 1)
				if bytes.Equal(disk, before) {
					t.Fatal("fixture has no version marker")
				}
				expected = bytes.Clone(disk)
			case "changed-id", "changed-importer":
				m, err := ParseMeta(before, "a.txt.meta")
				if err != nil {
					t.Fatal(err)
				}
				if reason == "changed-id" {
					m.ID = ids.NewAssetID()
				} else {
					m.Importer = "another"
				}
				disk = WriteMeta(m)
				expected = bytes.Clone(disk)
			case "different-disk":
				disk = append(disk, '\n')
			case "pending-recovery":
				if err := os.MkdirAll(filepath.Join(p.Root, filepath.FromSlash(sourceJournalDir)), 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				backup := file + ".other"
				if err := os.Rename(file+MetaSuffix, backup); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Base(backup), file+MetaSuffix); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			if reason != "symlink" {
				if err := os.WriteFile(file+MetaSuffix, disk, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := pl.RefreshMetadataSnapshot(id, expected); err == nil {
				t.Fatalf("accepted unsafe metadata: %s", reason)
			}
			if current, ok := pl.MetaOf(id); !ok || current.ID != id || current.Importer != "fake" || current.Settings.String("mode", "") != "plain" {
				t.Fatal("failed refresh changed cached metadata")
			}
			if imp.calls["assets/a.txt"] != 1 {
				t.Fatal("failed refresh ran the importer")
			}
			if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, disk) {
				t.Fatal("failed refresh changed external metadata", err)
			}
			if got, err := os.ReadFile(file); err != nil || string(got) != "original" {
				t.Fatal("failed refresh changed source", err)
			}
		})
	}
}
