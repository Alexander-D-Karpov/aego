package pipeline

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/format/aetext"
)

func TestCachedMetadataCannotImportPreviewOrOverwriteNewerSidecar(t *testing.T) {
	for _, operation := range []string{"import", "import-all", "preview", "settings", "subs"} {
		t.Run(operation, func(t *testing.T) {
			p := newProject(t)
			file := write(t, p, "assets/a.txt", "original")
			pl := openPipeline(t, p, &fakeImporter{version: 1})
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			id, ok := pl.Lookup("assets/a.txt")
			if !ok {
				t.Fatal("source was not indexed")
			}
			batch, err := pl.ImportAll()
			if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v %v", batch, err)
			}
			old, err := os.ReadFile(file + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			newer := bytes.Replace(old, []byte("version=1"), []byte("version=2"), 1)
			if bytes.Equal(newer, old) {
				t.Fatal("fixture does not have a version marker")
			}
			if err := os.WriteFile(file+MetaSuffix, newer, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte("external"), 0o644); err != nil { // same length, forces a new key
				t.Fatal(err)
			}
			switch operation {
			case "import", "import-all":
				if operation == "import" {
					batch, err = pl.Import(id)
				} else {
					batch, err = pl.ImportAll()
				}
				if err == nil && (batch == nil || batch.Failed[id] == nil) {
					t.Fatalf("import accepted unsupported metadata: %+v %v", batch, err)
				}
			case "preview":
				if result, err := pl.Preview("assets/a.txt", []byte("preview"), nil); err == nil {
					t.Fatalf("preview used obsolete cached metadata: %+v", result)
				}
			case "settings":
				if err := pl.SetSettings(id, Settings{"mode": aetext.Ident("new")}); err == nil {
					t.Fatal("SetSettings overwrote newer metadata")
				}
			case "subs":
				if err := pl.SetSubs(id, []SubAsset{{Name: "frame", ID: ids.NewAssetID(), Rect: [4]int{0, 0, 1, 1}}}); err == nil {
					t.Fatal("SetSubs overwrote newer metadata")
				}
			}
			if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, newer) {
				t.Fatal("operation changed external metadata", err)
			}
			if got, err := os.ReadFile(file); err != nil || string(got) != "external" {
				t.Fatal("operation changed external source", err)
			}
			if err := pl.VerifyCurrent(id); err == nil {
				t.Fatal("strict verification accepted unsupported metadata")
			}
		})
	}
}

type metadataChangingImporter struct {
	fakeImporter
	change func()
}

func (i *metadataChangingImporter) Import(ctx *ImportContext) (*Result, error) {
	if change := i.change; change != nil {
		i.change = nil
		change()
	}
	return i.fakeImporter.Import(ctx)
}

