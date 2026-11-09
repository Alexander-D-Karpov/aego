package editor

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"testing"

	"aego/anim"
	"aego/art"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/editor/core/assetops"
	"aego/platform/input"
)

func TestArtTimelineGridSelectionTransferAndPivotUseSharedHistory(t *testing.T) {
	p := artActionFixture(t)
	s := p.model.Session
	base := s.Document.Info().Layers[0].ID
	other, err := p.model.Execute(stdctx.Background(), art.Operation{Kind: "add-layer", Name: "Effects", LayerKind: art.Raster})
	if err != nil {
		t.Fatal(err)
	}
	info := s.Document.Info()
	ui := p.ctx.UI
	state := &input.State{}
	draw := func() {
		ui.Begin(state, mathx.Vec2{X: 800, Y: 500}, 1)
		p.timelineGrid(ui, mathx.Rect{W: 700, H: 300}, true)
		ui.End()
	}
	state.BeginFrame()
	draw()
	click := func(x, y float32, shift bool) {
		state.BeginFrame()
		if shift {
			state.ApplyKeyDown(input.KeyLShift, 0, input.ModShift, false)
		}
		point := mathx.Vec2{X: x, Y: y}
		state.ApplyMouseMove(point, mathx.Vec2{})
		state.ApplyMouseDown(point, input.MouseLeft)
		draw()
		state.BeginFrame()
		state.ApplyMouseUp(point, input.MouseLeft)
		draw()
		if shift {
			state.BeginFrame()
			state.ApplyKeyUp(input.KeyLShift, 0, 0)
			draw()
		}
	}
	columns := max(1, int((700-150-ui.Theme.ScrollbarW)/48))
	width := (700 - 150 - ui.Theme.ScrollbarW) / float32(columns)
	row := ui.Theme.RowHeight + ui.Theme.Spacing
	before := p.model.History.Cursor()
	click(150+width/2, row+ui.Theme.RowHeight/2, false)
	click(150+width*1.5, row*2+ui.Theme.RowHeight/2, true)
	if s.Layer != other || s.Frame != info.Frames[1].ID || len(p.view.timelineCells) != 4 || p.model.History.Cursor() != before {
		t.Fatal("grid range selection did not preserve the canonical layer/frame and history", p.view.timelineCells)
	}
	p.askCelTransfer(true, false)
	questions := p.ctx.TakeQuestions()
	questions[0].Submit([]string{"2", "0", "no"})
	if p.model.History.Cursor() != before {
		t.Fatal("occupied cels were replaced without explicit confirmation")
	}
	p.askCelTransfer(true, true)
	questions = p.ctx.TakeQuestions()
	questions[0].Submit([]string{"2", "0", "yes"})
	if p.err != nil || p.model.History.Cursor() != before+1 {
		t.Fatal("linked range transfer did not use one Undo", p.err)
	}
	imageOf := func(frame art.ID) art.ID {
		for _, c := range s.Document.Info().Cels {
			if c.Layer == base && c.Frame == frame {
				return c.Image
			}
		}
		return ""
	}
	if imageOf(info.Frames[0].ID) != imageOf(info.Frames[2].ID) {
		t.Fatal("linked range did not share pixels")
	}
	if err := p.model.SessionUndo(); err != nil {
		t.Fatal(err)
	}
	if imageOf(info.Frames[0].ID) == imageOf(info.Frames[2].ID) {
		t.Fatal("Undo retained an unintended cel link")
	}
	p.askPivot()
	questions = p.ctx.TakeQuestions()
	questions[0].Submit([]string{"6", "7"})
	if point, ok := s.Document.Info().FramePivot(s.Frame); !ok || point != image.Pt(6, 7) {
		t.Fatal("pivot dialog did not call core slice operation", point, ok)
	}
}

func artActionFixture(t *testing.T) *artPanel {
	t.Helper()
	ed, _, _ := importTestEditor(t)
	d, err := art.New(8, 8, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	e, err := d.Begin(d.Revision(), "Eight export frames")
	if err != nil {
		t.Fatal(err)
	}
	info := d.Info()
	last := info.Frames[0].ID
	for i := 0; i < 8; i++ {
		if i != 0 {
			last, err = e.AddFrame(last, 80+i*10)
			if err != nil {
				t.Fatal(err)
			}
		}
		cel, err := e.EnsureCel(info.Layers[0].ID, last, art.Albedo)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.SetPixel(cel, 1, 1, art.Paint{Color: art.Color{R: uint8(24 + i*30), G: 100, A: 255}}, nil, false); err != nil {
			t.Fatal(err)
		}
		if _, err := e.SetSlice(art.Slice{Name: "pivot", Frame: last, Rect: image.Rect(0, 0, 8, 8), Pivot: image.Pt(2, 7)}); err != nil {
			t.Fatal(err)
		}
	}
	change, err := e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if err := change.Apply(); err != nil {
		t.Fatal(err)
	}
	data, err := d.Encode()
	d.Close()
	if err != nil {
		t.Fatal(err)
	}
	id, err := assetops.CreateFile(ed, "assets/actions.aeart", data)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ed.OpenAsset(id)
	if err != nil {
		t.Fatal(err)
	}
	model := doc.Binding.Model.(*artedit.Document)
	_, err = model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 0, Y: 0}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{G: 255, A: 255}}}})
	if err != nil {
		t.Fatal(err)
	}
	return &artPanel{ctx: ed, doc: doc, model: model, view: &artView{size: 1}}
}

