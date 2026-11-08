package pipeline

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"aego/core/ids"
	"aego/format/aetext"
)

type canonicalRollbackImporter struct {
	fakeImporter
	fail          bool
	beforeResolve func() error
	afterResolve  func() error
}

func (imp *canonicalRollbackImporter) ResolveMetadata(ctx *ImportContext) (Settings, error) {
	if string(ctx.Source) != "candidate" {
		return nil, nil
	}
	if imp.beforeResolve != nil {
		if err := imp.beforeResolve(); err != nil {
			return nil, err
		}
		imp.beforeResolve = nil
	}
	settings := ctx.Meta.Settings.Clone()
	settings["mode"] = aetext.Ident("resolved")
	return settings, nil
}

func (imp *canonicalRollbackImporter) Import(ctx *ImportContext) (*Result, error) {
	if string(ctx.Source) == "candidate" {
		if imp.afterResolve != nil {
			if err := imp.afterResolve(); err != nil {
				return nil, err
			}
			imp.afterResolve = nil
		}
		if imp.fail {
			return nil, errors.New("injected failure after metadata resolution")
		}
	}
	return imp.fakeImporter.Import(ctx)
}

func TestApplySourcesPreservesExternalMetadataAcrossResolution(t *testing.T) {
	for _, phase := range []string{"before-resolver-write", "after-resolver-write"} {
		t.Run(phase, func(t *testing.T) {
			p := newProject(t)
			file := write(t, p, "assets/a.txt", "old")
			imp := &canonicalRollbackImporter{fakeImporter: fakeImporter{version: 1}}
			pl := openPipeline(t, p, imp)
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			batch, err := pl.ImportAll()
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v, %v", batch, err)
			}
			original, err := os.ReadFile(file + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			meta, err := ParseMeta(original, "assets/a.txt.meta")
			if err != nil {
				t.Fatal(err)
			}
			meta.Settings["mode"] = aetext.Ident("manual")
			manual := WriteMeta(meta)
			meta.Settings["mode"] = aetext.Ident("outside")
			outside := WriteMeta(meta)
			mutate := func() error { return os.WriteFile(file+MetaSuffix, outside, 0644) }
			if phase == "before-resolver-write" {
				imp.beforeResolve = mutate
			} else {
				imp.afterResolve = mutate
			}
			edits := []SourceEdit{
				{Path: "assets/a.txt", Before: []byte("old"), After: []byte("candidate")},
				{Path: "assets/a.txt.meta", Before: original, After: manual},
			}
			if _, err = pl.ApplySources(edits, false); err == nil || !strings.Contains(err.Error(), "changed") {
				t.Fatalf("expected external-change conflict, got %v", err)
			}
			if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, outside) {
				t.Fatalf("external sidecar was overwritten: %q, %v", got, err)
			}
			if got, err := os.ReadFile(file); err != nil || string(got) != "old" {
				t.Fatalf("uncontested source was not restored: %q, %v", got, err)
			}
			if id, ok := pl.Lookup("assets/a.txt"); !ok || pl.VerifyCurrent(id) == nil {
				t.Fatal("conflicted source was not marked stale")
			}
		})
	}
}

func TestApplySourcesCanonicalMetadataRollbackAndHistory(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "history"
		if fail {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			p := newProject(t)
			file := write(t, p, "assets/a.txt", "old")
			imp := &canonicalRollbackImporter{fakeImporter: fakeImporter{version: 1}, fail: fail}
			pl := openPipeline(t, p, imp)
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			batch, err := pl.ImportAll()
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v, %v", batch, err)
			}
			meta, err := os.ReadFile(file + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			candidate, err := ParseMeta(meta, "assets/a.txt.meta")
			if err != nil {
				t.Fatal(err)
			}
			candidate.Settings["mode"] = aetext.Ident("manual")
			edits := []SourceEdit{
				{Path: "assets/a.txt", Before: []byte("old"), After: []byte("candidate")},
				{Path: "assets/a.txt.meta", Before: meta, After: WriteMeta(candidate)},
			}
			batch, err = pl.ApplySources(edits, false)
			if fail {
				if err == nil {
					t.Fatal("failed import was accepted")
				}
				got, readErr := os.ReadFile(file + MetaSuffix)
				if readErr != nil || !bytes.Equal(got, meta) {
					t.Fatalf("engine-canonicalized sidecar was not rolled back: %q, %v; err=%v", got, readErr, err)
				}
				got, readErr = os.ReadFile(file)
				if readErr != nil || string(got) != "old" {
					t.Fatalf("source was not rolled back: %q, %v", got, readErr)
				}
				id, ok := pl.Lookup("assets/a.txt")
				if !ok || pl.VerifyCurrent(id) != nil {
					t.Fatal("original import was not restored")
				}
				return
			}
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("publication: %+v, %v", batch, err)
			}
			got, err := os.ReadFile(file + MetaSuffix)
			if err != nil || !bytes.Equal(got, edits[1].After) {
				t.Fatalf("history did not adopt published metadata: %q, %v", got, err)
			}
			resolved, err := ParseMeta(got, "assets/a.txt.meta")
			if err != nil || resolved.Settings.String("mode", "") != "resolved" {
				t.Fatalf("metadata was not resolved: %+v, %v", resolved, err)
			}
			for _, back := range []bool{true, false, true, false} {
				batch, err := pl.ApplySources(edits, back)
				if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
					t.Fatalf("history back=%v: %+v, %v", back, batch, err)
				}
				got, err := os.ReadFile(file + MetaSuffix)
				want := edits[1].After
				if back {
					want = edits[1].Before
				}
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("history back=%v: sidecar=%q, %v", back, got, err)
				}
			}
		})
	}
}

