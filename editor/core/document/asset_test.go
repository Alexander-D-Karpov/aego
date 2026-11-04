package document

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aego/asset"
	"aego/core/ids"
)

func imageDocument(t *testing.T) (*Manager, *AssetDocument, []byte) {
	t.Helper()
	m := newManager(t)
	if err := m.Handlers().Register(Handler{ID: "test.image", Kinds: []asset.ArtifactKind{asset.ArtTexture, asset.ArtSpriteRegion}, Capabilities: View | EditMetadata | Preview | RevealSource}); err != nil {
		t.Fatal(err)
	}
	pixels := []byte{0x89, 'P', 'N', 'G', 0, 0xff}
	if err := WriteAtomic(m.Abs("art/hero.png"), pixels); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(m.Abs("art/hero.png.meta"), []byte("meta { semantic=color unknown=17 }\n")); err != nil {
		t.Fatal(err)
	}
	d, err := m.OpenAsset(AssetRequest{Asset: ids.AssetID{Lo: 1}, Selected: ids.AssetID{Lo: 2}, Kind: asset.ArtSpriteRegion, Path: "art/hero.png", Subasset: "idle"})
	if err != nil {
		t.Fatal(err)
	}
	return m, d, pixels
}

func TestImageDocumentIdentityHistoryAndCapabilities(t *testing.T) {
	m, d, pixels := imageDocument(t)
	if d.Source != nil || d.Metadata == nil || d.Capabilities.Has(EditSource) || !d.Capabilities.Has(View|EditMetadata|Preview|RevealSource) {
		t.Fatal("PNG authoring capabilities are not truthful")
	}
	if len(m.Scripts()) != 1 || m.Script("art/hero.png.meta") != d.Metadata {
		t.Fatal("metadata has a separate private document")
	}
	sc := m.New("scenes/main.aescene", sampleScene(m, 1))
	if err := m.Save(sc.ID); err != nil {
		t.Fatal(err)
	}
	before := bytes.Clone(d.Metadata.Code.Bytes())
	for i, v := range []string{"meta { semantic=color unknown=17 strength=1 }\n", "meta { semantic=color unknown=17 strength=2 }\n", "meta { semantic=color unknown=17 strength=3 }\n"} {
		d.Metadata.Code.ReplaceSnapshot([]byte(v), i > 0)
	}
	other, err := m.OpenAsset(AssetRequest{Asset: d.Asset, Selected: ids.AssetID{Lo: 3}, Kind: asset.ArtSpriteRegion, Path: d.Path, Subasset: "run"})
	if err != nil || other != d || other.Selected.Lo != 3 || other.Subasset != "run" || len(m.AssetDocuments()) != 1 {
		t.Fatal("another region duplicated the sheet document", err)
	}
	if !d.Dirty() || !m.Dirty() || m.DirtyCount() != 1 || sc.Dirty() {
		t.Fatal("metadata history dirtied the scene or is absent from lifecycle")
	}
	if !d.Metadata.Code.Undo() || !bytes.Equal(d.Metadata.Code.Bytes(), before) || d.Metadata.Code.CanUndo() {
		t.Fatal("metadata drag is not a single undo step")
	}
	if !d.Metadata.Code.Redo() {
		t.Fatal("redo")
	}
	d.View = struct{ Zoom float32 }{4}
	if err := m.SaveAll(); err != nil {
		t.Fatal(err)
	}
	if d.Dirty() || m.Dirty() {
		t.Fatal("saved metadata stayed dirty")
	}
	if actual, _ := os.ReadFile(m.Abs(d.Path)); !bytes.Equal(actual, pixels) {
		t.Fatal("metadata save changed source pixels")
	}
	if _, err := m.OpenScript(d.Path); err == nil {
		t.Fatal("binary image opened as text")
	}
	if len(m.Scripts()) != 1 {
		t.Fatal("binary attempted open leaked a document")
	}
}

func TestStructuredAssetDiscardUsesExistingOwner(t *testing.T) {
	m, d, _ := imageDocument(t)
	dirty, discarded := true, false
	d.Binding = AssetBinding{Dirty: func() bool { return dirty }, Discard: func() { dirty, discarded = false, true }}
	if err := m.CloseAsset(d.Asset, false); err == nil || discarded {
		t.Fatal("ordinary close discarded a structured editor journal")
	}
	if err := m.CloseAsset(d.Asset, true); err != nil || !discarded || dirty || m.AssetDocument(d.Asset) != nil {
		t.Fatal("explicit Discard left the structured owner dirty", err)
	}
}

