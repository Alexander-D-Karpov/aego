package pipeline_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/project"
)

type metaAuditWatcher struct{ notify func([]pipeline.Change) }

func (w *metaAuditWatcher) Start(_ string, _ []string, _ time.Duration, notify func([]pipeline.Change)) (func(), error) {
	w.notify = notify
	return func() {}, nil
}

func metaAuditBatch(t *testing.T, batch *pipeline.Batch, err error) *pipeline.Batch {
	t.Helper()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("asset batch failed or needs retry: %+v, %v", batch, err)
	}
	return batch
}

func assertNoMetaIssues(t *testing.T, pl *pipeline.Pipeline) {
	t.Helper()
	issues, _ := pl.MetaIssues()
	if len(issues) != 0 {
		t.Fatalf("unexpected metadata diagnostics: %+v", issues)
	}
}

func assertStaleIDs(t *testing.T, pl *pipeline.Pipeline, rel string, want ...ids.AssetID) {
	t.Helper()
	remaining := make(map[ids.AssetID]bool, len(want))
	for _, id := range want {
		remaining[id] = true
	}
	issues, _ := pl.MetaIssues()
	for _, issue := range issues {
		if issue.Kind != pipeline.MetaStale || issue.Path != rel || !remaining[issue.ID] {
			t.Fatalf("unexpected or duplicate metadata diagnostic: %+v", issues)
		}
		delete(remaining, issue.ID)
	}
	if len(remaining) != 0 {
		t.Fatalf("stale indexed IDs were not diagnosed: missing %v, issues %+v", remaining, issues)
	}
}

func metaAuditArtFixture(t *testing.T) (*project.Project, *pipeline.ImporterRegistry, *pipeline.Pipeline, *art.Session, []byte, ids.AssetID) {
	t.Helper()
	p, err := project.Create(t.TempDir(), "Meta audit", false)
	if err != nil {
		t.Fatal(err)
	}
	registry := pipeline.NewImporterRegistry()
	if err := registry.Register(artsource.New()); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	doc, err := art.New(8, 8, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(doc.Close)
	session, err := art.NewSession(doc, art.NewJournal(art.DefaultLimits()))
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := session.Execute(context.Background(), art.Operation{Kind: "duplicate-frame"}); err != nil {
			t.Fatal(err)
		}
	}
	source, err := doc.Encode()
	if err != nil {
		t.Fatal(err)
	}
	const rel = "assets/sprites/slash 2.aeart"
	meta, err := pl.NewSourceMeta(rel)
	if err != nil {
		t.Fatal(err)
	}
	edits := []pipeline.SourceEdit{
		{Path: rel, After: source},
		{Path: rel + pipeline.MetaSuffix, After: pipeline.WriteMeta(meta)},
	}
	batch, err := pl.ApplySources(edits, false)
	metaAuditBatch(t, batch, err)
	if len(batch.Imported) != 6 { // source, four frames and animation
		t.Fatalf("expected real art source and five sub-assets, got %+v", batch.Imported)
	}
	return p, registry, pl, session, source, meta.ID
}

