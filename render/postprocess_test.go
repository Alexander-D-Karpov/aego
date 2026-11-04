package render

import (
	"math"
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/render/postfx"
	"aego/render/view"
)

func TestPostProcessBypassAndPoolLifetime(t *testing.T) {
	r, d := newTestRenderer(t, 8)
	v := view.ScreenView(17, 19, 1)
	v.OutputID = view.ID{Scope: 14}
	v.DoClear, v.Clear.A = true, 1
	draw := func(p *postfx.Plan) Stats {
		fr := d.BeginFrame(17, 19)
		if err := r.BeginFrame(fr, 17, 19, 1); err != nil {
			t.Fatal(err)
		}
		f := r.View(v)
		f.PostProcess(p)
		f.End()
		s := r.EndFrame()
		d.EndFrame()
		return s
	}
	old := testing.AllocsPerRun(20, func() { draw(nil) })
	res := d.ResourceStats()
	empty := postfx.Plan{}
	newAllocs := testing.AllocsPerRun(20, func() { draw(&empty) })
	if newAllocs != old || r.fx.programs != nil || res != d.ResourceStats() {
		t.Fatalf("bypass: allocs %v -> %v, resources %+v -> %+v", old, newAllocs, res, d.ResourceStats())
	}
	vignette, _ := postfx.NewRegistry().Lookup(postfx.VignetteID)
	p := postfx.Pass{Descriptor: vignette}
	plan := postfx.Plan{Passes: []postfx.Pass{p, p, p, p, p}}
	if s := draw(&plan); s.PostFXTargets != 2 || s.PostFXPasses != 5 || s.ResolvePasses != 1 {
		t.Fatalf("five-pass stats %+v err=%v", s, r.PostProcessError())
	}
	warm := d.ResourceStats()
	o := r.lightState.outputs[v.OutputID]
	if o.fxPool.Free() != 2 || o.fxPool.Live() != 2 {
		t.Fatal("scratch targets not returned")
	}
	for i := 0; i < 10; i++ {
		draw(&plan)
	}
	if warm != d.ResourceStats() {
		t.Fatalf("warmed resources changed: %+v -> %+v", warm, d.ResourceStats())
	}
	if err := r.ReleaseLightingView(v.ID, v.OutputID); err != nil {
		t.Fatal(err)
	}
	if now := d.ResourceStats(); now.Textures != res.Textures {
		t.Fatalf("preview release left scratch or scene texture: %d -> %d", res.Textures, now.Textures)
	}
}

func TestInspectionRecordsPhysicalBloomPassesOnRequestedOutput(t *testing.T) {
	r, d := newTestRenderer(t, 8)
	v := view.ScreenView(37, 29, 1)
	v.OutputID = view.ID{Scope: 77}
	v.DoClear, v.Clear.A = true, 1
	reg := postfx.NewRegistry()
	e, err := reg.NewInstance(postfx.BloomID, "lamp")
	if err != nil {
		t.Fatal(err)
	}
	var plan postfx.Plan
	if err := reg.Build(&plan, true, []postfx.Instance{e}, postfx.Profile{}); err != nil {
		t.Fatal(err)
	}
	draw := func() {
		if err := r.BeginFrame(d.BeginFrame(37, 29), 37, 29, 1); err != nil {
			t.Fatal(err)
		}
		f := r.View(v)
		f.PostProcess(&plan)
		f.End()
		r.EndFrame()
		d.EndFrame()
	}
	draw()
	if len(r.InspectOutputPasses()) != 0 {
		t.Fatal("captured without opt-in")
	}
	draw()
	rows := r.InspectOutputPasses()
	count := 0
	for _, row := range rows {
		if row.Output != v.OutputID {
			t.Fatal(row)
		}
		if strings.HasPrefix(row.Label, postfx.BloomID) {
			if row.Width <= 0 || row.Height <= 0 || row.InstanceID != "lamp" {
				t.Fatal(row)
			}
			count++
		}
	}
	if count != 4 {
		t.Fatalf("expected extract, two blur passes and composite: %+v", rows)
	}
}