func TestApplySourcesRollsBackCanonicalizedMetadataGuard(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	imp := &canonicalRollbackImporter{fakeImporter: fakeImporter{version: 1}, fail: true}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("initial import: %+v, %v", batch, err)
	}
	meta, err := os.ReadFile(file + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	edits := []SourceEdit{
		{Path: "assets/a.txt", Before: []byte("old"), After: []byte("candidate")},
		{Path: "assets/a.txt.meta", Before: meta, After: meta},
	}
	if _, err = pl.ApplySources(edits, false); err == nil || strings.Contains(err.Error(), "changed") {
		t.Fatalf("expected import failure without a false conflict: %v", err)
	}
	if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, meta) {
		t.Fatalf("canonicalized guard was not rolled back: %q, %v", got, err)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "old" {
		t.Fatalf("source was not rolled back: %q, %v", got, err)
	}
	if id, ok := pl.Lookup("assets/a.txt"); !ok || pl.VerifyCurrent(id) != nil {
		t.Fatal("the original import was not restored")
	}
}

func TestApplySourcesReadOnlyGuardPreventsMetadataResolution(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	imp := &canonicalRollbackImporter{fakeImporter: fakeImporter{version: 1}}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("initial import: %+v, %v", batch, err)
	}
	meta, err := os.ReadFile(file + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	edits := []SourceEdit{
		{Path: "assets/a.txt", Before: []byte("old"), After: []byte("candidate")},
		{Path: "assets/a.txt.meta", Before: meta, After: meta, ReadOnlyGuard: true},
	}
	if _, err = pl.ApplySources(edits, false); err == nil {
		t.Fatal("metadata resolver modified a read-only guard")
	}
	if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, meta) {
		t.Fatalf("guarded metadata was changed: %q, %v", got, err)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "old" {
		t.Fatalf("failed transaction did not restore source: %q, %v", got, err)
	}
	if id, ok := pl.Lookup("assets/a.txt"); !ok || pl.VerifyCurrent(id) != nil {
		t.Fatal("original import was not restored")
	}
	if _, err := pl.ApplySources([]SourceEdit{{Path: "assets/a.txt", Before: []byte("old"), After: []byte("candidate"), ReadOnlyGuard: true}}, false); err == nil {
		t.Fatal("a read-only guard accepted a requested write")
	}
}

func TestApplySourcesProtectsImplicitSourceMetadata(t *testing.T) {
	for _, scenario := range []string{"import-failure", "external-change"} {
		t.Run(scenario, func(t *testing.T) {
			p := newProject(t)
			file := write(t, p, "assets/a.txt", "old")
			imp := &canonicalRollbackImporter{fakeImporter: fakeImporter{version: 1}}
			pl := openPipeline(t, p, imp)
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			batch, err := pl.ImportAll()
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v, %v", batch, err)
			}
			original, err := os.ReadFile(file + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			want := original
			if scenario == "import-failure" {
				imp.fail = true
			} else {
				meta, err := ParseMeta(original, "assets/a.txt.meta")
				if err != nil {
					t.Fatal(err)
				}
				meta.Settings["mode"] = aetext.Ident("outside")
				want = WriteMeta(meta)
				imp.afterResolve = func() error { return os.WriteFile(file+MetaSuffix, want, 0644) }
			}
			// The caller supplies only the source, not an explicit .meta edit.
			batch, err = pl.ApplySources([]SourceEdit{{Path: "assets/a.txt", Before: []byte("old"), After: []byte("candidate")}}, false)
			if err == nil {
				t.Fatalf("%s was accepted: %+v", scenario, batch)
			}
			if got, err := os.ReadFile(file); err != nil || string(got) != "old" {
				t.Fatalf("source was not rolled back: %q, %v", got, err)
			}
			if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, want) {
				t.Fatalf("sidecar was not preserved: %q, %v", got, err)
			}
			if scenario == "external-change" {
				id, ok := pl.Lookup("assets/a.txt")
				if !ok || pl.VerifyCurrent(id) == nil {
					t.Fatal("externally changed metadata was not marked as a conflict")
				}
			}
		})
	}
}

