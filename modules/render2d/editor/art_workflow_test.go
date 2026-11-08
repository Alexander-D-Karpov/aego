package editor

import (
	"bytes"
	stdctx "context"
	"image"
	"os"
	"strings"
	"testing"

	"aego/art"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/editor/ext/inspect"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/project"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/ui/core"
)

func TestArtColorPickerPreservesDisplayedBytes(t *testing.T) {
	for _, want := range []art.Color{{R: 1, G: 50, B: 200, A: 128}, {R: 127, G: 128, B: 255, A: 255}, {R: 255, G: 0, B: 0, A: 0}} {
		got := artColorBytes(mathx.ColorRGBA8(want.R, want.G, want.B, want.A))
		if got != want {
			t.Errorf("color picker bytes = %+v, want %+v", got, want)
		}
	}
}

// Exercises the native panel and real document/import pipeline with simulated
// pointer events. This is not a replacement for an OS/GPU desktop smoke test.
func TestArtCreateDrawHistorySaveAndReopen(t *testing.T) {
	p, err := project.Create(t.TempDir(), "Art workflow", false)
	if err != nil {
		t.Fatal(err)
	}
	importers := pipeline.NewImporterRegistry()
	if err := importers.Register(artsource.New()); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, importers, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	commands := command.New()
	ed := &context.Editor{Pipeline: pl, Docs: document.NewManager(document.Options{Root: p.Root}), Commands: commands, UI: core.New(nil)}
	ws := workspace.New(p.Root, nil)
	registry := ext.New(ext.Deps{Editor: ed, Workspace: ws, Commands: commands})
	if err := registry.Run("render2d", imageHooks); err != nil {
		t.Fatal(err)
	}
	if err := commands.Run("asset.new_art"); err != nil {
		t.Fatal(err)
	}
	questions := ed.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatal("new art command did not open the creation dialog")
	}
	questions[0].Submit([]string{"assets/hero.aeart", "16", "16", "rgba"})
	if notice, ok := ed.LastNotice(); ok && notice.Level == context.LevelError {
		t.Fatalf("creation failed: %s", notice.Text)
	}
	id, ok := pl.Lookup("assets/hero.aeart")
	if !ok {
		t.Fatal("created art source is not in the asset index")
	}
	doc := ed.Docs.AssetDocument(id)
	if doc == nil || doc.Handler != artHandlerID || doc.Metadata == nil {
		t.Fatalf("source did not open in the shared art document with metadata: %+v", doc)
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok || model.Session == nil {
		t.Fatal("panel did not bind the canonical art document")
	}
	panelID := artHandlerID + "/" + id.String()
	panel, ok := ws.Panel(panelID)
	if !ok || !ws.Visible(panelID) {
		t.Fatal("art document did not open in the workspace")
	}
	initial, err := os.ReadFile(ed.Docs.Abs(doc.Path))
	if err != nil {
		t.Fatal(err)
	}
	state := &input.State{}
	draw := func() {
		ed.UI.Begin(state, mathx.Vec2{X: 900, Y: 700}, 1)
		panel.Draw(&services.Frame{UI: ed.UI, Focused: true}, mathx.Rect{W: 900, H: 700})
		ed.UI.End()
	}
	state.BeginFrame()
	draw()
	point := mathx.Vec2{X: 320, Y: 250}
	// An unresolved recovery must not be replaced by a new pointer gesture.
	model.PendingRecovery = true
	state.BeginFrame()
	draw()
	state.BeginFrame()
	state.ApplyMouseMove(point, mathx.Vec2{})
	state.ApplyMouseDown(point, input.MouseLeft)
	draw()
	state.BeginFrame()
	state.ApplyMouseUp(point, input.MouseLeft)
	draw()
	if doc.Dirty() || model.History.Cursor() != 0 || model.Session.Info().Gesture {
		t.Fatal("canvas edited a drawing with an unresolved recovery")
	}
	model.PendingRecovery = false
	state.BeginFrame()
	draw()
	state.BeginFrame()
	state.ApplyMouseMove(point, mathx.Vec2{})
	state.ApplyMouseDown(point, input.MouseLeft)
	draw()
	state.BeginFrame()
	state.ApplyMouseUp(point, input.MouseLeft)
	draw()
	if !doc.Dirty() || model.History.Cursor() != 1 {
		t.Fatalf("canvas stroke did not create one dirty history entry: dirty=%v cursor=%d", doc.Dirty(), model.History.Cursor())
	}
	frame := model.Session.Frame
	painted, _, err := model.Capture(frame, art.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(painted.Pix, []byte{255}) == 0 {
		t.Fatal("native pointer stroke did not paint the canonical pixels")
	}
	// Select Eyedropper and sample the just-painted pixel through the actual
	// canvas widget. Picking a color is presentation state, not a document edit.
	panelView := &artView{size: 1, grid: true, refresh: true, tool: artEyedropper, color: mathx.ColorRGBA8(255, 0, 0, 255)}
	sampler := &artPanel{ctx: ed, reg: registry, doc: doc, model: model, view: panelView}
	drawSampler := func() {
		ed.UI.Begin(state, mathx.Vec2{X: 900, Y: 700}, 1)
		sampler.Draw(&services.Frame{UI: ed.UI, Focused: true}, mathx.Rect{W: 900, H: 700})
		ed.UI.End()
	}
	state.BeginFrame()
	drawSampler()
	state.BeginFrame()
	state.ApplyMouseMove(point, mathx.Vec2{})
	state.ApplyMouseDown(point, input.MouseLeft)
	drawSampler()
	state.BeginFrame()
	state.ApplyMouseUp(point, input.MouseLeft)
	drawSampler()
	if panelView.color != mathx.White || model.History.Cursor() != 1 || !doc.Dirty() {
		t.Fatalf("eyedropper did not pick the canvas pixel without an edit: color=%+v history=%d", panelView.color, model.History.Cursor())
	}
	dragSelection := func(tool int, points ...mathx.Vec2) {
		t.Helper()
		panelView.tool = tool
		state.BeginFrame()
		state.ApplyMouseMove(points[0], mathx.Vec2{})
		state.ApplyMouseDown(points[0], input.MouseLeft)
		drawSampler()
		for i := 1; i < len(points); i++ {
			state.BeginFrame()
			state.ApplyMouseMove(points[i], points[i].Sub(points[i-1]))
			drawSampler()
		}
		state.BeginFrame()
		state.ApplyMouseUp(points[len(points)-1], input.MouseLeft)
		drawSampler()
	}
	dragSelection(artEllipseSelect, mathx.Vec2{X: 300, Y: 210}, mathx.Vec2{X: 455, Y: 355})
	selection := model.Session.Selection
	if selection == nil || selection.Count() == 0 || selection.Count() >= selection.Bounds().Dx()*selection.Bounds().Dy() || model.History.Cursor() != 1 {
		t.Fatalf("ellipse selection was not a non-rectangular mask without history: %+v", selection)
	}
	if edges, simplified := artSelectionOutline(selection); simplified || len(edges) <= 4 {
		t.Fatalf("ellipse outline did not follow the mask: %d edges simplified=%v", len(edges), simplified)
	}
	dragSelection(artLasso, mathx.Vec2{X: 300, Y: 210}, mathx.Vec2{X: 455, Y: 210}, mathx.Vec2{X: 455, Y: 355}, mathx.Vec2{X: 300, Y: 355})
	if selection = model.Session.Selection; selection == nil || selection.Count() == 0 || model.History.Cursor() != 1 {
		t.Fatal("lasso selection did not create a mask without editing the document")
	}
	// An external edit during the drag must not publish a selection against a
	// different frame or revision, and losing focus must cancel the draft.
	panelView.tool = artRectSelect
	state.BeginFrame()
	state.ApplyMouseMove(point, mathx.Vec2{})
	state.ApplyMouseDown(point, input.MouseLeft)
	drawSampler()
	if !panelView.selecting {
		t.Fatal("selection gesture did not start")
	}
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "duplicate-frame"}); err != nil {
		t.Fatal(err)
	}
	state.BeginFrame()
	state.ApplyMouseUp(mathx.Vec2{X: 455, Y: 355}, input.MouseLeft)
	drawSampler()
	if panelView.selecting || model.Session.Selection != selection || sampler.err == nil || !strings.Contains(sampler.err.Error(), "drawing changed") {
		t.Fatal("a stale selection gesture was published after the document changed")
	}
	if err := ed.UndoAsset(doc); err != nil || model.History.Cursor() != 1 {
		t.Fatalf("could not undo the external frame edit: %v", err)
	}
	sampler.err = nil
	panelView.tool = artLasso
	state.BeginFrame()
	state.ApplyMouseMove(point, mathx.Vec2{})
	state.ApplyMouseDown(point, input.MouseLeft)
	drawSampler()
	state.BeginFrame()
	ed.UI.Begin(state, mathx.Vec2{X: 900, Y: 700}, 1)
	sampler.Draw(&services.Frame{UI: ed.UI, Focused: false}, mathx.Rect{W: 900, H: 700})
	ed.UI.End()
	if panelView.selecting || model.Session.Selection != selection || model.History.Cursor() != 1 {
		t.Fatal("focus loss did not cancel the lasso draft")
	}
	state.BeginFrame()
	state.ApplyMouseUp(point, input.MouseLeft)
	drawSampler()
	if err := model.Session.SetSelection("none", nil, 0); err != nil {
		t.Fatal(err)
	}
	// Mid-tone UI color values must reach the canonical pixels unchanged; the
	// ColorField's internal linear representation must not darken the stroke.
	panelView.tool = 0
	panelView.color = mathx.ColorRGBA8(127, 128, 200, 255)
	otherPoint := mathx.Vec2{X: 360, Y: 250}
	state.BeginFrame()
	state.ApplyMouseMove(otherPoint, mathx.Vec2{})
	state.ApplyMouseDown(otherPoint, input.MouseLeft)
	drawSampler()
	state.BeginFrame()
	state.ApplyMouseUp(otherPoint, input.MouseLeft)
	drawSampler()
	colored, _, err := model.Capture(frame, art.Albedo)
	if err != nil || model.History.Cursor() != 2 || !bytes.Contains(colored.Pix, []byte{127, 128, 200, 255}) {
		t.Fatalf("canvas did not preserve the selected color bytes: history=%d, error=%v", model.History.Cursor(), err)
	}
	state.BeginFrame()
	drawSampler()
	if !strings.Contains(panelView.canvasStatus, "RGBA8 127 128 200 255") {
		t.Fatalf("canvas did not show the pixel under the cursor: %q", panelView.canvasStatus)
	}
	if err := ed.UndoAsset(doc); err != nil {
		t.Fatal(err)
	}
	colored, _, err = model.Capture(frame, art.Albedo)
	if err != nil || !bytes.Equal(colored.Pix, painted.Pix) || model.History.Cursor() != 1 {
		t.Fatal("Undo did not restore the drawing after the colored stroke")
	}
	if disk, err := os.ReadFile(ed.Docs.Abs(doc.Path)); err != nil || !bytes.Equal(initial, disk) {
		t.Fatal("drawing unexpectedly modified the file before Save")
	}
	if err := ed.UndoAsset(doc); err != nil || doc.Dirty() {
		t.Fatalf("shared Undo did not restore saved document: %v", err)
	}
	undone, _, err := model.Capture(frame, art.Albedo)
	if err != nil || bytes.Equal(painted.Pix, undone.Pix) {
		t.Fatalf("Undo did not restore pixels: %v", err)
	}
	if err := ed.RedoAsset(doc); err != nil {
		t.Fatal(err)
	}
	redone, _, err := model.Capture(frame, art.Albedo)
	if err != nil || !bytes.Equal(painted.Pix, redone.Pix) {
		t.Fatalf("Redo did not restore painted pixels: %v", err)
	}
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "duplicate-frame", Linked: true}); err != nil {
		t.Fatal(err)
	}
	if len(model.Session.Document.Info().Frames) != 2 {
		t.Fatal("linked frame was not added to the native document")
	}
	if err := ed.SaveAsset(doc); err != nil || doc.Dirty() {
		t.Fatalf("Save failed or left the art document dirty: %v", err)
	}
	// This standalone panel fixture has no app-level save notification route.
	if batch, err := pl.Reimport(id); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("fixture reimport failed: %v", err)
	}
	frames := model.Session.Document.Info().Frames
	frameAsset := artsource.StableID(id, artsource.FrameName(frames[1].ID, art.Albedo))
	openedFrame, err := ed.OpenAsset(frameAsset)
	if err != nil || openedFrame != doc || model.Session.Frame != frames[1].ID {
		t.Fatalf("frame AssetRef did not open its owning art document at the selected frame: %v", err)
	}
	if _, err := ed.OpenAsset(id); err != nil || model.Session.Frame != frames[1].ID {
		t.Fatalf("opening the source reset the current frame: %v", err)
	}
	firstAsset := artsource.StableID(id, artsource.FrameName(frames[0].ID, art.Albedo))
	cursor := model.History.Cursor()
	if err := model.Session.BeginGesture("pencil", image.Pt(1, 1), art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := ed.OpenAsset(frameAsset); err != nil || !model.Session.Info().Gesture {
		t.Fatalf("refocusing the current frame disrupted its in-progress gesture: %v", err)
	}
	if _, err := ed.OpenAsset(firstAsset); err == nil || model.Session.Frame != frames[1].ID || !model.Session.Info().Gesture {
		t.Fatal("opening a different frame did not preserve the in-progress gesture")
	}
	model.Session.CancelGesture()
	if _, err := ed.OpenAsset(firstAsset); err != nil || model.Session.Frame != frames[0].ID || model.History.Cursor() != cursor {
		t.Fatalf("frame selection changed document history or failed after cancel: %v", err)
	}
	panel.(workspace.Closer).RequestClose()
	if ed.Docs.AssetDocument(id) != nil || ws.Has(panelID) {
		t.Fatal("closing a saved art document retained its view or model")
	}
	// Open the second frame from the real SpriteRenderer inspector. These input
	// events are simulated at the widget boundary, not an OS/GPU acceptance run.
	sc, _ := lightingFixture(t)
	sceneDoc := ed.Docs.New("scenes/art-inspector.aescene", sc)
	hero := sc.Create(sc.Root(), "Hero")
	sprite := scene.Add[render2d.SpriteRenderer](sc, hero)
	*sprite = render2d.DefaultSpriteRenderer()
	sprite.Sprite.SetAssetRefID(frameAsset)
	sc.Flush()
	beforeScene := sceneio.Save(sc)
	clickSpriteInspector(t, ed, sc, hero)
	reopened := ed.Docs.AssetDocument(id)
	if reopened == nil || reopened.Handler != artHandlerID {
		t.Fatal("inspector click did not open the art source")
	}
	if sceneDoc.History.Cursor() != 0 || !bytes.Equal(beforeScene, sceneio.Save(sc)) {
		t.Fatal("opening the sprite from Inspector changed the scene")
	}
	again := reopened.Binding.Model.(*artedit.Document)
	if again.Session.Frame != frames[1].ID {
		t.Fatal("inspector did not select the referenced animation frame")
	}
	got, _, err := again.Capture(frame, art.Albedo)
	if err != nil || !bytes.Equal(got.Pix, painted.Pix) || len(again.Session.Document.Info().Frames) != 2 || reopened.Dirty() {
		t.Fatalf("reopened source did not retain pixels and timeline: %v", err)
	}
	// Layer icons remain actions on the canonical model and shared history.
	// The read-only variant must not consume a click or mutate that model.
	artPanel := &artPanel{ctx: ed, doc: reopened, model: again, view: &artView{}}
	area := mathx.Rect{W: 240, H: 500}
	clickLayerControl := func(x float32, editable bool) {
		y := ed.UI.Theme.Padding + 6*(ed.UI.Theme.RowHeight+ed.UI.Theme.Spacing) + ed.UI.Theme.RowHeight/2
		point := mathx.Vec2{X: x, Y: y}
		for step := range 3 {
			state.BeginFrame()
			state.ApplyMouseMove(point, mathx.Vec2{})
			if step == 1 {
				state.ApplyMouseDown(point, input.MouseLeft)
			} else if step == 2 {
				state.ApplyMouseUp(point, input.MouseLeft)
			}
			ed.UI.Begin(state, mathx.Vec2{X: 900, Y: 700}, 1)
			artPanel.properties(ed.UI, area, editable)
			ed.UI.End()
		}
	}
	visibleX := area.W - ed.UI.Theme.Padding - 43
	lockX := area.W - ed.UI.Theme.Padding - 13
	clickLayerControl(visibleX, false)
	if !again.Session.Info().Document.Layers[0].Visible || again.History.Cursor() != 0 {
		t.Fatal("read-only layer visibility button changed the document")
	}
	clickLayerControl(visibleX, true)
	if again.Session.Info().Document.Layers[0].Visible || again.History.Cursor() != 1 {
		t.Fatal("layer visibility icon did not update shared history")
	}
	clickLayerControl(lockX, true)
	if !again.Session.Info().Document.Layers[0].Locked || again.History.Cursor() != 2 {
		t.Fatal("layer lock icon did not update shared history")
	}
	if err := ed.UndoAsset(reopened); err != nil {
		t.Fatal(err)
	}
	if err := ed.UndoAsset(reopened); err != nil {
		t.Fatal(err)
	}
	if layer := again.Session.Info().Document.Layers[0]; !layer.Visible || layer.Locked || reopened.Dirty() {
		t.Fatal("Undo did not restore the layer state and saved document")
	}
	if err := ed.Docs.CloseAsset(id, false); err != nil {
		t.Fatal(err)
	}
}

