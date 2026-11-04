package lighting

import (
	"errors"
	"math"
	"slices"
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
)

func shadowTestInput(points []mathx.Vec2, closed bool) Input {
	return Input{Points: points, Occluders: []Occluder{{ID: ids.NodeID{Lo: 1}, GeometryRevision: 1, TransformRevision: 2, Count: len(points), Closed: closed, Layers: ^uint64(0)}}}
}
func shadowTestLight() Light {
	return Light{Type: Point, Range: 1, Height: 1, Direction: mathx.Vec2{X: 1}, ShadowCasterMask: ^uint64(0), Shadows: true}
}

func shadowTrianglesCover(vertices []mathx.Vec2, p mathx.Vec2) bool {
	cross := func(a, b, c mathx.Vec2) float64 {
		return (float64(b.X)-float64(a.X))*(float64(c.Y)-float64(a.Y)) - (float64(b.Y)-float64(a.Y))*(float64(c.X)-float64(a.X))
	}
	for i := 0; i+2 < len(vertices); i += 3 {
		a, b, c := vertices[i], vertices[i+1], vertices[i+2]
		area := cross(a, b, c)
		if area == 0 {
			continue
		}
		x, y, z := cross(a, b, p), cross(b, c, p), cross(c, a, p)
		if area > 0 && x >= -1e-8 && y >= -1e-8 && z >= -1e-8 || area < 0 && x <= 1e-8 && y <= 1e-8 && z <= 1e-8 {
			return true
		}
	}
	return false
}

// An independent ray/segment intersection is the oracle for the rasterized
// triangle coverage; these tests do not compare only generated vertex strings.
func shadowRayHits(a, b, p mathx.Vec2) bool {
	ax, ay, dx, dy, qx, qy := float64(a.X), float64(a.Y), float64(b.X-a.X), float64(b.Y-a.Y), float64(p.X), float64(p.Y)
	den := qx*dy - qy*dx
	if den == 0 {
		return false
	}
	t := (ax*dy - ay*dx) / den
	u := (ax*qy - ay*qx) / den
	return t >= 0 && t <= 1 && u >= 0 && u <= 1
}

func TestShadowTrianglesMatchRayCoverageForChainsConcaveAndReflectedPolygons(t *testing.T) {
	for _, tc := range []struct {
		name   string
		points []mathx.Vec2
		closed bool
	}{
		{"chain", []mathx.Vec2{{X: .23, Y: -.47}, {X: .23, Y: .39}}, false},
		{"closed", []mathx.Vec2{{X: .21, Y: -.38}, {X: .62, Y: -.38}, {X: .62, Y: .44}, {X: .21, Y: .44}}, true},
		{"concave", []mathx.Vec2{{X: .2, Y: -.5}, {X: .6, Y: -.5}, {X: .6, Y: -.1}, {X: .4, Y: -.1}, {X: .4, Y: .3}, {X: .2, Y: .3}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, reflect := range []float32{1, -1} {
				pts := slices.Clone(tc.points)
				for i := range pts {
					pts[i].X *= reflect
				}
				in, l := shadowTestInput(pts, tc.closed), shadowTestLight()
				v, err := BuildShadow(&l, &in, nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(v) == 0 || len(v)%3 != 0 {
					t.Fatal("no valid triangles")
				}
				for y := 0; y < 37; y++ {
					for x := 0; x < 41; x++ {
						p := mathx.Vec2{X: -.98 + 1.96*(float32(x)+.371)/41, Y: -.98 + 1.96*(float32(y)+.219)/37}
						want := false
						edges := len(pts) - 1
						if tc.closed {
							edges++
						}
						for j := 0; j < edges; j++ {
							want = want || shadowRayHits(pts[j], pts[(j+1)%len(pts)], p)
						}
						got := shadowTrianglesCover(v, mathx.Vec2{X: .5 + p.X*.5, Y: .5 + p.Y*.5})
						if got != want {
							t.Fatalf("reflect=%v point=%v triangle=%v ray=%v", reflect, p, got, want)
						}
					}
				}
			}
		})
	}
}

func TestShadowNearEdgeExtrudesToWholeLightSquareAndUnionIsBinary(t *testing.T) {
	l := shadowTestLight()
	in := shadowTestInput([]mathx.Vec2{{X: -1, Y: 1e-7}, {X: 1, Y: 1e-7}}, false)
	v, err := BuildShadow(&l, &in, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []mathx.Vec2{{X: .05, Y: .95}, {X: .95, Y: .95}, {X: .5, Y: .9}} {
		if !shadowTrianglesCover(v, p) {
			t.Fatalf("near-edge extrusion left a far-mask hole at %v", p)
		}
	}
	if shadowTrianglesCover(v, mathx.Vec2{X: .5, Y: .1}) {
		t.Fatal("near-edge shadow crossed the light")
	}
	in.Occluders = append(in.Occluders, in.Occluders[0])
	in.Occluders[1].ID.Lo = 2
	doubled, err := BuildShadow(&l, &in, nil)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			p := mathx.Vec2{X: (float32(x) + .3) / 16, Y: (float32(y) + .4) / 16}
			if shadowTrianglesCover(v, p) != shadowTrianglesCover(doubled, p) {
				t.Fatal("overlap changed binary union coverage")
			}
		}
	}
}