func TestArtActionDialogsGuardRevisionAndUseSharedUndo(t *testing.T) {
	p := artActionFixture(t)
	layer := p.model.Session.Document.Info().Layers[0]
	before := p.model.History.Cursor()
	p.askLayerProperties(layer)
	questions := p.ctx.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatal("layer settings did not open the native prompt")
	}
	questions[0].Submit([]string{"Foreground", "128", "multiply", ""})
	got := p.model.Session.Document.Info().Layers[0]
	if got.Name != "Foreground" || got.Opacity != 128 || got.Blend != art.BlendMultiply || p.model.History.Cursor() != before+1 {
		t.Fatal("layer prompt bypassed the canonical operation", got)
	}
	if err := p.model.SessionUndo(); err != nil {
		t.Fatal(err)
	}
	if got := p.model.Session.Document.Info().Layers[0]; got != layer {
		t.Fatal("one Undo did not restore all layer properties")
	}
	p.askLayerProperties(layer)
	questions = p.ctx.TakeQuestions()
	p.execute(art.Operation{Kind: "duration", DurationMS: 170})
	cursor := p.model.History.Cursor()
	questions[0].Submit([]string{"Stale", "80", "screen", ""})
	if p.model.Session.Document.Info().Layers[0] != layer || p.model.History.Cursor() != cursor {
		t.Fatal("stale native dialog overwrote a newer document")
	}
}