func clickSpriteInspector(t *testing.T, ed *context.Editor, sc *scene.Scene, node scene.NodeHandle) {
	t.Helper()
	inspectors := inspect.NewRegistry()
	inspectors.SetEditor(ed)
	if err := inspectors.Load(sc.Registry()); err != nil {
		t.Fatal(err)
	}
	inspectorExt := ext.New(ext.Deps{Editor: ed, Schema: sc.Registry(), Inspectors: inspectors})
	if err := inspectorExt.Run("render2d-sprite", spriteInspectorHooks); err != nil {
		t.Fatal(err)
	}
	td, ok := sc.Registry().ByName("engine.SpriteRenderer")
	if !ok {
		t.Fatal("sprite type was not registered")
	}
	custom, ok := inspectors.Custom(td.ID)
	if !ok {
		t.Fatal("sprite inspector was not registered")
	}
	nodes := []ids.NodeID{sc.ID(node)}
	defaults := 0
	state := &input.State{}
	drawInspector := func() {
		ed.UI.Begin(state, mathx.Vec2{X: 1000, Y: 500}, 1)
		ed.UI.Region(mathx.Rect{W: 1000, H: 500})
		inspectors.DrawCustom(custom, nodes, func() { defaults++ })
		if got, want := ed.UI.Cursor().Y, ed.UI.Theme.Padding+ed.UI.Theme.RowHeight+ed.UI.Theme.Spacing; got > want {
			t.Fatalf("sprite source action takes more than one row: %v > %v", got, want)
		}
		ed.UI.PopCursor()
		ed.UI.End()
	}
	point := mathx.Vec2{X: 200, Y: ed.UI.Theme.Padding + ed.UI.Theme.RowHeight/2}
	state.BeginFrame()
	drawInspector()
	state.BeginFrame()
	state.ApplyMouseMove(point, mathx.Vec2{})
	state.ApplyMouseDown(point, input.MouseLeft)
	drawInspector()
	state.BeginFrame()
	state.ApplyMouseUp(point, input.MouseLeft)
	drawInspector()
	if defaults != 3 {
		t.Fatal("sprite inspector did not preserve default component controls")
	}
}

