package inspector

import (
	"testing"

	"aego/core/curve"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/editor/ext/inspect"
	"aego/engine/module"
	"aego/platform/input"
	"aego/render/text"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/theme"
	"aego/ui/widgets"
)

type curveHistoryComp struct {
	Curve curve.Curve `aego:"curve"`
}

type curveHistoryModule struct{}

func (curveHistoryModule) Info() module.Info { return module.Info{ID: "curvehistory"} }
func (curveHistoryModule) Register(r *module.Registry) error {
	return module.Component[curveHistoryComp](r, schema.Name("test.CurveHistory")).Err()
}

func TestCurvePresetDragThroughInspectorUndoRedo(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Load(types, nil, curveHistoryModule{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	reg := inspect.NewRegistry()
	if err := reg.Load(types.Schema()); err != nil {
		t.Fatal(err)
	}
	sc := scene.New(types, nil, scene.Options{})
	node := sc.Create(sc.Root(), "Curve")
	original := curve.FromEasing(curve.OutQuad)
	original.Pre, original.Post = curve.WrapLoop, curve.WrapPingPong
	scene.Add[curveHistoryComp](sc, node).Curve = original.Clone()
	sc.Flush()
	mgr := document.NewManager(document.Options{Root: t.TempDir(), Types: types})
	doc := mgr.New("scenes/curve.aescene", sc)
	ed := &context.Editor{Docs: mgr}
	reg.SetEditor(ed)
	ui := core.New(text.NewFont(14, 1, 18, 14))
	ui.Theme = theme.Editor()
	p := New(reg, Deps{Docs: ed, Notices: ed})
	p.f, p.nodes = &services.Frame{UI: ui}, []ids.NodeID{sc.ID(node)}
	td, _ := types.Schema().ByName("test.CurveHistory")
	fd, _ := td.Field("curve")
	in := &input.State{}
	region := mathx.Rect{X: 10, Y: 10, W: 500, H: 500}
	frame := func(at mathx.Vec2, event string) {
		in.BeginFrame()
		in.ApplyMouseMove(at, mathx.Vec2{})
		switch event {
		case "down":
			in.ApplyMouseDown(at, input.MouseLeft)
		case "up":
			in.ApplyMouseUp(at, input.MouseLeft)
		}
		ui.Begin(in, mathx.V2(800, 600), 1)
		ui.Region(region)
		p.field(doc, td, fd)
		ui.PopCursor()
		ui.End()
	}
	pad, row, space := ui.Theme.Padding, ui.Theme.RowHeight, ui.Theme.Spacing
	fieldX := region.X + pad + widgets.LabelWidth(region.W-2*pad)
	fieldY := region.Y + pad
	field := mathx.V2(fieldX+30, fieldY+row/2)
	frame(field, "down")
	frame(field, "up")
	frame(field, "") // popup measures before it accepts interaction
	graphX, graphY := fieldX+pad, fieldY+row+pad+row+space
	plotW, plotH := float32(380)-2*pad-2*widgets.CurvePad, float32(180-2*widgets.CurvePad)
	at := mathx.V2(graphX+widgets.CurvePad+plotW*0.5, graphY+widgets.CurvePad+plotH*(1-(0.75+0.1)/1.2))
	before := doc.History.Cursor()
	frame(at, "down")
	end := at.Add(mathx.V2(8, -24))
	frame(end, "")
	frame(end, "up")
	got, _ := scene.Get[curveHistoryComp](sc, node)
	changed := got.Curve.Clone()
	if changed.Ease != curve.None || doc.History.Cursor() != before+1 {
		t.Fatalf("drag did not commit exactly one preset conversion: curve=%+v history=%d", changed, doc.History.Cursor()-before)
	}
	if err := doc.History.Undo(); err != nil || !got.Curve.Equal(original) {
		t.Fatalf("undo did not restore the preset: %v %+v", err, got.Curve)
	}
	if err := doc.History.Redo(); err != nil || !got.Curve.Equal(changed) {
		t.Fatalf("redo did not restore the edited curve: %v %+v", err, got.Curve)
	}
}
