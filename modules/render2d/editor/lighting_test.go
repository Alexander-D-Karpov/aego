package editor

import (
	"bytes"
	"slices"
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/gizmo"
	"aego/editor/core/history"
	"aego/editor/core/shape"
	"aego/editor/ext"
	"aego/editor/ext/inspect"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/modules/render2d"
	"aego/render/lighting"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
)

type lightingTarget struct{ s *scene.Scene }

func (t lightingTarget) Scene() *scene.Scene { return t.s }

func lightingFixture(t *testing.T) (*scene.Scene, *context.Editor) {
	t.Helper()
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.Load(types, nil, render2d.New()); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	docs := document.NewManager(document.Options{Root: t.TempDir(), Types: types})
	docs.New("lighting.aescene", s)
	return s, &context.Editor{Docs: docs, Types: types, Commands: command.New()}
}
func lightIn(s *scene.Scene) (scene.NodeHandle, *render2d.Light2D) {
	h := s.Create(s.Root(), "light")
	l := scene.Add[render2d.Light2D](s, h)
	*l = render2d.DefaultLight2D()
	s.Flush()
	l, _ = scene.Get[render2d.Light2D](s, h)
	return h, l
}
func beginLightingDrag(s *scene.Scene, h scene.NodeHandle, g gizmo.Gizmo, part gizmo.Part) (*gizmo.EditCtx, *history.History) {
	hist := history.New(lightingTarget{s})
	id := s.ID(h)
	e := gizmo.NewEdit(s, []ids.NodeID{id}, id, mathx.Vec2{}, gizmo.Snap{})
	e.Tx = hist.Begin("Lighting gesture")
	g.Begin(e, gizmo.Hit{Part: part})
	return e, hist
}
func finishLightingDrag(t *testing.T, e *gizmo.EditCtx, g gizmo.Gizmo) {
	t.Helper()
	g.End(e)
	gizmo.Commit(e)
	if e.Err != nil {
		e.Tx.Abort()
		t.Fatal(e.Err)
	}
	e.Tx.Commit()
}

func TestLightRangeDragHasOneUndoAndWorldUnits(t *testing.T) {
	s, _ := lightingFixture(t)
	h, l := lightIn(s)
	initial := l.Range
	s.SetLocal(h, scene.Transform2D{Position: mathx.Vec2{X: 2, Y: 3}, Scale: mathx.Vec2{X: 4, Y: .5}, Rotation: .3})
	s.UpdateTransforms()
	td, _ := s.Registry().ByName("engine.Light2D")
	g := &lightGizmo{td: td}
	e, hist := beginLightingDrag(s, h, g, partLightRange)
	for _, r := range []float32{6, 7, 8} {
		e.Mouse = mathx.Vec2{X: 2 + r, Y: 3}
		g.Update(e)
	}
	finishLightingDrag(t, e, g)
	l, _ = scene.Get[render2d.Light2D](s, h)
	if l.Range != 8 || hist.Cursor() != 1 {
		t.Fatalf("range=%v history=%d", l.Range, hist.Cursor())
	}
	if err := hist.Undo(); err != nil {
		t.Fatal(err)
	}
	l, _ = scene.Get[render2d.Light2D](s, h)
	if l.Range != initial {
		t.Fatal("range Undo failed")
	}
	if err := hist.Redo(); err != nil {
		t.Fatal(err)
	}
	l, _ = scene.Get[render2d.Light2D](s, h)
	if l.Range != 8 {
		t.Fatal("range Redo failed")
	}
}

func TestSpotConeDragClampsAndUndo(t *testing.T) {
	s, _ := lightingFixture(t)
	h, l := lightIn(s)
	l.Type = lighting.Spot
	initial := l.InnerAngle
	td, _ := s.Registry().ByName("engine.Light2D")
	g := &lightGizmo{td: td}
	e, hist := beginLightingDrag(s, h, g, partLightInner)
	e.Mouse = mathx.Vec2{Y: 4}
	g.Update(e)
	finishLightingDrag(t, e, g)
	l, _ = scene.Get[render2d.Light2D](s, h)
	if l.InnerAngle != l.OuterAngle {
		t.Fatalf("inner exceeds outer: %+v", l)
	}
	if err := hist.Undo(); err != nil {
		t.Fatal(err)
	}
	l, _ = scene.Get[render2d.Light2D](s, h)
	if l.InnerAngle != initial {
		t.Fatal("cone undo")
	}
}

