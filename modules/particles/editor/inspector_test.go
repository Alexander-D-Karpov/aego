package editor

import (
	"slices"
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/gizmo"
	"aego/engine/base"
	"aego/engine/module"
	"aego/modules/particles"
	"aego/platform/input"
	"aego/scene"
	"aego/schema"
	"aego/ui/core"
)

type renderSchemaFixture struct{}

func (renderSchemaFixture) Info() module.Info               { return module.Info{ID: "render2d"} }
func (renderSchemaFixture) Register(*module.Registry) error { return nil }

func inspectorFixture(t *testing.T) (*document.SceneDocument, scene.NodeHandle, *schema.TypeDescriptor) {
	t.Helper()
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.Load(types, nil, renderSchemaFixture{}, particles.New()); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	h := s.Create(s.Root(), "Dust")
	*scene.Add[particles.ParticleSystem2D](s, h) = particles.DefaultParticleSystem2D()
	s.Flush()
	d := document.NewManager(document.Options{Types: types}).New("dust.aescene", s)
	td, _ := types.Schema().ByName(particles.NameSystem)
	return d, h, td
}
func TestParticleInspectorFieldsInOneGroup(t *testing.T) {
	_, _, td := inspectorFixture(t)
	for _, f := range td.Fields {
		n := 0
		for _, g := range groups {
			if slices.Contains(g.fields, f.Name) {
				n++
			}
		}
		if n != 1 {
			t.Errorf("field %s occurs in %d sections", f.Name, n)
		}
	}
}
func TestShapeHandlesUndoInReflectedSpace(t *testing.T) {
	for _, tc := range []struct {
		part  gizmo.Part
		shape particles.Shape
		field string
		to    mathx.Vec2
	}{{partRadius, particles.Circle, "radius", mathx.V2(3, 4)}, {partDirection, particles.Point, "direction", mathx.V2(-2, 1)}, {partRect, particles.Rect, "rectSize", mathx.V2(2, 3)}, {partEdgeA, particles.Edge, "edgeA", mathx.V2(3, 5)}, {partEdgeB, particles.Edge, "edgeB", mathx.V2(2, 5)}} {
		t.Run(tc.field, func(t *testing.T) {
			d, h, td := inspectorFixture(t)
			s := d.Scene()
			c, _ := scene.Get[particles.ParticleSystem2D](s, h)
			c.Shape = tc.shape
			c.Offset = mathx.V2(.25, .1)
			s.SetLocal(h, scene.Transform2D{Position: mathx.V2(-2, 3), Scale: mathx.V2(-2, 3), Rotation: .3})
			s.UpdateTransforms()
			id := s.ID(h)
			fd, _ := td.Field(tc.field)
			before, _ := s.GetProperty(h, td.ID, fd.ID)
			e := gizmo.NewEdit(s, []ids.NodeID{id}, id, mathx.Vec2{}, gizmo.Snap{})
			e.Tx = d.History.Begin("Move handle")
			g := shapeGizmo{td}
			g.Begin(e, gizmo.Hit{Part: tc.part})
			m := s.World(h)
			for i := 1; i <= 4; i++ {
				e.Mouse = m.Apply(c.Offset.Add(tc.to.Scale(float32(i) / 4)))
				g.Update(e)
			}
			g.End(e)
			e.Tx.Commit()
			if e.Err != nil {
				t.Fatal(e.Err)
			}
			if _, n := d.History.Top(); n != 1 {
				t.Fatalf("drag committed %d steps", n)
			}
			after, _ := s.GetProperty(h, td.ID, fd.ID)
			if after.F == before.F && after.V == before.V {
				t.Fatal("drag did not modify property")
			}
			switch tc.part {
			case partRadius:
				if mathx.Abs(float32(after.F)-5) > 1e-4 {
					t.Fatalf("radius %g", after.F)
				}
			case partRect:
				if after.V.Sub(tc.to.Scale(2)).Len() > 1e-4 {
					t.Fatalf("extent %v", after.V)
				}
			case partEdgeA, partEdgeB:
				if after.V.Sub(tc.to).Len() > 1e-4 {
					t.Fatalf("edge %v", after.V)
				}
			case partDirection:
				if mathx.Abs(float32(after.F)-mathx.Atan2(tc.to.Y, tc.to.X)) > 1e-4 {
					t.Fatalf("angle %g", after.F)
				}
			}
			if err := d.History.Undo(); err != nil {
				t.Fatal(err)
			}
			undo, _ := s.GetProperty(h, td.ID, fd.ID)
			if undo.F != before.F || undo.V != before.V {
				t.Fatal("Undo lost original authoring field")
			}
		})
	}
}
func TestGradientDragSingleUndoAndStaleEdit(t *testing.T) {
	d, h, td := inspectorFixture(t)
	s := d.Scene()
	id := s.ID(h)
	c, _ := scene.Get[particles.ParticleSystem2D](s, h)
	c.ColorOverLife = []particles.ColorKey{{Time: 0, Color: mathx.White}, {Time: .5, Color: mathx.Color{R: 1, A: .7}}, {Time: 1, Color: mathx.White}}
	var w GradientEditor
	w.sel = 1
	w.begin(c.ColorOverLife)
	for _, at := range []float32{.2, .6, .7, .8} {
		w.move(at)
	}
	if c.ColorOverLife[1].Time != .5 || d.History.CanUndo() {
		t.Fatal("drag mutated original before commit")
	}
	apply := func(keys []particles.ColorKey) {
		t.Helper()
		if err := setGradient(d, td, []ids.NodeID{id}, keys); err != nil {
			t.Fatal(err)
		}
	}
	w.commit(c.ColorOverLife, apply)
	if _, n := d.History.Top(); n != 1 || c.ColorOverLife[1].Time != .8 {
		t.Fatalf("history depth %d; key %v", n, c.ColorOverLife)
	}
	if err := d.History.Undo(); err != nil || c.ColorOverLife[1].Time != .5 {
		t.Fatal("Undo did not restore gradient")
	}
	w.begin(c.ColorOverLife)
	w.move(.15)
	next, _ := addColor(c.ColorOverLife)
	apply(next)
	w.commit(c.ColorOverLife, apply)
	if len(c.ColorOverLife) != 4 {
		t.Fatal("stale gradient drag overwrote another edit")
	}
}