func TestArtMetaAuditAfterCreateSaveWatchAndReopen(t *testing.T) {
	const rel = "assets/sprites/slash 2.aeart"
	p, registry, pl, session, first, rootID := metaAuditArtFixture(t)
	assertNoMetaIssues(t, pl)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	assertNoMetaIssues(t, pl)

	// A watcher event scans before reimporting. The file really changed on disk;
	// strict-current and source preimage guards must still notice that change.
	if _, err := session.Execute(context.Background(), art.Operation{Kind: "duration", DurationMS: 230}); err != nil {
		t.Fatal(err)
	}
	second, err := session.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(p.Root, filepath.FromSlash(rel))
	if err := os.WriteFile(full, second, 0644); err != nil {
		t.Fatal(err)
	}
	if err := pl.VerifyCurrent(rootID); err == nil {
		t.Fatal("external source modification was accepted as current")
	}
	if _, err := pl.ApplySources([]pipeline.SourceEdit{{Path: rel, Before: first, After: []byte("would overwrite")}}, false); err == nil {
		t.Fatal("stale source preimage was accepted")
	}
	if got, err := os.ReadFile(full); err != nil || !bytes.Equal(got, second) {
		t.Fatalf("external source modification was overwritten: %v", err)
	}
	w := &metaAuditWatcher{}
	pl.SetWatcher(w)
	batches := make(chan *pipeline.Batch, 1)
	stop, err := pl.Watch(10*time.Millisecond, func(batch *pipeline.Batch) { batches <- batch })
	if err != nil {
		t.Fatal(err)
	}
	w.notify([]pipeline.Change{{Path: rel, Kind: pipeline.ChangeWrite}})
	select {
	case batch := <-batches:
		metaAuditBatch(t, batch, nil)
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not import the changed art")
	}
	stop()
	assertNoMetaIssues(t, pl)
	if err := pl.VerifyCurrent(rootID); err != nil {
		t.Fatalf("watcher did not publish a current source: %v", err)
	}

	if _, err := session.Execute(context.Background(), art.Operation{Kind: "duration", DurationMS: 320}); err != nil {
		t.Fatal(err)
	}
	third, err := session.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ApplySources([]pipeline.SourceEdit{{Path: rel, Before: second, After: third}}, false)
	metaAuditBatch(t, batch, err)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	assertNoMetaIssues(t, pl)

	metaData, err := os.ReadFile(full + pipeline.MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := pipeline.ParseMeta(metaData, rel+pipeline.MetaSuffix)
	if err != nil || len(meta.Subs) != 0 {
		t.Fatalf("fixture must have generated, not sidecar-listed, sub-assets: %+v, %v", meta, err)
	}
	pl.Close()
	reopened, err := pipeline.Open(p, registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	if _, _, err := reopened.Scan(); err != nil {
		t.Fatal(err)
	}
	// The initial scan precedes ImportAll and does not yet know generated IDs.
	assertNoMetaIssues(t, reopened)
	batch, err = reopened.ImportAll()
	metaAuditBatch(t, batch, err)
	assertNoMetaIssues(t, reopened)
	if got, ok := reopened.Lookup(rel); !ok || got != rootID {
		t.Fatalf("source identity changed on reopen: %v %v", got, ok)
	}
	for _, frame := range session.Document.Info().Frames {
		name := artsource.FrameName(frame.ID, art.Albedo)
		id, ok := reopened.Lookup(rel + pipeline.SubSep + name)
		if !ok || id != artsource.StableID(rootID, name) {
			t.Fatalf("frame identity changed on reopen: %s, %s, %v", name, id, ok)
		}
	}

	// Suppressing an inaccurate missing-file message must not accept an
	// out-of-band sidecar identity change as a valid imported source.
	meta.ID = ids.NewAssetID()
	changedMeta := pipeline.WriteMeta(meta)
	if err := os.WriteFile(full+pipeline.MetaSuffix, changedMeta, 0644); err != nil {
		t.Fatal(err)
	}
	if err := reopened.VerifyCurrent(rootID); err == nil {
		t.Fatal("external metadata identity change was accepted as current")
	}
	if _, err := reopened.ReloadMetadata(rootID); err == nil {
		t.Fatal("external metadata identity change was accepted by reload")
	}
	if _, err := reopened.ApplySources([]pipeline.SourceEdit{
		{Path: rel, Before: third, After: second},
		{Path: rel + pipeline.MetaSuffix, Before: metaData, After: metaData, ReadOnlyGuard: true},
	}, false); err == nil {
		t.Fatal("stale sidecar preimage was accepted")
	}
	if got, err := os.ReadFile(full); err != nil || !bytes.Equal(got, third) {
		t.Fatalf("stale sidecar guard did not protect the source: %v", err)
	}
	if got, err := os.ReadFile(full + pipeline.MetaSuffix); err != nil || !bytes.Equal(got, changedMeta) {
		t.Fatalf("external metadata was overwritten: %v", err)
	}
	metaWatcher := &metaAuditWatcher{}
	reopened.SetWatcher(metaWatcher)
	metaBatches := make(chan *pipeline.Batch, 1)
	stopMeta, err := reopened.Watch(10*time.Millisecond, func(batch *pipeline.Batch) { metaBatches <- batch })
	if err != nil {
		t.Fatal(err)
	}
	metaWatcher.notify([]pipeline.Change{{Path: rel + pipeline.MetaSuffix, Kind: pipeline.ChangeWrite}})
	select {
	case batch := <-metaBatches:
		if len(batch.Failed) != 1 || batch.Failed[rootID] == nil || len(batch.Retry) != 0 {
			t.Fatalf("watcher masked the external identity change: %+v", batch)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not report the changed identity")
	}
	stopMeta()
	if got, err := os.ReadFile(full + pipeline.MetaSuffix); err != nil || !bytes.Equal(got, changedMeta) {
		t.Fatalf("watcher overwrote external metadata: %v", err)
	}
	oldIDs := []ids.AssetID{rootID, artsource.StableID(rootID, artsource.ClipName(""))}
	for _, frame := range session.Document.Info().Frames {
		oldIDs = append(oldIDs, artsource.StableID(rootID, artsource.FrameName(frame.ID, art.Albedo)))
	}
	reopened.Close()
	changed, err := pipeline.Open(p, registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(changed.Close)
	if _, _, err := changed.Scan(); err != nil {
		t.Fatal(err)
	}
	// A path match alone cannot establish ownership of the old index IDs.
	assertStaleIDs(t, changed, rel, oldIDs...)
	if got, ok := changed.Lookup(rel); !ok || got != meta.ID {
		t.Fatalf("new source identity was not discovered: %v, %v", got, ok)
	}
	batch, err = changed.ImportAll()
	metaAuditBatch(t, batch, err)
	if err := changed.VerifyCurrent(rootID); err == nil {
		t.Fatal("old identity was accepted after the metadata changed")
	}
	if err := changed.VerifyCurrent(meta.ID); err != nil {
		t.Fatalf("new identity was not imported: %v", err)
	}
	if _, _, err := changed.Scan(); err != nil {
		t.Fatal(err)
	}
	assertStaleIDs(t, changed, rel, oldIDs...)
}

func TestArtMetaAuditSourceEditRejectsSymlinks(t *testing.T) {
	const rel = "assets/sprites/slash 2.aeart"
	for _, linkedSidecar := range []bool{false, true} {
		name := "source"
		if linkedSidecar {
			name = "sidecar"
		}
		t.Run(name, func(t *testing.T) {
			p, _, pl, _, source, _ := metaAuditArtFixture(t)
			full := filepath.Join(p.Root, filepath.FromSlash(rel))
			linked := full
			if linkedSidecar {
				linked += pipeline.MetaSuffix
			}
			original, err := os.ReadFile(linked)
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(target, original, 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(linked); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, linked); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			if _, err := pl.ApplySources([]pipeline.SourceEdit{{Path: rel, Before: source, After: []byte("would overwrite")}}, false); err == nil || !strings.Contains(err.Error(), "symlink") {
				t.Fatalf("symlink source edit was not rejected: %v", err)
			}
			if info, err := os.Lstat(linked); err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("external symlink was replaced: %v", err)
			}
			if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("external symlink target was changed: %v", err)
			}
			if got, err := os.ReadFile(full); err != nil || !bytes.Equal(got, source) {
				t.Fatalf("art source was changed: %v", err)
			}
		})
	}
}

func TestArtMetaAuditKeepsUnownedIndexWarning(t *testing.T) {
	const rel = "assets/sprites/slash 2.aeart"
	_, _, pl, _, _, _ := metaAuditArtFixture(t)
	unowned, unrelated := ids.NewAssetID(), ids.NewAssetID()
	pl.Index().Put(asset.Entry{ID: unowned, Source: rel, Deps: []ids.AssetID{unrelated}})
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	assertStaleIDs(t, pl, rel, unowned)
}

func TestArtMetaAuditKeepsInvalidSidecarWarnings(t *testing.T) {
	const rel = "assets/sprites/slash 2.aeart"
	p, registry, pl, session, _, rootID := metaAuditArtFixture(t)
	oldIDs := []ids.AssetID{rootID, artsource.StableID(rootID, artsource.ClipName(""))}
	for _, frame := range session.Document.Info().Frames {
		oldIDs = append(oldIDs, artsource.StableID(rootID, artsource.FrameName(frame.ID, art.Albedo)))
	}
	pl.Close()
	metaPath := filepath.Join(p.Root, filepath.FromSlash(rel+pipeline.MetaSuffix))
	invalid := []byte("meta { invalid = [")
	if err := os.WriteFile(metaPath, invalid, 0644); err != nil {
		t.Fatal(err)
	}
	reopened, err := pipeline.Open(p, registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	if _, _, err := reopened.Scan(); err != nil {
		t.Fatal(err)
	}
	assertStaleIDs(t, reopened, rel, oldIDs...)
	if err := reopened.VerifyCurrent(rootID); err == nil {
		t.Fatal("invalid external metadata was accepted as current")
	}
	if got, err := os.ReadFile(metaPath); err != nil || !bytes.Equal(got, invalid) {
		t.Fatalf("scan changed invalid external metadata: %q, %v", got, err)
	}
}

func TestArtMetaAuditReadOnlyStillReportsMissingSidecar(t *testing.T) {
	const rel = "assets/sprites/slash 2.aeart"
	p, registry, pl, _, _, rootID := metaAuditArtFixture(t)
	pl.Close()
	metaPath := filepath.Join(p.Root, filepath.FromSlash(rel+pipeline.MetaSuffix))
	if err := os.Remove(metaPath); err != nil {
		t.Fatal(err)
	}
	reopened, err := pipeline.Open(p, registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	reopened.SetMetaMode(pipeline.MetaReadOnly)
	if _, _, err := reopened.Scan(); err != nil {
		t.Fatal(err)
	}
	issues, _ := reopened.MetaIssues()
	missing, stale := 0, 0
	oldRoot := false
	for _, issue := range issues {
		switch {
		case issue.Kind == pipeline.MetaMissing && issue.Path == rel && issue.Code() == "META001":
			missing++
		case issue.Kind == pipeline.MetaStale && issue.Path == rel && issue.Code() == "META004":
			stale++
			oldRoot = oldRoot || issue.ID == rootID
		default:
			t.Fatalf("unexpected diagnostic: %+v", issues)
		}
	}
	if missing != 1 || stale != 6 || !oldRoot {
		t.Fatalf("missing metadata or old index IDs were not diagnosed: %+v", issues)
	}
	if _, err := os.Stat(metaPath); !os.IsNotExist(err) {
		t.Fatalf("read-only scan recreated metadata: %v", err)
	}
}

func TestArtMetaAuditReportsRemovedIndexedFrameAfterImport(t *testing.T) {
	const rel = "assets/sprites/slash 2.aeart"
	p, registry, pl, session, _, rootID := metaAuditArtFixture(t)
	frame := session.Document.Info().Frames[0].ID
	removedID := artsource.StableID(rootID, artsource.FrameName(frame, art.Albedo))
	pl.Close()
	if _, err := session.Execute(context.Background(), art.Operation{Kind: "delete-frame", Frame: frame}); err != nil {
		t.Fatal(err)
	}
	source, err := session.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, filepath.FromSlash(rel)), source, 0644); err != nil {
		t.Fatal(err)
	}
	reopened, err := pipeline.Open(p, registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	if _, _, err := reopened.Scan(); err != nil {
		t.Fatal(err)
	}
	// Before import, generated frames cannot yet be distinguished from stale IDs.
	assertNoMetaIssues(t, reopened)
	batch, err := reopened.ImportAll()
	metaAuditBatch(t, batch, err)
	if _, _, err := reopened.Scan(); err != nil {
		t.Fatal(err)
	}
	assertStaleIDs(t, reopened, rel, removedID)
	if err := reopened.VerifyCurrent(removedID); err == nil {
		t.Fatal("removed frame was accepted as current")
	}
}

func TestArtMetaAuditStillReportsExternallyRemovedSource(t *testing.T) {
	const rel = "assets/sprites/slash 2.aeart"
	p, registry, pl, _, source, rootID := metaAuditArtFixture(t)
	pl.Close()
	if err := os.Remove(filepath.Join(p.Root, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
	reopened, err := pipeline.Open(p, registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	if _, _, err := reopened.Scan(); err != nil {
		t.Fatal(err)
	}
	issues, missingRev := reopened.MetaIssues()
	missingMain, missingSubs, orphan := false, 0, false
	for _, issue := range issues {
		switch {
		case issue.Kind == pipeline.MetaStale && issue.Path == rel:
			if issue.Code() != "META004" || !strings.Contains(issue.Message(), "could not be matched") {
				t.Fatalf("wrong missing source diagnostic: %+v", issue)
			}
			if issue.ID == rootID {
				missingMain = true
			} else {
				missingSubs++
			}
		case issue.Kind == pipeline.MetaOrphan && issue.Path == rel+pipeline.MetaSuffix:
			orphan = true
		}
	}
	if !missingMain || missingSubs != 5 || !orphan {
		t.Fatalf("missing source and its index entries were not diagnosed: %+v", issues)
	}
	if err := reopened.VerifyCurrent(rootID); err == nil {
		t.Fatal("externally removed source was accepted as current")
	}
	if _, ok := reopened.Entry(rootID); !ok {
		t.Fatal("diagnosis must not remove the index entry")
	}
	if err := os.WriteFile(filepath.Join(p.Root, filepath.FromSlash(rel)), source, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reopened.Scan(); err != nil {
		t.Fatal(err)
	}
	issues, restoredRev := reopened.MetaIssues()
	if len(issues) != 0 || restoredRev <= missingRev {
		t.Fatalf("restored source did not clear stale diagnostics and bump their revision: %v, %d -> %d", issues, missingRev, restoredRev)
	}
	batch, err := reopened.ImportAll()
	metaAuditBatch(t, batch, err)
	if err := reopened.VerifyCurrent(rootID); err != nil {
		t.Fatalf("restored source did not reimport: %v", err)
	}
}

func TestArtMetaAuditExternalRemovalDuringSession(t *testing.T) {
	const rel = "assets/sprites/slash 2.aeart"
	p, _, pl, _, _, rootID := metaAuditArtFixture(t)
	if err := os.Remove(filepath.Join(p.Root, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
	if err := pl.VerifyCurrent(rootID); err == nil {
		t.Fatal("deleted source was accepted before the next scan")
	}
	_, removed, err := pl.Scan()
	if err != nil || len(removed) != 6 {
		t.Fatalf("source and sub-assets were not removed from the live records: %v, %v", removed, err)
	}
	for _, id := range removed {
		entry, ok := pl.Entry(id)
		if !ok || !entry.Missing {
			t.Fatalf("missing index entry not marked for %s: %+v, %v", id, entry, ok)
		}
	}
	issues, _ := pl.MetaIssues()
	if len(issues) != 1 || issues[0].Kind != pipeline.MetaOrphan || issues[0].Path != rel+pipeline.MetaSuffix {
		t.Fatalf("remaining sidecar did not report the missing source: %+v", issues)
	}
}

func TestArtMetaAuditRemovalOfSourceAndSidecarReportsRemovedIDs(t *testing.T) {
	const rel = "assets/sprites/slash 2.aeart"
	p, _, pl, _, _, rootID := metaAuditArtFixture(t)
	full := filepath.Join(p.Root, filepath.FromSlash(rel))
	for _, path := range []string{full, full + pipeline.MetaSuffix} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	if err := pl.VerifyCurrent(rootID); err == nil {
		t.Fatal("deleted source was accepted before the next scan")
	}
	_, removed, err := pl.Scan()
	if err != nil || len(removed) != 6 {
		t.Fatalf("source and sub-assets were not reported as removed: %v, %v", removed, err)
	}
	for _, id := range removed {
		entry, ok := pl.Entry(id)
		if !ok || !entry.Missing {
			t.Fatalf("removed index entry not marked for %s: %+v, %v", id, entry, ok)
		}
	}
	if issues, _ := pl.MetaIssues(); len(issues) == 0 {
		t.Log("with both files gone, the existing live-scan behavior reports removed IDs rather than a META issue")
	}
}

func TestArtMetaAuditStillReportsDuplicateSidecar(t *testing.T) {
	const original, copied = "assets/sprites/slash 2.aeart", "assets/sprites/copied.aeart"
	p, _, pl, _, source, _ := metaAuditArtFixture(t)
	meta, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(original+pipeline.MetaSuffix)))
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(p.Root, filepath.FromSlash(copied))
	if err := os.WriteFile(copyPath, source, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copyPath+pipeline.MetaSuffix, meta, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	issues, _ := pl.MetaIssues()
	if len(issues) != 1 || issues[0].Kind != pipeline.MetaDuplicate || issues[0].Path != copied || issues[0].Other != original || issues[0].Code() != "META003" {
		t.Fatalf("duplicate metadata was not diagnosed: %+v", issues)
	}
	if got, err := os.ReadFile(copyPath + pipeline.MetaSuffix); err != nil || !bytes.Equal(got, meta) {
		t.Fatalf("scan changed the duplicate sidecar: %v", err)
	}
}
