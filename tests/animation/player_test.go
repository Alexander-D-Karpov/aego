package animation_test

import (
	"testing"

	"aego/anim"
	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/recorder"
	"aego/format/aetext"
	"aego/modules/animation"
	"aego/modules/render2d"
	"aego/modules/tween"
	"aego/render/mesh"
	"aego/render/texture"
	"aego/scene"
	"aego/script/bind"
)

const dt = float32(1) / 64

var (
	runID   = ids.AssetID{Hi: 0xa11, Lo: 1}
	flashID = ids.AssetID{Hi: 0xa11, Lo: 2}
	spriteH = uint64(0xa0e1000000000000)
)

const runClip = `anim version=1 length=0.5 loop=loop {
  frames "" fps=8 {
    sprites=[asset("a0e10000000000000000000000000001"), asset("a0e10000000000000000000000000002"), asset("a0e10000000000000000000000000003"), asset("a0e10000000000000000000000000004")]
  }
  track "" prop="SpriteRenderer.tint" kind=color {
    key t=0 v=[1, 1, 1, 1] interp=linear {}
    key t=0.5 v=[1, 0, 0, 1] {}
  }
  track "" prop="transform.scale.y" kind=float32 {
    key t=0 v=1 interp=linear {}
    key t=0.5 v=1.5 {}
  }
  event t=0.25 name="Hit" payload=2 {}
}
`

const flashClip = `anim version=1 length=0.25 {
  track "" prop="SpriteRenderer.tint" kind=color onEnd=restore {
    key t=0 v=[1, 0, 0, 1] {}
    key t=0.25 v=[1, 0, 0, 1] {}
  }
  frames "" fps=4 {
    sprites=[asset("a0e10000000000000000000000000004")]
  }
}
`

type decKey struct {
	id  ids.AssetID
	key string
}

type assets struct {
	clips map[ids.AssetID][]byte
	ctrls map[ids.AssetID][]byte
	cache map[decKey]any
	gen   uint32
}

func (a *assets) Texture(ids.AssetID) (texture.Region, bool) { return texture.Region{}, false }
func (a *assets) Mesh(ids.AssetID) (*mesh.Mesh, bool)        { return nil, false }
func (a *assets) Scene(ids.AssetID) ([]byte, bool)           { return nil, false }
func (a *assets) Prefab(ids.AssetID) ([]byte, bool)          { return nil, false }
func (a *assets) Generation() uint32                         { return a.gen }
func (a *assets) Subscribe(func(asset.Reloaded)) func()      { return func() {} }

func (a *assets) Anim(id ids.AssetID) ([]byte, bool) {
	b, ok := a.clips[id]
	return b, ok
}

func (a *assets) Controller(id ids.AssetID) ([]byte, bool) {
	b, ok := a.ctrls[id]
	return b, ok
}

func (a *assets) Decoded(id ids.AssetID, key string, build func() (any, error)) (any, error) {
	k := decKey{id: id, key: key}
	if v, ok := a.cache[k]; ok {
		return v, nil
	}
	v, err := build()
	if err == nil {
		a.cache[k] = v
	}
	return v, err
}

func (a *assets) putCtrl(t testing.TB, id ids.AssetID, src string) {
	t.Helper()
	doc, err := aetext.Parse([]byte(src), "test.aectrl")
	if err != nil {
		t.Fatal(err)
	}
	g, probs, err := anim.DecodeGraph(doc)
	if err != nil || len(probs) > 0 || len(g.Check(nil)) > 0 {
		t.Fatalf("controller: %v %+v %+v", err, probs, g.Check(nil))
	}
	a.ctrls[id] = anim.MarshalGraph(g)
	clear(a.cache)
	a.gen++
}

func (a *assets) put(t testing.TB, id ids.AssetID, src string) {
	t.Helper()
	doc, err := aetext.Parse([]byte(src), "clip.aeanim")
	if err != nil {
		t.Fatal(err)
	}
	c, err := anim.Decode(doc)
	if err != nil {
		t.Fatal(err)
	}
	if ps := c.Check(nil); len(ps) > 0 {
		t.Fatalf("clip problems %+v", ps)
	}
	a.clips[id] = anim.Marshal(c)
	clear(a.cache)
	a.gen++
}

type rig struct {
	s    *scene.Scene
	w    *engine.World
	res  *assets
	svc  *animation.Service
	tick uint64
}

func newRig(t testing.TB) *rig {
	t.Helper()
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.New(), assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	res := &assets{clips: map[ids.AssetID][]byte{}, ctrls: map[ids.AssetID][]byte{}, cache: map[decKey]any{}, gen: 1}
	s := scene.New(types, res, scene.Options{})
	w := engine.NewWorld("anim")
	w.Recorder = recorder.New(1024)
	mod := animation.New()
	mod.SetEvents([]animation.EventDesc{{Name: "Hit", Payload: animation.PayloadFloat}, {Name: "Step"}})
	return &rig{s: s, w: w, res: res, svc: animation.NewService(s, w, mod)}
}

