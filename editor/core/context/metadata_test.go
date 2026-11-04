package context

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/asset"
	textureimport "aego/asset/importers/texture"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/code"
	"aego/editor/core/document"
	"aego/format/aetext"
	"aego/gpu"
	"aego/gpu/null"
	"aego/project"
	"aego/render/texture"
	"aego/scene/mutate"
)

// Capture upload bytes on gpu/null; these tests prove importer/transaction and
// resource lifetime behavior, not driver compilation or displayed pixels.
type metadataDevice struct {
	*null.Device
	data        map[gpu.TextureHandle][]byte
	failUploads bool
	uploads     int
}

func (d *metadataDevice) CreateTexture(desc gpu.TextureDesc) (gpu.TextureHandle, error) {
	d.uploads++
	if d.failUploads {
		return gpu.TextureHandle{}, errors.New("injected texture upload failure")
	}
	h, err := d.Device.CreateTexture(desc)
	if err == nil {
		d.data[h] = bytes.Clone(desc.Data)
	}
	return h, err
}

func (d *metadataDevice) DestroyTexture(h gpu.TextureHandle) {
	delete(d.data, h)
	d.Device.DestroyTexture(h)
}

type metadataFixture struct {
	ed        *Editor
	dev       *metadataDevice
	docs      []*document.AssetDocument
	workers   []func()
	main      []func()
	deviceGen uint64
	lost      bool
}

func newMetadataFixture(t *testing.T) *metadataFixture {
	t.Helper()
	p, err := project.Create(t.TempDir(), "Metadata", false)
	if err != nil {
		t.Fatal(err)
	}
	imp := textureimport.New()
	var imageIDs []ids.AssetID
	for _, name := range []string{"a", "b"} {
		id := ids.NewAssetID()
		imageIDs = append(imageIDs, id)
		im := image.NewNRGBA(image.Rect(0, 0, 8, 8))
		for y := range 8 {
			for x := range 8 {
				im.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 28), G: uint8(y * 28), A: 255})
			}
		}
		var data bytes.Buffer
		if err := png.Encode(&data, im); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(p.Root, "assets", name+".png")
		if err := os.WriteFile(path, data.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
		meta := asset.NewSourceMeta("texture", imp.Version(), imp.Defaults())
		meta.ID = id
		meta.Settings["role"] = aetext.String("height")
		meta.Settings["generate_normal"] = aetext.Bool(true)
		meta.Settings["height_strength"] = aetext.Float32(2)
		if err := os.WriteFile(path+".meta", asset.MarshalSourceMeta(meta), 0644); err != nil {
			t.Fatal(err)
		}
	}
	imps := pipeline.NewImporterRegistry()
	if err := imps.Register(imp); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, imps, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || len(batch.Failed) != 0 {
		t.Fatalf("import: %v %+v", err, batch.Failed)
	}
	f := &metadataFixture{dev: &metadataDevice{Device: null.New(), data: map[gpu.TextureHandle][]byte{}}, deviceGen: 1}
	t.Cleanup(f.dev.Close)
	load := loader.New(f.dev, texture.NewUploader(f.dev), log.Nop())
	if err := load.Mount(pl.Index(), pl.Artifacts()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(load.Close)
	f.ed = &Editor{Pipeline: pl, Loader: load, Docs: document.NewManager(document.Options{Root: p.Root})}
	if err := f.ed.Docs.Handlers().Register(document.Handler{ID: "test.image", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Capabilities: document.View | document.EditMetadata}); err != nil {
		t.Fatal(err)
	}
	f.ed.Docs.Subscribe(func(ev document.Event) {
		if ev.Kind != document.EvSourceSaved || !strings.HasSuffix(ev.Path, ".meta") {
			return
		}
		id, ok := pl.Lookup(strings.TrimSuffix(ev.Path, ".meta"))
		if !ok {
			t.Fatal("saved metadata has no source")
		}
		batch, err := pl.ReloadMetadata(id)
		if err != nil {
			t.Fatal(err)
		}
		f.ed.ApplyBatch(batch)
	})
	q := f.ed.metadataService()
	q.submit = func(fn func()) { f.workers = append(f.workers, fn) }
	q.onMain = func(fn func()) { f.main = append(f.main, fn) }
	q.device = func() (uint64, bool) { return f.deviceGen, f.lost }
	t.Cleanup(f.ed.CloseAssetPreviews)
	for _, id := range imageIDs {
		d, err := f.ed.OpenAsset(id)
		if err != nil {
			t.Fatal(err)
		}
		f.docs = append(f.docs, d)
		f.pixels(t, d)
	}
	return f
}

