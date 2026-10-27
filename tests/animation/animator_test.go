package animation_test

import (
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine/recorder"
	"aego/modules/animation"
	"aego/modules/render2d"
	"aego/scene"
	"aego/script/bind"
)

const hiTest = uint64(0xa0e2000000000000)

var (
	idleID = ids.AssetID{Hi: hiTest, Lo: 1}
	runID2 = ids.AssetID{Hi: hiTest, Lo: 2}
	hitID  = ids.AssetID{Hi: hiTest, Lo: 3}
	ctrlID = ids.AssetID{Hi: hiTest, Lo: 9}
)

const idleClip = `anim version=1 length=1 loop=loop {
  track "" prop="SpriteRenderer.tint" kind=color {
    key t=0 v=[1, 1, 1, 1] {}
  }
}
`

const runClip2 = `anim version=1 length=1 loop=loop {
  track "" prop="SpriteRenderer.tint" kind=color {
    key t=0 v=[1, 0, 0, 1] {}
  }
}
`

const hitClip = `anim version=1 length=0.25 {
  track "" prop="SpriteRenderer.tint" kind=color {
    key t=0 v=[0, 0, 1, 1] {}
  }
  event t=0.0625 name="Hit" payload=3 {}
}
`

const ctrlSrc = `controller version=1 {
  entry="Idle"
  param "Speed" kind=float {}
  param "Attack" kind=trigger {}
  state "Idle" clip=asset("a0e20000000000000000000000000001") {}
  state "Run" clip=asset("a0e20000000000000000000000000002") {}
  state "Hit" clip=asset("a0e20000000000000000000000000003") loop=once {}
  transition "Idle" to="Run" duration=0.125 {
    cond "Speed" op=gt value=0.5 {}
  }
  transition "Run" to="Idle" duration=0.125 {
    cond "Speed" op=le value=0.5 {}
  }
  transition "any" to="Hit" duration=0.125 interrupt=true {
    cond "Attack" {}
  }
  transition "Hit" to="Idle" exitTime=1 duration=0.125 {}
}
`

func ctrlRig(t testing.TB) *rig {
	r := newRig(t)
	r.res.put(t, idleID, idleClip)
	r.res.put(t, runID2, runClip2)
	r.res.put(t, hitID, hitClip)
	r.res.putCtrl(t, ctrlID, ctrlSrc)
	return r
}

func (r *rig) animator(ctrl ids.AssetID) scene.NodeHandle {
	h := r.s.Create(r.s.Root(), "hero")
	*scene.Add[render2d.SpriteRenderer](r.s, h) = render2d.DefaultSpriteRenderer()
	a := animation.DefaultAnimator()
	a.Controller = animation.ControllerRef{ID: ctrl}
	*scene.Add[animation.Animator](r.s, h) = a
	return h
}

func (r *rig) anim(h scene.NodeHandle) *animation.Animator {
	a, _ := scene.Get[animation.Animator](r.s, h)
	return a
}

func (r *rig) count(name, detail string) int {
	n := 0
	for _, rec := range r.w.Recorder.Snapshot(nil) {
		if rec.Kind == recorder.Animation && rec.Name == name && rec.Detail == detail {
			n++
		}
	}
	return n
}

func TestAnimatorBlendsBetweenStates(t *testing.T) {
	r := ctrlRig(t)
	h := r.animator(ctrlID)
	r.steps(1)
	if a := r.anim(h); a.State != "Idle" || r.tint(h) != (mathx.Color{R: 1, G: 1, B: 1, A: 1}) {
		t.Fatalf("start %q %+v", a.State, r.tint(h))
	}
	if !r.svc.SetParam(h, "Speed", 1) {
		t.Fatal("Speed is a parameter")
	}
	r.steps(1)
	if a := r.anim(h); a.State != "Run" || !a.Blending {
		t.Fatalf("transition %q blending %v", a.State, a.Blending)
	}
	r.steps(4)
	if c := r.tint(h); !near(c.G, 0.5) || !near(c.R, 1) {
		t.Fatalf("half way %+v", c)
	}
	r.steps(4)
	if a := r.anim(h); a.Blending || r.tint(h) != (mathx.Color{R: 1, A: 1}) {
		t.Fatalf("after the blend blending %v tint %+v", a.Blending, r.tint(h))
	}
	if r.count("Run", "enter") != 1 || r.count("Idle", "exit") != 1 {
		t.Fatal("StateEntered and StateExited must be recorded once")
	}
}

