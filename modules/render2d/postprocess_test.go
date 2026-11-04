package render2d

import (
	"slices"
	"testing"

	"aego/schema"

	"aego/engine/settings"

	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/render/postfx"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/propref"
	"aego/scene/sceneio"
)

func TestPostProcessPropertySurvivesReorderButNotDeletion(t *testing.T) {
	f := lightingTestWorld(t)
	h := f.camera("camera", ^uint64(0))
	reg := postfx.NewRegistry()
	a, _ := reg.NewInstance(postfx.VignetteID, "a")
	b, _ := reg.NewInstance(postfx.VignetteID, "b")
	*scene.Add[PostProcess](f.s, h) = PostProcess{Enabled: true, Effects: []postfx.Instance{a, b}}
	f.s.Flush()
	r := propref.New(f.s, nil)
	ref, err := r.Resolve(h, "render2d.PostProcess.a:Intensity")
	if err != nil {
		t.Fatal(err)
	}
	c, _ := scene.Get[PostProcess](f.s, h)
	slices.Reverse(c.Effects)
	if !r.Write(&ref, h, [4]float32{.8}) || c.Effects[0].Parameters[0].Value.X != .35 || c.Effects[1].Parameters[0].Value.X != .8 {
		t.Fatalf("reorder retargeted the track: %+v", c)
	}
	if r.Write(&ref, h, [4]float32{1.2}) || c.Effects[1].Parameters[0].Value.X != .8 {
		t.Fatal("out-of-range write changed the stack")
	}
	c.Effects = c.Effects[:1]
	if r.Write(&ref, h, [4]float32{.2}) {
		t.Fatal("deleted track targeted its neighbor")
	}
}

