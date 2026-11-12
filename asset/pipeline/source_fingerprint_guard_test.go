package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/format/aetext"
	"aego/project"
)

type fingerprintGuardFixture struct {
	project *project.Project
	pipe    *Pipeline
	file    string
	meta    []byte
	edits   []SourceEdit
}

func newFingerprintGuardFixture(t *testing.T, large bool) fingerprintGuardFixture {
	t.Helper()
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "original")
	if large {
		f, err := os.OpenFile(file, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate((16 << 20) + 37); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	pl := openPipeline(t, p, &fakeImporter{version: 1})
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("fixture import: %+v %v", batch, err)
	}
	meta, err := os.ReadFile(file + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseMeta(meta, "assets/a.txt.meta")
	if err != nil {
		t.Fatal(err)
	}
	m.Settings["mode"] = aetext.Ident("edited")
	fingerprint, err := pl.FingerprintSource(context.Background(), "assets/a.txt", MaxSourceFingerprintBytes)
	if err != nil || !fingerprint.Exists {
		t.Fatal("cannot fingerprint fixture", err)
	}
	return fingerprintGuardFixture{project: p, pipe: pl, file: file, meta: meta,
		edits: []SourceEdit{{Path: "assets/a.txt", ReadOnlyGuard: true, Fingerprint: &fingerprint},
			{Path: "assets/a.txt.meta", Before: bytes.Clone(meta), After: WriteMeta(m)}}}
}

func TestFingerprintGuardPublishesLargeSourceMetadataAndHistory(t *testing.T) {
	f := newFingerprintGuardFixture(t, true)
	original := *f.edits[0].Fingerprint
	sawJournal := false
	f.pipe.sourceTransactionCheckpoint = func(stage, _ string) {
		if stage != "prepared" {
			return
		}
		sawJournal = true
		data, err := os.ReadFile(filepath.Join(f.project.Root, filepath.FromSlash(sourceJournalFile)))
		if err != nil {
			t.Fatal(err)
		}
		var record sourceJournalRecord
		if err := json.Unmarshal(bytes.TrimSpace(data), &record); err != nil || record.Version != 2 || len(record.Entries) != 2 ||
			!record.Entries[0].ReadOnly || record.Entries[0].GuardSize != original.Size || record.Entries[0].Backup != "" {
			t.Fatalf("incorrect bounded read-only journal: %+v %v", record, err)
		}
		if _, err := os.Stat(filepath.Join(f.project.Root, filepath.FromSlash(sourceJournalDir), "before-0000")); !os.IsNotExist(err) {
			t.Fatalf("large read-only source was backed up: %v", err)
		}
	}
	for _, back := range []bool{false, true, false} {
		batch, err := f.pipe.ApplySources(f.edits, back)
		if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
			t.Fatalf("fingerprint guarded transaction back=%v: %+v %v", back, batch, err)
		}
		if !sawJournal {
			t.Fatal("transaction did not persist its guard")
		}
		got, err := f.pipe.FingerprintSource(context.Background(), "assets/a.txt", MaxSourceFingerprintBytes)
		if err != nil || got != original {
			t.Fatal("transaction changed guarded source", err)
		}
		want := f.edits[1].After
		if back {
			want = f.edits[1].Before
		}
		if data, err := os.ReadFile(f.file + MetaSuffix); err != nil || !bytes.Equal(data, want) {
			t.Fatal("metadata does not match the transaction history", err)
		}
		if status, err := SourceRecoveryStatus(f.project); err != nil || status != nil {
			t.Fatalf("completed transaction left recovery: %+v %v", status, err)
		}
	}
}

func TestFingerprintGuardRejectsExternalSourceChangeBeforePublication(t *testing.T) {
	f := newFingerprintGuardFixture(t, false)
	if err := os.WriteFile(f.file, []byte("external"), 0o644); err != nil { // Same length.
		t.Fatal(err)
	}
	if batch, err := f.pipe.ApplySources(f.edits, false); err == nil {
		t.Fatalf("accepted externally changed source: %+v", batch)
	}
	if got, err := os.ReadFile(f.file); err != nil || string(got) != "external" {
		t.Fatal("rejected transaction changed external source", err)
	}
	if got, err := os.ReadFile(f.file + MetaSuffix); err != nil || !bytes.Equal(got, f.meta) {
		t.Fatal("rejected transaction changed metadata", err)
	}
	if status, err := SourceRecoveryStatus(f.project); err != nil || status != nil {
		t.Fatalf("preflight refusal created recovery: %+v %v", status, err)
	}
}