func TestCachedMetadataChangeDuringImportDoesNotPublish(t *testing.T) {
	for _, operation := range []string{"import", "preview"} {
		t.Run(operation, func(t *testing.T) {
			p := newProject(t)
			file := write(t, p, "assets/a.txt", "original")
			imp := &metadataChangingImporter{fakeImporter: fakeImporter{version: 1}}
			pl := openPipeline(t, p, imp)
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			id, ok := pl.Lookup("assets/a.txt")
			if !ok {
				t.Fatal("source was not indexed")
			}
			batch, err := pl.ImportAll()
			if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v %v", batch, err)
			}
			artifact, ok := pl.PrimaryArtifact(id)
			if !ok || artifact.Kind != asset.ArtScene {
				t.Fatal("initial artifact is missing")
			}
			artifactPath := filepath.Join(p.CacheDir(), "artifacts", filepath.Base(artifact.Path))
			beforeArtifact, err := os.ReadFile(artifactPath)
			if err != nil {
				t.Fatal(err)
			}
			old, err := os.ReadFile(file + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			newer := bytes.Replace(old, []byte("version=1"), []byte("version=2"), 1)
			if bytes.Equal(newer, old) {
				t.Fatal("missing version marker")
			}
			imp.change = func() {
				if err := os.WriteFile(file+MetaSuffix, newer, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(file, []byte("external"), 0o644); err != nil {
				t.Fatal(err)
			}
			if operation == "import" {
				batch, err = pl.Import(id)
				if err == nil && (batch == nil || batch.Failed[id] == nil) {
					t.Fatalf("published against metadata changed inside importer: %+v %v", batch, err)
				}
			} else if result, err := pl.Preview("assets/a.txt", []byte("preview"), nil); err == nil {
				t.Fatalf("preview accepted metadata changed inside importer: %+v", result)
			}
			if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, newer) {
				t.Fatal("import overwrote changed metadata", err)
			}
			if got, err := os.ReadFile(artifactPath); err != nil || !bytes.Equal(got, beforeArtifact) {
				t.Fatal("import published an artifact after metadata changed", err)
			}
		})
	}
}

func TestMetadataResolverPreservesByteOnlyExternalChanges(t *testing.T) {
	for _, phase := range []string{"before-resolver", "after-resolver"} {
		t.Run(phase, func(t *testing.T) {
			p := newProject(t)
			file := write(t, p, "assets/a.txt", "old")
			imp := &canonicalRollbackImporter{fakeImporter: fakeImporter{version: 1}}
			pl := openPipeline(t, p, imp)
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			id, ok := pl.Lookup("assets/a.txt")
			if !ok {
				t.Fatal("source was not indexed")
			}
			batch, err := pl.ImportAll()
			if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v %v", batch, err)
			}
			artifact, ok := pl.PrimaryArtifact(id)
			if !ok {
				t.Fatal("initial artifact is missing")
			}
			artifactPath := filepath.Join(p.CacheDir(), "artifacts", filepath.Base(artifact.Path))
			beforeArtifact, err := os.ReadFile(artifactPath)
			if err != nil {
				t.Fatal(err)
			}
			var external []byte
			change := func() error {
				current, err := os.ReadFile(file + MetaSuffix)
				if err != nil {
					return err
				}
				external = append(bytes.Clone(current), '\n')
				return os.WriteFile(file+MetaSuffix, external, 0o644)
			}
			if phase == "before-resolver" {
				imp.beforeResolve = change
			} else {
				imp.afterResolve = change
			}
			if err := os.WriteFile(file, []byte("candidate"), 0o644); err != nil {
				t.Fatal(err)
			}
			batch, err = pl.Import(id)
			if err == nil && (batch == nil || batch.Failed[id] == nil && len(batch.Retry) == 0) {
				t.Fatalf("accepted a metadata edit during resolution: %+v %v", batch, err)
			}
			if len(external) == 0 {
				t.Fatal("fixture did not reach the resolver")
			}
			if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, external) {
				t.Fatal("resolver overwrote external metadata", err)
			}
			if got, err := os.ReadFile(artifactPath); err != nil || !bytes.Equal(got, beforeArtifact) {
				t.Fatal("import published an artifact after the metadata conflict", err)
			}
		})
	}
}