type inertPreview struct{}

func (inertPreview) controls(*context.Editor, ids.NodeID) {}
func TestMouseEditsGradientInParticleInspector(t *testing.T) {
	fixture, h, td := inspectorFixture(t)
	s := fixture.Scene()
	mgr := document.NewManager(document.Options{})
	doc := mgr.New("test.aescene", s)
	ui := core.New(nil)
	ed := &context.Editor{Docs: mgr, UI: ui}
	p := particleInspector{parent: inertPreview{}, td: td}
	for i := range p.closed {
		p.closed[i] = i != 4
	}
	c, _ := scene.Get[particles.ParticleSystem2D](s, h)
	c.ColorOverLife = []particles.ColorKey{{Time: 0, Color: mathx.White}, {Time: .5, Color: mathx.Color{R: 1, A: .7}}, {Time: 1, Color: mathx.White}}
	nodes := []ids.NodeID{s.ID(h)}
	var inputState input.State
	var strip, buttons mathx.Rect
	frame := func(at mathx.Vec2, press, release bool) {
		inputState.BeginFrame()
		inputState.ApplyMouseMove(at, mathx.Vec2{})
		if press {
			inputState.ApplyMouseDown(at, input.MouseLeft)
		}
		if release {
			inputState.ApplyMouseUp(at, input.MouseLeft)
		}
		ui.Begin(&inputState, mathx.V2(800, 600), 1)
		ui.PushCursor(core.Cursor{X: 10, Y: 12, W: 400, RowH: 18, Space: 2})
		p.Draw(ed, nodes, func() {
			if p.section == 4 {
				layout := *ui.Cursor()
				layout.Next(0)
				strip = layout.Next(28)
				if len(c.ColorOverLife) > 0 {
					layout.Next(0)
				}
				buttons = layout.Next(0)
			}
		})
		ui.PopCursor()
		ui.End()
	}
	frame(mathx.Vec2{}, false, false)
	pos := func(r mathx.Rect, f float32) mathx.Vec2 { return mathx.V2(r.X+f*r.W, r.Y+r.H*.5) }
	click := func(at mathx.Vec2) { frame(at, true, false); frame(at, false, true); frame(at, false, false) }
	frame(pos(strip, .5), true, false)
	frame(pos(strip, .62), false, false)
	frame(pos(strip, .75), false, false)
	frame(pos(strip, .75), false, true)
	if _, depth := doc.History.Top(); depth != 1 || c.ColorOverLife[1].Time != .75 {
		t.Fatalf("mouse drag left gradient %v history depth %d", c.ColorOverLife, depth)
	}
	if err := doc.History.Undo(); err != nil || c.ColorOverLife[1].Time != .5 {
		t.Fatal("mouse drag undo failed")
	}
	click(pos(buttons, .2))
	if len(c.ColorOverLife) != 4 {
		t.Fatal("click Add did not create a color key")
	}
	click(pos(buttons, .8))
	if len(c.ColorOverLife) != 3 {
		t.Fatal("click Delete did not remove selected key")
	}
	if err := doc.History.Undo(); err != nil || len(c.ColorOverLife) != 4 {
		t.Fatal("delete undo lost key")
	}
}

func BenchmarkParticleGradientWidget(b *testing.B) {
	var widget GradientEditor
	keys := []particles.ColorKey{{Time: 0, Color: mathx.White}, {Time: .5, Color: mathx.Color{R: .2, G: .4, B: .6, A: .5}}, {Time: 1, Color: mathx.Color{R: 1, A: .1}}}
	ui := core.New(nil)
	in := new(input.State)
	onChange := func([]particles.ColorKey) { b.Fatal("idle widget wrote to document") }
	draw := func() {
		in.BeginFrame()
		ui.Begin(in, mathx.V2(800, 600), 1)
		ui.PushCursor(core.Cursor{X: 12, Y: 12, W: 360, RowH: 18, Space: 2})
		widget.Draw(ui, keys, onChange)
		ui.PopCursor()
		ui.End()
	}
	draw()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		draw()
	}
}

func TestParticleInspectorRangeAndFoldDefaults(t *testing.T) {
	_, _, td := inspectorFixture(t)
	for _, name := range []string{"lifetime", "speed", "size", "rotation", "gravity", "rectSize", "offset"} {
		f, ok := td.Field(name)
		if !ok {
			t.Fatal(name)
		}
		want := name == "lifetime" || name == "speed" || name == "size" || name == "rotation"
		if f.Hints.MinMaxPair != want {
			t.Fatalf("%s: min/max hint %v", name, f.Hints.MinMaxPair)
		}
	}
	folds := defaultParticleFolds()
	if folds[1] || folds[2] || !folds[4] || !folds[5] || !folds[7] {
		t.Fatal("expected emission and shape expanded and advanced controls collapsed", folds)
	}
	for _, tc := range []struct {
		width float32
		cols  int
	}{{52, 1}, {124, 2}, {186, 2}, {260, 4}} {
		if got := transportColumns(tc.width, 58, 4); got != tc.cols {
			t.Fatalf("width %g: got %d columns; want %d", tc.width, got, tc.cols)
		}
	}
}