func TestFingerprintGuardRejectsInvalidCombinations(t *testing.T) {
	for _, reason := range []string{"not read-only", "byte preimage", "byte postimage", "absent source", "negative size", "oversized", "metadata path", "auxiliary", "nil importer registry"} {
		t.Run(reason, func(t *testing.T) {
			f := newFingerprintGuardFixture(t, false)
			guard := &f.edits[0]
			switch reason {
			case "not read-only":
				guard.ReadOnlyGuard = false
			case "byte preimage":
				guard.Before = []byte{}
			case "byte postimage":
				guard.After = []byte{}
			case "absent source":
				guard.Fingerprint.Exists = false
			case "negative size":
				guard.Fingerprint.Size = -1
			case "oversized":
				guard.Fingerprint.Size = MaxSourceFingerprintBytes + 1
			case "metadata path":
				guard.Path += MetaSuffix
			case "auxiliary":
				guard.Auxiliary = true
			case "nil importer registry":
				f.pipe.imps = nil
			}
			if batch, err := f.pipe.ApplySources(f.edits, false); err == nil {
				t.Fatalf("accepted invalid guard: %+v", batch)
			}
			if got, err := os.ReadFile(f.file); err != nil || string(got) != "original" {
				t.Fatal("invalid guard changed source", err)
			}
			if got, err := os.ReadFile(f.file + MetaSuffix); err != nil || !bytes.Equal(got, f.meta) {
				t.Fatal("invalid guard changed metadata", err)
			}
			if status, err := SourceRecoveryStatus(f.project); err != nil || status != nil {
				t.Fatalf("invalid guard created recovery: %+v %v", status, err)
			}
		})
	}
}

func TestFingerprintGuardCopiesCallerSnapshot(t *testing.T) {
	f := newFingerprintGuardFixture(t, false)
	f.pipe.sourceTransactionCheckpoint = func(stage, _ string) {
		if stage == "prepared" {
			// The transaction must not start consulting this mutable caller copy
			// after it has captured and validated its own guard.
			f.edits[0].Fingerprint.Size = 0
		}
	}
	batch, err := f.pipe.ApplySources(f.edits, false)
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("caller mutation changed in-flight guard: %+v %v", batch, err)
	}
	if got, err := os.ReadFile(f.file); err != nil || string(got) != "original" {
		t.Fatal("transaction changed guarded source", err)
	}
}

func TestFingerprintGuardCopiesCallerMetadataBytes(t *testing.T) {
	f := newFingerprintGuardFixture(t, false)
	want := bytes.Clone(f.edits[1].After)
	f.pipe.sourceTransactionCheckpoint = func(stage, _ string) {
		if stage == "prepared" {
			// Both slices remain owned by the caller, not by the transaction.
			f.edits[1].Before[0] ^= 1
			f.edits[1].After[0] ^= 1
		}
	}
	batch, err := f.pipe.ApplySources(f.edits, false)
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("caller changed the transaction snapshots: %+v %v", batch, err)
	}
	if got, err := os.ReadFile(f.file + MetaSuffix); err != nil || !bytes.Equal(got, want) {
		t.Fatal("publication used the mutated caller bytes", err)
	}
}

