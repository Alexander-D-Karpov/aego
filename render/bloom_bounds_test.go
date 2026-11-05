package render

import (
	"math"
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render/lighting"
	"aego/render/material"
	"aego/render/postfx"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/view"
)

type bloomCoverageDevice struct {
	gpu.Device
	opaque bool
	format gpu.TextureFormat
}

func (d *bloomCoverageDevice) OpaqueRegion(gpu.TextureHandle, float32, float32, float32, float32) bool {
	return d.opaque
}

func (d *bloomCoverageDevice) TextureInfo(gpu.TextureHandle) (gpu.TextureDesc, bool) {
	return gpu.TextureDesc{Format: d.format}, true
}

func TestBloomOpaqueReceiverRequiresProofOfDarkness(t *testing.T) {
	dev := &bloomCoverageDevice{opaque: true, format: gpu.TexSRGBA8}
	env := lighting.DefaultEnvironment()
	env.AmbientIntensity = .4
	input := &lighting.Input{HasEnvironment: true, Environment: env, Lights: []lighting.Light{{Type: lighting.Directional, Color: mathx.White, Intensity: .5}}}
	f := &ViewFrame{r: &Renderer{dev: dev}, brightThreshold: .75, lighting: input, sets: []lighting.Set{{Count: 1}}}
	sh := &material.Shader{StockLit: true, Variants: []material.Variant{{}}, Schema: material.NewSchema([]shader.Param{{Name: "EmissionIntensity", Kind: shader.KindFloat, Instance: true}}, 0, 1)}
	m, ok := material.New("receiver", sh, 0, shader.BlendPremultiplied)
	if !ok {
		t.Fatal("material")
	}
	d := sprite.Draw{Lighting: true, LightSet: 1, Material: m, Tint: mathx.Color{R: .25, G: .25, B: .25, A: 1}}
	check := func(want bool) {
		t.Helper()
		if f.belowBloomThreshold(&d) != want {
			t.Fatalf("below threshold should be %t", want)
		}
	}
	check(true)
	input.Environment.Ambient.R = -.25
	check(false)
	input.Environment.Ambient.R = 1
	input.Lights[0].Direction.X = 100
	check(false)
	input.Lights[0].Direction.X = 1
	d.HasLinear, d.Linear = true, [4]float32{1, 1, 1, 1.00001}
	check(false)
	d.HasLinear = false
	input.Lights[0].Type = lighting.Point
	input.Lights[0].Range, input.Lights[0].Falloff = .00001, 1
	check(false)
	input.Lights[0].Type = lighting.Directional
	input.Lights[0].Intensity = 4
	check(false)
	input.Lights[0].Intensity = .5
	f.brightThreshold = .226
	check(false) // no proof inside the storage-rounding margin
	f.brightThreshold = .75
	sh.StockLit = false
	sh.OpaqueWithOpaqueAlbedo = true
	check(false)
	sh.StockLit = true
	m.Set("EmissionIntensity", [4]float32{2})
	check(false)
	d.Lanes = &[16]float32{}
	check(true) // draw-time override, exactly as uploaded to the instance stream
	d.Lanes[0] = 1
	check(false)
	d.Lanes[0] = 0
	dev.format = gpu.TexRGBA16F
	check(false)
	dev.format = gpu.TexRGBA8
	d.Lanes[4] = float32(math.Inf(1))
	check(false)
	d.Lanes[4] = 0
	dev.opaque = false
	check(false)
	dev.opaque = true
	input.Debug = lighting.DebugNormals
	check(false)
	input.Debug = lighting.DebugLighting
	d.Tint.A = .9
	check(false)
	d.Tint.A = 1
	d.Material, _ = material.New("additive", sh, 0, shader.BlendAdditive)
	check(false)
}

func TestBloomAuxiliaryAndFinalCoverageAreIndependent(t *testing.T) {
	r, _ := newTestRenderer(t, 4)
	v := view.ScreenView(400, 200, 1)
	v.DoClear, v.Clear.A = true, 1
	full := v.Viewport
	reg := postfx.NewRegistry()
	e, _ := reg.NewInstance(postfx.BloomID, "bloom")
	var plan postfx.Plan
	if err := reg.Build(&plan, true, []postfx.Instance{e}, postfx.Profile{}); err != nil {
		t.Fatal(err)
	}
	o := &lightingOutput{info: OutputInfo{Width: 400, Height: 200}, fxRect: full, clears: []mathx.Rect{full}}
	f := &ViewFrame{r: r, output: o, view: v, trackResolveBounds: true, hasResolveBounds: true,
		resolveBounds:   mathx.Rect{X: 15, Y: 15, W: 200, H: 110}.ToAABB(),
		hasBrightBounds: true, brightBounds: mathx.Rect{X: 205, Y: 60, W: 10, H: 9}.ToAABB()}
	f.PostProcess(&plan)
	pass := &plan.Passes[0]
	scene := r.auxiliaryScissor(f, pass)
	bright := r.auxiliaryBounds(f, pass, true)
	if !(scene.W > bright.W && bright.W > 0 && scene.H > bright.H && bright.H > 0) {
		t.Fatalf("scene %v, bright %v", scene, bright)
	}
	f.submission++
	f.PostProcess(&plan) // earlier submissions were not classified for a new stack
	if r.auxiliaryBounds(f, pass, true) != scene {
		t.Fatal("late stack change reused incomplete brightness information")
	}
}

func TestSceneGroupsCoverAllSpritesAfterCoalescing(t *testing.T) {
	var groups sceneGroups
	var rects []mathx.AABB2
	for i := 0; i < 40; i++ {
		rect := mathx.Rect{X: float32((i*29)%87 - 20), Y: float32((i * 13) % 47), W: float32(1 + i%13), H: float32(1 + i%8)}.ToAABB()
		rects = append(rects, rect)
		groups.add(rect)
	}
	if groups.count != len(groups.box) {
		t.Fatal(groups)
	}
	for _, rect := range rects {
		covered := false
		for _, group := range groups.box[:groups.count] {
			covered = covered || group.Contains(rect.Min) && group.Contains(rect.Max)
		}
		if !covered {
			t.Fatalf("lost %v in %v", rect, groups)
		}
	}
}

// Pool pixels may be stale: clamped linear-filter taps must land in the write footprint, and clamping may move only centers whose corresponding channel was already black.
func TestBloomSampleWindowNeverReadsStalePoolPixels(t *testing.T) {
	for _, width := range []int{63, 64, 129} {
		half := (width + 1) / 2
		for lo := 0; lo < half; lo++ {
			for hi := lo + 1; hi <= half; hi++ {
				window := mathx.Rect{X: float32(lo), W: float32(hi - lo), H: 1}
				dst := mathx.Rect{W: float32(half), H: 1}
				guard, writes := auxSampleWindow(window, dst)
				for i := 0; i < width; i++ {
					x := mathx.Clamp((float32(i)+0.5)*float32(half)/float32(width), 0.5, float32(half)-0.5)
					y := mathx.Clamp(x, guard.X, guard.X+guard.W)
					a, b := int(math.Floor(float64(y-.5))), int(math.Ceil(float64(y-.5)))
					if a < int(writes.X) || b >= int(writes.X+writes.W) {
						t.Fatalf("sample %f needs [%d,%d] outside %v", y, a, b, writes)
					}
					if y != x && !((x+.5 <= float32(lo) && y+.5 <= float32(lo)) || (x-.5 >= float32(hi-1) && y-.5 >= float32(hi-1))) {
						t.Fatalf("clamp %f -> %f can touch [%d,%d)", x, y, lo, hi)
					}
				}
			}
		}
	}
}