func TestSpotHandlesUseFullConeAngles(t *testing.T) {
	l := render2d.DefaultLight2D()
	l.Type, l.InnerAngle, l.OuterAngle = lighting.Spot, mathx.Pi/3, mathx.Pi/2
	handles, n := lightHandles(&l, mathx.Vec2{}, mathx.Vec2{X: 1}, .01)
	for _, handle := range handles[:n] {
		angle := mathx.Atan2(handle.at.Y, handle.at.X)
		switch handle.part {
		case partLightInner:
			if mathx.Abs(angle+l.InnerAngle/2) > 1e-5 {
				t.Fatal("inner handle does not match shading cone")
			}
		case partLightOuter:
			if mathx.Abs(angle-l.OuterAngle/2) > 1e-5 {
				t.Fatal("outer handle does not match shading cone")
			}
		}
	}
}

func TestLightDirectionUnderMirroredNonuniformParent(t *testing.T) {
	s, _ := lightingFixture(t)
	parent := s.Create(s.Root(), "parent")
	s.SetLocal(parent, scene.Transform2D{Rotation: .2, Scale: mathx.Vec2{X: 2, Y: -.7}})
	h := s.Create(parent, "spot")
	l := scene.Add[render2d.Light2D](s, h)
	*l = render2d.DefaultLight2D()
	l.Type = lighting.Spot
	initial := scene.Transform2D{Position: mathx.Vec2{X: 1, Y: 2}, Rotation: -.3, Scale: mathx.Vec2{X: -1, Y: 2}}
	s.SetLocal(h, initial)
	s.Flush()
	s.UpdateTransforms()
	td, _ := s.Registry().ByName("engine.Light2D")
	g := &lightGizmo{td: td}
	e, hist := beginLightingDrag(s, h, g, partLightDirection)
	at, _ := lightPose(s, h)
	e.Mouse = at.Add(mathx.Vec2{X: 1, Y: 2})
	g.Update(e)
	finishLightingDrag(t, e, g)
	_, dir := lightPose(s, h)
	want := mathx.Vec2{X: 1, Y: 2}
	want = want.Scale(1 / want.Len())
	if dir.Dot(want) < .9999 {
		t.Fatalf("local +X points incorrectly after parent inverse: %v", dir)
	}
	if err := hist.Undo(); err != nil {
		t.Fatal(err)
	}
	if s.Local(h) != initial {
		t.Fatal("direction Undo did not restore transform")
	}
}

func TestPolygonInsertMoveUndoAndCancel(t *testing.T) {
	s, ed := lightingFixture(t)
	h := s.Create(s.Root(), "occluder")
	o := scene.Add[render2d.LightOccluder2D](s, h)
	*o = render2d.DefaultLightOccluder2D()
	initial := slices.Clone(o.Polygon)
	s.Flush()
	td, _ := s.Registry().ByName("engine.LightOccluder2D")
	g := &occluderGizmo{td: td, p: &lightingEditor{ed: ed}, hoverIndex: 0}
	e, hist := beginLightingDrag(s, h, g, partOccluderInsert)
	e.Mouse = mathx.Vec2{Y: -1}
	g.Update(e)
	e.Mouse.Y = -1.5
	g.Update(e)
	finishLightingDrag(t, e, g)
	o, _ = scene.Get[render2d.LightOccluder2D](s, h)
	if len(o.Polygon) != 5 || o.Polygon[1] != (mathx.Vec2{Y: -1.5}) || hist.Cursor() != 1 {
		t.Fatalf("insert/move %+v undo=%d", o.Polygon, hist.Cursor())
	}
	if err := hist.Undo(); err != nil {
		t.Fatal(err)
	}
	o, _ = scene.Get[render2d.LightOccluder2D](s, h)
	if !slices.Equal(o.Polygon, initial) {
		t.Fatal("polygon undo aliases mutable storage")
	}
	g.hoverIndex = 0
	e, hist = beginLightingDrag(s, h, g, partOccluderVertex)
	e.Mouse = mathx.Vec2{X: -2, Y: -2}
	g.Update(e)
	g.End(e)
	gizmo.Cancel(e)
	e.Tx.Abort()
	o, _ = scene.Get[render2d.LightOccluder2D](s, h)
	if !slices.Equal(o.Polygon, initial) || hist.Cursor() != 0 {
		t.Fatal("cancel polluted document/history")
	}
}

