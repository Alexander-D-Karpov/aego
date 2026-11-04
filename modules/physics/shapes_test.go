package physics

import (
	"errors"
	"math"
	"testing"

	"aego/contracts/shapes2d"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/module"
	"aego/modules/physics/components"
	"aego/scene"
	"aego/schema"
)

func shapeScene(t *testing.T) (*scene.Scene, *shapeProvider) {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if _, err := module.Load(types, nil, New()); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	p := newShapeProvider(s)
	t.Cleanup(p.Close)
	return s, p
}

func shapes(t *testing.T, p shapes2d.Provider, node ids.NodeID, key string) shapes2d.Buffer {
	t.Helper()
	var b shapes2d.Buffer
	if err := p.Append(node, key, &b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestShapeProviderAttachesWithoutSolverInEveryWorld(t *testing.T) {
	for _, mode := range []struct {
		name string
		play bool
	}{{"edit", false}, {"preview", false}, {"play", true}} {
		t.Run(mode.name, func(t *testing.T) {
			s, _ := shapeScene(t)
			h := s.Create(s.Root(), "source")
			*scene.Add[components.BoxCollider2D](s, h) = components.DefaultBox()
			w, m := engine.NewWorld(mode.name), New()
			ctx := &module.AttachContext{World: w, Scene: s, Play: mode.play, Editor: true}
			before := s.Events().Len()
			if err := m.AttachWorld(ctx); err != nil {
				t.Fatal(err)
			}
			p, ok := engine.ServiceOf[shapes2d.Provider](w, shapes2d.ServiceID)
			if !ok || len(shapes(t, p, s.ID(h), "").Contours) != 1 {
				t.Fatal("attached world has no geometry")
			}
			if (SimOf(w) != nil) != mode.play {
				t.Fatal("geometry changed the solver attachment policy")
			}
			m.DetachWorld(ctx)
			if _, ok := w.ServiceAny(shapes2d.ServiceID); ok || s.Events().Len() != before {
				t.Fatal("detach retained a shape service or event subscription")
			}
			var b shapes2d.Buffer
			if err := p.Append(s.ID(h), "", &b); !errors.Is(err, shapes2d.ErrClosed) {
				t.Fatalf("detached provider remained usable: %v", err)
			}
		})
	}
}

func TestShapeProviderLocalContoursAndStableComponentIdentity(t *testing.T) {
	s, p := shapeScene(t)
	parent := s.Create(s.Root(), "parent")
	s.SetLocal(parent, scene.Transform2D{Position: mathx.Vec2{X: 90, Y: 40}, Scale: mathx.Vec2{X: -2, Y: 3}, Rotation: 0.7})
	h := s.Create(parent, "shapes")
	s.SetPosition(h, mathx.Vec2{X: 10, Y: -4})
	box := scene.Add[components.BoxCollider2D](s, h)
	*box = components.DefaultBox()
	box.Offset, box.Size = mathx.Vec2{X: 3, Y: 5}, mathx.Vec2{X: 2, Y: 4}
	*scene.Add[components.CircleCollider2D](s, h) = components.DefaultCircle()
	*scene.Add[components.CapsuleCollider2D](s, h) = components.DefaultCapsule()
	*scene.Add[components.PolygonCollider2D](s, h) = components.DefaultPolygon()
	edge := scene.Add[components.EdgeCollider2D](s, h)
	*edge = components.DefaultEdge()
	edge.Points = []mathx.Vec2{{X: -1}, {}, {X: 1, Y: 1}}
	s.Flush()
	b := shapes(t, p, s.ID(h), "")
	counts := []int{4, 32, 34, 4, 3}
	if len(b.Contours) != len(counts) {
		t.Fatalf("got %d contours", len(b.Contours))
	}
	for i, c := range b.Contours {
		if c.ID != (shapes2d.ID{Node: s.ID(h), Part: uint32(i + 1)}) || c.Key != components.Names[i] || c.Owner != s.ID(h) || c.Count != counts[i] || c.Revision == 0 || c.Closed != (i != 4) {
			t.Fatalf("contour %d: %+v", i, c)
		}
	}
	if got := b.Vertices(b.Contours[0]); got[0] != (mathx.Vec2{X: 2, Y: 3}) || got[2] != (mathx.Vec2{X: 4, Y: 7}) {
		t.Fatalf("box points were world-transformed or offset twice: %v", got)
	}
	capPoints := b.Vertices(b.Contours[2])
	lo, hi := mathx.Vec2{X: 100, Y: 100}, mathx.Vec2{X: -100, Y: -100}
	for _, v := range capPoints {
		lo.X, lo.Y, hi.X, hi.Y = min(lo.X, v.X), min(lo.Y, v.Y), max(hi.X, v.X), max(hi.Y, v.Y)
	}
	if lo != (mathx.Vec2{X: -0.5, Y: -0.25}) || hi != (mathx.Vec2{X: 0.5, Y: 0.25}) {
		t.Fatalf("capsule no longer matches the physics local-X convention: %v..%v", lo, hi)
	}
	circle := b.Contours[1]
	scene.Remove[components.BoxCollider2D](s, h)
	s.Flush()
	after := shapes(t, p, s.ID(h), "")
	if after.Contours[0].ID != circle.ID || after.Contours[0].Revision != circle.Revision || len(p.cache) != 4 {
		t.Fatal("removing another component changed shape identity or retained its cache")
	}
}

func TestShapeProviderSeparatesGeometryFromPresentationRevision(t *testing.T) {
	s, p := shapeScene(t)
	parent := s.Create(s.Root(), "parent")
	h := s.Create(parent, "box")
	c := scene.Add[components.BoxCollider2D](s, h)
	*c = components.DefaultBox()
	s.Flush()
	c, _ = scene.Get[components.BoxCollider2D](s, h)
	initial := shapes(t, p, s.ID(h), components.NameBox)
	rev := initial.Contours[0].Revision
	pose := s.Local(parent)
	pose.Position = mathx.Vec2{X: 123, Y: -88}
	pose.Scale = mathx.Vec2{X: -3, Y: 2}
	s.SetPresentationLocal(parent, pose)
	c.Friction, c.Restitution, c.IsTrigger = 0.9, 0.5, true
	after := shapes(t, p, s.ID(h), components.NameBox)
	if after.Contours[0].Revision != rev || after.Points[0] != initial.Points[0] {
		t.Fatal("presentation or collision settings invalidated local geometry")
	}
	if s.RenderWorld(s.Handle(after.Contours[0].Owner))[6] != 123 {
		t.Fatal("consumer cannot resolve the presentation owner")
	}
	c.Offset.X = 2
	after = shapes(t, p, s.ID(h), components.NameBox)
	if after.Contours[0].Revision <= rev || after.Points[0].X != initial.Points[0].X+2 || initial.Points[0].X != -0.5 {
		t.Fatalf("offset revision did not change, or previous output aliased provider state: before=%+v %v after=%+v %v component=%+v", initial.Contours, initial.Points, after.Contours, after.Points, c)
	}
	rev = after.Contours[0].Revision
	c.Offset.X = 0
	after = shapes(t, p, s.ID(h), components.NameBox)
	if after.Contours[0].Revision <= rev || after.Points[0] != initial.Points[0] {
		t.Fatal("restoring geometry reused an old revision")
	}
	other := s.Create(s.Root(), "other parent")
	s.SetPosition(other, mathx.Vec2{X: -20})
	if err := s.Reparent(h, other, -1); err != nil {
		t.Fatal(err)
	}
	reparented := shapes(t, p, s.ID(h), "")
	if reparented.Contours[0].Revision != after.Contours[0].Revision || reparented.Points[0] != after.Points[0] {
		t.Fatal("reparent changed local shape geometry")
	}
}

func TestShapeProviderSnapshotsDoNotAliasComponentPoints(t *testing.T) {
	s, p := shapeScene(t)
	h := s.Create(s.Root(), "poly")
	c := scene.Add[components.PolygonCollider2D](s, h)
	*c = components.DefaultPolygon()
	s.Flush()
	c, _ = scene.Get[components.PolygonCollider2D](s, h)
	before := shapes(t, p, s.ID(h), "")
	c.Points[0].X = -2
	if before.Points[0].X != -0.5 {
		t.Fatal("output aliases component geometry")
	}
	after := shapes(t, p, s.ID(h), "")
	if after.Points[0].X != -2 || after.Contours[0].Revision <= before.Contours[0].Revision {
		t.Fatal("in-place component mutation did not invalidate geometry")
	}
	after.Points[0].X = -99
	last := shapes(t, p, s.ID(h), "")
	if last.Points[0].X != -2 || c.Points[0].X != -2 {
		t.Fatal("caller output changed provider or component state")
	}
}

func TestShapeProviderDisableDestroyAndRepeatedLifetime(t *testing.T) {
	s, p := shapeScene(t)
	parent := s.Create(s.Root(), "parent")
	h := s.Create(parent, "box")
	*scene.Add[components.BoxCollider2D](s, h) = components.DefaultBox()
	s.Flush()
	before := shapes(t, p, s.ID(h), "")
	s.SetEnabled(parent, false)
	if got := shapes(t, p, s.ID(h), ""); len(got.Contours) != 0 {
		t.Fatal("disabled parent still supplies geometry")
	}
	s.SetEnabled(parent, true)
	if got := shapes(t, p, s.ID(h), ""); got.Contours[0].Revision != before.Contours[0].Revision {
		t.Fatal("enabling unchanged geometry changed its revision")
	}
	id := s.ID(h)
	s.Destroy(h)
	if len(p.cache) != 0 {
		t.Fatal("destroy retained historical cached geometry")
	}
	var b shapes2d.Buffer
	if err := p.Append(id, "", &b); !errors.Is(err, shapes2d.ErrMissingNode) {
		t.Fatalf("destroyed node: %v", err)
	}
	s.Flush()
	for i := 0; i < 100; i++ {
		h = s.CreateWithID(parent, "restored", id)
		*scene.Add[components.BoxCollider2D](s, h) = components.DefaultBox()
		s.Flush()
		got := shapes(t, p, id, "")
		if got.Contours[0].ID != before.Contours[0].ID || got.Contours[0].Revision <= before.Contours[0].Revision {
			t.Fatal("restored ID reused stale geometry generation")
		}
		s.Destroy(h)
		s.Flush()
		if len(p.cache) != 0 {
			t.Fatal("repeated remove/restore retained history in geometry cache")
		}
	}
}

func TestShapeProviderValidatesContoursAndCachesDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name   string
		pts    []mathx.Vec2
		closed bool
	}{
		{"nan", []mathx.Vec2{{X: float32(math.NaN())}, {X: 1}}, false},
		{"inf", []mathx.Vec2{{}, {X: float32(math.Inf(1))}}, false},
		{"zero edge", []mathx.Vec2{{}, {}, {Y: 1}}, false},
		{"self crossing", []mathx.Vec2{{}, {X: 2, Y: 2}, {Y: 2}, {X: 2}}, true},
		{"backtracking", []mathx.Vec2{{}, {X: 2}, {X: 1}}, false},
		{"zero area", []mathx.Vec2{{}, {X: 1}, {X: 2}}, true},
		{"short closed", []mathx.Vec2{{}, {X: 1}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, p := shapeScene(t)
			h := s.Create(s.Root(), "invalid")
			c := scene.Add[components.EdgeCollider2D](s, h)
			*c = components.DefaultEdge()
			c.Points, c.Loop = tc.pts, tc.closed
			s.Flush()
			var b shapes2d.Buffer
			err := p.Append(s.ID(h), "", &b)
			if err == nil || len(b.Contours) != 0 || len(b.Points) != 0 {
				t.Fatal("invalid contour produced a partial output")
			}
			if again := p.Append(s.ID(h), "", &b); again != err {
				t.Fatal("unchanged invalid geometry recreated its diagnostic")
			}
		})
	}
}