func TestBloomBoundsRequireKnownBlackSupportAndFullCameraOwnership(t *testing.T) {
	r, _ := newTestRenderer(t, 8)
	const width, height = 487, 301
	vp := mathx.Rect{W: width, H: height}
	v := view.ScreenView(width, height, 1)
	v.DoClear, v.Clear.A = true, 1
	o := &lightingOutput{info: OutputInfo{Width: width, Height: height}, opaque: true, fxOpaque: true, fxRect: vp, fxRegions: []mathx.Rect{vp}, clears: []mathx.Rect{vp}}
	f := &ViewFrame{view: v, output: o, trackResolveBounds: true, hasResolveBounds: true, resolveBounds: mathx.AABB2{Min: mathx.V2(10, 10), Max: mathx.V2(20, 20)}}
	reg := postfx.NewRegistry()
	e, _ := reg.NewInstance(postfx.BloomID, "lamp")
	var plan postfx.Plan
	if err := reg.Build(&plan, true, []postfx.Instance{e}, postfx.Profile{}); err != nil {
		t.Fatal(err)
	}
	pass := &plan.Passes[0]
	if clip := r.auxiliaryScissor(f, pass); clip.W <= 0 || clip.H <= 0 || clip.X < 0 || clip.Y < 0 || clip.X+clip.W > 244 || clip.Y+clip.H > 151 {
		t.Fatal(clip)
	}
	if !replacePostClear(o, pass.Descriptor) {
		t.Fatal("full covered Bloom did not replace its clear")
	}
	if replacePostClear(o, &postfx.Descriptor{}) {
		t.Fatal("unqualified program discarded the clear")
	}
	f.view.Clear.R = 1
	if r.auxiliaryScissor(f, pass).W != 0 {
		t.Fatal("near-threshold background clipped")
	}
	f.view.Clear.R = 0
	f.trackResolveBounds = false
	if r.auxiliaryScissor(f, pass).W != 0 {
		t.Fatal("unknown geometry clipped")
	}
	f.trackResolveBounds = true
	o.clears[0].W--
	if replacePostClear(o, pass.Descriptor) || r.auxiliaryScissor(f, pass).W != 0 {
		t.Fatal("partial camera clear incorrectly treated as full")
	}
}

func TestBloomIntermediateScissorsCoverEveryFilteredRead(t *testing.T) {
	desc, _ := postfx.NewRegistry().Lookup(postfx.BloomID)
	for _, full := range []mathx.Rect{{W: 65, H: 47}, {W: 64, H: 48}} {
		dst := mathx.Rect{W: float32((int(full.W) + 1) / 2), H: float32((int(full.H) + 1) / 2)}
		for _, clip := range []mathx.Rect{{X: 8, Y: 7, W: 6, H: 5}, {X: 0, Y: 0, W: 3, H: 4}, {X: dst.W - 2, Y: dst.H - 3, W: 2, H: 3}, dst} {
			for _, radius := range []float32{0, .5, 2, 7.75, 32} {
				var params [4][4]float32
				params[2][0] = radius
				regions, bounded := auxiliaryRegions(desc, params, clip, dst, full)
				if !bounded || regions[2] != clip {
					t.Fatal("missing last clear footprint", regions)
				}
				for i := 1; i <= 2; i++ {
					consumer, written := regions[i], regions[i-1]
					shiftX, shiftY := desc.HalfReadOffset(i, params)
					for y := int(consumer.Y); y < int(consumer.Y+consumer.H); y++ {
						for x := int(consumer.X); x < int(consumer.X+consumer.W); x++ {
							for _, tap := range []float32{-1, -.5, 0, .5, 1} {
								xx := float64(float32(x) + tap*shiftX*dst.W/full.W)
								yy := float64(float32(y) + tap*shiftY*dst.H/full.H)
								for _, sx := range []int{int(math.Floor(xx)), int(math.Ceil(xx))} {
									for _, sy := range []int{int(math.Floor(yy)), int(math.Ceil(yy))} {
										px := float32(max(0, min(sx, int(dst.W)-1)))
										py := float32(max(0, min(sy, int(dst.H)-1)))
										if px < written.X || py < written.Y || px >= written.X+written.W || py >= written.Y+written.H {
											t.Fatalf("step %d reads stale pixel (%v,%v) outside %+v", i, px, py, written)
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	copy := *desc
	copy.HalfReadOffset = nil
	clip := mathx.Rect{X: 1, Y: 2, W: 5, H: 5}
	regions, bounded := auxiliaryRegions(&copy, [4][4]float32{}, clip, mathx.Rect{W: 16, H: 16}, mathx.Rect{W: 32, H: 32})
	if bounded || regions[0] != clip || regions[1] != clip || regions[2] != clip {
		t.Fatal("uncertified passes must retain their original clear policy")
	}
}
