package editor

import (
	"bytes"
	stdctx "context"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/importers/preset"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/selection"
	"aego/gpu/null"
	"aego/modules/particles"
	"aego/platform/input"
	"aego/project"
	"aego/render/texture"
	"aego/scene"
	"aego/ui/core"
	"aego/ui/widgets"
)

type drawingFixture struct {
	ed     *context.Editor
	doc    *document.AssetDocument
	model  *artedit.Document
	panel  *presetPanel
	frames []ids.AssetID
}

func newDrawingFixture(t *testing.T) drawingFixture {
	t.Helper()
	p, err := project.Create(t.TempDir(), "Particle frames", false)
	if err != nil {
		t.Fatal(err)
	}
	d, err := art.New(8, 8, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	edit, err := d.Begin(d.Revision(), "two frames")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := edit.AddFrame(d.Info().Frames[0].ID, 240); err != nil {
		t.Fatal(err)
	}
	change, err := edit.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if err := change.Apply(); err != nil {
		t.Fatal(err)
	}
	artFrames := d.Info().Frames
	data, err := d.Encode()
	d.Close()
	if err != nil {
		t.Fatal(err)
	}
	const artPath, presetPath = "assets/fx.aeart", "assets/effect.aepreset"
	if err := os.WriteFile(filepath.Join(p.Root, artPath), data, 0644); err != nil {
		t.Fatal(err)
	}
	imp := artsource.New()
	meta := pipeline.NewMeta(imp.ID(), imp.Version(), imp.Defaults())
	// Explicit metadata mappings take priority over the deterministic default.
	custom := ids.NewAssetID()
	meta.Subs = []pipeline.SubAsset{{Name: artsource.FrameName(artFrames[0].ID, art.Albedo), ID: custom}}
	if err := os.WriteFile(filepath.Join(p.Root, artPath+pipeline.MetaSuffix), pipeline.WriteMeta(meta), 0644); err != nil {
		t.Fatal(err)
	}
	effect, err := particles.PresetSource("Dust")
	if err != nil {
		t.Fatal(err)
	}
	effect = bytes.Replace(effect, []byte("order=3"), []byte("frameFPS=0\n      order=3"), 1)
	if err := os.WriteFile(filepath.Join(p.Root, presetPath), effect, 0644); err != nil {
		t.Fatal(err)
	}
	registry := pipeline.NewImporterRegistry()
	for _, imp := range []pipeline.Importer{artsource.New(), preset.New()} {
		if err := registry.Register(imp); err != nil {
			t.Fatal(err)
		}
	}
	pl, err := pipeline.Open(p, registry, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("import: %+v, %v", batch, err)
	}
	dev := null.New()
	t.Cleanup(dev.Close)
	load := loader.New(dev, texture.NewUploader(dev), log.Nop())
	if err := load.Mount(pl.Index(), pl.Artifacts()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(load.Close)
	sceneDoc, _, _ := inspectorFixture(t)
	types := sceneDoc.Scene().Types()
	mgr := document.NewManager(document.Options{Root: p.Root, Types: types})
	for _, h := range []document.Handler{
		{ID: "render2d.art", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Extensions: []string{".aeart"}, SourceExtensionsOnly: true, Capabilities: document.View | document.EditSource | document.EditMetadata},
		{ID: "particles.preset", Kinds: []asset.ArtifactKind{asset.ArtPreset}, Roles: []string{particles.NameSystem}, TextSource: true, Capabilities: document.View | document.EditSource},
	} {
		if err := mgr.Handlers().Register(h); err != nil {
			t.Fatal(err)
		}
	}
	ed := &context.Editor{Project: p, Pipeline: pl, Loader: load, Docs: mgr, Types: types}
	artID, ok := pl.Lookup(artPath)
	if !ok {
		t.Fatal("drawing not found")
	}
	doc, err := ed.OpenAsset(artID)
	if err != nil {
		t.Fatal(err)
	}
	model, err := artedit.Bind(mgr, doc)
	if err != nil {
		t.Fatal(err)
	}
	presetID, ok := pl.Lookup(presetPath)
	if !ok {
		t.Fatal("preset not found")
	}
	presetDoc, err := ed.OpenAsset(presetID)
	if err != nil || presetDoc.Source == nil {
		t.Fatalf("open preset: %+v, %v", presetDoc, err)
	}
	a := &adapter{ed: ed, panels: map[document.ID]*presetPanel{}}
	panel := a.panel(presetDoc).(*presetPanel)
	want := []ids.AssetID{custom, artsource.StableID(meta.ID, artsource.FrameName(artFrames[1].ID, art.Albedo))}
	return drawingFixture{ed: ed, doc: doc, model: model, panel: panel, frames: want}
}

func TestPresetDrawingFramesUndoRedoAndSavedIdentity(t *testing.T) {
	f := newDrawingFixture(t)
	code := f.panel.d.Source.Code
	before := slices.Clone(code.Bytes())
	if err := f.panel.assignDrawing(f.doc); err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		cfg, err := particles.ReadPreset(f.ed.Types, f.ed.Loader, code.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		got := make([]ids.AssetID, len(cfg.Frames))
		for i := range cfg.Frames {
			got[i] = cfg.Frames[i].ID
		}
		if !slices.Equal(got, f.frames) || cfg.FrameFPS != 12 {
			t.Fatalf("frames=%v FPS=%g, want %v at 12 FPS", got, cfg.FrameFPS, f.frames)
		}
		preview := particles.NewPreview(f.ed.Types, f.ed.Loader)
		defer preview.Close()
		if err := preview.Configure(cfg, 1); err != nil {
			t.Fatalf("actual particle preview rejected the authored preset: %v", err)
		}
		preview.Burst(1)
		if preview.Count() != 1 {
			t.Fatalf("particle preview did not spawn: %d", preview.Count())
		}
	}
	check()
	if !f.panel.d.Dirty() || !code.Undo() || !bytes.Equal(code.Bytes(), before) {
		t.Fatal("one Undo did not restore the original preset")
	}
	if !code.Redo() {
		t.Fatal("Redo did not restore the preset")
	}
	check()
	if err := f.ed.Docs.SaveAsset(f.panel.d); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(filepath.Join(f.ed.Project.Root, f.panel.d.Path))
	if err != nil || !bytes.Equal(onDisk, code.Bytes()) {
		t.Fatalf("saved preset differs: %v", err)
	}
	if _, err := particles.ReadPreset(f.ed.Types, f.ed.Loader, onDisk); err != nil {
		t.Fatalf("saved preset cannot reopen: %v", err)
	}
}

func TestDrawingFramesRejectStaleAndUnsavedSources(t *testing.T) {
	t.Run("unpublished drawing", func(t *testing.T) {
		f := newDrawingFixture(t)
		original := slices.Clone(f.panel.d.Source.Code.Bytes())
		if _, err := f.model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 1, Y: 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}}); err != nil {
			t.Fatal(err)
		}
		if err := f.panel.assignDrawing(f.doc); err == nil || !strings.Contains(err.Error(), "preview") {
			t.Fatalf("unpublished pixels were accepted: %v", err)
		}
		if !bytes.Equal(original, f.panel.d.Source.Code.Bytes()) {
			t.Fatal("failed assignment changed preset source")
		}
	})
	t.Run("stale snapshot", func(t *testing.T) {
		f := newDrawingFixture(t)
		frames, err := prepareDrawingFrames(f.ed, f.doc)
		if err != nil || !frames.current() {
			t.Fatalf("initial snapshot invalid: %v", err)
		}
		if _, err := f.model.Execute(stdctx.Background(), art.Operation{Kind: "duplicate-frame"}); err != nil {
			t.Fatal(err)
		}
		if frames.current() {
			t.Fatal("changed drawing accepted a stale frame snapshot")
		}
	})
	t.Run("metadata draft", func(t *testing.T) {
		f := newDrawingFixture(t)
		f.doc.Metadata.Code.ReplaceSnapshot(append(slices.Clone(f.doc.Metadata.Code.Bytes()), '\n'), false)
		if err := f.panel.assignDrawing(f.doc); err == nil || !strings.Contains(err.Error(), "metadata") {
			t.Fatalf("dirty metadata accepted: %v", err)
		}
	})
	t.Run("external source change", func(t *testing.T) {
		f := newDrawingFixture(t)
		prepared, err := prepareDrawingFrames(f.ed, f.doc)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(f.ed.Project.Root, f.doc.Path), []byte("external change"), 0644); err != nil {
			t.Fatal(err)
		}
		if prepared.current() {
			t.Fatal("an external file change did not invalidate the prepared frames")
		}
		if err := f.panel.assignDrawing(f.doc); err == nil || !strings.Contains(err.Error(), "reimport") {
			t.Fatalf("external source edit accepted: %v", err)
		}
	})
	t.Run("closed drawing", func(t *testing.T) {
		f := newDrawingFixture(t)
		if err := f.ed.Docs.CloseAsset(f.doc.Asset, false); err != nil {
			t.Fatal(err)
		}
		if err := f.panel.assignDrawing(f.doc); err == nil || !strings.Contains(err.Error(), "no longer open") {
			t.Fatalf("closed drawing accepted: %v", err)
		}
	})
}