func TestArtNativeExportKeepsDirtyFramesPivotAndRuntimeReferences(t *testing.T) {
	p := artActionFixture(t)
	model := p.model
	before := model.PreviewRevision()
	want, err := model.Session.Document.PNG(model.Session.Frame, art.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct{ path, format string }{{"assets/exported.png", "png"}, {"assets/sheet.png", "sheet"}, {"assets/independent.aeart", "aeart"}, {"assets/palette.json", "palette"}} {
		if err := p.exportTo(v.path, v.format); err != nil {
			t.Fatalf("%s: %v", v.format, err)
		}
	}
	pngBytes, err := p.ctx.Pipeline.SourceBytes("assets/exported.png")
	if err != nil || !bytes.Equal(pngBytes, want) {
		t.Fatal("PNG did not export the unsaved current frame", err)
	}
	if !p.doc.Dirty() || !model.PreviewCurrent(before) {
		t.Fatal("export saved or replaced the original dirty document")
	}
	copyBytes, err := p.ctx.Pipeline.SourceBytes("assets/independent.aeart")
	if err != nil {
		t.Fatal(err)
	}
	copyDoc, err := art.Decode(copyBytes, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer copyDoc.Close()
	if len(copyDoc.Info().Frames) != 8 || len(copyDoc.Info().Slices) != 8 {
		t.Fatal("editable copy lost frames or pivots")
	}
	copyPixels, err := copyDoc.PNG(copyDoc.Info().Frames[0].ID, art.Albedo)
	if err != nil || !bytes.Equal(copyPixels, want) {
		t.Fatal("editable copy did not contain dirty pixels")
	}
	metadata, err := p.ctx.Pipeline.SourceBytes("assets/sheet.frames.json")
	if err != nil {
		t.Fatal(err)
	}
	var sheet art.Sheet
	if err := json.Unmarshal(metadata, &sheet); err != nil {
		t.Fatal(err)
	}
	if len(sheet.Frames) != 8 || len(sheet.Slices) != 8 {
		t.Fatal("sheet export lost animation metadata")
	}
	sheetBytes, err := p.ctx.Pipeline.SourceBytes("assets/sheet.png")
	if err != nil {
		t.Fatal(err)
	}
	pixels, err := png.Decode(bytes.NewReader(sheetBytes))
	if err != nil {
		t.Fatal(err)
	}
	if pixels.Bounds() != image.Rect(0, 0, 64, 8) {
		t.Fatal("sprite sheet is not all eight frames")
	}
	sheetID, ok := p.ctx.Pipeline.Lookup("assets/sheet.png")
	if !ok {
		t.Fatal("sheet was not imported")
	}
	meta, ok := p.ctx.Pipeline.MetaOf(sheetID)
	if !ok || len(meta.Subs) != 8 {
		t.Fatal("sheet source metadata is incomplete")
	}
	clipID, ok := p.ctx.Pipeline.Lookup("assets/sheet.aeanim")
	if !ok {
		t.Fatal("animation source was not imported")
	}
	artifact, ok := p.ctx.Pipeline.PrimaryArtifact(clipID)
	if !ok {
		t.Fatal("animation has no runtime artifact")
	}
	data, err := p.ctx.Pipeline.Artifacts().ReadFile(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	clip, err := anim.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(clip.Tracks) != 1 || len(clip.Tracks[0].Keys) != 8 {
		t.Fatal("runtime clip does not contain eight keys")
	}
	for i, sub := range meta.Subs {
		if !sub.HasPivot || sub.Pivot != ([2]float32{.25, .875}) {
			t.Fatal("export lost per-frame pivot")
		}
		if err := p.ctx.Pipeline.VerifyCurrent(sub.ID); err != nil {
			t.Fatal(err)
		}
		if clip.Tracks[0].Keys[i].V.A != sub.ID {
			t.Fatal("exported animation refers to another source's frame")
		}
	}
	if err := p.exportTo("assets/sheet.png", "sheet"); err == nil {
		t.Fatal("export overwrote an existing bundle")
	}
	after, err := p.ctx.Pipeline.SourceBytes("assets/sheet.png")
	if err != nil || !bytes.Equal(after, sheetBytes) {
		t.Fatal("rejected export changed existing pixels")
	}
	if _, err := os.Stat(p.ctx.Docs.Abs("assets/sheet.aeanim.meta")); err != nil {
		t.Fatal(err)
	}
}

func TestArtTagDialogUsesCorePlaybackAndOneUndo(t *testing.T) {
	p := artActionFixture(t)
	cursor := p.model.History.Cursor()
	p.askTagProperties(art.Tag{Name: "Run", Direction: art.Forward})
	questions := p.ctx.TakeQuestions()
	if len(questions) != 1 {
		t.Fatal("tag prompt absent")
	}
	questions[0].Submit([]string{"Run", "2", "4", "ping-pong"})
	info := p.model.Session.Document.Info()
	if len(info.Tags) != 1 || info.Tags[0].Direction != art.PingPong || p.model.History.Cursor() != cursor+1 {
		t.Fatal("tag dialog did not use common history")
	}
	snapshot, err := p.model.Session.Document.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	order, err := snapshot.PlaybackFrames(p.view.tag)
	if err != nil {
		t.Fatal(err)
	}
	want := []art.ID{info.Frames[1].ID, info.Frames[2].ID, info.Frames[3].ID, info.Frames[2].ID}
	if len(order) != len(want) {
		t.Fatal("wrong native tag playback length")
	}
	for i, f := range order {
		if f.ID != want[i] {
			t.Fatal("native tag did not use shared ping-pong semantics")
		}
	}
	if err := p.model.SessionUndo(); err != nil || len(p.model.Session.Document.Info().Tags) != 0 {
		t.Fatal("tag Undo failed", err)
	}
}

func TestArtAnimationClipboardAcrossDocumentsUsesOneNativeHistoryStep(t *testing.T) {
	source, dest := artActionFixture(t), artActionFixture(t)
	frames := source.model.Session.Document.Info().Frames
	ids := make([]art.ID, len(frames))
	for i, f := range frames {
		ids[i] = f.ID
	}
	clipboard, err := source.model.Session.Document.CopyAnimation(ids)
	if err != nil {
		t.Fatal(err)
	}
	// The snapshot remains valid after its source editor/document closes.
	source.model.Close()
	cursor := dest.model.History.Cursor()
	if err := dest.model.PasteAnimation(stdctx.Background(), clipboard); err != nil {
		t.Fatal(err)
	}
	info := dest.model.Session.Document.Info()
	if len(info.Frames) != 16 || len(info.Layers) != 2 || dest.model.History.Cursor() != cursor+1 {
		t.Fatal("animation paste did not use one native history step")
	}
	if err := dest.model.SessionUndo(); err != nil {
		t.Fatal(err)
	}
	if len(dest.model.Session.Document.Info().Frames) != 8 || len(dest.model.Session.Document.Info().Layers) != 1 {
		t.Fatal("Undo did not remove the complete animation paste")
	}
	if err := dest.model.SessionRedo(); err != nil {
		t.Fatal(err)
	}
	if len(dest.model.Session.Document.Info().Frames) != 16 {
		t.Fatal("Redo did not restore the copied animation")
	}
}
