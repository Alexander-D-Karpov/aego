package render

import (
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/render/lighting"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/view"
)

// This measures our prepared CPU render path with gpu/null. Actual pixel and
// reflection tests use the same renderer in tests/gl and tests/weblighting.
func lightingFixture(t testing.TB, count, lights int, moving bool) (*Renderer, func()) {
	t.Helper()
	r, dev := newTestRenderer(t, max(count, 64))
	draws := testDraws(t, r, count, 20)
	sh := &material.Shader{Name: "lighting-benchmark", Domain: shader.DomainSprite, Features: shader.FeatureAffine | shader.FeatureLighting | shader.FeatureSurfaceChannels,
		Schema: material.NewSchema(nil, 0, 0), Variants: []material.Variant{{Sources: [4]string{"v", "f", "v", "f"}}}}
	m, ok := material.New("lighting-benchmark", sh, 0, shader.BlendPremultiplied)
	if !ok {
		t.Fatal("material construction")
	}
	for i := range draws {
		draws[i].Material = m
		draws[i].VisibilityLayers = ^uint64(0)
	}
	in := lighting.Input{Environment: lighting.DefaultEnvironment(), HasEnvironment: true}
	in.Environment.ColorMode = lighting.LDR
	in.Environment.MaxShadowedLights = min(lights, lighting.MaxShadows)
	in.Lights = make([]lighting.Light, lights)
	for i := range in.Lights {
		in.Lights[i] = lighting.Light{ID: ids.NodeID{Lo: uint64(i + 1)}, Type: lighting.Point,
			Position: mathx.Vec2{X: float32(i*3 - lights)}, Direction: mathx.Vec2{X: 1}, Color: mathx.White,
			Intensity: 1, Height: 2, Range: 12, Falloff: 1, Layers: ^uint64(0), ShadowCasterMask: ^uint64(0), Shadows: true}
	}
	in.Points = []mathx.Vec2{{X: -1, Y: -1}, {X: 1, Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1}}
	in.Occluders = []lighting.Occluder{{ID: ids.NodeID{Lo: 100}, GeometryRevision: 1, TransformRevision: 1,
		Count: 4, Closed: true, Layers: ^uint64(0), Bounds: mathx.AABB2{Min: in.Points[0], Max: in.Points[2]}}}
	v := worldView()
	v.ID, v.OutputID = view.ID{Scope: 901}, view.ID{Scope: 902}
	v.DoClear, v.Clear = true, mathx.Color{A: 1}
	frame := func() {
		if moving {
			in.Occluders[0].TransformRevision++
		}
		f := dev.BeginFrame(1280, 720)
		if err := r.BeginFrame(f, 1280, 720, 1); err != nil {
			t.Fatal(err)
		}
		vf := r.View(v)
		if err := vf.Lighting(&in); err != nil {
			t.Fatal(err)
		}
		vf.Sprites(draws)
		vf.End()
		r.EndFrame()
		dev.EndFrame()
		if err := r.LightingError(); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10; i++ {
		frame()
	}
	return r, frame
}

func TestLightingWarmedRenderPathNoAllocations(t *testing.T) {
	for _, moving := range []bool{false, true} {
		name := "cached"
		if moving {
			name = "moving"
		}
		t.Run(name, func(t *testing.T) {
			r, frame := lightingFixture(t, 600, 8, moving)
			if n := testing.AllocsPerRun(30, frame); n != 0 {
				t.Fatalf("warmed %s lighting render path: %v allocations/frame", name, n)
			}
			if r.stats.LightSets == 0 || r.stats.ResolvePasses != 1 || r.stats.LightingFailures != 0 {
				t.Fatalf("fixture skipped lighting: %+v", r.stats)
			}
			if moving && r.stats.DirtyShadowMasks == 0 || !moving && r.stats.DirtyShadowMasks != 0 {
				t.Fatalf("shadow cache did not follow transform revision: %+v", r.stats)
			}
		})
	}
}

func BenchmarkLightingRender(b *testing.B) {
	for _, tc := range []struct {
		name   string
		lights int
		moving bool
	}{{"8-cached", 8, false}, {"8-moving", 8, true}, {"16-stress", 16, true}} {
		b.Run(tc.name, func(b *testing.B) {
			r, frame := lightingFixture(b, 1000, tc.lights, tc.moving)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				frame()
			}
			b.StopTimer()
			b.ReportMetric(float64(r.stats.Batches), "batches/frame")
			b.ReportMetric(float64(r.stats.InstanceBytes), "instance-B/frame")
			b.ReportMetric(float64(r.stats.LightingUBOBytes), "lighting-UBO-B/frame")
			b.ReportMetric(float64(r.stats.DirtyShadowMasks), "dirty-masks/frame")
			b.ReportMetric(float64(r.stats.ShadowCacheHits), "shadow-cache-hits/frame")
		})
	}
}