func TestSceneParticleDrawingFramesOneUndo(t *testing.T) {
	f := newDrawingFixture(t)
	sc := scene.New(f.ed.Types, f.ed.Loader, scene.Options{})
	h := sc.Create(sc.Root(), "Effect")
	component := scene.Add[particles.ParticleSystem2D](sc, h)
	*component = particles.DefaultParticleSystem2D()
	component.FrameFPS = 0
	sc.Flush()
	d := f.ed.Docs.New("effect.aescene", sc)
	f.ed.Selection = selection.New()
	f.ed.Selection.Set(selection.Item{Doc: d.ID, Node: sc.ID(h)})
	td, ok := f.ed.Types.Schema().ByName(particles.NameSystem)
	if !ok {
		t.Fatal("particle schema is unavailable")
	}
	if err := assignSceneDrawing(f.ed, d, sc.ID(h), td, f.doc); err != nil {
		t.Fatal(err)
	}
	component, ok = scene.Get[particles.ParticleSystem2D](sc, h)
	if !ok {
		t.Fatal("particle system disappeared")
	}
	check := func() {
		t.Helper()
		got := make([]ids.AssetID, len(component.Frames))
		for i, ref := range component.Frames {
			got[i] = ref.ID
		}
		if !slices.Equal(got, f.frames) || component.FrameFPS != 12 {
			t.Fatalf("frames=%v, FPS=%g", got, component.FrameFPS)
		}
	}
	check()
	if _, count := d.History.Top(); count != 1 {
		t.Fatalf("expected one scene history step, got %d", count)
	}
	if err := d.History.Undo(); err != nil || len(component.Frames) != 0 || component.FrameFPS != 0 {
		t.Fatalf("Undo failed: frames=%v FPS=%g err=%v", component.Frames, component.FrameFPS, err)
	}
	if err := d.History.Redo(); err != nil {
		t.Fatal(err)
	}
	check()
	f.ed.Selection.Clear()
	if err := assignSceneDrawing(f.ed, d, sc.ID(h), td, f.doc); err == nil {
		t.Fatal("assignment to a no-longer-selected node succeeded")
	}
	if _, count := d.History.Top(); count != 1 {
		t.Fatalf("rejected stale selection changed history: %d", count)
	}
}