func TestFingerprintGuardRefusesImplicitMetadataNormalization(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "candidate")
	imp := &canonicalRollbackImporter{fakeImporter: fakeImporter{version: 1}}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("initial import: %+v %v", batch, err)
	}
	before, err := os.ReadFile(file + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ParseMeta(before, "assets/a.txt.meta")
	if err != nil {
		t.Fatal(err)
	}
	meta.Settings["mode"] = aetext.Ident("manual")
	guard, err := pl.FingerprintSource(context.Background(), "assets/a.txt", MaxSourceFingerprintBytes)
	if err != nil {
		t.Fatal(err)
	}
	edits := []SourceEdit{{Path: "assets/a.txt", ReadOnlyGuard: true, Fingerprint: &guard},
		{Path: "assets/a.txt.meta", Before: before, After: WriteMeta(meta)}}
	if batch, err := pl.ApplySources(edits, false); err == nil {
		t.Fatalf("allowed the importer to normalize a guarded sidecar: %+v", batch)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "candidate" {
		t.Fatal("failed normalization changed guarded source", err)
	}
	if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, before) {
		t.Fatal("failed normalization did not roll back the explicit metadata write", err)
	}
	if status, err := SourceRecoveryStatus(p); err != nil || status != nil {
		t.Fatalf("fully rolled-back transaction left a recovery journal: %+v %v", status, err)
	}
}

func TestFingerprintGuardSupportsSidecarFirst(t *testing.T) {
	f := newFingerprintGuardFixture(t, false)
	f.edits[0], f.edits[1] = f.edits[1], f.edits[0]
	for _, back := range []bool{false, true, false} {
		batch, err := f.pipe.ApplySources(f.edits, back)
		if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
			t.Fatalf("sidecar-first back=%v: %+v %v", back, batch, err)
		}
	}
}

func TestFingerprintGuardEmptySourceStaysRecoverableWhenImportRetries(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "")
	pl := openPipeline(t, p, &fakeImporter{version: 1})
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 1 {
		t.Fatalf("initial empty-source import: %+v %v", batch, err)
	}
	before, err := os.ReadFile(file + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ParseMeta(before, "assets/a.txt.meta")
	if err != nil {
		t.Fatal(err)
	}
	meta.Settings["mode"] = aetext.Ident("edited")
	guard, err := pl.FingerprintSource(context.Background(), "assets/a.txt", 0)
	if err != nil || !guard.Exists || guard.Size != 0 {
		t.Fatalf("empty source fingerprint: %+v %v", guard, err)
	}
	edits := []SourceEdit{{Path: "assets/a.txt", ReadOnlyGuard: true, Fingerprint: &guard},
		{Path: "assets/a.txt.meta", Before: before, After: WriteMeta(meta)}}
	if batch, err := pl.ApplySources(edits, false); err == nil {
		t.Fatalf("accepted an empty source that the importer reports as unstable: %+v", batch)
	}
	if status, err := SourceRecoveryStatus(p); err != nil || status == nil || status.State != "prepared" {
		t.Fatalf("retrying import must leave an explicit recovery: %+v %v", status, err)
	}
	pl.Close()
	if err := RestoreSourceRecovery(p); err != nil {
		t.Fatal("could not explicitly recover an existing empty source", err)
	}
	if data, err := os.ReadFile(file); err != nil || len(data) != 0 {
		t.Fatal("transaction changed existing empty source", err)
	}
	if data, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(data, before) {
		t.Fatal("empty source recovery did not preserve metadata", err)
	}
}