func TestShadowClosedInsideBoundaryAndOpenSingularityUseFullMask(t *testing.T) {
	l := shadowTestLight()
	for _, tc := range []struct {
		name   string
		pts    []mathx.Vec2
		closed bool
	}{
		{"inside", []mathx.Vec2{{X: -.2, Y: -.2}, {X: .2, Y: -.2}, {X: .2, Y: .2}, {X: -.2, Y: .2}}, true},
		{"edge", []mathx.Vec2{{X: 0, Y: -.2}, {X: .2, Y: -.2}, {X: .2, Y: .2}, {X: 0, Y: .2}}, true},
		{"vertex", []mathx.Vec2{{}, {X: .2}, {Y: .2}}, true},
		{"open source on edge", []mathx.Vec2{{X: -.2}, {X: .2}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := shadowTestInput(tc.pts, tc.closed)
			v, err := BuildShadow(&l, &in, nil)
			if err != nil || len(v) != 6 {
				t.Fatalf("inside/boundary policy: %d %v", len(v), err)
			}
			for _, p := range []mathx.Vec2{{X: .01, Y: .01}, {X: .99, Y: .01}, {X: .01, Y: .99}, {X: .99, Y: .99}} {
				if !shadowTrianglesCover(v, p) {
					t.Fatal("full mask has a hole")
				}
			}
		})
	}
}