func TestPostProcessUnknownPreservedAndStrictValidation(t *testing.T) {
	f := lightingTestWorld(t)
	h := f.camera("camera", ^uint64(0))
	reg := postfx.NewRegistry()
	e, _ := reg.NewInstance(postfx.VignetteID, "stable")
	e.Enabled = false
	e.TypeID = "game.future"
	e.Version = 42
	*scene.Add[PostProcess](f.s, h) = PostProcess{Enabled: true, Effects: []postfx.Instance{e}}
	f.s.Flush()
	loaded, _, err := sceneio.Load(sceneio.Save(f.s), "roundtrip.aescene", f.s.Types(), nil, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	c, _ := scene.Get[PostProcess](loaded, loaded.Handle(f.s.ID(h)))
	item := c.Effects[0]
	if item.TypeID != e.TypeID || item.InstanceID != e.InstanceID || item.Enabled || item.Version != 42 || !slices.Equal(item.Parameters, e.Parameters) {
		t.Fatalf("unknown effect lost data: %+v", item)
	}
	if err := reg.ValidateForOutput(c.Enabled, c.Effects, postfx.Profile{}); err != nil {
		t.Fatal(err)
	}
	c.Effects[0].Enabled = true
	if err := reg.ValidateForOutput(c.Enabled, c.Effects, postfx.Profile{}); err == nil {
		t.Fatal("required unknown effect was accepted")
	}
}

func TestPostProcessCameraOwnsItsActualOutput(t *testing.T) {
	f := lightingTestWorld(t)
	a, b := f.camera("cam", ^uint64(0)), f.camera("cam", ^uint64(0))
	reg := postfx.NewRegistry()
	e, _ := reg.NewInstance(postfx.VignetteID, "vignette")
	*scene.Add[PostProcess](f.s, a) = PostProcess{Enabled: true, Effects: []postfx.Instance{e}}
	*scene.Add[PostProcess](f.s, b) = PostProcess{Enabled: true}
	f.tick()
	ds, _ := LightingDiagnostics(f.w)
	hasConflict := func(ds []problems.Diagnostic) bool {
		return slices.ContainsFunc(ds, func(d problems.Diagnostic) bool { return d.Code == "PFX002" })
	}
	if !hasConflict(ds) {
		t.Fatal("empty active second stack didn't claim output")
	}
	ctx := &engine.Context{World: f.w}
	f.r.Prepare(400, 300, 1)
	cameras{}.Run(ctx)
	views := f.r.Views()
	out := view.ID{Scope: 57}
	views[1].OutputID = out
	extract{reg}.Run(ctx)
	ds, _ = LightingDiagnostics(f.w)
	if hasConflict(ds) {
		t.Fatal("same camera labels on independent output produced a conflict")
	}
	var i int
	if views[1].ID.Node == f.s.ID(a) {
		i = 1
	}
	p := f.r.Bucket(i).PostProcess
	if p == nil || len(p.Passes) != 1 || !f.r.Bucket(1-i).PostProcessOwner || f.r.Bucket(1-i).PostProcess != nil {
		t.Fatal("wrong plan on a camera")
	}
	views[i].OutputPolicy = view.OutputLinear
	extract{reg}.Run(ctx)
	if len(p.Passes) != 0 || p.States[0].State != postfx.Unsupported {
		t.Fatal("display effect reached a linear output")
	}
	c, _ := scene.Get[PostProcess](f.s, a)
	c.Enabled = false
	f.r.SetFixedView(view.View{})
	f.tick()
	if f.r.Bucket(0).PostProcessOwner {
		t.Fatal("synthetic Scene View claimed a camera stack")
	}
	state, _ := engine.ServiceOf[*lightingState](f.w, lightingServiceID)
	if len(state.postProcess) != 0 {
		t.Fatal("stale camera plan retained after removal from view")
	}
}

func TestPostProcessNumericUpdatesReusePlan(t *testing.T) {
	f := lightingTestWorld(t)
	h := f.camera("camera", ^uint64(0))
	reg := postfx.NewRegistry()
	e, _ := reg.NewInstance(postfx.VignetteID, "v")
	*scene.Add[PostProcess](f.s, h) = PostProcess{Enabled: true, Effects: []postfx.Instance{e}}
	f.tick()
	st, _ := engine.ServiceOf[*lightingState](f.w, lightingServiceID)
	c, _ := scene.Get[PostProcess](f.s, h)
	update := func() { p := &c.Effects[0].Parameters[0]; p.Value.X = 1 - p.Value.X; st.preparePostProcess(f.r, reg) }
	update()
	p := f.r.Bucket(0).PostProcess
	first := &p.Passes[0]
	cached := st.postProcess[f.r.Views()[0].ID]
	builds := cached.builds
	if n := testing.AllocsPerRun(50, update); n != 0 {
		t.Fatalf("warmed numeric edits allocated %.1f objects", n)
	}
	if cached.builds != builds {
		t.Fatal("uniform update rebuilt the structural plan")
	}
	if f.r.Bucket(0).PostProcess != p || &p.Passes[0] != first || p.Passes[0].Params[0][0] != c.Effects[0].Parameters[0].Value.X {
		t.Fatal("numeric edit replaced the pass or retained stale uniforms")
	}
	for _, tc := range []struct {
		n    float32
		want int
	}{{0, 0}, {.75, 1}} {
		c.Effects[0].Parameters[0].Value.X = tc.n
		st.preparePostProcess(f.r, reg)
		if len(p.Passes) != tc.want {
			t.Fatal("neutral state did not change the physical plan")
		}
	}
	if cached.builds != builds+2 {
		t.Fatal("bypass transitions did not rebuild")
	}
}

type testPostExtension struct{}

func (testPostExtension) Info() module.Info { return module.Info{ID: "screenpalette", Version: "1"} }
func (testPostExtension) Register(r *module.Registry) error {
	r.Provide(postfx.ProvisionKind, postfx.Descriptor{ID: "screenpalette.identity", Label: "Palette", Version: 1, Phase: postfx.DisplayLinear, Fragment: "vec4 aego_effect(vec4 c, vec2 uv, vec2 size, vec4 p[4]){return c;}"})
	return nil
}

func TestPostProcessModuleProvisionSharedByStrictValidatorAndRuntime(t *testing.T) {
	rendering := New()
	for _, extension := range []bool{true, false} {
		types, err := base.NewTypes()
		if err != nil {
			t.Fatal(err)
		}
		modules := []module.Module{rendering}
		if extension {
			modules = append(modules, testPostExtension{})
		}
		man, err := module.Load(types, nil, modules...)
		if err != nil {
			t.Fatal(err)
		}
		if err = types.Schema().Freeze(); err != nil {
			t.Fatal(err)
		}
		sc := scene.New(types, nil, scene.Options{})
		h := sc.Create(sc.Root(), "Camera")
		*scene.Add[Camera2D](sc, h) = DefaultCamera2D()
		*scene.Add[PostProcess](sc, h) = PostProcess{Enabled: true, Effects: []postfx.Instance{{TypeID: "screenpalette.identity", InstanceID: "effect", Version: 1, Enabled: true}}}
		sc.Flush()
		ds := module.ValidateScene(sc, nil)
		found := slices.ContainsFunc(ds, func(d problems.Diagnostic) bool { return d.Code == "PFX001" })
		if found == extension {
			t.Fatalf("extension present=%v: unexpected result from strict scene validator: %+v", extension, ds)
		}
		r := frame.New()
		r.Bind(sc, nil, nil)
		w := engine.NewWorld("test")
		w.SetView(frame.ServiceID, r)
		r.Prepare(32, 32, 1)
		for _, system := range man.Systems {
			if system.ID == SystemCameras || system.ID == SystemExtract {
				system.System.Run(&engine.Context{World: w})
			}
		}
		p := r.Bucket(0).PostProcess
		if active := p != nil && len(p.Passes) == 1; active != extension {
			t.Fatalf("extension present=%v: active pass=%v", extension, active)
		}

	}
}

func TestPostProcessPlayerBypass(t *testing.T) {
	f := lightingTestWorld(t)
	h := f.camera("camera", ^uint64(0))
	reg := postfx.NewRegistry()
	bloom, _ := reg.NewInstance(postfx.BloomID, "bloom")
	author := scene.Add[PostProcess](f.s, h)
	*author = PostProcess{Enabled: true, Effects: []postfx.Instance{bloom}}
	s, _ := engine.ServiceOf[*lightingState](f.w, lightingServiceID)
	s.settings = settings.New()
	k, err := s.settings.Register(settings.Desc{ID: PostProcessingKey, Owner: ID, Kind: schema.KindBool, Default: schema.Bool(true)})
	if err != nil {
		t.Fatal(err)
	}
	s.postProcessingKey = k
	for _, enabled := range []bool{true, false, true} {
		if err := s.settings.Override(k, schema.Bool(enabled)); err != nil {
			t.Fatal(err)
		}
		f.tick()
		b := f.r.Bucket(0)
		if (b.PostProcess != nil) != enabled || b.PostProcessOwner != enabled || !author.Enabled || !author.Effects[0].Enabled {
			t.Fatalf("enabled=%v owner=%v plan=%v author=%+v", enabled, b.PostProcessOwner, b.PostProcess, author)
		}
	}
}

func TestPostProcessCacheDropsDeletedInstanceParameters(t *testing.T) {
	reg := postfx.NewRegistry()
	v, _ := reg.NewInstance(postfx.VignetteID, "grade")
	unknown := postfx.Instance{TypeID: "unloaded", InstanceID: "old", Parameters: []postfx.Parameter{{Name: "retained-name"}}}
	c := PostProcess{Enabled: true, Effects: []postfx.Instance{unknown, v}}
	var p postProcessPlan
	p.copyStack(&c)
	p.reg, p.revision = reg, reg.Revision()
	if err := reg.Build(&p.plan, true, c.Effects, p.profile); err != nil {
		t.Fatal(err)
	}
	c.Effects[0].Parameters = nil
	if !p.refresh(&c, reg, p.profile, nil) {
		t.Fatal("disabled unknown effect should accept a parameter-only update")
	}
	old := p.stack.Effects[0].Parameters
	if cap(old) > 0 && old[:cap(old)][0].Name != "" {
		t.Fatal("numeric refresh kept removed parameter names")
	}
	c.Effects = c.Effects[:1]
	p.copyStack(&c)
	tail := p.stack.Effects[:cap(p.stack.Effects)][1]
	if tail.TypeID != "" || tail.InstanceID != "" || tail.Parameters != nil {
		t.Fatal("cached stack retained a deleted instance")
	}
}