func TestCachedMetadataKeepsMissingAndDuplicateWorkflows(t *testing.T) {
	t.Run("missing read-only", func(t *testing.T) {
		p := newProject(t)
		file := write(t, p, "assets/a.txt", "original")
		pl := openPipeline(t, p, &fakeImporter{version: 1})
		pl.SetMetaMode(MetaReadOnly)
		if _, _, err := pl.Scan(); err != nil {
			t.Fatal(err)
		}
		id, ok := pl.Lookup("assets/a.txt")
		if !ok {
			t.Fatal("missing metadata did not get a derived ID")
		}
		batch, err := pl.Import(id)
		if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
			t.Fatalf("derived import failed: %+v %v", batch, err)
		}
		if _, err := os.Stat(file + MetaSuffix); !os.IsNotExist(err) {
			t.Fatalf("read-only import wrote metadata: %v", err)
		}
		// A new external sidecar must not be treated as the same missing one.
		external := WriteMeta(NewMeta("fake", 1, Settings{}))
		if err := os.WriteFile(file+MetaSuffix, external, 0o644); err != nil {
			t.Fatal(err)
		}
		batch, err = pl.Import(id)
		if err == nil && (batch == nil || batch.Failed[id] == nil && len(batch.Retry) == 0) {
			t.Fatalf("accepted sidecar created after scan: %+v %v", batch, err)
		}
		if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, external) {
			t.Fatal("derived import changed external metadata", err)
		}
	})
	t.Run("duplicate", func(t *testing.T) {
		p := newProject(t)
		a := write(t, p, "assets/a.txt", "first")
		b := write(t, p, "assets/b.txt", "second")
		pl := openPipeline(t, p, &fakeImporter{version: 1})
		if _, _, err := pl.Scan(); err != nil {
			t.Fatal(err)
		}
		meta, err := os.ReadFile(a + MetaSuffix)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(b+MetaSuffix, meta, 0o644); err != nil {
			t.Fatal(err)
		}
		// Rescan with b newly discovered so duplicate ownership is resolved.
		pl.Close()
		pl = openPipeline(t, p, &fakeImporter{version: 1})
		if _, _, err := pl.Scan(); err != nil {
			t.Fatal(err)
		}
		batch, err := pl.ImportAll()
		if err != nil || batch == nil || len(batch.Imported) != 2 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
			t.Fatalf("duplicate-derived import failed: %+v %v", batch, err)
		}
	})
}

func TestCachedMetadataInReadOnlyPipeline(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "original")
	meta := NewMeta("fake", 1, Settings{"mode": aetext.Ident("plain")})
	original := WriteMeta(meta)
	if err := os.WriteFile(file+MetaSuffix, original, 0o644); err != nil {
		t.Fatal(err)
	}
	pl := openPipeline(t, p, &fakeImporter{version: 1})
	pl.SetMetaMode(MetaReadOnly)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	id, ok := pl.Lookup("assets/a.txt")
	if !ok || id != meta.ID {
		t.Fatalf("source was not indexed with its existing ID: %v %v", id, ok)
	}
	batch, err := pl.Import(id)
	if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("read-only import with current metadata: %+v %v", batch, err)
	}
	external := append(bytes.Clone(original), '\n')
	if err := os.WriteFile(file+MetaSuffix, external, 0o644); err != nil {
		t.Fatal(err)
	}
	batch, err = pl.Import(id)
	if err == nil && (batch == nil || batch.Failed[id] == nil && len(batch.Retry) == 0) {
		t.Fatalf("read-only import accepted a changed sidecar: %+v %v", batch, err)
	}
	if result, err := pl.Preview("assets/a.txt", []byte("preview"), nil); err == nil {
		t.Fatalf("read-only preview accepted a changed sidecar: %+v", result)
	}
	if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, external) {
		t.Fatal("read-only pipeline changed external metadata", err)
	}
}

func TestRepairDoesNotOverwriteMetadataThatAppearedAfterScan(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "original")
	pl := openPipeline(t, p, &fakeImporter{version: 1})
	pl.SetMetaMode(MetaReadOnly)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	issues, _ := pl.MetaIssues()
	if len(issues) != 1 || issues[0].Kind != MetaMissing {
		t.Fatalf("expected missing metadata: %+v", issues)
	}
	external := WriteMeta(NewMeta("fake", 1, Settings{}))
	if err := os.WriteFile(file+MetaSuffix, external, 0o644); err != nil {
		t.Fatal(err)
	}
	if n, err := pl.Repair(issues); err == nil || n != 0 {
		t.Fatalf("repair overwrote metadata that appeared externally: repaired=%d err=%v", n, err)
	}
	if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, external) {
		t.Fatal("repair changed external metadata", err)
	}
}