func TestAssetMetadataConflictNeverSilentlyOverwrites(t *testing.T) {
	m, d, _ := imageDocument(t)
	s := d.Metadata
	s.Code.ReplaceSnapshot([]byte("mine\n"), false)
	before := bytes.Clone(s.Code.Bytes())
	st, err := os.Stat(m.Abs(s.Path))
	if err != nil {
		t.Fatal(err)
	}
	external := []byte("disk { semantic=normal extra=23 }\n")
	if err := os.WriteFile(m.Abs(s.Path), external, 0o644); err != nil {
		t.Fatal(err)
	}
	// Saving checks content, not just a cached watcher timestamp.
	if err := os.Chtimes(m.Abs(s.Path), st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveAsset(d); err == nil || !s.Conflict {
		t.Fatal("concurrent sidecar edit was overwritten")
	}
	if actual, _ := os.ReadFile(m.Abs(s.Path)); !bytes.Equal(actual, external) || !bytes.Equal(s.Code.Bytes(), before) {
		t.Fatal("conflict lost disk or in-memory data")
	}
	m.KeepScript(s)
	if s.Conflict {
		t.Fatal("explicit Keep mine did not acknowledge conflict")
	}
	if err := m.SaveAsset(d); err != nil {
		t.Fatal(err)
	}
	if actual, _ := os.ReadFile(m.Abs(s.Path)); !bytes.Equal(actual, before) {
		t.Fatal("explicit resolution was not saved")
	}
	s.Code.ReplaceSnapshot([]byte("unsaved second change\n"), false)
	touchLater(t, m.Abs(s.Path), []byte("external second change\n"))
	_, conflicts := m.CheckExternalScripts()
	if len(conflicts) != 1 || conflicts[0] != s || !s.Conflict {
		t.Fatal("dirty external change did not surface conflict")
	}
	if err := m.ReloadScript(s); err != nil {
		t.Fatal(err)
	}
	if s.Conflict || s.Dirty() || string(s.Code.Bytes()) != "external second change\n" {
		t.Fatal("Reload did not adopt the disk version")
	}
	touchLater(t, m.Abs(s.Path), []byte("clean external change\n"))
	// Ensure the polling timestamp differs even on coarse filesystems.
	later := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(m.Abs(s.Path), later, later); err != nil {
		t.Fatal(err)
	}
	reloaded, conflicts := m.CheckExternalScripts()
	if len(reloaded) != 1 || len(conflicts) != 0 || s.Dirty() || string(s.Code.Bytes()) != "clean external change\n" {
		t.Fatal("clean sidecar did not reload in the shared model")
	}
}

func TestImageDocumentRenameRecoveryAndClose(t *testing.T) {
	m, d, _ := imageDocument(t)
	s := d.Metadata
	s.Code.ReplaceSnapshot([]byte("meta { unknown=19 }\n"), false)
	if err := m.CloseAsset(d.Asset, false); err == nil {
		t.Fatal("dirty image close accepted")
	}
	if n, err := m.WriteRecovery(); err != nil || n != 1 {
		t.Fatal("shared recovery missing", n, err)
	}
	for _, suffix := range []string{"", ".meta"} {
		if err := os.Rename(m.Abs("art/hero.png"+suffix), m.Abs("art/runner.png"+suffix)); err != nil {
			t.Fatal(err)
		}
	}
	m.Renamed("art/hero.png", "art/runner.png")
	if d.Path != "art/runner.png" || s.Path != d.Path+".meta" || m.AssetDocument(d.Asset) != d || m.Script(s.Path) != s {
		t.Fatal("rename split stable identity and source metadata")
	}
	if err := m.SaveAsset(d); err != nil {
		t.Fatal(err)
	}
	if err := m.CloseAsset(d.Asset, false); err != nil {
		t.Fatal(err)
	}
	if len(m.AssetDocuments()) != 0 || len(m.Scripts()) != 0 {
		t.Fatal("closed image retained its document buffers")
	}
	entries, _ := os.ReadDir(filepath.Join(m.opt.Root, RecoveryDir))
	if len(entries) != 0 {
		t.Fatal("saved/closed metadata retained stale recovery")
	}
}

func TestUnknownBinaryAndMissingMetadataAreHonestFallbacks(t *testing.T) {
	m := newManager(t)
	d, err := m.OpenAsset(AssetRequest{Asset: ids.AssetID{Lo: 99}, Path: "assets/music.ogg", Kind: asset.ArtAudio})
	if err != nil || d.Handler != "asset.readonly" || d.Capabilities != View|RevealSource || d.Source != nil || d.Metadata != nil || len(m.Scripts()) != 0 {
		t.Fatal("unknown binary was treated as editable text", err)
	}
	if err := m.Handlers().Register(Handler{ID: "image", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Capabilities: View | EditMetadata | Preview | RevealSource}); err != nil {
		t.Fatal(err)
	}
	d, err = m.OpenAsset(AssetRequest{Asset: ids.AssetID{Lo: 100}, Path: "assets/no-meta.png", Kind: asset.ArtTexture})
	if err != nil || d.Metadata != nil || d.Capabilities.Has(EditMetadata) || d.Note == "" {
		t.Fatal("missing metadata was silently invented", err)
	}
	if _, err := os.Stat(m.Abs("assets/no-meta.png.meta")); !os.IsNotExist(err) {
		t.Fatal("opening an asset wrote metadata")
	}
}

func TestDocumentHandlerSemanticRoleResolution(t *testing.T) {
	var r HandlerRegistry
	for _, h := range []Handler{
		{ID: "image", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Extensions: []string{".png"}, Capabilities: View | Preview},
		{ID: "normal", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Roles: []string{"normal"}, Capabilities: View | Preview},
	} {
		if err := r.Register(h); err != nil {
			t.Fatal(err)
		}
	}
	if h := r.Resolve(asset.ArtTexture, "stone.png", "normal"); h.ID != "normal" {
		t.Fatal("normal image selected by extension rather than declared semantic role")
	}
	if h := r.Resolve(asset.ArtTexture, "stone.png", "color"); h.ID != "image" {
		t.Fatal("color fallback")
	}
	if h := r.Resolve("", "stone.PNG", ""); h.ID != "image" {
		t.Fatal("failed importer extension fallback")
	}
	r.Unregister("normal")
	if h := r.Resolve(asset.ArtTexture, "stone.png", "normal"); h.ID != "image" {
		t.Fatal("removed handler remained live")
	}
	if err := r.Register(Handler{ID: "duplicate", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Capabilities: View}); err == nil {
		t.Fatal("ambiguous kind registration accepted")
	}
}

func TestAcquireAssetMovesDirtyMetadataByStableIDWithoutOldPathSave(t *testing.T) {
	m, d, _ := imageDocument(t)
	metadata := d.Metadata
	metadata.Code.ReplaceSnapshot([]byte("meta { unknown=31 }\n"), false)
	for _, suffix := range []string{"", ".meta"} {
		if err := os.Rename(m.Abs("art/hero.png"+suffix), m.Abs("art/runner.png"+suffix)); err != nil {
			t.Fatal(err)
		}
	}
	// An external rename/index refresh has not called Manager.Renamed.
	again, err := m.AcquireAsset(AssetRequest{Asset: d.Asset, Path: "art/runner.png", Kind: asset.ArtTexture})
	if err != nil || again != d || again.Metadata != metadata || metadata.Path != "art/runner.png.meta" || m.Script("art/hero.png.meta") != nil {
		t.Fatal("stable ID rename duplicated/lost the metadata journal", err)
	}
	if !metadata.Code.CanUndo() || !metadata.Dirty() {
		t.Fatal("rename reset history or dirty state")
	}
	if err := m.SaveAsset(d); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.Abs("art/hero.png.meta")); !os.IsNotExist(err) {
		t.Fatal("Save recreated the old sidecar path")
	}
	if raw, _ := os.ReadFile(m.Abs(metadata.Path)); string(raw) != "meta { unknown=31 }\n" {
		t.Fatal("Save did not use the renamed source identity")
	}
}