func TestPolygonAll256VerticesRemainEditable(t *testing.T) {
	s, ed := lightingFixture(t)
	h := s.Create(s.Root(), "large contour")
	o := scene.Add[render2d.LightOccluder2D](s, h)
	*o = render2d.DefaultLightOccluder2D()
	o.Polygon = make([]mathx.Vec2, 256)
	for i := range o.Polygon {
		a := 2 * mathx.Pi * float32(i) / 256
		o.Polygon[i] = mathx.Vec2{X: mathx.Cos(a), Y: mathx.Sin(a)}
	}
	s.Flush()
	td, _ := s.Registry().ByName("engine.LightOccluder2D")
	g := &occluderGizmo{td: td, p: &lightingEditor{ed: ed}, hoverIndex: 255}
	e, _ := beginLightingDrag(s, h, g, partOccluderVertex)
	want := o.Polygon[255].Scale(1.1)
	e.Mouse = want
	g.Update(e)
	finishLightingDrag(t, e, g)
	o, _ = scene.Get[render2d.LightOccluder2D](s, h)
	if o.Polygon[255] != want || len(o.Polygon) != 256 {
		t.Fatal("gizmo part ID truncated a supported contour")
	}
}

type testLightingBridge struct {
	runner *frame.Runner
	world  *engine.World
}

func (b testLightingBridge) SetLightingView(v bool, m lighting.DebugMode) {
	b.runner.SetLightingView(v, m)
}
func (b testLightingBridge) LightingView() (bool, lighting.DebugMode) { return b.runner.LightingView() }
func (b testLightingBridge) LightingOutput() view.ID                  { return b.runner.OutputID() }
func (b testLightingBridge) LightingWorld() *engine.World             { return b.world }
func (b testLightingBridge) SetLightingEnvironmentPreview(id ids.NodeID) {
	b.runner.SetLightingEnvironmentPreview(id)
}
func (b testLightingBridge) LightingEnvironmentPreview() ids.NodeID {
	return b.runner.LightingEnvironmentPreview()
}

func TestLightingCommandsAndConditionalFieldsUseSharedRegistry(t *testing.T) {
	s, ed := lightingFixture(t)
	point, _ := lightIn(s)
	spot, ls := lightIn(s)
	lp, _ := scene.Get[render2d.Light2D](s, point)
	ls.Type = lighting.Spot
	ls.Shadows = true
	lp.Cookie.SetAssetRefID(ids.AssetID{Lo: 7})
	runner := frame.New()
	ed.SceneLighting = testLightingBridge{runner, engine.NewWorld("preview")}
	inspectors := inspect.NewRegistry()
	inspectors.SetEditor(ed)
	if err := inspectors.Load(s.Registry()); err != nil {
		t.Fatal(err)
	}
	r := ext.New(ext.Deps{Editor: ed, Schema: s.Registry(), Inspectors: inspectors, Gizmos: gizmo.NewRegistry(), Shapes: shape.NewRegistry(), Commands: ed.Commands})
	if err := r.Run("render2d", lightingHooks); err != nil {
		t.Fatal(err)
	}
	before := sceneio.Save(s)
	for _, id := range []string{CmdLighting, CmdLightingNormals, CmdLightingShadows, CmdLightingShadows} {
		if err := ed.Commands.Run(id); err != nil {
			t.Fatal(err)
		}
	}
	enabled, mode := runner.LightingView()
	if !enabled || mode != lighting.DebugLighting {
		t.Fatal("debug toggles failed")
	}
	if !bytes.Equal(before, sceneio.Save(s)) || ed.Doc().History.Cursor() != 0 {
		t.Fatal("Scene View toggle mutated the scene/history")
	}
	if got := ed.Commands.Toolbar(command.ScopeScene); len(got) != 3 {
		t.Fatalf("module toolbar commands %d", len(got))
	}
	td, _ := s.Registry().ByName("engine.Light2D")
	cone, _ := td.Field("innerAngle")
	cookie, _ := td.Field("cookie")
	height, _ := td.Field("height")
	if inspectors.FieldVisible(td.ID, []ids.NodeID{s.ID(point)}, cone) {
		t.Fatal("Point inspector exposes Spot cone")
	}
	if !inspectors.FieldVisible(td.ID, []ids.NodeID{s.ID(point), s.ID(spot)}, cone) {
		t.Fatal("mixed selection should expose union of applicable fields")
	}
	if inspectors.FieldVisible(td.ID, []ids.NodeID{s.ID(point)}, cookie) {
		t.Fatal("Point inspector exposes Sprite cookie")
	}
	if !inspectors.FieldVisible(td.ID, []ids.NodeID{s.ID(point)}, height) {
		t.Fatal("local height hidden")
	}
	if lp.Cookie.AssetRefID() != (ids.AssetID{Lo: 7}) {
		t.Fatal("hiding cookie removed unknown/inactive semantic data")
	}
}
