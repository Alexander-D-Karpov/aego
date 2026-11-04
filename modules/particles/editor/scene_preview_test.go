package editor

import (
	"testing"

	"aego/asset/loader"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/gizmo"
	"aego/editor/core/selection"
	"aego/editor/ext"
	"aego/editor/ext/inspect"
	"aego/engine"
	"aego/engine/module"
	"aego/engine/res"
	"aego/gpu/null"
	"aego/modules/particles"
	"aego/render"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/vfs"
)

func TestManualDustRecoversWhenFirstConfigurationIsInvalid(t *testing.T) {
	d, _, _ := inspectorFixture(t)
	ed := &context.Editor{Types: d.Scene().Types(), Loader: loader.New(nil, nil, nil)}
	c := particles.DefaultParticleSystem2D()
	c.PlayOnStart = false
	c.Rate = 0
	c.MaxParticles = 0
	var v transport
	v.configure(ed, c, true)
	if v.bad == "" {
		t.Fatal("invalid config accepted")
	}
	c.MaxParticles = 16
	v.configure(ed, c, false)
	if v.bad != "" || v.sim.Count() != 8 {
		t.Fatalf("first repaired manual effect: count=%d, diagnostic=%q", v.sim.Count(), v.bad)
	}
}
func TestEditorPreviewWithoutLoader(t *testing.T) {
	d, _, _ := inspectorFixture(t)
	ed := &context.Editor{Types: d.Scene().Types()}
	c := particles.DefaultParticleSystem2D()
	var v transport
	v.configure(ed, c, true)
	if v.bad != "" {
		t.Fatal(v.bad)
	}
}

func TestScenePreviewReadsOnlyTheVisibleActiveWorld(t *testing.T) {
	d, h, _ := inspectorFixture(t)
	sc := d.Scene()
	dev := null.New()
	defer dev.Close()
	fs, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	rnd, err := render.New(dev, dev.Caps(), fs, log.Nop(), render.Options{MaxSprites: 128})
	if err != nil {
		t.Fatal(err)
	}
	defer rnd.Close()
	ld := loader.New(dev, rnd.Textures(), log.Nop())
	docs := document.NewManager(document.Options{Types: sc.Types()})
	doc := docs.New("demo.aescene", sc)
	ed := &context.Editor{Docs: docs, Types: sc.Types(), Renderer: rnd, Loader: ld, Selection: selection.New()}
	inspectors := inspect.NewRegistry()
	if err := inspectors.Load(sc.Registry()); err != nil {
		t.Fatal(err)
	}
	ex := ext.New(ext.Deps{Editor: ed, Schema: sc.Registry(), Inspectors: inspectors, Gizmos: gizmo.NewRegistry()})
	if err = ex.Run(particles.ID, Hook); err != nil {
		t.Fatal(err)
	}
	c, _ := scene.Get[particles.ParticleSystem2D](sc, h)
	c.Rate, c.PlayOnStart = 0, false
	sc.SetLayers(h, 1<<50)
	sc.SetPosition(h, mathx.V2(30, 30))
	ed.Select(sc.ID(h))
	r := frame.New()
	r.Bind(sc, rnd, ld)
	defer r.Close()
	draw := func(s *scene.Scene, layers uint64) int {
		r.Bind(s, rnd, ld)
		r.Prepare(100, 100, 1)
		v := view.ScreenView(100, 100, 1)
		v.Layers = view.LayerMask(layers)
		r.AddView(v, 0)
		ex.RunTicks()
		ed.ScenePreviews[0](r)
		return len(r.Bucket(0).Sprites)
	}
	if n := draw(sc, 1<<50); n != 8 {
		t.Fatalf("edit=%d", n)
	}
	if n := draw(sc, 1); n != 0 {
		t.Errorf("excluded high-bit emitter generated %d draws", n)
	}
	r.SetHidden(func(scene.NodeHandle) bool { return true })
	if n := draw(sc, 1<<50); n != 0 {
		t.Errorf("isolated emitter generated %d draws", n)
	}
	r.SetHidden(nil)
	sc.SetEnabled(h, false)
	if n := draw(sc, 1<<50); n != 0 {
		t.Errorf("disabled emitter generated %d draws", n)
	}
	sc.SetEnabled(h, true)
	// Give the Play scene a fresh world and a different live count.
	ps := scene.New(sc.Types(), ld, scene.Options{})
	ph := ps.CreateWithID(ps.Root(), "Dust", sc.ID(h))
	*scene.Add[particles.ParticleSystem2D](ps, ph) = *c
	ps.SetLayers(ph, 1<<50)
	ps.SetPosition(ph, mathx.V2(30, 30))
	ps.Flush()
	ps.UpdateTransforms()
	w := engine.NewWorld("play")
	m := particles.New()
	ac := &module.AttachContext{World: w, Scene: ps, Play: true}
	if err = m.AttachWorld(ac); err != nil {
		t.Fatal(err)
	}
	defer m.DetachWorld(ac)
	fx := particles.Of(w)
	fx.Burst(ph, 3)
	docs.SetPlay(doc.PlayCopy(ps), w)
	if n := draw(ps, 1<<50); n != 3 {
		t.Errorf("playing view shows %d particles, expected actual three", n)
	}
	ed.Select()
	if n := draw(ps, 1<<50); n != 3 {
		t.Errorf("deselection hid live emitter; got %d", n)
	}
	if fx.Count(ph) != 3 || fx.Stats().Submitted != 0 {
		t.Errorf("scene view advanced or charged runtime metrics: %+v", fx.Stats())
	}
	fx.SetEnabled(false)
	ed.Select(ps.ID(ph))
	if n := draw(ps, 1<<50); n != 0 {
		t.Errorf("disabled world drew %d phantom particles", n)
	}
}

func TestSparseThumbnailSamplesAnActualBirth(t *testing.T) {
	ed := &context.Editor{}
	for _, tc := range []struct {
		name     string
		rate     float32
		bursts   []mathx.Vec2
		duration float32
		count    int
	}{{"delayed", 0, []mathx.Vec2{{X: .8, Y: 3}}, 1, 3}, {"sparse", .125, nil, 10, 1}, {"too short for a rate birth", .125, nil, 1, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			c := particles.DefaultParticleSystem2D()
			c.Loop = false
			c.Duration = tc.duration
			c.Bursts = tc.bursts
			c.Lifetime = mathx.V2(.02, .02)
			c.Rate = tc.rate
			var v transport
			defer v.close()
			v.configure(ed, c, true)
			v.sampleThumbnail()
			if n := v.sim.Count(); n != tc.count {
				t.Fatalf("sampled %d live particles, want %d", n, tc.count)
			}
		})
	}
}
