package editor

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"aego/asset"
	"aego/asset/importers/atlas"
	textureimport "aego/asset/importers/texture"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/format/aetext"
	"aego/gpu/null"
	"aego/platform/input"
	"aego/project"
	"aego/render/texture"
	"aego/scene/mutate"
	"aego/ui/core"
	"aego/ui/widgets"
)

type imageFixture struct {
	ed                     *context.Editor
	ws                     *workspace.Workspace
	ext                    *ext.Registry
	albedo, normal, height ids.AssetID
	frames, normalFrames   [2]ids.AssetID
}

func imageAuthoringFixture(t *testing.T) *imageFixture {
	t.Helper()
	p, err := project.Create(t.TempDir(), "Viewer", false)
	if err != nil {
		t.Fatal(err)
	}
	f := &imageFixture{albedo: ids.NewAssetID(), normal: ids.NewAssetID(), height: ids.NewAssetID()}
	for i := range f.frames {
		f.frames[i], f.normalFrames[i] = ids.NewAssetID(), ids.NewAssetID()
	}
	imp := textureimport.New()
	for _, a := range []struct {
		name, role string
		id         ids.AssetID
	}{{"surface.png", "color", f.albedo}, {"surface_n.png", "normal", f.normal}, {"surface_height.png", "height", f.height}} {
		img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				c := color.NRGBA{R: 200, G: 120, B: 80, A: 200}
				if a.role == "normal" {
					c = color.NRGBA{R: 128, G: 128, B: 255, A: 17}
				}
				if a.role == "height" {
					c = color.NRGBA{R: byte(x * 30), G: byte(y * 30), B: 0, A: 255}
				}
				img.SetNRGBA(x, y, c)
			}
		}
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, img); err != nil {
			t.Fatal(err)
		}
		full := filepath.Join(p.Root, "assets", a.name)
		if err := os.WriteFile(full, encoded.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
		meta := asset.NewSourceMeta("texture", imp.Version(), imp.Defaults())
		meta.ID = a.id
		meta.Settings["role"] = aetext.String(a.role)
		if a.role == "height" {
			meta.Settings["atlas"] = aetext.String("viewer")
			meta.Settings["generate_normal"] = aetext.Bool(true)
			meta.Settings["height_strength"] = aetext.Float32(2)
		}
		if a.role != "height" {
			frames := f.frames
			if a.role == "normal" {
				frames = f.normalFrames
			}
			meta.Subs = []asset.SourceFrame{{ID: frames[0], Name: "left", Rect: [4]int{0, 0, 4, 8}}, {ID: frames[1], Name: "right", Rect: [4]int{4, 0, 4, 8}}}
		}
		if err := os.WriteFile(full+".meta", asset.MarshalSourceMeta(meta), 0644); err != nil {
			t.Fatal(err)
		}
	}
	imps := pipeline.NewImporterRegistry()
	if err := imps.Register(atlas.Wrap(imp)); err != nil {
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
	if err != nil || len(batch.Failed) > 0 {
		t.Fatalf("fixture import: %v %+v", err, batch.Failed)
	}
	dev := null.New()
	t.Cleanup(dev.Close)
	r := imageTestRenderer(t, dev)
	load := loader.New(dev, texture.NewUploader(dev), log.Nop())
	load.SetMaterials(r.Materials())
	if err := load.Mount(pl.Index(), pl.Artifacts()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(load.Close)
	f.ed = &context.Editor{Docs: document.NewManager(document.Options{Root: p.Root}), Pipeline: pl, Loader: load, Renderer: r}
	f.ws = workspace.New(p.Root, nil)
	f.ext = ext.New(ext.Deps{Editor: f.ed, Workspace: f.ws})
	if err := f.ext.Run("render2d", imageHooks); err != nil {
		t.Fatal(err)
	}
	f.ed.Docs.Subscribe(func(ev document.Event) {
		if ev.Kind == document.EvSourceSaved {
			id, ok := pl.Lookup(ev.Path[:len(ev.Path)-len(".meta")])
			if ok {
				b, err := pl.ReloadMetadata(id)
				if err != nil {
					t.Error(err)
				}
				f.ed.ApplyBatch(b)
			}
		}
	})
	return f
}

func TestSourceInspectorSwitchesSlicingAndChannelsAgainstWorkingMetadata(t *testing.T) {
	f := imageAuthoringFixture(t)
	f.ed.UI = core.New(nil)
	in := &input.State{}
	var shown *asset.SourceMeta
	f.ed.DrawSpriteSlicer = func(_ ids.AssetID, m *asset.SourceMeta) { shown = m }
	draw := func(id ids.AssetID) {
		shown = nil
		ui := f.ed.UI
		r := mathx.Rect{W: 800, H: 640}
		ui.Begin(in, mathx.Vec2{X: r.W, Y: r.H}, 1)
		ui.PushClip(r)
		ui.Region(r)
		entry, _ := f.ed.Pipeline.Entry(id)
		f.ed.AssetViews[string(entry.Kind)](f.ed, id)
		ui.PopCursor()
		ui.PopClip()
		ui.End()
	}
	toggle := func(id ids.AssetID) {
		p := mathx.Vec2{X: 789, Y: 14}
		in.BeginFrame()
		in.ApplyMouseDown(p, input.MouseLeft)
		draw(id)
		in.BeginFrame()
		in.ApplyMouseUp(p, input.MouseLeft)
		draw(id)
	}
	draw(f.albedo)
	if shown == nil || len(shown.Subs) != 2 {
		t.Fatal("an existing sheet did not open its slicing tools")
	}
	doc := f.ed.Docs.AssetDocument(f.albedo)
	env, err := f.ed.TextureAuthoringEnv(f.albedo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := document.ApplyTextureMutation(doc.Metadata, env, mutate.Tx{Ops: []mutate.Op{mutate.TextureFrameRename(f.frames[0], "unsaved")}}, false); err != nil {
		t.Fatal(err)
	}
	draw(f.albedo)
	if shown == nil || shown.Subs[0].Name != "unsaved" || shown.Subs[0].ID != f.frames[0] || !doc.Dirty() {
		t.Fatal("slicer read the saved pipeline index instead of the shared working metadata")
	}
	toggle(f.albedo)
	if shown != nil {
		t.Fatal("sheet cannot switch to its channels")
	}
	draw(f.height)
	if shown != nil {
		t.Fatal("an unsliced source should open its channels")
	}
	toggle(f.height)
	if shown == nil || len(shown.Subs) != 0 {
		t.Fatal("slicing is unreachable before an image has frames")
	}
}

func TestImageOpeningSharesDocumentFrameHistoryAndWorkspaceTab(t *testing.T) {
	f := imageAuthoringFixture(t)
	d, err := f.ed.OpenAsset(f.frames[0])
	if err != nil {
		t.Fatal(err)
	}
	v := d.View.(*imageState)
	if d.Asset != f.albedo || d.Selected != f.frames[0] || d.Source != nil || d.Metadata == nil || !d.Metadata.ExplicitSave {
		t.Fatal("binary source or frame identity bypassed the shared document")
	}
	if !f.ws.Visible(imagePanelID(d)) {
		t.Fatal("image did not open in the normal workspace tab")
	}
	inspected, err := f.ed.DocumentForAsset(f.frames[0])
	if err != nil || inspected != d {
		t.Fatal("Inspector created a second image model")
	}
	second, err := f.ed.OpenAsset(f.frames[1])
	if err != nil || second != d || second.View != v || second.Metadata != d.Metadata {
		t.Fatal("another source frame opened another metadata/history document")
	}
	if v.source.data.Width != 4 || v.source.data.Height != 8 || v.content.SourceRect[0] != 4 {
		t.Fatal("frame selection did not use source pixels")
	}
	if v.imported.data.Width != 4 || v.imported.data.Height != 8 {
		t.Fatal("imported frame uses whole atlas dimensions")
	}
	surface := v.surface(f.ed, d)
	if surface.Normal.IsZero() || surface.Albedo.IsZero() || surface.Normal.Texture == surface.Albedo.Texture {
		t.Fatal("frame normal has no independent data-atlas binding")
	}
	var p imagePreview
	p.init()
	p.controls.FlipX = true
	p.controls.FlipY = true
	before := bytes.Clone(d.Metadata.Code.Bytes())
	_ = p.lighting()
	if !bytes.Equal(before, d.Metadata.Code.Bytes()) || d.Dirty() {
		t.Fatal("preview-only controls dirtied source metadata")
	}
}

func TestImageHeightCandidateSaveAndOneDragUndoKeepSourcePixels(t *testing.T) {
	f := imageAuthoringFixture(t)
	d, err := f.ed.OpenAsset(f.height)
	if err != nil {
		t.Fatal(err)
	}
	v := d.View.(*imageState)
	encodedBefore, err := os.ReadFile(f.ed.Docs.Abs(d.Path))
	if err != nil {
		t.Fatal(err)
	}
	metadataBefore := bytes.Clone(d.Metadata.Code.Bytes())
	for i, value := range []float32{3, 4, 5} {
		v.mutate(f.ed, d, "height_strength", widgets.Edit{Changed: true, Begin: i == 0, End: i == 2}, mutate.TextureSetting("height_strength", aetext.Float32(value)))
	}
	if v.message != "" {
		t.Fatal(v.message)
	}
	if !d.Dirty() {
		t.Fatal("height setting did not enter the shared journal")
	}
	v.refresh(f.ed, d)
	if !waitImageCandidate(t, &v.candidate, v.inputRevision, 0, false) {
		t.Fatal("height normal candidate failed")
	}
	if v.candidate.good.semantic != asset.TextureNormal {
		t.Fatal("height preview has no real generated normal")
	}
	if current, err := os.ReadFile(f.ed.Docs.Abs(d.Metadata.Path)); err != nil || !bytes.Equal(current, metadataBefore) {
		t.Fatal("private candidate silently saved import settings")
	}
	if err := f.ed.UndoAsset(d); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(d.Metadata.Code.Bytes(), metadataBefore) {
		t.Fatal("one drag required more than one Undo")
	}
	if err := f.ed.RedoAsset(d); err != nil {
		t.Fatal(err)
	}
	v.refresh(f.ed, d)
	if !waitImageCandidate(t, &v.candidate, v.inputRevision, 0, false) {
		t.Fatal("redo candidate failed")
	}
	prepared := bytes.Clone(v.candidate.good.data.Pixels)
	v.saveNow(f.ed, d)
	if v.message != "" || d.Dirty() {
		t.Fatalf("save did not commit metadata: %s", v.message)
	}
	if d.Metadata.SavedEdits == 0 {
		t.Fatal("explicit Save was not recorded for live Undo")
	}
	after, err := os.ReadFile(f.ed.Docs.Abs(d.Path))
	if err != nil || !bytes.Equal(after, encodedBefore) {
		t.Fatal("height generation edited source image pixels")
	}
	v.refresh(f.ed, d)
	if v.imported == nil || !bytes.Equal(v.imported.data.Pixels, prepared) {
		t.Fatal("saved deterministic artifact differs from the prepared candidate")
	}
}

func TestImageChannelMutationRetainsUnknownReferencesAndRejectsDataMisuse(t *testing.T) {
	f := imageAuthoringFixture(t)
	d, err := f.ed.OpenAsset(f.frames[0])
	if err != nil {
		t.Fatal(err)
	}
	v := d.View.(*imageState)
	env, err := f.ed.TextureAuthoringEnv(d.Asset)
	if err != nil {
		t.Fatal(err)
	}
	source, ok := env.Resolve(f.frames[0])
	if !ok {
		t.Fatal("source info missing")
	}
	normal, ok := env.Resolve(f.normalFrames[0])
	if !ok || !imageChannelCompatible("normal", source, normal) {
		t.Fatal("normal frame drag would be rejected")
	}
	albedo, _ := env.Resolve(f.frames[1])
	if imageChannelCompatible("normal", source, albedo) {
		t.Fatal("color texture accepted as a normal")
	}
	before := bytes.Clone(d.Metadata.Code.Bytes())
	v.mutate(f.ed, d, "normal", oneEdit(), mutate.TextureChannel(f.frames[0], "normal", f.height))
	if v.message == "" || !bytes.Equal(before, d.Metadata.Code.Bytes()) {
		t.Fatal("incompatible frame dimensions committed partially")
	}
	missing := ids.NewAssetID()
	v.mutate(f.ed, d, "normal", oneEdit(), mutate.TextureChannel(f.frames[0], "normal", missing))
	if v.message != "" {
		t.Fatal(v.message)
	}
	v.refresh(f.ed, d)
	ref, explicit, err := asset.TextureChannel(v.meta, f.frames[0], "normal")
	if err != nil || !explicit || ref != missing {
		t.Fatal("unresolved channel ID was silently removed")
	}
	if !v.surface(f.ed, d).Normal.IsZero() {
		t.Fatal("missing explicit normal silently inherited another frame")
	}
	v.mutate(f.ed, d, "normal", oneEdit(), mutate.TextureChannel(f.frames[0], "normal", ids.AssetID{}))
	v.refresh(f.ed, d)
	ref, explicit, _ = asset.TextureChannel(v.meta, f.frames[0], "normal")
	if !explicit || !ref.IsZero() {
		t.Fatal("explicit channel disable became inheritance")
	}
}

func TestImagePendingCandidateRejectsChangedSourceBeforeWatcherRevision(t *testing.T) {
	f := imageAuthoringFixture(t)
	d, err := f.ed.OpenAsset(f.height)
	if err != nil {
		t.Fatal(err)
	}
	v := d.View.(*imageState)
	if !waitImageCandidate(t, &v.candidate, v.inputRevision, 0, false) {
		t.Fatal("initial candidate failed")
	}
	good := v.candidate.good
	initialRevision := v.inputRevision
	f.ed.AssetRev++ // unrelated import refreshes status, not the height request
	v.refresh(f.ed, d)
	if v.inputRevision != initialRevision || v.candidate.pending {
		t.Fatal("unrelated asset revision restarted the candidate")
	}
	blocked := make(chan struct{})
	v.candidate.start(v.inputRevision, 0, func() (*imagePixels, error) { <-blocked; return normalPreviewPixels(255, 128, 128), nil })
	v.pendingSave = true
	before, err := os.ReadFile(f.ed.Docs.Abs(d.Metadata.Path))
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 64, A: 255})
		}
	}
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.ed.Docs.Abs(d.Path), pngBytes.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	close(blocked)
	deadline := time.Now().Add(time.Second)
	for v.candidate.pending && time.Now().Before(deadline) {
		v.poll(f.ed, d)
		runtime.Gosched()
	}
	if v.candidate.pending || v.candidate.good != good || v.candidate.err == nil || v.pendingSave {
		t.Fatal("stale source candidate replaced last-good or saved metadata")
	}
	current, err := os.ReadFile(f.ed.Docs.Abs(d.Metadata.Path))
	if err != nil || !bytes.Equal(before, current) || d.Metadata.SavedEdits != 0 {
		t.Fatal("stale Save request touched disk")
	}
	v.refresh(f.ed, d)
	if v.inputRevision == initialRevision || !waitImageCandidate(t, &v.candidate, v.inputRevision, 0, false) {
		t.Fatal("new source did not prepare a new candidate")
	}
	if bytes.Equal(v.candidate.good.data.Pixels, good.data.Pixels) {
		t.Fatal("new source still shows the old normal pixels")
	}
}

