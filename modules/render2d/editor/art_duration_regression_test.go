package editor

import (
	stdctx "context"
	"fmt"
	"image"
	"os"
	"testing"

	"aego/art"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/platform/input"
	"aego/project"
	"aego/ui/core"
)

func artDurationFixture(t *testing.T) (*artPanel, *input.State, func(), mathx.Rect) {
	t.Helper()
	project, err := project.Create(t.TempDir(), "Timeline duration", false)
	if err != nil {
		t.Fatal(err)
	}
	importers := pipeline.NewImporterRegistry()
	if err := importers.Register(artsource.New()); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(project, importers, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	commands := command.New()
	ed := &context.Editor{Pipeline: pl, Docs: document.NewManager(document.Options{Root: project.Root}), Commands: commands, UI: core.New(nil)}
	ws := workspace.New(project.Root, nil)
	registry := ext.New(ext.Deps{Editor: ed, Workspace: ws, Commands: commands})
	if err := registry.Run("render2d", imageHooks); err != nil {
		t.Fatal(err)
	}
	if err := commands.Run("asset.new_art"); err != nil {
		t.Fatal(err)
	}
	questions := ed.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatal("missing Create Art dialog")
	}
	questions[0].Submit([]string{"assets/duration.aeart", "16", "16", "rgba"})
	id, ok := pl.Lookup("assets/duration.aeart")
	if !ok {
		t.Fatal("created art was not imported")
	}
	doc := ed.Docs.AssetDocument(id)
	if doc == nil {
		t.Fatal("created art was not opened")
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok {
		t.Fatal("missing art model")
	}
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "duration", DurationMS: 250}); err != nil {
		t.Fatal(err)
	}
	view := &artView{}
	panel := &artPanel{ctx: ed, reg: registry, doc: doc, model: model, view: view}
	state := &input.State{}
	area := mathx.Rect{W: 480, H: 240}
	draw := func() {
		ed.UI.Begin(state, mathx.Vec2{X: 480, Y: 240}, 1)
		panel.timeline(ed.UI, area, true)
		ed.UI.End()
	}
	return panel, state, draw, area
}

// The toolbar is deliberately compact, while the frame strip below uses the
// full region width. Calculate pointer targets for the actual toolbar bounds.
func artDurationToolbar(ui *core.Context, area mathx.Rect) mathx.Rect {
	pad := ui.Theme.Padding
	return mathx.Rect{X: area.X + pad, Y: area.Y + pad, W: min(area.W-2*pad, 400), H: ui.Theme.RowHeight}
}

func TestArtTimelineArrowsSelectFramesWithoutChangingHistory(t *testing.T) {
	panel, state, draw, area := artDurationFixture(t)
	model, session := panel.model, panel.model.Session
	for i := 0; i < 8; i++ {
		if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "add-frame", DurationMS: 100 + i}); err != nil {
			t.Fatal(err)
		}
	}
	frames := session.Document.Info().Frames
	if err := session.Select("", frames[0].ID, ""); err != nil {
		t.Fatal(err)
	}
	state.BeginFrame()
	draw()
	toolbar := artDurationToolbar(panel.ctx.UI, area)
	click := func(column int) {
		t.Helper()
		point := artCell(toolbar, column, 4).Center()
		state.BeginFrame()
		state.ApplyMouseMove(point, mathx.Vec2{})
		state.ApplyMouseDown(point, input.MouseLeft)
		draw()
		state.BeginFrame()
		state.ApplyMouseUp(point, input.MouseLeft)
		draw()
	}
	cursor := model.History.Cursor()
	panel.view.playing = true
	for index := 1; index < len(frames); index++ {
		click(3)
		if panel.err != nil || session.Frame != frames[index].ID || panel.view.playing || model.History.Cursor() != cursor {
			t.Fatalf("next frame %d: selected %s, error %v", index, session.Frame, panel.err)
		}
		stripColumns := max(1, int((area.W-2*panel.ctx.UI.Theme.Padding)/68))
		if panel.view.framePage != index/stripColumns || panel.view.timelineColumns < 1 || panel.view.timelinePage != index/panel.view.timelineColumns {
			t.Fatalf("frame %d is not shown by both timeline views", index)
		}
	}
	click(3)
	if session.Frame != frames[len(frames)-1].ID || model.History.Cursor() != cursor {
		t.Fatal("next at the last frame changed the document")
	}
	click(2)
	if session.Frame != frames[len(frames)-2].ID || model.History.Cursor() != cursor {
		t.Fatal("previous did not select the previous frame")
	}
	if err := session.BeginGesture("pencil", image.Pt(0, 0), art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{A: 255}}}, false); err != nil {
		t.Fatal(err)
	}
	selected := session.Frame
	panel.stepArtFrame(-1)
	if panel.err == nil || session.Frame != selected || !session.Info().Gesture || model.History.Cursor() != cursor {
		t.Fatal("frame navigation changed or cancelled an active stroke")
	}
	session.CancelGesture()
}