func TestAnimatorTriggerAndDominantEvent(t *testing.T) {
	r := ctrlRig(t)
	h := r.animator(ctrlID)
	r.steps(1)
	r.svc.SetParam(h, "Attack", 1)
	r.steps(1)
	if r.anim(h).State != "Hit" {
		t.Fatalf("state %q", r.anim(h).State)
	}
	r.steps(30)
	if n := r.count("Hit", ""); n != 1 {
		t.Fatalf("the Hit event at the middle of the blend fired %d times", n)
	}
	if r.anim(h).State != "Idle" || r.count("Hit", "enter") != 1 {
		t.Fatalf("after the hit %q", r.anim(h).State)
	}
}

func TestAnimatorsAllocateNothing(t *testing.T) {
	r := ctrlRig(t)
	var hs []scene.NodeHandle
	for range 100 {
		hs = append(hs, r.animator(ctrlID))
	}
	n := 0
	tick := func() {
		n++
		if n%16 == 0 {
			v := float64((n / 16) % 2)
			for _, h := range hs {
				r.svc.SetParam(h, "Speed", v)
			}
		}
		r.step()
		r.present(0.5)
	}
	for range 128 {
		tick()
	}
	if a := testing.AllocsPerRun(64, tick); a != 0 {
		t.Fatalf("%v allocations per tick", a)
	}
}

func BenchmarkAnimator100(b *testing.B) {
	r := ctrlRig(b)
	for range 100 {
		r.animator(ctrlID)
	}
	r.steps(70)
	b.ReportAllocs()
	for b.Loop() {
		r.step()
		r.present(0.5)
	}
}

func TestPlatformerPresetHasAnimator(t *testing.T) {
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	man, err := builtin.Default().Load(types, bind.New(), assembly.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var setup func(*scene.Scene, scene.NodeHandle)
	for _, td := range man.Templates {
		if td.ID == "gameplay.platformer" {
			setup, _ = td.Setup.(func(*scene.Scene, scene.NodeHandle))
		}
	}
	if setup == nil {
		t.Fatal("no Platformer Character preset")
	}
	s := scene.New(types, nil, scene.Options{})
	h := s.Create(s.Root(), "hero")
	setup(s, h)
	a, ok := scene.Get[animation.Animator](s, h)
	if !ok || a.Controller.ID != asset.BuiltinPlatformerCtrl || a.Speed != 1 {
		t.Fatalf("preset animator %v %+v", ok, a)
	}
}

// The built-in platformer controller shows its air and hit states: the Animator try scene jumps with Space and hits with J.
func TestBuiltinControllerJumpAndHitAreVisible(t *testing.T) {
	r := newRig(t)
	for _, id := range []ids.AssetID{asset.BuiltinRunClip, asset.BuiltinJumpClip, asset.BuiltinHitClip} {
		src, ok := asset.BuiltinAnimText(id)
		if !ok {
			t.Fatalf("no built-in clip %v", id)
		}
		r.res.put(t, id, src)
	}
	src, _ := asset.BuiltinCtrlText(asset.BuiltinPlatformerCtrl)
	r.res.putCtrl(t, asset.BuiltinPlatformerCtrl, src)
	h := r.animator(asset.BuiltinPlatformerCtrl)
	r.steps(1)
	if r.anim(h).State != "Idle" {
		t.Fatalf("start %q", r.anim(h).State)
	}
	r.svc.SetParam(h, "Grounded", 0)
	r.steps(2)
	if r.anim(h).State != "Jump" || r.sprite(h) != asset.BuiltinRunner(2) {
		t.Fatalf("in the air: %q, sprite %v", r.anim(h).State, r.sprite(h))
	}
	r.svc.SetParam(h, "Grounded", 1)
	r.steps(10)
	if r.anim(h).State != "Idle" {
		t.Fatalf("after landing %q", r.anim(h).State)
	}
	r.svc.SetParam(h, "Attack", 1)
	r.steps(6)
	if r.anim(h).State != "Hit" || r.s.Local(h).Rotation > -0.2 {
		t.Fatalf("hit: %q, rotation %v", r.anim(h).State, r.s.Local(h).Rotation)
	}
	r.steps(40)
	if r.anim(h).State != "Idle" || mathx.Abs(r.s.Local(h).Rotation) > 0.01 {
		t.Fatalf("after the hit: %q, rotation %v", r.anim(h).State, r.s.Local(h).Rotation)
	}
}