func (f *metadataFixture) pixels(t *testing.T, d *document.AssetDocument) []byte {
	t.Helper()
	r, ok := f.ed.Loader.Texture(d.Asset)
	if !ok {
		t.Fatal("working texture did not resolve")
	}
	b, ok := f.dev.data[r.Texture]
	if !ok || len(b) != 8*8*4 {
		t.Fatal("texture upload did not contain generated normal bytes")
	}
	return bytes.Clone(b)
}

func (f *metadataFixture) strength(t *testing.T, d *document.AssetDocument, value float32) {
	t.Helper()
	env, err := f.ed.TextureAuthoringEnv(d.Asset)
	if err != nil {
		t.Fatal(err)
	}
	_, err = document.ApplyTextureMutation(d.Metadata, env, mutate.Tx{Label: "Height strength", Ops: []mutate.Op{mutate.TextureSetting("height_strength", aetext.Float32(value))}}, false)
	if err != nil {
		t.Fatal(err)
	}
}

func (f *metadataFixture) work(t *testing.T) {
	t.Helper()
	if len(f.workers) == 0 {
		t.Fatal("no pending CPU job")
	}
	fn := f.workers[0]
	f.workers = f.workers[1:]
	fn()
}

func (f *metadataFixture) publish(t *testing.T) {
	t.Helper()
	if len(f.main) == 0 {
		t.Fatal("no main-thread completion")
	}
	fn := f.main[0]
	f.main = f.main[1:]
	fn()
}

func (f *metadataFixture) drain(t *testing.T) {
	t.Helper()
	for n := 0; len(f.workers)+len(f.main) > 0; n++ {
		if n > 12 {
			t.Fatal("metadata jobs did not settle")
		}
		if len(f.workers) != 0 {
			f.work(t)
		}
		if len(f.main) != 0 {
			f.publish(t)
		}
	}
}

func (f *metadataFixture) saved(t *testing.T, d *document.AssetDocument) []byte {
	t.Helper()
	f.strength(t, d, 5)
	if err := f.ed.SaveAsset(d); err != nil {
		t.Fatal(err)
	}
	return f.pixels(t, d)
}

func TestMetadataSavedUndoPublishesWorkingArtifactAndRedoRestores(t *testing.T) {
	f := newMetadataFixture(t)
	d := f.docs[0]
	initial := f.pixels(t, d)
	saved := f.saved(t, d)
	if bytes.Equal(initial, saved) {
		t.Fatal("height strength did not affect the normal map")
	}
	disk, _ := os.ReadFile(f.ed.Docs.Abs(d.Metadata.Path))
	entry, _ := f.ed.Pipeline.Entry(d.Asset)
	beforeEntry := entry.Artifacts[0].Path
	baseline := f.dev.ResourceStats()
	if err := f.ed.UndoAsset(d); err != nil {
		t.Fatal(err)
	}
	if !d.Dirty() || !d.WorkingPreview.Pending || !bytes.Equal(saved, f.pixels(t, d)) {
		t.Fatal("Undo bypassed the candidate transaction")
	}
	f.drain(t)
	if !bytes.Equal(initial, f.pixels(t, d)) || d.WorkingPreview.Applied == 0 || d.WorkingPreview.Error != "" {
		t.Fatal("Undo did not publish the earlier generated normal")
	}
	if current, _ := os.ReadFile(f.ed.Docs.Abs(d.Metadata.Path)); !bytes.Equal(current, disk) {
		t.Fatal("live Undo silently saved metadata")
	}
	entry, _ = f.ed.Pipeline.Entry(d.Asset)
	if entry.Artifacts[0].Path != beforeEntry {
		t.Fatal("working import changed strict-export pipeline index")
	}
	if err := f.ed.RedoAsset(d); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	if d.Dirty() || !bytes.Equal(saved, f.pixels(t, d)) || len(f.ed.metadata.working) != 0 {
		t.Fatal("Redo did not return to saved artifacts and clean state")
	}
	if f.dev.ResourceStats() != baseline {
		t.Fatal("metadata generations leaked GPU resources")
	}
}