func TestFingerprintGuardKeepsExternalChangesAndRequiresExplicitRecovery(t *testing.T) {
	for _, stage := range []string{"prepared", "published"} {
		t.Run(stage, func(t *testing.T) {
			f := newFingerprintGuardFixture(t, false)
			var metaAtConflict []byte
			f.pipe.sourceTransactionCheckpoint = func(actual, rel string) {
				if actual == stage && (stage != "published" || rel == "assets/a.txt.meta") {
					var err error
					metaAtConflict, err = os.ReadFile(f.file + MetaSuffix)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(f.file, []byte("external"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			if batch, err := f.pipe.ApplySources(f.edits, false); err == nil {
				t.Fatalf("accepted changed source at %s: %+v", stage, batch)
			}
			if got, err := os.ReadFile(f.file); err != nil || string(got) != "external" {
				t.Fatal("transaction overwrote external source", err)
			}
			if got, err := os.ReadFile(f.file + MetaSuffix); err != nil || !bytes.Equal(got, metaAtConflict) {
				t.Fatal("rollback changed metadata after the source conflict", err)
			}
			status, err := SourceRecoveryStatus(f.project)
			if err != nil || status == nil || status.State != "prepared" {
				t.Fatalf("unresolved transaction did not retain recovery: %+v %v", status, err)
			}
			f.pipe.Close()
			if err := RestoreSourceRecovery(f.project); err == nil {
				t.Fatal("recovery ignored the changed read-only source")
			}
			if err := os.WriteFile(f.file, []byte("original"), 0o644); err != nil { // Explicit test-owner resolution.
				t.Fatal(err)
			}
			if err := RestoreSourceRecovery(f.project); err != nil {
				t.Fatal("explicit recovery after resolving source conflict failed", err)
			}
			if got, err := os.ReadFile(f.file); err != nil || string(got) != "original" {
				t.Fatal("recovery changed the read-only source", err)
			}
			if got, err := os.ReadFile(f.file + MetaSuffix); err != nil || !bytes.Equal(got, f.meta) {
				t.Fatal("recovery did not restore metadata", err)
			}
		})
	}
}

func TestFingerprintGuardRechecksMetadataAfterGuardBeforePublication(t *testing.T) {
	f := newFingerprintGuardFixture(t, false)
	changed := false
	outside := []byte("external metadata: do not overwrite")
	f.pipe.sourceTransactionCheckpoint = func(stage, rel string) {
		if stage == "fingerprint-checked" && rel == "assets/a.txt.meta" {
			changed = true
			if err := os.WriteFile(f.file+MetaSuffix, outside, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if batch, err := f.pipe.ApplySources(f.edits, false); !changed || err == nil {
		t.Fatalf("accepted a metadata change after checking the guard: changed=%v batch=%+v err=%v", changed, batch, err)
	}
	if got, err := os.ReadFile(f.file + MetaSuffix); err != nil || !bytes.Equal(got, outside) {
		t.Fatal("publication overwrote external metadata", err)
	}
	if status, err := SourceRecoveryStatus(f.project); err != nil || status == nil || status.State != "prepared" {
		t.Fatalf("metadata conflict did not retain recovery: %+v %v", status, err)
	}
}

func TestFingerprintGuardRechecksMetadataAfterGuardBeforeRollback(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "candidate")
	imp := &canonicalRollbackImporter{fakeImporter: fakeImporter{version: 1}}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("initial import: %+v %v", batch, err)
	}
	before, err := os.ReadFile(file + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ParseMeta(before, "assets/a.txt.meta")
	if err != nil {
		t.Fatal(err)
	}
	meta.Settings["mode"] = aetext.Ident("manual")
	guard, err := pl.FingerprintSource(context.Background(), "assets/a.txt", MaxSourceFingerprintBytes)
	if err != nil {
		t.Fatal(err)
	}
	edits := []SourceEdit{{Path: "assets/a.txt", ReadOnlyGuard: true, Fingerprint: &guard},
		{Path: "assets/a.txt.meta", Before: before, After: WriteMeta(meta)}}
	changed := false
	out := []byte("external metadata during rollback")
	// This import requires implicit normalization, which the fingerprint
	// transaction rejects, forcing rollback while the source still matches.
	pl.sourceTransactionCheckpoint = func(stage, rel string) {
		if stage == "rollback-fingerprint-checked" && rel == "assets/a.txt.meta" {
			changed = true
			if err := os.WriteFile(file+MetaSuffix, out, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := pl.ApplySources(edits, false); !changed || err == nil {
		t.Fatalf("rollback did not reject changed metadata: changed=%v err=%v", changed, err)
	}
	if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, out) {
		t.Fatal("rollback overwrote external metadata", err)
	}
	if status, err := SourceRecoveryStatus(p); err != nil || status == nil || status.State != "prepared" {
		t.Fatalf("rollback conflict did not retain recovery: %+v %v", status, err)
	}
}

func TestFingerprintGuardRejectsUnboundedOrUnrelatedEdits(t *testing.T) {
	for _, reason := range []string{"second guard", "extra edit", "unrelated metadata", "missing metadata"} {
		t.Run(reason, func(t *testing.T) {
			f := newFingerprintGuardFixture(t, false)
			switch reason {
			case "second guard":
				f.edits = append(f.edits, SourceEdit{Path: "assets/b.txt", ReadOnlyGuard: true, Fingerprint: f.edits[0].Fingerprint})
			case "extra edit":
				f.edits = append(f.edits, SourceEdit{Path: "assets/extra.data", Auxiliary: true, After: []byte("new")})
			case "unrelated metadata":
				f.edits[1].Path = "assets/unrelated.txt.meta"
			case "missing metadata":
				f.edits = f.edits[:1]
			}
			if batch, err := f.pipe.ApplySources(f.edits, false); err == nil {
				t.Fatalf("accepted unsupported fingerprint transaction: %+v", batch)
			}
			if got, err := os.ReadFile(f.file + MetaSuffix); err != nil || !bytes.Equal(got, f.meta) {
				t.Fatal("invalid transaction changed metadata", err)
			}
			if status, err := SourceRecoveryStatus(f.project); err != nil || status != nil {
				t.Fatalf("invalid transaction left a journal: %+v %v", status, err)
			}
		})
	}
}

func TestFingerprintGuardSIGKILLRequiresExplicitRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, boundary string
		committed      bool
	}{
		{"prepared", "prepared:", false},
		{"published", "published:assets/a.txt.meta", false},
		{"committed", "committed:", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newProject(t)
			source := write(t, p, "assets/a.txt", "original")
			pl := recoveryTestPipeline(t, p)
			pl.Close()
			before, err := os.ReadFile(source + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			killSourceJournalWorkerMode(t, p, tc.boundary, false, "fingerprint")
			status, err := SourceRecoveryStatus(p)
			if err != nil || status == nil {
				t.Fatalf("missing interrupted fingerprint recovery: %+v %v", status, err)
			}
			if !tc.committed {
				if err := os.WriteFile(source, []byte("external"), 0o644); err != nil {
					t.Fatal(err)
				}
				diskMeta, err := os.ReadFile(source + MetaSuffix)
				if err != nil {
					t.Fatal(err)
				}
				if err := RestoreSourceRecovery(p); err == nil {
					t.Fatal("interrupted recovery ignored the external source")
				}
				if got, err := os.ReadFile(source + MetaSuffix); err != nil || !bytes.Equal(got, diskMeta) {
					t.Fatal("rejected recovery changed metadata", err)
				}
				if err := os.WriteFile(source, []byte("original"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := RestoreSourceRecovery(p); err != nil {
				t.Fatal("explicit source recovery failed", err)
			}
			if got, err := os.ReadFile(source); err != nil || string(got) != "original" {
				t.Fatal("recovery changed guarded source", err)
			}
			got, err := os.ReadFile(source + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.committed && !bytes.Equal(got, before) {
				t.Fatal("recovery did not restore previous metadata")
			}
			if tc.committed {
				meta, err := ParseMeta(got, "assets/a.txt.meta")
				if err != nil || meta.Settings.String("mode", "") != "manual" {
					t.Fatal("committed recovery did not preserve metadata", err)
				}
			}
			if status, err := SourceRecoveryStatus(p); err != nil || status != nil {
				t.Fatalf("recovery remains: %+v %v", status, err)
			}
		})
	}
}

func TestFingerprintGuardRechecksBeforeRecoveryWrites(t *testing.T) {
	p := newProject(t)
	source := write(t, p, "assets/a.txt", "original")
	pl := recoveryTestPipeline(t, p)
	pl.Close()
	originalMeta, err := os.ReadFile(source + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	killSourceJournalWorkerMode(t, p, "published:assets/a.txt.meta", false, "fingerprint")
	publishedMeta, err := os.ReadFile(source + MetaSuffix)
	if err != nil || bytes.Equal(publishedMeta, originalMeta) {
		t.Fatal("crash did not leave the published metadata", err)
	}
	err = restoreSourceRecovery(p, func(stage, _ string) {
		if stage == "recovery-validated" {
			if err := os.WriteFile(source, []byte("external"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	})
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatal("recovery did not recheck source before writing metadata", err)
	}
	if got, err := os.ReadFile(source + MetaSuffix); err != nil || !bytes.Equal(got, publishedMeta) {
		t.Fatal("rejected recovery overwrote published metadata", err)
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "external" {
		t.Fatal("recovery overwrote external source", err)
	}
	if err := os.WriteFile(source, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSourceRecovery(p); err != nil {
		t.Fatal("resolved recovery failed", err)
	}
	if got, err := os.ReadFile(source + MetaSuffix); err != nil || !bytes.Equal(got, originalMeta) {
		t.Fatal("resolved recovery did not restore metadata", err)
	}
}

func TestFingerprintGuardRechecksMetadataAfterGuardBeforeRecovery(t *testing.T) {
	p := newProject(t)
	source := write(t, p, "assets/a.txt", "original")
	pl := recoveryTestPipeline(t, p)
	pl.Close()
	killSourceJournalWorkerMode(t, p, "published:assets/a.txt.meta", false, "fingerprint")
	out := []byte("external metadata: recovery must preserve")
	changed := false
	err := restoreSourceRecovery(p, func(stage, rel string) {
		if stage == "recovery-fingerprint-checked" && rel == "assets/a.txt.meta" {
			changed = true
			if err := os.WriteFile(source+MetaSuffix, out, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	})
	if !changed || err == nil {
		t.Fatalf("recovery accepted changed metadata: changed=%v err=%v", changed, err)
	}
	if got, err := os.ReadFile(source + MetaSuffix); err != nil || !bytes.Equal(got, out) {
		t.Fatal("recovery overwrote external metadata", err)
	}
	if status, err := SourceRecoveryStatus(p); err != nil || status == nil || status.State != "prepared" {
		t.Fatalf("recovery conflict did not retain journal: %+v %v", status, err)
	}
}

func TestFingerprintGuardRejectsMalformedRecoveryHeaders(t *testing.T) {
	for _, reason := range []string{"legacy version", "missing guard flag", "missing source", "different versions", "backup", "negative size", "oversized", "metadata path", "unrelated metadata", "extra entry", "second guard", "unknown field"} {
		t.Run(reason, func(t *testing.T) {
			f := newFingerprintGuardFixture(t, false)
			root, err := os.OpenRoot(f.project.Root)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if _, err := startSourceJournal(root, f.edits, false); err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(f.project.Root, filepath.FromSlash(sourceJournalFile))
			raw, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			var record sourceJournalRecord
			if err := json.Unmarshal(raw, &record); err != nil || record.Version != 2 || !record.Entries[0].ReadOnly {
				t.Fatal("fixture did not create a version 2 guard", err)
			}
			e := &record.Entries[0]
			switch reason {
			case "legacy version":
				record.Version = 1
			case "missing guard flag":
				e.ReadOnly, e.GuardSize = false, 0
				e.Backup = "before-0000"
			case "missing source":
				e.Before, e.After = sourceVersion{}, sourceVersion{}
			case "different versions":
				e.After = sourceVersion{}
			case "backup":
				e.Backup = "before-0000"
			case "negative size":
				e.GuardSize = -1
			case "oversized":
				e.GuardSize = MaxSourceFingerprintBytes + 1
			case "metadata path":
				e.Path = "assets/b.txt.meta"
			case "unrelated metadata":
				record.Entries[1].Path = "assets/unrelated.txt.meta"
			case "extra entry":
				record.Entries = append(record.Entries, sourceJournalEntry{Path: "assets/unrelated", Before: sourceVersion{}, After: sourceVersion{}})
			case "second guard":
				record.Entries[1] = *e
				record.Entries[1].Path = "assets/b.txt"
			}
			raw, err = json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if reason == "unknown field" {
				raw = bytes.Replace(raw, []byte(`"readOnly":true`), []byte(`"readOnly":true,"unexpected":true`), 1)
			}
			raw = append(raw, '\n')
			if err := os.WriteFile(name, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadSourceJournal(root, f.pipe); err == nil {
				t.Fatal("accepted malformed fingerprint recovery")
			}
			if got, err := os.ReadFile(name); err != nil || !bytes.Equal(got, raw) {
				t.Fatal("rejected recovery changed the journal", err)
			}
			if got, err := os.ReadFile(f.file); err != nil || string(got) != "original" {
				t.Fatal("rejected recovery changed source", err)
			}
			if got, err := os.ReadFile(f.file + MetaSuffix); err != nil || !bytes.Equal(got, f.meta) {
				t.Fatal("rejected recovery changed metadata", err)
			}
		})
	}
}