func TestArtTimelineDurationBlurBeforeNextFrame(t *testing.T) {
	panel, state, draw, area := artDurationFixture(t)
	model, session := panel.model, panel.model.Session
	first := session.Frame
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "add-frame", DurationMS: 100}); err != nil {
		t.Fatal(err)
	}
	if err := session.Select("", first, ""); err != nil {
		t.Fatal(err)
	}
	state.BeginFrame()
	draw()
	pad, row, gap := panel.ctx.UI.Theme.Padding, panel.ctx.UI.Theme.RowHeight, panel.ctx.UI.Theme.Spacing
	click := func(point mathx.Vec2) {
		t.Helper()
		state.BeginFrame()
		state.ApplyMouseMove(point, mathx.Vec2{})
		state.ApplyMouseDown(point, input.MouseLeft)
		draw()
		state.BeginFrame()
		state.ApplyMouseUp(point, input.MouseLeft)
		draw()
	}
	click(mathx.Vec2{X: pad + (area.W-2*pad)/6, Y: pad + 3*(row+gap) + row/2})
	state.BeginFrame()
	state.ApplyText([]byte("275"))
	draw()
	toolbar := artDurationToolbar(panel.ctx.UI, area)
	click(artCell(toolbar, 3, 4).Center())
	frames := session.Document.Info().Frames
	if panel.err != nil || session.Frame != frames[1].ID || frames[0].DurationMS != 275 || frames[1].DurationMS != 100 {
		t.Fatalf("next frame lost or misapplied pending duration: %+v, %v", frames, panel.err)
	}
}

// Simulates pointer input at the native widget boundary. It does not claim OS
// input or hardware GPU coverage; the integrated SDL test is separate.
func TestArtTimelineNewFrameDurationFollowsSelection(t *testing.T) {
	panel, state, draw, area := artDurationFixture(t)
	ed, model, view := panel.ctx, panel.model, panel.view
	state.BeginFrame()
	draw()
	if view.duration != "250" {
		t.Fatalf("first frame duration field = %q, want 250", view.duration)
	}
	oldFrame := model.Session.Frame
	pad, row, gap := ed.UI.Theme.Padding, ed.UI.Theme.RowHeight, ed.UI.Theme.Spacing
	add := mathx.Vec2{X: pad + (area.W-2*pad)/8, Y: pad + 2*(row+gap) + row/2}
	state.BeginFrame()
	state.ApplyMouseMove(add, mathx.Vec2{})
	state.ApplyMouseDown(add, input.MouseLeft)
	draw()
	state.BeginFrame()
	state.ApplyMouseUp(add, input.MouseLeft)
	draw()
	info := model.Session.Info()
	if panel.err != nil || len(info.Document.Frames) != 2 || info.Frame == oldFrame {
		t.Fatalf("Add Frame did not select the new frame: frames=%d error=%v", len(info.Document.Frames), panel.err)
	}
	if got := info.Document.Frames[1].DurationMS; got != 100 {
		t.Fatalf("new frame duration = %d, want 100", got)
	}
	state.BeginFrame()
	draw()
	if view.duration != "100" {
		t.Fatalf("new frame duration field = %q, want 100", view.duration)
	}
}