func TestSceneParticleDrawingPickerViaUI(t *testing.T) {
	f := newDrawingFixture(t)
	sc := scene.New(f.ed.Types, f.ed.Loader, scene.Options{})
	h := sc.Create(sc.Root(), "Effect")
	*scene.Add[particles.ParticleSystem2D](sc, h) = particles.DefaultParticleSystem2D()
	sc.Flush()
	d := f.ed.Docs.New("effect.aescene", sc)
	f.ed.Selection = selection.New()
	node := sc.ID(h)
	f.ed.Selection.Set(selection.Item{Doc: d.ID, Node: node})
	td, _ := f.ed.Types.Schema().ByName(particles.NameSystem)
	ui := core.New(nil)
	f.ed.UI = ui
	w := particleInspector{parent: inertPreview{}, td: td}
	for i := range w.closed {
		w.closed[i] = i != 6
	}
	var events input.State
	var combo, assign mathx.Rect
	draw := func(at mathx.Vec2, down, up bool) {
		events.BeginFrame()
		events.ApplyMouseMove(at, mathx.Vec2{})
		if down {
			events.ApplyMouseDown(at, input.MouseLeft)
		}
		if up {
			events.ApplyMouseUp(at, input.MouseLeft)
		}
		ui.Begin(&events, mathx.V2(800, 600), 1)
		ui.PushCursor(core.Cursor{X: 10, Y: 12, W: 400, RowH: 18, Space: 2})
		w.Draw(f.ed, []ids.NodeID{node}, func() {
			if w.section == 6 {
				cur := *ui.Cursor()
				r := cur.Next(0)
				lw := widgets.LabelWidth(r.W)
				combo = mathx.Rect{X: r.X + lw, Y: r.Y, W: r.W - lw, H: r.H}
				hint := "Particles use constant FPS, not individual frame durations. Save new frames before assigning them. For multiple frames, zero FPS becomes 12."
				for range widgets.WrapLines(ui, hint, cur.W) {
					cur.Next(0)
				}
				assign = cur.Next(0)
			}
		})
		ui.PopCursor()
		ui.End()
	}
	click := func(at mathx.Vec2) {
		draw(at, true, false)
		draw(at, false, true)
		draw(at, false, false)
	}
	draw(mathx.Vec2{}, false, false)
	click(mathx.V2(combo.X+combo.W/2, combo.Y+combo.H/2))
	if !ui.AnyPopup() {
		t.Fatal("particle inspector drawing picker did not open")
	}
	click(mathx.V2(combo.X+20, combo.Y+combo.H+ui.Theme.Padding+ui.Theme.RowHeight+ui.Theme.Spacing+ui.Theme.RowHeight/2))
	if w.drawing.selected != f.doc.Asset {
		t.Fatalf("picker selected %s, want %s", w.drawing.selected, f.doc.Asset)
	}
	click(mathx.V2(assign.X+assign.W/2, assign.Y+assign.H/2))
	c, _ := scene.Get[particles.ParticleSystem2D](sc, h)
	if len(c.Frames) != len(f.frames) || c.Frames[0].ID != f.frames[0] || c.Frames[1].ID != f.frames[1] {
		notice, _ := f.ed.LastNotice()
		t.Fatalf("inspector UI did not assign drawing frames: %+v; notice=%+v", c.Frames, notice)
	}
	if err := d.History.Undo(); err != nil || len(c.Frames) != 0 {
		t.Fatalf("inspector assignment Undo failed: %v", err)
	}
}
