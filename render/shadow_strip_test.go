package render

import (
	"encoding/binary"
	"math"
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/render/lighting"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/view"
)

func TestShadowStripFollowsRenderedGeometryAndSoftness(t *testing.T) {
	r, dev := newTestRenderer(t, 64)
	draws := testDraws(t, r, 1, 0)
	sh := &material.Shader{Name: "shadow-strip", Domain: shader.DomainSprite, Features: shader.FeatureAffine | shader.FeatureLighting,
		Schema: material.NewSchema(nil, 0, 0), Variants: []material.Variant{{Sources: [4]string{"v", "f", "v", "f"}}}}
	m, ok := material.New("shadow strip", sh, 0, shader.BlendPremultiplied)
	if !ok {
		t.Fatal("material")
	}
	draws[0].Material = m
	draws[0].VisibilityLayers = 1
	env := lighting.DefaultEnvironment()
	env.ColorMode, env.ShadowResolution, env.MaxShadowedLights = lighting.LDR, 128, 1
	in := lighting.Input{HasEnvironment: true, Environment: env, Lights: []lighting.Light{{ID: ids.NodeID{Lo: 1}, Type: lighting.Point, Direction: mathx.Vec2{X: 1},
		Color: mathx.White, Intensity: 1, Range: 10, Height: 2, Falloff: 1, Layers: 1, Shadows: true, ShadowCasterMask: 1}}}
	v := worldView()
	v.ID, v.OutputID = view.ID{Scope: 841}, view.ID{Scope: 842}
	v.DoClear = true
	for i, tc := range []struct {
		y, soft, minimum float32
		enabled          bool
	}{{5, 1, .75, true}, {5, 1, .75, true}, {5, 3.5, .75, true}, {5, 5, .75, true}, {-5, 1, 0, true}, {5, 1, 0, false}} {
		in.Lights[0].ShadowSoftness = tc.soft
		in.Lights[0].Shadows = tc.enabled
		in.Points = []mathx.Vec2{{X: -5, Y: tc.y}, {X: 5, Y: tc.y}}
		in.Occluders = []lighting.Occluder{{ID: ids.NodeID{Lo: 2}, Layers: 1, Count: 2, GeometryRevision: 1, TransformRevision: uint64(math.Float32bits(tc.y))}}
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
		if len(r.lightState.uploadedSets) == 0 || r.stats.LightingFailures != 0 {
			t.Fatalf("no light binding at step %d", i)
		}
		got := math.Float32frombits(binary.LittleEndian.Uint32(r.lightState.uploadedSets[0][532:]))
		want := max(0, tc.minimum-(min(tc.soft, 4)+2)/128)
		if got != want {
			t.Fatalf("step %d: clear bound %g, want %g", i, got, want)
		}
		if i == 1 && r.stats.ShadowCacheHits == 0 {
			t.Fatal("unchanged mask did not reuse its bound")
		}
	}
}