func TestCachedMetadataRequiresRefreshAfterValidExternalChange(t *testing.T) {
	for _, operation := range []string{"import", "preview", "settings", "subs"} {
		t.Run(operation, func(t *testing.T) {
			p := newProject(t)
			file := write(t, p, "assets/a.txt", "original")
			pl := openPipeline(t, p, &fakeImporter{version: 1})
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			id, ok := pl.Lookup("assets/a.txt")
			if !ok {
				t.Fatal("source was not indexed")
			}
			batch, err := pl.ImportAll()
			if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v %v", batch, err)
			}
			onDisk, err := os.ReadFile(file + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			meta, err := ParseMeta(onDisk, "assets/a.txt.meta")
			if err != nil {
				t.Fatal(err)
			}
			meta.Settings["mode"] = aetext.Ident("external")
			external := WriteMeta(meta)
			if err := os.WriteFile(file+MetaSuffix, external, 0o644); err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "import":
				batch, err = pl.Import(id)
				if err == nil && (batch == nil || batch.Failed[id] == nil && len(batch.Retry) == 0) {
					t.Fatalf("import used obsolete metadata: %+v %v", batch, err)
				}
			case "preview":
				if result, err := pl.Preview("assets/a.txt", []byte("preview"), nil); err == nil {
					t.Fatalf("preview used obsolete metadata: %+v", result)
				}
			case "settings":
				if err := pl.SetSettings(id, Settings{"mode": aetext.Ident("overwrite")}); err == nil {
					t.Fatal("SetSettings overwrote external metadata")
				}
			case "subs":
				if err := pl.SetSubs(id, []SubAsset{{Name: "frame", ID: ids.NewAssetID(), Rect: [4]int{0, 0, 1, 1}}}); err == nil {
					t.Fatal("SetSubs overwrote external metadata")
				}
			}
			if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, external) {
				t.Fatal("stale operation changed the external metadata", err)
			}
			batch, err = pl.ReloadMetadata(id)
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("explicit metadata refresh failed: %+v %v", batch, err)
			}
			if current, ok := pl.MetaOf(id); !ok || current.Settings.String("mode", "") != "external" {
				t.Fatal("refresh did not adopt external metadata")
			}
		})
	}
}

func TestCachedMetadataPreservesByteOnlyExternalChanges(t *testing.T) {
	for _, operation := range []string{"import", "preview", "settings", "subs"} {
		t.Run(operation, func(t *testing.T) {
			p := newProject(t)
			file := write(t, p, "assets/a.txt", "original")
			pl := openPipeline(t, p, &fakeImporter{version: 1})
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			id, ok := pl.Lookup("assets/a.txt")
			if !ok {
				t.Fatal("source was not indexed")
			}
			batch, err := pl.ImportAll()
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v %v", batch, err)
			}
			original, err := os.ReadFile(file + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			// Semantically equivalent formatting is still another owner's bytes.
			external := append(bytes.Clone(original), '\n')
			if err := os.WriteFile(file+MetaSuffix, external, 0o644); err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "import":
				batch, err = pl.Import(id)
				if err == nil && (batch == nil || batch.Failed[id] == nil && len(batch.Retry) == 0) {
					t.Fatalf("import accepted a different metadata preimage: %+v %v", batch, err)
				}
			case "preview":
				if result, err := pl.Preview("assets/a.txt", []byte("preview"), nil); err == nil {
					t.Fatalf("preview accepted a different metadata preimage: %+v", result)
				}
			case "settings":
				if err := pl.SetSettings(id, Settings{"mode": aetext.Ident("new")}); err == nil {
					t.Fatal("SetSettings overwrote different metadata bytes")
				}
			case "subs":
				if err := pl.SetSubs(id, []SubAsset{{Name: "frame", ID: ids.NewAssetID(), Rect: [4]int{0, 0, 1, 1}}}); err == nil {
					t.Fatal("SetSubs overwrote different metadata bytes")
				}
			}
			if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, external) {
				t.Fatal("operation changed externally formatted metadata", err)
			}
			batch, err = pl.ReloadMetadata(id)
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("explicit refresh did not adopt new bytes: %+v %v", batch, err)
			}
		})
	}
}

