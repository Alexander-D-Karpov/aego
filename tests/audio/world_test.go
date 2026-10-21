package audio

import (
	"math"
	"testing"

	"aego/app/assembly"
	"aego/asset"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/core/rewind"
	"aego/demos"
	"aego/engine"
	"aego/engine/module"
	"aego/engine/settings"
	audiomod "aego/modules/audio"
	"aego/scene"
	"aego/script/bind"
)

type rig struct {
	t     *testing.T
	e     *engine.Engine
	man   *module.Manifest
	mod   *audiomod.Module
	st    *settings.Store
	types *scene.Types
}

type playWorld struct {
	s      *scene.Scene
	w      *engine.World
	svc    *audiomod.Service
	detach func()
}

func newRig(t *testing.T) *rig {
	t.Helper()
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	man, err := module.Load(types, bind.New(), audiomod.New())
	if err != nil {
		t.Fatal(err)
	}
	st, err := man.NewSettings()
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.NewBuilder(engine.Config{
		Headless: true, HeadlessFrameTime: 1.0 / 60, UserRoot: t.TempDir(), AppID: "aego-audio-world-test",
	}).WithLogger(log.Nop()).Use(man.Engine()).Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := man.Init(&module.InitContext{Engine: e, Settings: st, Log: log.Nop()}); err != nil {
		t.Fatal(err)
	}
	st.Start()
	t.Cleanup(func() {
		man.Shutdown()
		e.Close()
	})
	mod := audiomod.ModuleOf(e)
	if mod == nil || mod.Mixer() == nil {
		t.Fatal("no audio module")
	}
	return &rig{t: t, e: e, man: man, mod: mod, st: st, types: types}
}

func (r *rig) world(name string) *playWorld {
	r.t.Helper()
	s := scene.New(r.types, nil, scene.Options{})
	w := engine.NewWorld(name)
	w.SetService(scene.ServiceID, s)
	detach, err := r.man.Attach(&module.AttachContext{World: w, Scene: s, Play: true, Log: log.Nop()})
	if err != nil {
		r.t.Fatal(err)
	}
	p := &playWorld{s: s, w: w, svc: audiomod.Of(w), detach: detach}
	if p.svc == nil {
		r.t.Fatal("no audio service in the play world")
	}
	r.t.Cleanup(func() {
		if p.detach != nil {
			p.detach()
		}
	})
	return p
}

func (r *rig) frames(n int, ps ...*playWorld) {
	r.t.Helper()
	for i := 0; i < n; i++ {
		for _, p := range ps {
			r.e.TickWorld(p.w, engine.ScopePlay, 1.0/60)
		}
		if !r.e.Step(demos.Base{}) {
			r.t.Fatalf("engine stopped: %v", r.e.Err())
		}
	}
}

func place(p *playWorld, name string, pos mathx.Vec2) scene.NodeHandle {
	h := p.s.Create(p.s.Root(), name)
	p.s.SetLocal(h, scene.Transform2D{Position: pos, Scale: mathx.Vec2{X: 1, Y: 1}})
	p.s.Flush()
	p.s.UpdateTransforms()
	return h
}

func loop() audiomod.Options {
	return audiomod.Options{Loop: true}
}

func hashOf(s *audiomod.Service) [32]byte {
	h := rewind.NewHasher()
	s.Hash(h)
	return h.Sum()
}

func TestEventVoicesSteal(t *testing.T) {
	r := newRig(t)
	p := r.world("a")
	r.frames(1, p)
	ev := audiomod.DefaultEvent()
	ev.Variants = []audiomod.ClipRef{{ID: asset.BuiltinHum}}
	ev.Loop, ev.MaxVoices, ev.Steal = true, 4, audiomod.StealOldest
	key := ids.AssetID{Hi: 7, Lo: 1}
	var snd [10]audiomod.Sound
	for i := range snd {
		if snd[i] = p.svc.Trigger(key, &ev, scene.NodeHandle{}); snd[i] == 0 {
			t.Fatalf("trigger %d refused", i)
		}
	}
	r.frames(10, p)
	if n := p.svc.Voices(); n != 4 {
		t.Fatalf("%d voices, want 4", n)
	}
	for i, s := range snd {
		if done := p.svc.Done(s); done != (i < 6) {
			t.Fatalf("sound %d done=%v", i, done)
		}
	}
	ev.Steal = audiomod.StealNone
	if p.svc.Trigger(key, &ev, scene.NodeHandle{}) != 0 {
		t.Fatal("Steal None played over Max Voices")
	}
}

func TestEventCooldown(t *testing.T) {
	r := newRig(t)
	p := r.world("a")
	r.frames(1, p)
	ev := audiomod.DefaultEvent()
	ev.Variants = []audiomod.ClipRef{{ID: asset.BuiltinBeep}}
	ev.Cooldown = 0.5
	key := ids.AssetID{Hi: 7, Lo: 2}
	if p.svc.Trigger(key, &ev, scene.NodeHandle{}) == 0 {
		t.Fatal("first play refused")
	}
	if p.svc.Trigger(key, &ev, scene.NodeHandle{}) != 0 {
		t.Fatal("cooldown did not block the second play")
	}
	r.frames(40, p)
	if p.svc.Trigger(key, &ev, scene.NodeHandle{}) == 0 {
		t.Fatal("cooldown still blocks after 0.66 s of game time")
	}
}