func TestImageReloadDiskReplacesMetadataIdentityAndPendingCandidate(t *testing.T) {
	f := imageAuthoringFixture(t)
	d, err := f.ed.OpenAsset(f.height)
	if err != nil {
		t.Fatal(err)
	}
	v := d.View.(*imageState)
	if !waitImageCandidate(t, &v.candidate, v.inputRevision, 0, false) {
		t.Fatal("initial candidate failed")
	}
	good, previousCode, previousRevision := v.candidate.good, d.Metadata.Code, v.inputRevision
	blocked := make(chan struct{})
	v.candidate.start(v.inputRevision, 0, func() (*imagePixels, error) {
		<-blocked
		return normalPreviewPixels(255, 128, 128), nil
	})
	v.pendingSave = true
	meta, err := asset.ParseSourceMeta(d.Metadata.Code.Bytes(), d.Metadata.Path)
	if err != nil {
		t.Fatal(err)
	}
	meta.Settings["height_strength"] = aetext.Float32(9)
	disk := asset.MarshalSourceMeta(meta)
	if err := os.WriteFile(f.ed.Docs.Abs(d.Metadata.Path), disk, 0644); err != nil {
		t.Fatal(err)
	}
	v.reload(f.ed, d)
	if v.message != "" {
		t.Fatal(v.message)
	}
	if d.Metadata.Code == previousCode || d.Metadata.Code.Rev() != previousCode.Rev() {
		t.Fatal("fixture did not replace the journal with a reused revision")
	}
	if v.meta.Settings.Float("height_strength", 0) != 9 || v.inputRevision == previousRevision || v.pendingSave || d.Dirty() {
		t.Fatal("Reload Disk retained cached metadata or an old pending Save")
	}
	close(blocked)
	if !waitImageCandidate(t, &v.candidate, v.inputRevision, 0, false) {
		t.Fatal("reloaded metadata did not prepare its current normal")
	}
	if bytes.Equal(good.data.Pixels, v.candidate.good.data.Pixels) || v.candidate.good.data.Width != 8 {
		t.Fatal("a candidate from the replaced journal was published")
	}
	v.mutate(f.ed, d, "height_strength", oneEdit(), mutate.TextureSetting("height_strength", aetext.Float32(4)))
	v.refresh(f.ed, d)
	meta.Settings["height_strength"] = aetext.Float32(7)
	disk = asset.MarshalSourceMeta(meta)
	if err := os.WriteFile(f.ed.Docs.Abs(d.Metadata.Path), disk, 0644); err != nil {
		t.Fatal(err)
	}
	changedAt := d.Metadata.DiskAt.Add(time.Second)
	if err := os.Chtimes(f.ed.Docs.Abs(d.Metadata.Path), changedAt, changedAt); err != nil {
		t.Fatal(err)
	}
	_, conflicts := f.ed.Docs.CheckExternalScripts()
	if len(conflicts) != 1 || !d.Metadata.Conflict {
		t.Fatal("dirty external metadata change did not become a conflict")
	}
	v.reload(f.ed, d)
	if v.message != "" || d.Metadata.Conflict || d.Dirty() || v.meta.Settings.Float("height_strength", 0) != 7 {
		t.Fatal("discard did not adopt the saved metadata document", v.message)
	}
	if d.Metadata.SavedEdits != 0 {
		t.Fatal("Reload Disk silently performed a Save")
	}
	if current, err := os.ReadFile(f.ed.Docs.Abs(d.Metadata.Path)); err != nil || !bytes.Equal(current, disk) {
		t.Fatal("Reload Disk wrote over the external change")
	}
	if !waitImageCandidate(t, &v.candidate, v.inputRevision, 0, false) {
		t.Fatal("discarded metadata did not produce the current private preview")
	}
	v.candidate.close()
}