func TestMetadataLatestCompositeRestoresTwoClosedDocuments(t *testing.T) {
	f := newMetadataFixture(t)
	a, b := f.docs[0], f.docs[1]
	sa, sb := f.saved(t, a), f.saved(t, b)
	baseline := f.dev.ResourceStats()
	for _, d := range f.docs {
		if err := f.ed.UndoAsset(d); err != nil {
			t.Fatal(err)
		}
		f.drain(t)
	}
	if len(f.ed.metadata.working) != 2 {
		t.Fatal("two independent working generations were not retained")
	}
	if err := f.ed.Docs.CloseAsset(a.Asset, true); err != nil {
		t.Fatal(err)
	}
	f.work(t) // A restore is prepared but has not reached the main thread.
	if err := f.ed.Docs.CloseAsset(b.Asset, true); err != nil {
		t.Fatal(err)
	}
	if len(f.ed.metadata.queue) != maxMetadataRequests {
		t.Fatal("metadata pending queue is not bounded to the complete latest snapshot")
	}
	f.publish(t) // Superseded A must not publish its incomplete view of B.
	f.drain(t)
	if !bytes.Equal(sa, f.pixels(t, a)) || !bytes.Equal(sb, f.pixels(t, b)) {
		t.Fatal("closing the second document lost the first document's restore")
	}
	if len(f.ed.metadata.working) != 0 || len(f.ed.metadata.tracked) != 0 || len(f.ed.Docs.Scripts()) != 0 {
		t.Fatal("closed metadata retained a historical working owner")
	}
	if f.dev.ResourceStats() != baseline {
		t.Fatal("discard/close leaked a texture generation")
	}
}

func TestMetadataStaleJobCannotPublishAcrossEditDeviceOrShutdown(t *testing.T) {
	f := newMetadataFixture(t)
	d := f.docs[0]
	saved := f.saved(t, d)
	if err := f.ed.UndoAsset(d); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	if err := f.ed.RedoAsset(d); err != nil {
		t.Fatal(err)
	}
	generation := f.ed.Loader.Generation()
	f.publish(t)
	if generation != f.ed.Loader.Generation() || !bytes.Equal(saved, f.pixels(t, d)) {
		t.Fatal("older CPU job replaced a newer document state")
	}
	f.drain(t)
	if err := f.ed.UndoAsset(d); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	generation = f.ed.Loader.Generation()
	f.deviceGen++
	f.lost = true
	f.publish(t)
	f.ed.PollAssetPreviews()
	if generation != f.ed.Loader.Generation() || len(f.workers) != 0 {
		t.Fatal("lost/old context received a working generation")
	}
	f.lost = false
	f.ed.PollAssetPreviews()
	f.drain(t)
	if bytes.Equal(saved, f.pixels(t, d)) {
		t.Fatal("restored device did not prepare a fresh generation")
	}
	if err := f.ed.RedoAsset(d); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	generation = f.ed.Loader.Generation()
	f.ed.CloseAssetPreviews()
	f.publish(t)
	if generation != f.ed.Loader.Generation() {
		t.Fatal("completion published after editor shutdown")
	}
}