func TestZones(t *testing.T) {
	r := newRig(t)
	p := r.world("a")
	lis := place(p, "Listener", mathx.Vec2{X: -10})
	*scene.Add[audiomod.AudioListener2D](p.s, lis) = audiomod.DefaultListener()
	za := place(p, "A", mathx.Vec2{})
	*scene.Add[audiomod.AudioZone2D](p.s, za) = audiomod.AudioZone2D{Area: mathx.Rect{X: -4, Y: -4, W: 8, H: 8}, Bus: audiomod.BusMusic, Gain: 0.5}
	zb := place(p, "B", mathx.Vec2{X: 3})
	*scene.Add[audiomod.AudioZone2D](p.s, zb) = audiomod.AudioZone2D{Area: mathx.Rect{X: -2, Y: -2, W: 4, H: 4}, Bus: audiomod.BusMusic, Gain: 0.2}
	p.s.Flush()
	music := int(audiomod.BusMusic)
	at := func(x, want float32) {
		t.Helper()
		p.s.SetLocal(lis, scene.Transform2D{Position: mathx.Vec2{X: x}, Scale: mathx.Vec2{X: 1, Y: 1}})
		p.s.UpdateTransforms()
		r.frames(3, p)
		if g := r.mod.Mixer().BusGain(music); math.Abs(float64(g-want)) > 1e-5 {
			t.Fatalf("listener at %v: music gain %v, want %v", x, g, want)
		}
	}
	at(-10, 1)
	at(0, 0.5)
	at(3, 0.2)
	at(0, 0.5)
	at(-10, 1)
}

func TestPauseWith(t *testing.T) {
	r := newRig(t)
	p := r.world("a")
	r.frames(1, p)
	game := p.svc.Play(asset.BuiltinHum, loop(), scene.NodeHandle{})
	never := p.svc.Play(asset.BuiltinHum, audiomod.Options{Loop: true, Unscaled: true}, scene.NodeHandle{})
	r.frames(2, p)
	p.w.Time.SetPaused(true)
	r.frames(3, p)
	if p.svc.Playing(game) || !p.svc.Playing(never) {
		t.Fatalf("paused world: game %v, never %v", p.svc.Playing(game), p.svc.Playing(never))
	}
	p.w.Time.SetPaused(false)
	r.frames(2, p)
	if !p.svc.Playing(game) {
		t.Fatal("the game sound did not resume")
	}
}

func TestReplayIsSilent(t *testing.T) {
	r := newRig(t)
	p := r.world("a")
	r.frames(1, p)
	before := r.mod.Mixer().Commands().Len()
	p.w.SetReplaying(true)
	if s := p.svc.Play(asset.BuiltinBeep, audiomod.Options{}, scene.NodeHandle{}); s != 0 {
		t.Fatal("a replay played a sound")
	}
	if n := r.mod.Mixer().Commands().Len(); n != before {
		t.Fatalf("a replay queued %d commands", n-before)
	}
	p.w.SetReplaying(false)
}

func TestRestoreStopsAndHashIgnoresSounds(t *testing.T) {
	r := newRig(t)
	p := r.world("a")
	r.frames(2, p)
	h0 := hashOf(p.svc)
	key := p.svc.Capture(nil)
	p.svc.Play(asset.BuiltinHum, loop(), scene.NodeHandle{})
	r.frames(3, p)
	if hashOf(p.svc) != h0 {
		t.Fatal("a sound changed the state hash")
	}
	p.svc.Restore(key)
	if n := p.svc.Voices(); n != 0 {
		t.Fatalf("%d voices after Restore", n)
	}
	r.frames(2, p)
	if n := r.mod.Mixer().Active(); n != 0 {
		t.Fatalf("%d mixer voices after Restore", n)
	}
}

func TestWorldsAndPreview(t *testing.T) {
	r := newRig(t)
	a, b := r.world("a"), r.world("b")
	r.frames(1, a, b)
	a.svc.Play(asset.BuiltinHum, loop(), scene.NodeHandle{})
	if err := r.mod.Preview(nil, asset.BuiltinMelody, true); err != nil {
		t.Fatal(err)
	}
	r.frames(2, a, b)
	if a.svc.Voices() != 1 || b.svc.Voices() != 0 {
		t.Fatalf("voices a=%d b=%d", a.svc.Voices(), b.svc.Voices())
	}
	a.detach()
	a.detach = nil
	r.frames(6, b)
	if _, ok := r.mod.PreviewPosition(); !ok {
		t.Fatal("stopping a world stopped the preview")
	}
	if n := r.mod.Mixer().Active(); n != 1 {
		t.Fatalf("%d mixer voices, want only the preview", n)
	}
	r.mod.StopPreview()
}

func TestOwnerDeathStops(t *testing.T) {
	r := newRig(t)
	p := r.world("a")
	h := place(p, "Owner", mathx.Vec2{})
	r.frames(1, p)
	snd := p.svc.Play(asset.BuiltinHum, loop(), h)
	r.frames(2, p)
	p.s.Destroy(h)
	p.s.Flush()
	r.frames(8, p)
	if !p.svc.Done(snd) {
		t.Fatal("the sound outlived its node")
	}
}

func TestSessionVolumeIsNotSaved(t *testing.T) {
	r := newRig(t)
	r.mod.SetBusVolume(audiomod.BusMusic, 0.5)
	r.frames(3)
	if v := r.st.Float(r.st.Key(audiomod.VolumeKey("music"))); v != 1 {
		t.Fatalf("the setting changed to %v", v)
	}
	if g := r.mod.Mixer().BusGain(int(audiomod.BusMusic)); math.Abs(float64(g-0.5)) > 1e-5 {
		t.Fatalf("music gain %v", g)
	}
	r.mod.ResetSession()
}