func TestSpriteInspectorOpensPreparedImageWithoutChangingScene(t *testing.T) {
	f := imageAuthoringFixture(t)
	f.ed.UI = core.New(nil)
	sc, _ := lightingFixture(t)
	sceneDoc := f.ed.Docs.New("scenes/image-inspector.aescene", sc)
	hero := sc.Create(sc.Root(), "Hero")
	sprite := scene.Add[render2d.SpriteRenderer](sc, hero)
	*sprite = render2d.DefaultSpriteRenderer()
	sprite.Sprite.SetAssetRefID(f.frames[1])
	sc.Flush()
	beforeScene := sceneio.Save(sc)
	beforeImage, err := os.ReadFile(f.ed.Docs.Abs("assets/surface.png"))
	if err != nil {
		t.Fatal(err)
	}
	clickSpriteInspector(t, f.ed, sc, hero)
	doc := f.ed.Docs.AssetDocument(f.albedo)
	if doc == nil || doc.Handler != imageHandlerID || doc.Selected != f.frames[1] || doc.Capabilities.Has(document.EditSource) {
		t.Fatal("the prepared frame did not open in the existing read-only Image Viewer")
	}
	afterImage, err := os.ReadFile(f.ed.Docs.Abs("assets/surface.png"))
	if err != nil || !bytes.Equal(beforeImage, afterImage) || sceneDoc.History.Cursor() != 0 || !bytes.Equal(beforeScene, sceneio.Save(sc)) {
		t.Fatal("opening a prepared image changed the scene or source")
	}
}