func TestShadowTypesMasksCullAndDegenerateEdges(t *testing.T) {
	l := shadowTestLight()
	in := shadowTestInput([]mathx.Vec2{{X: .25, Y: -.5}, {X: .25, Y: .5}}, false)
	base, err := BuildShadow(&l, &in, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []Type{Point, Spot, Sprite} {
		l.Type = kind
		got, err := BuildShadow(&l, &in, nil)
		if err != nil || !slices.Equal(base, got) {
			t.Fatal("local light masks diverged")
		}
	}
	l.Type = Directional
	if _, err := BuildShadow(&l, &in, nil); !errors.Is(err, ErrShadowLight) {
		t.Fatal("directional used fake local shadows")
	}
	l.Type = Point
	in.Occluders[0].Layers = uint64(1) << 63
	l.ShadowCasterMask = uint64(1) << 32
	if v, err := BuildShadow(&l, &in, nil); err != nil || len(v) != 0 {
		t.Fatal("64-bit shadow masks ignored")
	}
	l.ShadowCasterMask = uint64(1) << 63
	in.Occluders[0].Cull = CullCounterClockwise
	if v, _ := BuildShadow(&l, &in, nil); len(v) != 0 {
		t.Fatal("counterclockwise-facing edge was not culled")
	}
	in.Occluders[0].Cull = CullClockwise
	if v, _ := BuildShadow(&l, &in, nil); len(v) == 0 {
		t.Fatal("opposite winding incorrectly culled")
	}
	slices.Reverse(in.Points)
	if v, _ := BuildShadow(&l, &in, nil); len(v) != 0 {
		t.Fatal("reflected/directed chain culling did not reverse")
	}
	in = shadowTestInput([]mathx.Vec2{{X: .2}, {X: .2}, {X: .2}}, true)
	if v, err := BuildShadow(&l, &in, nil); err != nil || len(v) != 0 {
		t.Fatal("fully collapsed polygon casts a shadow")
	}
}

func TestShadowClippingLargeWorldCoordinatesAndOffscreenCaster(t *testing.T) {
	l := shadowTestLight()
	in := shadowTestInput([]mathx.Vec2{{X: -1e30, Y: .25}, {X: 1e30, Y: .25}}, false)
	v, err := BuildShadow(&l, &in, nil)
	if err != nil || len(v) == 0 {
		t.Fatalf("large edge: %v", err)
	}
	for _, p := range v {
		if !finite(p.X) || !finite(p.Y) || p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1 {
			t.Fatalf("unclipped/nonfinite mask vertex %v", p)
		}
	}
	if !shadowTrianglesCover(v, mathx.Vec2{X: .9, Y: .9}) {
		t.Fatal("large offscreen segment did not cast into the light")
	}
	l.Position = mathx.Vec2{X: 1e6, Y: -2e6}
	l.Range = 16
	in = shadowTestInput([]mathx.Vec2{{X: 1e6 + 4, Y: -2e6 - 8}, {X: 1e6 + 4, Y: -2e6 + 8}}, false)
	v, err = BuildShadow(&l, &in, v)
	if err != nil || !shadowTrianglesCover(v, mathx.Vec2{X: .9, Y: .5}) {
		t.Fatal("translation changed light-space geometry")
	}
}

func TestShadowRejectsInvalidAndOverBudgetWithoutPartialMask(t *testing.T) {
	l := shadowTestLight()
	in := shadowTestInput([]mathx.Vec2{{X: .2, Y: -.5}, {X: .2, Y: .5}}, false)
	for _, bad := range []Occluder{{First: -1, Count: 2, Layers: 1}, {First: 1, Count: 2, Layers: 1}, {Count: 1, Layers: 1}, {Count: 2, Layers: 1, Closed: true}} {
		in.Occluders = append(in.Occluders[:1], bad)
		if v, err := BuildShadow(&l, &in, nil); !errors.Is(err, ErrShadowGeometry) || len(v) != 0 {
			t.Fatalf("invalid contour emitted partial triangles: %d %v", len(v), err)
		}
	}
	in = shadowTestInput([]mathx.Vec2{{X: float32(math.NaN())}, {X: 1}}, false)
	if v, err := BuildShadow(&l, &in, nil); !errors.Is(err, ErrShadowGeometry) || len(v) != 0 {
		t.Fatal("NaN geometry bypassed validation")
	}
	points := make([]mathx.Vec2, 64)
	for i := range points {
		a := 2 * math.Pi * float64(i) / float64(len(points))
		points[i] = mathx.Vec2{X: .45 + .2*float32(math.Cos(a)), Y: .2 * float32(math.Sin(a))}
	}
	in = shadowTestInput(points, true)
	one := in.Occluders[0]
	for len(in.Occluders) < MaxOccluders {
		one.ID.Lo++
		in.Occluders = append(in.Occluders, one)
	}
	if v, err := BuildShadow(&l, &in, nil); !errors.Is(err, ErrShadowBudget) || len(v) != 0 {
		t.Fatalf("budget failure returned partial mask: %d %v", len(v), err)
	}
}

func TestShadowKeyTracksOnlyRelevantGeometryAndDeviceState(t *testing.T) {
	l := shadowTestLight()
	in := shadowTestInput([]mathx.Vec2{{X: .2, Y: -.5}, {X: .2, Y: .5}}, false)
	base := ShadowKey(&l, &in, 256, 1)
	l.Color = mathx.Color{R: .2, G: .7, B: 1, A: 1}
	l.Intensity = 10
	l.Height = 3
	in.Environment.Exposure = 9
	if ShadowKey(&l, &in, 256, 1) != base {
		t.Fatal("shading-only change invalidates planar mask")
	}
	for _, change := range []func(){func() { l.Position.X = .1 }, func() { l.Direction.Y = 1 }, func() { l.Range = 2 }, func() { l.ShadowSoftness = 1 }, func() { l.ShadowCasterMask = 0 }, func() { in.Occluders[0].GeometryRevision++ }, func() { in.Occluders[0].TransformRevision++ }, func() { in.Occluders[0].Cull = CullClockwise }} {
		oldL, oldO := l, in.Occluders[0]
		change()
		if ShadowKey(&l, &in, 256, 1) == base {
			t.Fatal("mask state failed to invalidate key")
		}
		l, in.Occluders[0] = oldL, oldO
	}
	if ShadowKey(&l, &in, 512, 1) == base || ShadowKey(&l, &in, 256, 2) == base {
		t.Fatal("resolution/device restore kept stale mask")
	}
	far := Occluder{ID: ids.NodeID{Lo: 9}, First: len(in.Points), Count: 2, Layers: 1, Bounds: mathx.AABB2{Min: mathx.Vec2{X: 10, Y: 10}, Max: mathx.Vec2{X: 11, Y: 11}}}
	in.Points = append(in.Points, far.Bounds.Min, far.Bounds.Max)
	in.Occluders = append(in.Occluders, far)
	if ShadowKey(&l, &in, 256, 1) != base {
		t.Fatal("irrelevant off-light caster invalidates cache")
	}
	in.Occluders[1].Bounds = mathx.AABB2{Min: mathx.Vec2{X: .3, Y: .1}, Max: mathx.Vec2{X: .6, Y: .4}}
	in.Points[2], in.Points[3] = in.Occluders[1].Bounds.Min, in.Occluders[1].Bounds.Max
	withTwo := ShadowKey(&l, &in, 256, 1)
	if withTwo == base {
		t.Fatal("caster moving into light did not invalidate")
	}
	slices.Reverse(in.Occluders)
	if ShadowKey(&l, &in, 256, 1) != withTwo {
		t.Fatal("extraction iteration order changes mask cache key")
	}
}

func TestShadowWarmBuildAndKeyAllocateNothing(t *testing.T) {
	l := shadowTestLight()
	in := shadowTestInput([]mathx.Vec2{{X: .2, Y: -.5}, {X: .2, Y: .5}, {X: .4, Y: .6}}, false)
	vertices, err := BuildShadow(&l, &in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := testing.AllocsPerRun(500, func() { vertices, err = BuildShadow(&l, &in, vertices); _ = ShadowKey(&l, &in, 256, 1) }); n != 0 || err != nil {
		t.Fatalf("shadow warmed path: %v allocations %v", n, err)
	}
}

func FuzzShadowSegmentClippingFiniteBounded(f *testing.F) {
	f.Add(float32(.2), float32(-.5), float32(.2), float32(.5))
	f.Add(float32(-1e30), float32(.25), float32(1e30), float32(.25))
	f.Fuzz(func(t *testing.T, ax, ay, bx, by float32) {
		if !finite(ax) || !finite(ay) || !finite(bx) || !finite(by) {
			return
		}
		l := shadowTestLight()
		in := shadowTestInput([]mathx.Vec2{{X: ax, Y: ay}, {X: bx, Y: by}}, false)
		v, err := BuildShadow(&l, &in, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(v)%3 != 0 || len(v) > 30 {
			t.Fatal("single segment exceeded bounded triangle output")
		}
		for _, p := range v {
			if !finite(p.X) || !finite(p.Y) || p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1 {
				t.Fatal("nonfinite or out-of-square mask coordinate")
			}
		}
	})
}