func TestShapeProviderAtomicOutputBudgetAndZeroSteadyAllocations(t *testing.T) {
	s, p := shapeScene(t)
	h := s.Create(s.Root(), "shapes")
	*scene.Add[components.BoxCollider2D](s, h) = components.DefaultBox()
	*scene.Add[components.CircleCollider2D](s, h) = components.DefaultCircle()
	s.Flush()
	var b shapes2d.Buffer
	if err := b.Append(shapes2d.Contour{Key: "sentinel"}, []mathx.Vec2{{}, {X: 1}}); err != nil {
		t.Fatal(err)
	}
	b.Limits.Contours = 2
	if err := p.Append(s.ID(h), "", &b); !errors.Is(err, shapes2d.ErrBudget) {
		t.Fatalf("expected aggregate budget rejection: %v", err)
	}
	if len(b.Contours) != 1 || len(b.Points) != 2 || b.Contours[0].Key != "sentinel" {
		t.Fatal("failed multi-shape append was not atomic")
	}
	b.Limits = shapes2d.Limits{}
	b.Reset()
	if err := p.Append(s.ID(h), "", &b); err != nil {
		t.Fatal(err)
	}
	var callErr error
	if n := testing.AllocsPerRun(500, func() {
		b.Reset()
		callErr = p.Append(s.ID(h), "", &b)
	}); n != 0 || callErr != nil {
		t.Fatalf("warmed geometry path: %v allocations, %v", n, callErr)
	}
	if err := p.Append(s.ID(h), "missing.Shape", &b); !errors.Is(err, shapes2d.ErrMissingShape) {
		t.Fatalf("missing explicit selection: %v", err)
	}
}
