package animation_test

import (
	"testing"

	"aego/contracts/motion2d"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/modules/animation"
	"aego/scene"
)

var (
	walkClipID = ids.AssetID{Hi: hiTest, Lo: 20}
	walkCtrlID = ids.AssetID{Hi: hiTest, Lo: 21}
)

const walkClip = `anim version=1 length=1 loop=loop {
  track "" prop="transform.position" kind=vec2 root=true {
    key t=0 v=[0, 0] interp=linear {}
    key t=1 v=[1, 0] {}
  }
}
`

const walkCtrl = `controller version=1 {
  entry="Walk"
  state "Walk" clip=asset("a0e20000000000000000000000000014") {}
}
`

type sink struct {
	sum mathx.Vec2
}

func (k *sink) AddDisplacement(_ scene.NodeHandle, d mathx.Vec2) bool {
	k.sum = k.sum.Add(d)
	return true
}

func walkRig(t testing.TB, mode animation.RootMotion, fallback bool) (*rig, scene.NodeHandle) {
	r := newRig(t)
	r.res.put(t, walkClipID, walkClip)
	r.res.putCtrl(t, walkCtrlID, walkCtrl)
	h := r.animator(walkCtrlID)
	a := r.anim(h)
	a.RootMotion, a.RootFallback = mode, fallback
	return r, h
}

func TestRootMotionToTransformAcrossLoops(t *testing.T) {
	r, h := walkRig(t, animation.RootToTransform, false)
	r.steps(64)
	if x := r.s.Local(h).Position.X; !near(x, 1) {
		t.Fatalf("after one cycle x=%v", x)
	}
	r.steps(64)
	if x := r.s.Local(h).Position.X; !near(x, 2) {
		t.Fatalf("the root must not jump back at the loop, x=%v", x)
	}
	r.anim(h).RootScale = mathx.Vec2{X: -1, Y: 1}
	r.steps(32)
	if x := r.s.Local(h).Position.X; !near(x, 1.5) {
		t.Fatalf("a mirrored root goes back, x=%v", x)
	}
}

func TestRootMotionToMotion(t *testing.T) {
	r, h := walkRig(t, animation.RootToMotion, false)
	k := &sink{}
	r.w.SetView(motion2d.ServiceID, motion2d.Receiver(k))
	r.steps(32)
	if !near(k.sum.X, 0.5) || r.s.Local(h).Position.X != 0 {
		t.Fatalf("receiver got %v, node at %v", k.sum, r.s.Local(h).Position)
	}
}

func TestRootMotionWithoutReceiver(t *testing.T) {
	r, h := walkRig(t, animation.RootToMotion, false)
	r.steps(32)
	if x := r.s.Local(h).Position.X; x != 0 {
		t.Fatalf("without a receiver and fallback the node moved to %v", x)
	}
	r2, h2 := walkRig(t, animation.RootToMotion, true)
	r2.steps(32)
	if x := r2.s.Local(h2).Position.X; !near(x, 0.5) {
		t.Fatalf("the fallback must move the transform, x=%v", x)
	}
}
