package sceneview

import (
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/gizmo"
	"aego/editor/core/props"
	"aego/editor/core/shape"
	"aego/scene"
)

func spatialScene(t *testing.T) (*scene.Scene, asset.Resolver) {
	t.Helper()
	return scene.New(testTypes(t), nil, scene.Options{DefaultSpace: scene.Space3D}), nil
}

func cube(s *scene.Scene, at mathx.Vec3) scene.NodeHandle {
	h := s.Create3D(s.Root(), "Cube")
	*scene.Add[Block](s, h) = Block{Half: 0.5}
	s.SetPosition3D(h, at)
	return h
}

type fakeHost struct {
	s       *scene.Scene
	res     asset.Resolver
	sh      shape.Set
	sel     []ids.NodeID
	op      gizmo.Op3
	commits [][]props.Transform3DEdit
}

func (h *fakeHost) Scene() *scene.Scene    { return h.s }
func (h *fakeHost) Shapes() shape.Set      { return h.sh }
func (h *fakeHost) Selected() []ids.NodeID { return h.sel }
func (h *fakeHost) Tool3() gizmo.Op3       { return h.op }
func (h *fakeHost) ShowGizmo() bool        { return true }
func (h *fakeHost) LocalAxes() bool        { return false }
func (h *fakeHost) Snap3() gizmo.Snap3     { return gizmo.Snap3{} }
func (h *fakeHost) Select(n []ids.NodeID, add bool) {
	if add {
		h.sel = append(h.sel, n...)
		return
	}
	h.sel = append([]ids.NodeID(nil), n...)
}
func (h *fakeHost) Commit3D(_ string, e []props.Transform3DEdit) {
	h.commits = append(h.commits, e)
	for _, x := range e {
		h.s.SetLocal3D(h.s.Handle(x.Node), x.After)
	}
}

func frontController() *Controller3 {
	c := NewController3()
	c.Cam = Camera3D{Distance: 10, FOV: 60}
	c.SetViewport(800, 600)
	return c
}

func screenOf(t *testing.T, c *Controller3, p mathx.Vec3) mathx.Vec2 {
	t.Helper()
	q, ok := c.View().WorldToScreen3(p)
	if !ok {
		t.Fatalf("%v is not on screen", p)
	}
	return q
}

func TestEyeLooksAtTarget(t *testing.T) {
	c := DefaultCamera3D()
	c.Target = mathx.Vec3{X: 1, Y: 2, Z: 3}
	eye := c.Eye()
	fwd := eye.TransformDir(mathx.Vec3{Z: -1}).Normalized()
	to := c.Target.Sub(eye.Translation()).Normalized()
	if !fwd.NearlyEqual(to, 1e-5) {
		t.Fatalf("forward %v, to target %v", fwd, to)
	}
	c.Orbit(0, -1e6)
	if c.Pitch > maxPitch {
		t.Fatal("pitch not clamped")
	}
}

func TestPanAndDolly(t *testing.T) {
	c := DefaultCamera3D()
	before := c
	c.Pan(30, -20, 600)
	if d := c.Target.Sub(before.Target); mathx.Abs(d.Dot(c.Forward())) > 1e-5 || d.Len() == 0 {
		t.Fatalf("pan moved along forward: %v", d)
	}
	c.Dolly(1e6)
	if c.Distance != minDistance {
		t.Fatalf("dolly not clamped: %v", c.Distance)
	}
}

func TestFrameFitsBox(t *testing.T) {
	c := frontController()
	b := mathx.AABB3{Min: mathx.Vec3{X: 9, Y: -1, Z: -1}, Max: mathx.Vec3{X: 11, Y: 1, Z: 1}}
	c.Cam.Frame(b)
	v := c.View()
	for _, x := range []float32{b.Min.X, b.Max.X} {
		for _, y := range []float32{b.Min.Y, b.Max.Y} {
			for _, z := range []float32{b.Min.Z, b.Max.Z} {
				p, ok := v.WorldToScreen3(mathx.Vec3{X: x, Y: y, Z: z})
				if !ok || p.X < 0 || p.X > 800 || p.Y < 0 || p.Y > 600 {
					t.Fatalf("corner off screen: %v %v", p, ok)
				}
			}
		}
	}
}

func TestPick3DNearestAndDisabled(t *testing.T) {
	s, res := spatialScene(t)
	a := cube(s, mathx.Vec3{})
	b := cube(s, mathx.Vec3{Z: -5})
	s.Flush()
	ray := mathx.Ray3{Origin: mathx.Vec3{Z: 10}, Dir: mathx.Vec3{Z: -1}}
	h, tt, ok := Pick3D(s, testShapes(t, s, res), ray, ^uint64(0))
	if !ok || h != a || mathx.Abs(tt-9.5) > 1e-4 {
		t.Fatalf("pick = %v %v %v", h, tt, ok)
	}
	s.SetEnabled(a, false)
	s.UpdateTransforms()
	if h, _, _ := Pick3D(s, testShapes(t, s, res), ray, ^uint64(0)); h != b {
		t.Fatal("disabled cube was picked")
	}
}