// The dependent is reimported when a.txt changes. Its reference resolver can
// update the dependent sidecar even though the caller did not edit b.txt.
type dependentMetadataImporter struct {
	fakeImporter
	failSource, noMetadataChange, resolved bool
}

func (imp *dependentMetadataImporter) ResolveMetadata(ctx *ImportContext) (Settings, error) {
	if ctx.Path != "assets/b.txt" {
		return nil, nil
	}
	source, err := ctx.Read("assets/a.txt")
	if err != nil || string(source) != "candidate" {
		return nil, err
	}
	imp.resolved = true
	settings := ctx.Meta.Settings.Clone()
	if !imp.noMetadataChange {
		settings["mode"] = aetext.Ident("resolved")
	}
	return settings, nil
}

func (imp *dependentMetadataImporter) Import(ctx *ImportContext) (*Result, error) {
	if imp.failSource && ctx.Path == "assets/a.txt" && string(ctx.Source) == "candidate" {
		return nil, errors.New("injected source import failure")
	}
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

func TestApplySourcesGuardsDependentMetadataResolution(t *testing.T) {
	for _, tc := range []struct {
		name             string
		guard            bool
		readOnly         bool
		failSource       bool
		noMetadataChange bool
	}{
		{name: "unguarded-change"},
		{name: "unguarded-change-with-import-failure", failSource: true},
		{name: "unguarded-unchanged", noMetadataChange: true},
		{name: "guarded-change", guard: true},
		{name: "guarded-read-only", guard: true, readOnly: true},
		{name: "guarded-change-with-import-failure", guard: true, failSource: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newProject(t)
			a := write(t, p, "assets/a.txt", "old")
			b := write(t, p, "assets/b.txt", "dependent")
			imp := &dependentMetadataImporter{fakeImporter: fakeImporter{version: 1}, failSource: tc.failSource, noMetadataChange: tc.noMetadataChange}
			pl := openPipeline(t, p, imp)
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			batch, err := pl.ImportAll()
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial import: %+v, %v", batch, err)
			}
			before, err := os.ReadFile(b + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			edits := []SourceEdit{{Path: "assets/a.txt", Before: []byte("old"), After: []byte("candidate")}}
			if tc.guard {
				edits = append(edits, SourceEdit{Path: "assets/b.txt.meta", Before: before, After: before, ReadOnlyGuard: tc.readOnly})
			}
			batch, err = pl.ApplySources(edits, false)
			if !imp.resolved {
				t.Fatal("dependent resolver did not run")
			}
			success := (tc.guard && !tc.readOnly || tc.noMetadataChange) && !tc.failSource
			if success {
				if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
					t.Fatalf("guarded dependent import: %+v, %v", batch, err)
				}
			} else if err == nil {
				t.Error("transaction accepted an unguarded or failed dependent import")
			} else if !tc.guard && !strings.Contains(err.Error(), "dependent metadata") {
				t.Errorf("missing explicit dependent metadata diagnostic: %v", err)
			}
			got, err := os.ReadFile(b + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			if success && !tc.noMetadataChange {
				if bytes.Equal(got, before) || !bytes.Equal(got, edits[1].After) {
					t.Fatal("guarded sidecar was not updated in history")
				}
			} else if !bytes.Equal(got, before) {
				t.Fatal("dependent metadata changed despite failed transaction")
			}
			wantSource := "old"
			if success {
				wantSource = "candidate"
			}
			if got, err := os.ReadFile(a); err != nil || string(got) != wantSource {
				t.Fatalf("source after transaction = %q, %v; want %q", got, err, wantSource)
			}
			if success && tc.guard {
				for _, back := range []bool{true, false} {
					batch, err := pl.ApplySources(edits, back)
					if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
						t.Fatalf("history back=%v: %+v, %v", back, batch, err)
					}
					want := edits[1].After
					if back {
						want = before
					}
					if got, err := os.ReadFile(b + MetaSuffix); err != nil || !bytes.Equal(got, want) {
						t.Fatalf("dependent metadata after history back=%v: %v", back, err)
					}
				}
			}
		})
	}
}