func TestAcquireAssetRenameCannotStealAnotherOpenDocument(t *testing.T) {
	m, d, _ := imageDocument(t)
	if err := WriteAtomic(m.Abs("art/target.png.meta"), []byte("other metadata\n")); err != nil {
		t.Fatal(err)
	}
	other, err := m.OpenScript("art/target.png.meta")
	if err != nil {
		t.Fatal(err)
	}
	other.Code.ReplaceSnapshot([]byte("other unsaved metadata\n"), false)
	if _, err := m.AcquireAsset(AssetRequest{Asset: d.Asset, Path: "art/target.png", Kind: asset.ArtTexture}); err == nil {
		t.Fatal("rename silently replaced the destination's open journal")
	}
	if d.Path != "art/hero.png" || d.Metadata.Path != "art/hero.png.meta" || m.Script("art/target.png.meta") != other || !other.Dirty() {
		t.Fatal("rejected rename changed document identities")
	}
}

func TestAcquireAssetMovesSharedTextSourceByStableID(t *testing.T) {
	m := newManager(t)
	if err := m.Handlers().Register(Handler{ID: "script", Kinds: []asset.ArtifactKind{asset.ArtScript}, TextSource: true, Capabilities: View | EditSource}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(m.Abs("scripts/old.aes"), []byte("old source\n")); err != nil {
		t.Fatal(err)
	}
	d, err := m.OpenAsset(AssetRequest{Asset: ids.AssetID{Lo: 901}, Kind: asset.ArtScript, Path: "scripts/old.aes"})
	if err != nil {
		t.Fatal(err)
	}
	code := d.Source
	code.Code.ReplaceSnapshot([]byte("working source\n"), false)
	if err := os.Rename(m.Abs(d.Path), m.Abs("scripts/new.aes")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AcquireAsset(AssetRequest{Asset: d.Asset, Kind: asset.ArtScript, Path: "scripts/new.aes"}); err != nil {
		t.Fatal(err)
	}
	if d.Source != code || code.Path != d.Path || m.Script(d.Path) != code {
		t.Fatal("renamed source did not keep one Code document")
	}
	if err := m.SaveAsset(d); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.Abs("scripts/old.aes")); !os.IsNotExist(err) {
		t.Fatal("renamed source saved to its old path")
	}
}

func TestAssetAndCodeViewsRetainOneTextJournalAcrossEitherCloseOrder(t *testing.T) {
	m := newManager(t)
	if err := m.Handlers().Register(Handler{ID: "material", Kinds: []asset.ArtifactKind{asset.ArtMaterial}, TextSource: true, Capabilities: View | EditSource}); err != nil {
		t.Fatal(err)
	}
	const path = "assets/shared.aemat"
	if err := WriteAtomic(m.Abs(path), []byte("saved material\n")); err != nil {
		t.Fatal(err)
	}
	req := AssetRequest{Asset: ids.AssetID{Lo: 905}, Kind: asset.ArtMaterial, Path: path}
	d, err := m.OpenAsset(req)
	if err != nil {
		t.Fatal(err)
	}
	source := d.Source
	release := source.RetainView()
	source.Code.ReplaceSnapshot([]byte("working material\n"), false)
	if err := m.CloseAsset(d.Asset, true); err != nil {
		t.Fatal(err)
	}
	if m.Script(path) != source || source.Dirty() || string(source.Code.Bytes()) != "saved material\n" {
		t.Fatal("discarding asset view removed the active Code model or retained hidden edits")
	}
	again, err := m.OpenAsset(req)
	if err != nil || again.Source != source || len(m.Scripts()) != 1 {
		t.Fatal("reopening asset view duplicated the Code document", err)
	}
	release()
	release() // duplicate disposal cannot underflow ownership.
	m.CloseScript(source)
	if m.Script(path) != source {
		t.Fatal("closing Code first removed the asset view's journal")
	}
	if err := m.CloseAsset(again.Asset, false); err != nil {
		t.Fatal(err)
	}
	if len(m.Scripts()) != 0 {
		t.Fatal("last view retained a historical text journal")
	}
}

func TestClosedSceneDoesNotRetainItsMutableSceneInAssetIdentity(t *testing.T) {
	m := newManager(t)
	sc := m.New("scenes/shared.aescene", sampleScene(m, 1))
	if err := m.Save(sc.ID); err != nil {
		t.Fatal(err)
	}
	d, err := m.AcquireAsset(AssetRequest{Asset: ids.AssetID{Lo: 906}, Kind: asset.ArtScene, Path: sc.Path})
	if err != nil {
		t.Fatal(err)
	}
	d.Scene = sc
	if err := m.Close(sc.ID, false); err != nil {
		t.Fatal(err)
	}
	if d.Scene != nil || d.Dirty() {
		t.Fatal("closed Scene remained reachable as a mutable asset document")
	}
}