func TestAssetOperationsRefuseStaleMetadata(t *testing.T) {
	for _, operation := range []string{"move", "move-directory", "duplicate", "delete"} {
		for _, change := range []string{"unsupported-version", "byte-only"} {
			t.Run(operation+"/"+change, func(t *testing.T) {
				p := newProject(t)
				file := write(t, p, "assets/group/a.txt", "original")
				pl := openPipeline(t, p, &fakeImporter{version: 1})
				if _, _, err := pl.Scan(); err != nil {
					t.Fatal(err)
				}
				id, ok := pl.Lookup("assets/group/a.txt")
				if !ok {
					t.Fatal("source was not indexed")
				}
				batch, err := pl.ImportAll()
				if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
					t.Fatalf("initial import: %+v %v", batch, err)
				}
				original, err := os.ReadFile(file + MetaSuffix)
				if err != nil {
					t.Fatal(err)
				}
				external := append(bytes.Clone(original), '\n')
				if change == "unsupported-version" {
					external = bytes.Replace(original, []byte("version=1"), []byte("version=2"), 1)
					if bytes.Equal(original, external) {
						t.Fatal("missing version marker")
					}
				}
				if err := os.WriteFile(file+MetaSuffix, external, 0o644); err != nil {
					t.Fatal(err)
				}
				switch operation {
				case "move":
					err = pl.Move(id, "assets/group/moved.txt")
				case "move-directory":
					err = pl.MoveDir("assets/group", "assets/moved")
				case "duplicate":
					_, _, err = pl.Duplicate(id)
				case "delete":
					_, err = pl.Delete(id)
				}
				if err == nil {
					t.Fatalf("%s accepted stale %s metadata", operation, change)
				}
				if got, err := os.ReadFile(file); err != nil || string(got) != "original" {
					t.Fatal("operation changed the source", err)
				}
				if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, external) {
					t.Fatal("operation changed external metadata", err)
				}
				entries, err := os.ReadDir(filepath.Dir(file))
				if err != nil || len(entries) != 2 {
					t.Fatalf("operation left unexpected files: %v, %v", entries, err)
				}
			})
		}
	}
}

func TestScanPreservesMetadataCreatedAfterMissingCheck(t *testing.T) {
	for _, unsupported := range []bool{false, true} {
		name := "valid"
		if unsupported {
			name = "unsupported-version"
		}
		t.Run(name, func(t *testing.T) {
			p := newProject(t)
			file := write(t, p, "assets/a.txt", "original")
			pl := openPipeline(t, p, &fakeImporter{version: 1})
			meta := NewMeta("fake", 1, Settings{"mode": aetext.Ident("external")})
			external := WriteMeta(meta)
			if unsupported {
				external = bytes.Replace(external, []byte("version=1"), []byte("version=2"), 1)
			}
			checkpoint := false
			pl.metadataCreateCheckpoint = func(rel string) {
				if rel != "assets/a.txt.meta" {
					t.Fatalf("unexpected scan path %s", rel)
				}
				pl.metadataCreateCheckpoint = nil
				checkpoint = true
				if err := os.WriteFile(file+MetaSuffix, external, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			if !checkpoint {
				t.Fatal("did not reach the missing-metadata boundary")
			}
			if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, external) {
				t.Fatal("scan overwrote the newly appeared metadata", err)
			}
			if _, ok := pl.Lookup("assets/a.txt"); ok {
				t.Fatal("scan indexed metadata it had not read")
			}
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			id, indexed := pl.Lookup("assets/a.txt")
			if unsupported {
				if indexed {
					t.Fatal("next scan accepted unsupported metadata")
				}
			} else {
				if !indexed || id != meta.ID {
					t.Fatalf("next scan did not adopt valid metadata: indexed=%v id=%v", indexed, id)
				}
				batch, err := pl.Import(id)
				if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
					t.Fatalf("import after retrying scan: %+v %v", batch, err)
				}
			}
		})
	}
}