func (r *rig) node(clip ids.AssetID) scene.NodeHandle {
	h := r.s.Create(r.s.Root(), "runner")
	*scene.Add[render2d.SpriteRenderer](r.s, h) = render2d.DefaultSpriteRenderer()
	p := animation.DefaultPlayer()
	p.Clip = animation.ClipRef{ID: clip}
	*scene.Add[animation.AnimationPlayer](r.s, h) = p
	return h
}

func (r *rig) step() {
	r.tick++
	r.w.Recorder.Stamp(uint64(r.w.ID()), r.tick, r.tick)
	r.s.Flush()
	r.s.UpdateTransforms()
	r.svc.Fixed(dt)
}

func (r *rig) present(alpha float32) {
	r.svc.Present(alpha, dt, dt)
}

func (r *rig) steps(n int) {
	for range n {
		r.step()
		r.present(0)
	}
}

func (r *rig) sprite(h scene.NodeHandle) ids.AssetID {
	sr, _ := scene.Get[render2d.SpriteRenderer](r.s, h)
	return sr.Sprite.ID
}

func (r *rig) tint(h scene.NodeHandle) mathx.Color {
	sr, _ := scene.Get[render2d.SpriteRenderer](r.s, h)
	return sr.Tint
}

func (r *rig) ticks(name string) []uint64 {
	var out []uint64
	for _, rec := range r.w.Recorder.Snapshot(nil) {
		if rec.Kind == recorder.Animation && rec.Name == name && rec.Detail == "" {
			out = append(out, rec.Tick)
		}
	}
	return out
}

func near(a, b float32) bool {
	d := a - b
	return d < 1e-4 && d > -1e-4
}

func TestFlipbookTintAndPose(t *testing.T) {
	r := newRig(t)
	r.res.put(t, runID, runClip)
	h := r.node(runID)
	r.steps(1)
	if id := r.sprite(h); id.Hi != spriteH || id.Lo != 1 {
		t.Fatalf("first frame %v", id)
	}
	r.steps(7)
	if id := r.sprite(h); id.Lo != 2 {
		t.Fatalf("frame at 0.125: %v", id)
	}
	r.steps(8)
	if c := r.tint(h); !near(c.R, 1) || !near(c.G, 0.5) || !near(c.B, 0.5) {
		t.Fatalf("tint at 0.25: %+v", c)
	}
	if sy := r.s.Local(h).Scale.Y; sy != 1 {
		t.Fatalf("presentation track wrote the simulation scale: %v", sy)
	}
	p, ok := r.s.PresentationLocal(h)
	if !ok || !near(p.Scale.Y, 1.25) {
		t.Fatalf("presentation scale %v %v", ok, p.Scale)
	}
}

func TestGameplayEventOncePerCycle(t *testing.T) {
	r := newRig(t)
	r.res.put(t, runID, runClip)
	r.node(runID)
	r.steps(64)
	if got := r.ticks("Hit"); len(got) != 2 || got[0] != 16 || got[1] != 48 {
		t.Fatalf("Hit on ticks %v, want [16 48]", got)
	}
}

func TestPresentDoesNotChangeSimulation(t *testing.T) {
	a, b := newRig(t), newRig(t)
	a.res.put(t, runID, runClip)
	b.res.put(t, runID, runClip)
	ha, hb := a.node(runID), b.node(runID)
	for range 96 {
		a.step()
		a.present(0)
		b.step()
		for k := range 3 {
			b.present(float32(k) / 3)
		}
	}
	ta, tb := a.ticks("Hit"), b.ticks("Hit")
	if len(ta) != len(tb) {
		t.Fatalf("events %v vs %v", ta, tb)
	}
	for i := range ta {
		if ta[i] != tb[i] {
			t.Fatalf("events %v vs %v", ta, tb)
		}
	}
	if a.sprite(ha) != b.sprite(hb) || a.tint(ha) != b.tint(hb) || a.s.Local(ha) != b.s.Local(hb) {
		t.Fatal("presentation rate changed the simulation state")
	}
}

