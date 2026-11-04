package particles

import (
	"aego/scene/sceneio"
	"bytes"
	"testing"

	"aego/core/log"
	"aego/core/mathx"
	"aego/core/rewind"
	"aego/core/rng"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/engine/res"
	"aego/engine/settings"
	"aego/gpu/null"
	"aego/render"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/schema"
	"aego/vfs"
)

func testWorld(t *testing.T, store *settings.Store) (*Service, scene.NodeHandle, *ParticleSystem2D) {
	t.Helper()
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	if err = m.Register(module.New(types, nil)); err != nil {
		t.Fatal(err)
	}
	if err = types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	sc := scene.New(types, nil, scene.Options{})
	h := sc.Create(sc.Root(), "dust")
	c := scene.Add[ParticleSystem2D](sc, h)
	*c = DefaultParticleSystem2D()
	c.PlayOnStart = false
	c.Rate = 0
	c.Space = World
	c.Speed = mathx.V2(0, 0)
	c.Lifetime = mathx.V2(2, 2)
	sc.Flush()
	sc.UpdateTransforms()
	w := engine.NewWorld("test")
	w.Random = rng.New(17)
	m.Init(&module.InitContext{Settings: store})
	ctx := &module.AttachContext{World: w, Scene: sc, Play: true}
	if err = m.AttachWorld(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.DetachWorld(ctx) })
	c, _ = scene.Get[ParticleSystem2D](sc, h)
	return Of(w), h, c
}

func TestTransientWorldCommandsAndRestore(t *testing.T) {
	a, h, _ := testWorld(t, nil)
	b, k, c := testWorld(t, nil)
	c.TimeDomain = Unscaled
	before := a.world.Random.State()
	a.Burst(h, 8)
	if a.Emitting(h) || a.Count(h) != 8 || b.Count(k) != 0 {
		t.Fatalf("independent burst: a=%v b=%v", a.Stats(), b.Stats())
	}
	if a.world.Random.State() != before {
		t.Fatal("presentation consumed gameplay RNG")
	}
	a.step(0, .4)
	b.Burst(k, 1)
	b.step(0, .4)
	if a.systems[h].pool.Particles()[0].Age != 0 || b.systems[k].pool.Particles()[0].Age != .4 {
		t.Fatal("pause domain")
	}
	hasher := rewind.NewHasher()
	old := hasher.Sum()
	a.Hash(hasher)
	if hasher.Sum() != old {
		t.Fatal("particle service modified the gameplay digest")
	}
	key := a.Capture(nil)
	a.Restart(h)
	a.Burst(h, 2)
	a.Restore(key)
	a.world.SetReplaying(true)
	a.Burst(h, 8)
	a.step(.2, .2)
	a.world.SetReplaying(false)
	a.step(.2, .2)
	if a.Count(h) != 0 || b.Count(k) != 1 {
		t.Fatal("seek repeated a burst, or affected another world")
	}
	a.Burst(h, 3)
	a.scene.Destroy(h)
	a.scene.Flush()
	if a.Count(h) != 0 {
		t.Fatal("pool retained after destroy")
	}
}

func TestOffIsPerWorldAndUsesTheProjectSetting(t *testing.T) {
	store := settings.New()
	key, err := store.Register(settings.Desc{ID: KeyEnabled, Owner: ID, Kind: schema.KindBool, Default: schema.Bool(true), Flags: settings.Project | settings.User})
	if err != nil {
		t.Fatal(err)
	}
	store.Freeze()
	store.Start()
	a, h, _ := testWorld(t, store)
	b, k, _ := testWorld(t, store)
	a.SetEnabled(false)
	a.Burst(h, 4)
	b.Burst(k, 2)
	if a.Enabled() || a.Stats() != (Stats{}) || b.Count(k) != 2 {
		t.Fatal("toggle crossed world boundary")
	}
	if err = store.Override(key, schema.Bool(false)); err != nil {
		t.Fatal(err)
	}
	b.Burst(k, 2)
	if b.Enabled() || b.Stats() != (Stats{}) {
		t.Fatalf("setting did not clear world: %+v", b.Stats())
	}
	if err = store.ClearOverride(key); err != nil {
		t.Fatal(err)
	}
	b.Burst(k, 3)
	if !a.Enabled() || b.Count(k) != 3 {
		t.Fatal("re-enable")
	}
}