func TestMetadataGuardsRefuseSymlinkReplacement(t *testing.T) {
	for _, phase := range []string{"during-scan", "after-scan"} {
		t.Run(phase, func(t *testing.T) {
			p := newProject(t)
			file := write(t, p, "assets/a.txt", "original")
			outside := filepath.Join(t.TempDir(), "external.meta")
			external := WriteMeta(NewMeta("fake", 1, Settings{}))
			if err := os.WriteFile(outside, external, 0o644); err != nil {
				t.Fatal(err)
			}
			pl := openPipeline(t, p, &fakeImporter{version: 1})
			link := func() {
				if err := os.Symlink(outside, file+MetaSuffix); err != nil {
					t.Skipf("symlinks are unavailable: %v", err)
				}
			}
			if phase == "during-scan" {
				pl.metadataCreateCheckpoint = func(string) {
					pl.metadataCreateCheckpoint = nil
					link()
				}
			}
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			if phase == "after-scan" {
				id, ok := pl.Lookup("assets/a.txt")
				if !ok {
					t.Fatal("source was not indexed")
				}
				if err := os.Remove(file + MetaSuffix); err != nil {
					t.Fatal(err)
				}
				link()
				batch, err := pl.Import(id)
				if err == nil && (batch == nil || batch.Failed[id] == nil) {
					t.Fatalf("import accepted a symlinked metadata replacement: %+v %v", batch, err)
				}
				if err := pl.SetSettings(id, Settings{"mode": aetext.Ident("new")}); err == nil {
					t.Fatal("SetSettings accepted a symlinked metadata replacement")
				}
			} else if _, ok := pl.Lookup("assets/a.txt"); ok {
				t.Fatal("scan indexed metadata it had not read")
			}
			if info, err := os.Lstat(file + MetaSuffix); err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatal("operation replaced the external symlink", err)
			}
			if got, err := os.ReadFile(outside); err != nil || !bytes.Equal(got, external) {
				t.Fatal("operation modified the symlink target", err)
			}
		})
	}
}

func TestAssetOperationsPreserveExistingDestinationSidecars(t *testing.T) {
	for _, operation := range []string{"move", "duplicate"} {
		for _, symlink := range []bool{false, true} {
			name := operation + "/file"
			if symlink {
				name = operation + "/symlink"
			}
			t.Run(name, func(t *testing.T) {
				p := newProject(t)
				file := write(t, p, "assets/a.txt", "original")
				pl := openPipeline(t, p, &fakeImporter{version: 1})
				if _, _, err := pl.Scan(); err != nil {
					t.Fatal(err)
				}
				id, ok := pl.Lookup("assets/a.txt")
				if !ok {
					t.Fatal("source was not indexed")
				}
				before, err := os.ReadFile(file + MetaSuffix)
				if err != nil {
					t.Fatal(err)
				}
				dst := filepath.Join(p.Root, "assets", "moved.txt")
				if operation == "duplicate" {
					dst = filepath.Join(p.Root, "assets", "a copy.txt")
				}
				external := []byte("orphan sidecar belongs to another asset")
				target := dst + MetaSuffix
				if symlink {
					target = filepath.Join(t.TempDir(), "external.meta")
				}
				if err := os.WriteFile(target, external, 0o644); err != nil {
					t.Fatal(err)
				}
				if symlink {
					if err := os.Symlink(target, dst+MetaSuffix); err != nil {
						t.Skipf("symlinks are unavailable: %v", err)
					}
				}
				if operation == "move" {
					err = pl.Move(id, "assets/moved.txt")
				} else {
					_, _, err = pl.Duplicate(id)
				}
				if err == nil {
					t.Fatal("operation accepted an occupied sidecar destination")
				}
				if got, err := os.ReadFile(file); err != nil || string(got) != "original" {
					t.Fatal("operation changed the source", err)
				}
				if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, before) {
					t.Fatal("operation changed the source metadata", err)
				}
				if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, external) {
					t.Fatal("operation changed the destination sidecar", err)
				}
				if symlink {
					if info, err := os.Lstat(dst + MetaSuffix); err != nil || info.Mode()&os.ModeSymlink == 0 {
						t.Fatal("operation replaced the destination symlink", err)
					}
				}
				if _, err := os.Lstat(dst); !os.IsNotExist(err) {
					t.Fatalf("operation created the destination source: %v", err)
				}
			})
		}
	}
}