func TestRestoreOnEnd(t *testing.T) {
	r := newRig(t)
	r.res.put(t, flashID, flashClip)
	h := r.node(flashID)
	sr, _ := scene.Get[render2d.SpriteRenderer](r.s, h)
	grey := mathx.Color{R: 0.5, G: 0.5, B: 0.5, A: 1}
	sr.Tint = grey
	r.steps(4)
	if c := r.tint(h); c.G != 0 {
		t.Fatalf("flash tint %+v", c)
	}
	r.steps(28)
	p, _ := scene.Get[animation.AnimationPlayer](r.s, h)
	if p.Playing || r.tint(h) != grey {
		t.Fatalf("playing %v tint %+v", p.Playing, r.tint(h))
	}
	if r.sprite(h).Lo != 4 {
		t.Fatal("a hold track must keep its last value")
	}
	if n := len(r.ticks("Finished")); n != 1 {
		t.Fatalf("Finished recorded %d times", n)
	}
}

func TestTweenStopsOnAnimatedProperty(t *testing.T) {
	r := newRig(t)
	r.res.put(t, runID, runClip)
	h := r.node(runID)
	tw := tween.NewService(r.s, r.w)
	r.steps(1)
	hd, err := tw.ToColor(h, "SpriteRenderer.Tint", mathx.Color{G: 1, A: 1}, 1, tween.Options{})
	if err != nil {
		t.Fatal(err)
	}
	tw.Step(dt, false)
	if !tw.Done(hd) {
		t.Fatal("a tween on a property the animation drives must stop")
	}
}

func hundred(t testing.TB) *rig {
	r := newRig(t)
	r.res.put(t, runID, runClip)
	for range 100 {
		r.node(runID)
	}
	r.steps(70)
	return r
}

func TestHundredPlayersAllocateNothing(t *testing.T) {
	r := hundred(t)
	if n := testing.AllocsPerRun(50, func() {
		r.step()
		r.present(0.5)
	}); n != 0 {
		t.Fatalf("%v allocations per tick", n)
	}
}

func BenchmarkPlayers100(b *testing.B) {
	r := hundred(b)
	b.ReportAllocs()
	for b.Loop() {
		r.step()
		r.present(0.5)
	}
}

func TestBuiltinClips(t *testing.T) {
	for _, id := range []ids.AssetID{asset.BuiltinRunClip, asset.BuiltinBobClip} {
		src, ok := asset.BuiltinAnimText(id)
		if !ok {
			t.Fatalf("%v has no text", id)
		}
		doc, err := aetext.Parse([]byte(src), "builtin.aeanim")
		if err != nil {
			t.Fatal(err)
		}
		c, err := anim.Decode(doc)
		if err != nil {
			t.Fatal(err)
		}
		if ps := c.Check(nil); len(ps) > 0 {
			t.Fatalf("%v: %+v", id, ps)
		}
		if id == asset.BuiltinRunClip {
			keys := c.Tracks[0].Keys
			if len(keys) != 8 || keys[3].V.A != asset.BuiltinRunner(3) {
				t.Fatalf("run frames %+v", keys)
			}
		}
	}
	a, _ := asset.BuiltinTexture(asset.BuiltinRunner(0))
	b, _ := asset.BuiltinTexture(asset.BuiltinRunner(2))
	if a == nil || b == nil || a.Width != 16 || string(a.Pixels) == string(b.Pixels) {
		t.Fatal("runner frames must be 16x16 and differ")
	}
	src, ok := asset.BuiltinCtrlText(asset.BuiltinPlatformerCtrl)
	if !ok {
		t.Fatal("no built-in platformer controller")
	}
	doc, err := aetext.Parse([]byte(src), "builtin.aectrl")
	if err != nil {
		t.Fatal(err)
	}
	g, probs, err := anim.DecodeGraph(doc)
	if err != nil || len(probs) > 0 || len(g.Check(nil)) > 0 {
		t.Fatalf("platformer controller: %v %+v %+v", err, probs, g.Check(nil))
	}
	if g.StateIndex("Run") < 0 || g.States[g.StateIndex("Run")].Clip != asset.BuiltinRunClip {
		t.Fatal("Run plays the built-in run clip")
	}
	mod := animation.New()
	if id, ok := mod.EventID(animation.EventHit); !ok || mod.EventName(id) != animation.EventHit {
		t.Fatal("Hit must be declared without a project section")
	}
}

func TestDisabledPlayerKeepsItsBindings(t *testing.T) {
	r := newRig(t)
	r.res.put(t, runID, runClip)
	h := r.node(runID)
	r.steps(8)
	if r.sprite(h).Lo != 2 {
		t.Fatalf("frame at 0.125: %v", r.sprite(h))
	}
	r.s.SetEnabled(h, false)
	r.steps(2)
	r.s.Create(r.s.Root(), "other")
	r.steps(2)
	r.s.SetEnabled(h, true)
	r.steps(8)
	if id := r.sprite(h); id.Lo != 3 {
		t.Fatalf("after a rebind while disabled the player must go on writing, frame %v", id)
	}
}