func TestPickPointAndBoxSelect(t *testing.T) {
	s, _ := spatialScene(t)
	e := s.Create3D(s.Root(), "Empty")
	s.SetPosition3D(e, mathx.Vec3{X: 3})
	behind := s.Create3D(s.Root(), "Behind")
	s.SetPosition3D(behind, mathx.Vec3{Z: 20})
	s.Flush()
	o := mathx.Vec3{Z: 10}
	ray := mathx.Ray3{Origin: o, Dir: mathx.Vec3{X: 3}.Sub(o).Normalized()}
	h, _, ok := PickPoint3D(s, testShapes(t, s, nil), ray, ^uint64(0), func(mathx.Vec3) float32 { return 0.1 })
	if !ok || h != e {
		t.Fatal("empty node not picked")
	}
	c := frontController()
	got := BoxSelect3D(s, shape.Set{}, c.View(), mathx.Vec2{}, mathx.Vec2{X: 800, Y: 600}, ^uint64(0))
	if len(got) != 1 || got[0] != s.ID(e) {
		t.Fatalf("box select = %v", got)
	}
}

func TestGizmoDragCommitsOnce(t *testing.T) {
	s, res := spatialScene(t)
	n := s.Create3D(s.Root(), "N")
	s.Flush()
	h := &fakeHost{s: s, res: res, sh: testShapes(t, s, res), sel: []ids.NodeID{s.ID(n)}}
	c := frontController()
	l := gizmo.PxToWorld(c.View(), mathx.Vec3{}, gizmo.DefaultSize3)
	grab := screenOf(t, c, mathx.Vec3{X: l * 0.7})
	c.Update(h, Input3{Mouse: grab, Hovered: true})
	if c.Hot() != gizmo.Part3X {
		t.Fatalf("hot = %d", c.Hot())
	}
	c.Update(h, Input3{Mouse: grab, Hovered: true, Down: [3]bool{true}, Pressed: [3]bool{true}})
	to := screenOf(t, c, mathx.Vec3{X: l*0.7 + 1})
	c.Update(h, Input3{Mouse: to, Hovered: true, Down: [3]bool{true}})
	if got := s.Local3D(n).Position.X; mathx.Abs(got-1) > 1e-2 {
		t.Fatalf("preview x = %v", got)
	}
	c.Update(h, Input3{Mouse: to, Hovered: true})
	if len(h.commits) != 1 || len(h.commits[0]) != 1 {
		t.Fatalf("commits = %+v", h.commits)
	}
	e := h.commits[0][0]
	if e.Before.Position != (mathx.Vec3{}) || !e.After.Position.NearlyEqual(mathx.Vec3{X: 1}, 1e-2) {
		t.Fatalf("edit = %+v", e)
	}
}

// A press on a handle that the hand moves a few pixels, scaled for HiDPI, is a click: nothing moves and nothing commits.
func TestGizmoClickWithinSlopDoesNotMove(t *testing.T) {
	s, res := spatialScene(t)
	n := s.Create3D(s.Root(), "N")
	s.Flush()
	h := &fakeHost{s: s, res: res, sh: testShapes(t, s, res), sel: []ids.NodeID{s.ID(n)}}
	c := frontController()
	l := gizmo.PxToWorld(c.View(), mathx.Vec3{}, gizmo.DefaultSize3)
	grab := screenOf(t, c, mathx.Vec3{X: l * 0.7})
	c.Update(h, Input3{Mouse: grab, Hovered: true, Scale: 2})
	c.Update(h, Input3{Mouse: grab, Hovered: true, Down: [3]bool{true}, Pressed: [3]bool{true}, Scale: 2})
	off := grab.Add(mathx.Vec2{X: 6})
	c.Update(h, Input3{Mouse: off, Hovered: true, Down: [3]bool{true}, Scale: 2})
	c.Update(h, Input3{Mouse: off, Hovered: true, Scale: 2})
	if got := s.Local3D(n).Position; got != (mathx.Vec3{}) || len(h.commits) != 0 {
		t.Fatalf("a click moved the node to %v with %d commits", got, len(h.commits))
	}
}

func TestClickSelectsAndClears(t *testing.T) {
	s, res := spatialScene(t)
	a := cube(s, mathx.Vec3{})
	cube(s, mathx.Vec3{Z: -5})
	s.Flush()
	h := &fakeHost{s: s, res: res, sh: testShapes(t, s, res)}
	c := frontController()
	click := func(p mathx.Vec2) {
		c.Update(h, Input3{Mouse: p, Hovered: true, Down: [3]bool{true}, Pressed: [3]bool{true}})
		c.Update(h, Input3{Mouse: p, Hovered: true})
	}
	click(screenOf(t, c, mathx.Vec3{}))
	if len(h.sel) != 1 || h.sel[0] != s.ID(a) {
		t.Fatalf("selection = %v", h.sel)
	}
	click(mathx.Vec2{X: 5, Y: 5})
	if len(h.sel) != 0 {
		t.Fatal("click on empty space did not clear")
	}
}