func TestWorldDustAcrossViewsAndTeleport(t *testing.T) {
	s, h, _ := testWorld(t, nil)
	dev := null.New()
	defer dev.Close()
	files, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := render.New(dev, dev.Caps(), files, log.Nop(), render.Options{MaxSprites: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()
	r := frame.New()
	r.Bind(s.scene, renderer, nil)
	r.AddView(view.ScreenView(128, 128, 1), 0)
	r.AddView(view.ScreenView(128, 128, 1), 1)
	s.scene.SetPosition(h, mathx.V2(15, 22))
	s.Burst(h, 8)
	s.scene.SetPosition(h, mathx.V2(655, 410))
	s.step(1.0/60, 1.0/60)
	age := s.systems[h].pool.Particles()[0].Age
	s.extract(r)
	for i := 0; i < 2; i++ {
		draws := r.Bucket(i).Sprites
		if len(draws) != 8 || draws[0].Position != mathx.V2(15, 22) || draws[0].Lighting || draws[0].Region.Texture.IsZero() {
			t.Fatalf("view %d: %+v", i, draws)
		}
	}
	if s.Stats().Submitted != 16 || s.systems[h].pool.Particles()[0].Age != age {
		t.Fatal("draw advanced simulation")
	}
	cost := testing.AllocsPerRun(100, func() {
		r.Bucket(0).Sprites = r.Bucket(0).Sprites[:0]
		r.Bucket(1).Sprites = r.Bucket(1).Sprites[:0]
		s.step(.0001, .0001)
		s.extract(r)
	})
	if cost != 0 {
		t.Fatalf("steady simulate/extract allocs: %f", cost)
	}
	s.SetEnabled(false)
	r.Bucket(0).Sprites = nil
	r.Bucket(1).Sprites = nil
	s.extract(r)
	if s.Stats() != (Stats{}) || len(r.Bucket(0).Sprites)+len(r.Bucket(1).Sprites) != 0 {
		t.Fatal("disabled emitter submitted visible draws")
	}
}

func TestAbsentModulePreservesEmitterScene(t *testing.T) {
	src, _, _ := testWorld(t, nil)
	start := sceneio.Save(src.scene)
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	without, report, err := sceneio.Load(start, "missing-module.aescene", types, nil, sceneio.LoadOptions{KeepIDs: true})
	if err != nil || len(report.Unknown) != 1 {
		t.Fatalf("unknown component diagnostics: %+v, %v", report, err)
	}
	if out := sceneio.Save(without); !bytes.Equal(start, out) {
		t.Fatal("absent particle module changed authored emitter")
	}
}

func TestWorldBudgetKeepsWorkingPoolAndRecovers(t *testing.T) {
	s, primary, config := testWorld(t, nil)
	s.Burst(primary, 2)
	var others []scene.NodeHandle
	for n := 0; n < 4; n++ {
		h := s.scene.Create(s.scene.Root(), "effect")
		c := scene.Add[ParticleSystem2D](s.scene, h)
		*c = DefaultParticleSystem2D()
		c.PlayOnStart = false
		c.MaxParticles = MaxCapacity
		if n == 3 {
			c.MaxParticles -= config.MaxParticles
		}
		others = append(others, h)
	}
	s.scene.Flush()
	s.scene.UpdateTransforms()
	s.step(0, 0)
	if s.allocated != MaxWorldCapacity {
		t.Fatalf("reserved %d", s.allocated)
	}
	config, _ = scene.Get[ParticleSystem2D](s.scene, primary)
	config.MaxParticles = MaxCapacity
	s.step(0, 0)
	if s.Count(primary) != 2 || s.systems[primary].err != errWorldCapacity || s.allocated != MaxWorldCapacity {
		t.Fatal("oversized edit replaced living particles")
	}
	s.scene.Destroy(others[0])
	s.scene.Flush()
	s.scene.UpdateTransforms()
	s.step(0, 0)
	if s.systems[primary].err != nil || cap(s.systems[primary].pool.Particles()) != MaxCapacity {
		t.Fatal("released capacity was not reused")
	}
}