func TestUnlitViewsAllocateNoLightingResources(t *testing.T) {
	r, dev := newTestRenderer(t, 64)
	draws := testDraws(t, r, 1, 0)
	for i := 0; i < 3; i++ {
		f := dev.BeginFrame(1280, 720)
		r.BeginFrame(f, 1280, 720, 1)
		vf := r.View(worldView())
		vf.Lighting(&lighting.Input{})
		vf.Sprites(draws)
		vf.End()
		s := r.EndFrame()
		dev.EndFrame()
		if s.ResolvePasses != 0 || s.LightingResources != 0 || s.LightingUBOBytes != 0 || len(r.lightState.outputs) != 0 || len(r.lightState.views) != 0 {
			t.Fatalf("Unlit allocated lighting resources: %+v", s)
		}
	}
}

func TestLightingDroppedIdentitiesAreUniquePerView(t *testing.T) {
	r, dev := newTestRenderer(t, 64)
	var in lighting.Input
	for i := 0; i < 10; i++ {
		in.Lights = append(in.Lights, lighting.Light{ID: ids.NodeID{Lo: uint64(i + 1)}, Color: mathx.White, Intensity: 1, Height: 1, Range: 10, Layers: 1})
	}
	f := dev.BeginFrame(1280, 720)
	r.BeginFrame(f, 1280, 720, 1)
	vf := r.View(worldView())
	if err := vf.Lighting(&in); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		vf.lightSet(mathx.AABB2{}, 1)
	}
	if r.stats.DroppedLights != 2 || r.stats.DroppedLightAssignments != 10 {
		t.Fatalf("unique lights vs receiver assignments: %+v", r.stats)
	}
	r.EndFrame()
	dev.EndFrame()
}

func TestLightingSetSharesBindingsWhenReceiverInfluenceOrderChanges(t *testing.T) {
	r, dev := newTestRenderer(t, 64)
	in := lighting.Input{Lights: []lighting.Light{
		{ID: ids.NodeID{Lo: 20}, Type: lighting.Point, Position: mathx.Vec2{X: -3}, Color: mathx.White, Intensity: 1, Height: 1, Range: 20, Layers: 3},
		{ID: ids.NodeID{Lo: 10}, Type: lighting.Point, Position: mathx.Vec2{X: 3}, Color: mathx.White, Intensity: 1, Height: 1, Range: 20, Layers: 1},
	}}
	left := mathx.AABB2{Min: mathx.Vec2{X: -2}, Max: mathx.Vec2{X: -2}}
	right := mathx.AABB2{Min: mathx.Vec2{X: 2}, Max: mathx.Vec2{X: 2}}
	a, _ := lighting.Select(in.Lights, left, 1)
	b, _ := lighting.Select(in.Lights, right, 1)
	if a.Indices[0] == b.Indices[0] || a.Count != 2 || b.Count != 2 {
		t.Fatal("fixture must reverse influence order while retaining both lights")
	}
	f := dev.BeginFrame(1280, 720)
	if err := r.BeginFrame(f, 1280, 720, 1); err != nil {
		t.Fatal(err)
	}
	vf := r.View(worldView())
	if err := vf.Lighting(&in); err != nil {
		t.Fatal(err)
	}
	first, second := vf.lightSet(left, 1), vf.lightSet(right, 1)
	if first != second || len(vf.sets) != 1 || r.stats.LightSets != 1 {
		t.Fatalf("same membership split bindings: %d/%d, sets=%d", first, second, len(vf.sets))
	}
	if vf.sets[0].Indices[0] != 1 || vf.sets[0].Indices[1] != 0 {
		t.Fatal("published light order must follow stable identity")
	}
	if only := vf.lightSet(left, 2); only == first || vf.sets[only-1].Count != 1 {
		t.Fatal("a different receiver mask must retain its own light bindings")
	}
	r.EndFrame()
	dev.EndFrame()
}
