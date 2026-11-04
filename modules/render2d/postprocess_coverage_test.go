package render2d

import (
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/module"
	"aego/engine/settings"
	"aego/render/postfx"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/schema"
	"strings"
	"testing"
)

// Create the overlay first: draw order, not component/storage order, is decisive.
func TestPostProcessComposedCameraCoverage(t *testing.T) {
	f := lightingTestWorld(t)
	overlay := f.camera("Detail layer", ^uint64(0))
	background := f.camera("Background", ^uint64(0))
	top, _ := scene.Get[Camera2D](f.s, overlay)
	top.Order, top.Clear = 3, false
	reg := postfx.NewRegistry()
	bloom, _ := reg.NewInstance(postfx.BloomID, "halo")
	vignette, _ := reg.NewInstance(postfx.VignetteID, "grade")
	*scene.Add[PostProcess](f.s, overlay) = PostProcess{Enabled: true, Effects: []postfx.Instance{bloom, vignette}}
	check := func(want postfx.State, strict bool) {
		t.Helper()
		f.tick()
		for i, v := range f.r.Views() {
			if v.ID.Node == f.s.ID(overlay) {
				p := f.r.Bucket(i).PostProcess
				if p == nil || len(p.States) != 2 || p.States[0].State != want || p.States[1].State != postfx.Active {
					t.Fatalf("runtime %+v, want Bloom state %v", p, want)
				}
			}
		}
		issues := module.ValidateScene(f.s, nil)
		for _, d := range issues {
			if d.Code == "PFX001" {
				if strict {
					t.Fatalf("export rejected composed camera: %+v", d)
				}
				return
			}
		}
		if !strict {
			t.Fatal("strict export accepted output with uncovered pixels")
		}
	}
	check(postfx.Active, true)
	top, _ = scene.Get[Camera2D](f.s, overlay)
	bottom, _ := scene.Get[Camera2D](f.s, background)
	top.Clear = true
	top.ClearColor.A = .5
	check(postfx.Unsupported, false)
	bottom.Order = 4
	check(postfx.Active, true) // a later opaque clear restores the covered region
	top.Clear = false
	bottom.Order = 0
	bottom.Viewport = mathx.Rect{W: .5, H: 1}
	check(postfx.Unsupported, false) // uncovered right half
	top.Viewport = bottom.Viewport
	check(postfx.Active, true)
	// The default camera grouping can change in extraction; decide against
	// actual logical outputs, not a shared attachment (or matching extents).
	views := f.r.Views()
	for i := range views {
		if views[i].ID.Node == f.s.ID(background) {
			views[i].OutputID = view.ID{Scope: 77}
		} else {
			views[i].OutputPolicy = view.OutputLinear
		}
	}
	extract{reg}.Run(&engine.Context{World: f.w})
	for i, v := range views {
		if v.ID.Node == f.s.ID(overlay) {
			p := f.r.Bucket(i).PostProcess
			if p == nil || len(p.Passes) != 0 || p.States[0].State != postfx.Unsupported || p.States[1].State != postfx.Unsupported {
				t.Fatalf("foreign clear or display effect reached linear output: %+v", p)
			}
		}
	}
}

func TestPostEffectObservationUsesEffectivePlan(t *testing.T) {
	f := lightingTestWorld(t)
	cam := f.camera("Camera", ^uint64(0))
	reg := postfx.NewRegistry()
	v, _ := reg.NewInstance(postfx.VignetteID, "grade")
	b, _ := reg.NewInstance(postfx.BloomID, "halo")
	*scene.Add[PostProcess](f.s, cam) = PostProcess{Enabled: true, Effects: []postfx.Instance{b, v}}
	f.tick()
	id := f.s.ID(cam)
	get := func(want postfx.State, reason string) {
		t.Helper()
		st, ok := ObservedPostEffect(f.w, f.r.OutputID(), id, "halo", frame.PostPreview{})
		if !ok || st.State != want || !strings.Contains(st.Reason, reason) {
			t.Fatalf("observation %+v, %v", st, ok)
		}
	}
	get(postfx.Active, "")
	st, ok := ObservedPostEffect(f.w, f.r.OutputID(), id, "halo", frame.PostPreview{Camera: id, Instance: "halo"})
	if !ok || st.Reason != "preview bypass" {
		t.Fatal("preview did not follow selected instance", st)
	}
	if _, ok = ObservedPostEffect(f.w, view.ID{Scope: 999}, id, "halo", frame.PostPreview{}); ok {
		t.Fatal("foreign output observed")
	}
	c, _ := scene.Get[PostProcess](f.s, cam)
	c.Effects = append(c.Effects, postfx.Instance{TypeID: "missing.type", InstanceID: "unavailable", Enabled: true, Version: 1})
	f.tick()
	get(postfx.Invalid, "missing.type") // The locally valid Bloom did not execute.
	c.Effects[2].Enabled = false
	f.tick()
	unknown, ok := ObservedPostEffect(f.w, f.r.OutputID(), id, "unavailable", frame.PostPreview{})
	if !ok || unknown.State != postfx.Unknown {
		t.Fatal("unknown disabled instance lost", unknown)
	}
	lighting, _ := engine.ServiceOf[*lightingState](f.w, lightingServiceID)
	lighting.settings = settings.New()
	k, err := lighting.settings.Register(settings.Desc{ID: PostProcessingKey, Owner: ID, Kind: schema.KindBool, Default: schema.Bool(true)})
	if err != nil {
		t.Fatal(err)
	}
	lighting.postProcessingKey = k
	if err := lighting.settings.Override(k, schema.Bool(false)); err != nil {
		t.Fatal(err)
	}
	f.tick()
	get(postfx.Disabled, "setting")
	if err := lighting.settings.Override(k, schema.Bool(true)); err != nil {
		t.Fatal(err)
	}
	motion, err := lighting.settings.Register(settings.Desc{ID: "accessibility.reducedMotion", Owner: "accessibility", Kind: schema.KindBool, Default: schema.Bool(true)})
	if err != nil {
		t.Fatal(err)
	}
	lighting.reducedKey = motion
	chromatic, _ := reg.NewInstance(postfx.ChromaticID, "split")
	c.Effects = append(c.Effects, chromatic)
	f.tick()
	split, ok := ObservedPostEffect(f.w, f.r.OutputID(), id, "split", frame.PostPreview{})
	if !ok || split.State != postfx.Disabled || split.Reason != "reduced motion" || !c.Effects[len(c.Effects)-1].Enabled {
		t.Fatal("player accessibility changed authored state or hid bypass reason", split)
	}
	second := f.camera("Conflict", ^uint64(0))
	scene.Add[PostProcess](f.s, second).Enabled = true
	f.tick()
	get(postfx.Invalid, "conflicting")
}