func TestArtTimelineDurationBlurStaysOnOriginalFrame(t *testing.T) {
	panel, state, draw, area := artDurationFixture(t)
	ed, model := panel.ctx, panel.model
	state.BeginFrame()
	draw()
	pad, row, gap := ed.UI.Theme.Padding, ed.UI.Theme.RowHeight, ed.UI.Theme.Spacing
	field := mathx.Vec2{X: pad + (area.W-2*pad)/6, Y: pad + 3*(row+gap) + row/2}
	click := func(at mathx.Vec2) {
		t.Helper()
		state.BeginFrame()
		state.ApplyMouseMove(at, mathx.Vec2{})
		state.ApplyMouseDown(at, input.MouseLeft)
		draw()
		state.BeginFrame()
		state.ApplyMouseUp(at, input.MouseLeft)
		draw()
	}
	click(field)
	if !ed.UI.WantsKeyboard() {
		t.Fatal("duration field did not take focus")
	}
	state.BeginFrame()
	state.ApplyText([]byte("275"))
	draw()
	add := mathx.Vec2{X: pad + (area.W-2*pad)/8, Y: pad + 2*(row+gap) + row/2}
	click(add)
	frames := model.Session.Info().Document.Frames
	if len(frames) != 2 {
		t.Fatalf("Add Frame created %d frames, want 2; panel error %v", len(frames), panel.err)
	}
	if frames[0].DurationMS != 275 || frames[1].DurationMS != 100 {
		t.Fatalf("duration edit followed selection: frame durations = %d, %d; want 275, 100", frames[0].DurationMS, frames[1].DurationMS)
	}
	state.BeginFrame()
	draw()
	if panel.view.duration != "100" {
		t.Fatalf("selected new frame displays %q, want 100", panel.view.duration)
	}
	if err := ed.UndoAsset(panel.doc); err != nil {
		t.Fatal(err)
	}
	state.BeginFrame()
	draw()
	if len(model.Session.Info().Document.Frames) != 1 || panel.view.duration != "275" {
		t.Fatalf("Undo frame creation did not restore selection and duration: %q", panel.view.duration)
	}
	if err := ed.UndoAsset(panel.doc); err != nil {
		t.Fatal(err)
	}
	state.BeginFrame()
	draw()
	if panel.view.duration != "250" {
		t.Fatalf("Undo duration displays %q, want 250", panel.view.duration)
	}
	if err := ed.RedoAsset(panel.doc); err != nil {
		t.Fatal(err)
	}
	state.BeginFrame()
	draw()
	if panel.view.duration != "275" {
		t.Fatalf("Redo duration displays %q, want 275", panel.view.duration)
	}
	if err := ed.RedoAsset(panel.doc); err != nil {
		t.Fatal(err)
	}
	state.BeginFrame()
	draw()
	frames = model.Session.Info().Document.Frames
	if len(frames) != 2 || frames[0].DurationMS != 275 || frames[1].DurationMS != 100 {
		t.Fatalf("Redo did not restore both frame durations: %+v", frames)
	}
	play := mathx.Vec2{X: pad + (area.W-2*pad)/8, Y: pad + row/2}
	click(play)
	if !panel.view.playing {
		t.Fatal("timeline did not enter Play")
	}
	click(play)
	if panel.view.playing {
		t.Fatal("timeline did not Pause")
	}
	if err := ed.SaveAsset(panel.doc); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ed.Docs.Abs(panel.doc.Path))
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := art.Decode(data, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	frames = reopened.Info().Frames
	if len(frames) != 2 || frames[0].DurationMS != 275 || frames[1].DurationMS != 100 {
		t.Fatalf("saved and decoded durations = %+v, want 275 and 100", frames)
	}
}

// Exercises pointer targets at multiple dock widths without requiring a GPU or
// real OS input. A click in the unused area must not navigate or edit history.
func TestArtTimelineCompactToolbarPointerTargets(t *testing.T) {
	for _, width := range []float32{300, 480, 1000} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			panel, state, _, area := artDurationFixture(t)
			area.W = width
			session, model, ui := panel.model.Session, panel.model, panel.ctx.UI
			first := session.Frame
			if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "add-frame", DurationMS: 100}); err != nil {
				t.Fatal(err)
			}
			frames := session.Document.Info().Frames
			if err := session.Select("", first, ""); err != nil {
				t.Fatal(err)
			}
			cursor := model.History.Cursor()
			draw := func() {
				ui.Begin(state, mathx.Vec2{X: width, Y: area.H}, 1)
				panel.timeline(ui, area, true)
				ui.End()
			}
			click := func(point mathx.Vec2) {
				state.BeginFrame()
				state.ApplyMouseMove(point, mathx.Vec2{})
				state.ApplyMouseDown(point, input.MouseLeft)
				draw()
				state.BeginFrame()
				state.ApplyMouseUp(point, input.MouseLeft)
				draw()
			}
			state.BeginFrame()
			draw()
			toolbar := artDurationToolbar(ui, area)
			click(artCell(toolbar, 3, 4).Center())
			if session.Frame != frames[1].ID || model.History.Cursor() != cursor || panel.err != nil {
				t.Fatalf("next at width %g: frame=%s history=%d error=%v", width, session.Frame, model.History.Cursor(), panel.err)
			}
			click(artCell(toolbar, 2, 4).Center())
			if session.Frame != first || model.History.Cursor() != cursor || panel.err != nil {
				t.Fatalf("previous at width %g: frame=%s history=%d error=%v", width, session.Frame, model.History.Cursor(), panel.err)
			}
			if area.W-2*ui.Theme.Padding > 400 {
				click(mathx.Vec2{X: area.W - ui.Theme.Padding - 2, Y: toolbar.Center().Y})
				if session.Frame != first || model.History.Cursor() != cursor || panel.err != nil {
					t.Fatalf("empty toolbar area changed frame or history at width %g", width)
				}
			}
		})
	}
}