func TestMetadataCloseReopenAndFailedCandidateKeepLastGood(t *testing.T) {
	f := newMetadataFixture(t)
	d := f.docs[0]
	saved := f.saved(t, d)
	if err := f.ed.UndoAsset(d); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	if err := f.ed.Docs.CloseAsset(d.Asset, true); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	reopened, err := f.ed.OpenAsset(d.Asset)
	if err != nil || reopened == d {
		t.Fatal("reopen did not create a fresh shared document", err)
	}
	f.publish(t)
	if !bytes.Equal(saved, f.pixels(t, reopened)) {
		t.Fatal("a reopened clean tab kept the discarded working artifact")
	}
	baseline := f.dev.ResourceStats()
	reopened.Metadata.Code.ReplaceSnapshot([]byte("not valid metadata {{{"), false)
	if err := f.ed.ApplyMetadataPreview(reopened.Asset); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	if reopened.WorkingPreview.Error == "" || reopened.WorkingPreview.Pending || !bytes.Equal(saved, f.pixels(t, reopened)) || f.dev.ResourceStats() != baseline {
		t.Fatal("invalid metadata candidate changed last-good resources or claimed success")
	}
}

func TestMetadataFailedClosedRestoreRollsBackWholeGenerationWithoutLosingJournals(t *testing.T) {
	f := newMetadataFixture(t)
	a, b := f.docs[0], f.docs[1]
	sa, sb := f.saved(t, a), f.saved(t, b)
	baseline := f.dev.ResourceStats()
	for _, d := range f.docs {
		if err := f.ed.UndoAsset(d); err != nil {
			t.Fatal(err)
		}
		f.drain(t)
	}
	localB := bytes.Clone(b.Metadata.Code.Bytes())
	if err := os.WriteFile(f.ed.Docs.Abs(a.Metadata.Path), []byte("broken external metadata {{{"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := f.ed.Docs.CloseAsset(a.Asset, true); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	if !bytes.Equal(sa, f.pixels(t, a)) || !bytes.Equal(sb, f.pixels(t, b)) {
		t.Fatal("failed discard left an invisible private generation resident")
	}
	if !b.Dirty() || !bytes.Equal(b.Metadata.Code.Bytes(), localB) || b.WorkingPreview.Applied != 0 || b.WorkingPreview.Error == "" {
		t.Fatal("rollback lost an open journal or still claimed its working preview was applied")
	}
	if len(f.ed.metadata.working) != 0 || f.ed.metadata.files != nil || f.dev.ResourceStats() != baseline {
		t.Fatal("failed restore retained its private resources")
	}
}

func TestMetadataFailedRollbackRetriesAfterContextRecovery(t *testing.T) {
	for _, missing := range []bool{false, true} {
		name := "failed import completion"
		if missing {
			name = "failed close snapshot"
		}
		t.Run(name, func(t *testing.T) {
			f := newMetadataFixture(t)
			a, b := f.docs[0], f.docs[1]
			sa, sb := f.saved(t, a), f.saved(t, b)
			baseline := f.dev.ResourceStats()
			for _, d := range f.docs {
				if err := f.ed.UndoAsset(d); err != nil {
					t.Fatal(err)
				}
				f.drain(t)
			}
			workingA := f.pixels(t, a)
			journalB := bytes.Clone(b.Metadata.Code.Bytes())
			if missing {
				if err := os.Remove(f.ed.Docs.Abs(a.Metadata.Path)); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(f.ed.Docs.Abs(a.Metadata.Path), []byte("invalid metadata {{{"), 0644); err != nil {
				t.Fatal(err)
			}
			f.dev.failUploads = true
			if err := f.ed.Docs.CloseAsset(a.Asset, true); err != nil {
				t.Fatal(err)
			}
			f.drain(t)
			q := f.ed.metadata
			if q.rollback == nil || q.files == nil || !bytes.Equal(workingA, f.pixels(t, a)) {
				t.Fatal("failed rollback lost its bounded retry or replaced last-good resources")
			}
			attempts := f.dev.uploads
			for range 5 {
				f.ed.PollAssetPreviews()
			}
			if f.dev.uploads != attempts || len(f.workers) != 0 || len(q.queue) != 0 {
				t.Fatal("failed rollback retried every frame or grew its queue")
			}
			f.deviceGen++
			f.lost = true
			f.ed.PollAssetPreviews()
			if f.dev.uploads != attempts {
				t.Fatal("rollback uploaded into a lost device")
			}
			f.lost, f.dev.failUploads = false, false
			f.ed.PollAssetPreviews()
			if q.rollback != nil || q.retry != nil || q.files != nil || len(q.working) != 0 || len(q.published) != 0 {
				t.Fatal("recovered device retained the invisible working generation")
			}
			if !bytes.Equal(sa, f.pixels(t, a)) || !bytes.Equal(sb, f.pixels(t, b)) || f.dev.ResourceStats() != baseline {
				t.Fatal("retry did not atomically restore saved artifacts and release working resources")
			}
			if !b.Dirty() || !bytes.Equal(journalB, b.Metadata.Code.Bytes()) || b.WorkingPreview.Applied != 0 {
				t.Fatal("retry lost an open journal or kept a misleading applied marker")
			}
		})
	}
}

func TestMetadataUndoDropsFrameIDsThatNeverExistedInPersistentIndex(t *testing.T) {
	f := newMetadataFixture(t)
	d := f.docs[0]
	f.saved(t, d)
	baseline := f.dev.ResourceStats()
	frame := ids.NewAssetID()
	env, err := f.ed.TextureAuthoringEnv(d.Asset)
	if err != nil {
		t.Fatal(err)
	}
	_, err = document.ApplyTextureMutation(d.Metadata, env, mutate.Tx{Label: "Add private frame", Ops: []mutate.Op{mutate.TextureFrameAdd(asset.SourceFrame{ID: frame, Name: "added", Rect: [4]int{0, 0, 4, 8}})}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.ed.ApplyMetadataPreview(d.Asset); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	if _, ok := f.ed.Loader.Texture(frame); !ok {
		t.Fatalf("private frame was not published: %s", d.WorkingPreview.Error)
	}
	if _, ok := f.ed.Pipeline.Entry(frame); ok {
		t.Fatal("private frame polluted the persistent index")
	}
	if err := f.ed.UndoAsset(d); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	if _, ok := f.ed.Loader.Texture(frame); ok {
		t.Fatal("Undo retained a private frame ID and stale region handle")
	}
	if f.dev.ResourceStats() != baseline {
		t.Fatal("private slicing frame leaked a texture")
	}
}

func TestMetadataReloadRestoresAppliedGenerationAndInvalidatesReusedCodeRevision(t *testing.T) {
	f := newMetadataFixture(t)
	d := f.docs[0]
	saved := f.saved(t, d)
	if err := f.ed.UndoAsset(d); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	if err := f.ed.ReloadMetadata(d); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	if d.Dirty() || !bytes.Equal(saved, f.pixels(t, d)) || len(f.ed.metadata.working) != 0 {
		t.Fatal("Reload disk cleared the journal but retained its applied working normal")
	}
	// Revisions are local to a Code document. Replacing the journal can reuse
	// its counter, which must not make a pre-Reload candidate current again.
	d.Metadata.Code = code.New([]byte("invalid old metadata"))
	if err := f.ed.ApplyMetadataPreview(d.Asset); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	oldRev := d.Metadata.Code.Rev()
	raw, _ := os.ReadFile(f.ed.Docs.Abs(d.Metadata.Path))
	d.Metadata.Code = code.New(raw)
	if d.Metadata.Code.Rev() != oldRev {
		t.Fatal("fixture did not reuse the document-local revision")
	}
	f.publish(t)
	if d.WorkingPreview.Error != "" || !bytes.Equal(saved, f.pixels(t, d)) {
		t.Fatal("an old Code document candidate overwrote the replacement journal")
	}
	f.ed.PollAssetPreviews()
	f.drain(t)
	if d.WorkingPreview.Error != "" {
		t.Fatal("fresh replacement metadata did not revalidate")
	}
}
