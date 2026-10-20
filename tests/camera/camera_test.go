package camera_test

import (
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/core/curve"
	"aego/core/mathx"
	"aego/modules/camera"
	"aego/render/view"
	"aego/scene"
	"aego/script/bind"
)

const dt = float32(1.0 / 60)

type rig struct {
	t   testing.TB
	s   *scene.Scene
	svc *camera.Service
	cam scene.NodeHandle
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
	s := scene.New(types, nil, scene.Options{})
	r := &rig{t: t, s: s, svc: camera.NewService(s, nil)}
	r.cam = s.Create(s.Root(), "cam")
	s.Flush()
	return r
}

func baseView() view.View {
	return view.View{
		Camera:     view.Camera2D{Position: mathx.Vec2{X: -100, Y: -100}, Zoom: 64},
		Viewport:   mathx.Rect{W: 960, H: 540},
		PixelScale: 1,
	}
}

func (r *rig) node(name string, p mathx.Vec2) scene.NodeHandle {
	h := r.s.Create(r.s.Root(), name)
	r.s.SetPosition(h, p)
	return h
}

func (r *rig) vcam(name string, p mathx.Vec2, prio int32) scene.NodeHandle {
	h := r.node(name, p)
	vc := scene.Add[camera.VirtualCamera2D](r.s, h)
	*vc = camera.DefaultVirtual()
	vc.Priority = prio
	return h
}

func (r *rig) frameOn(node scene.NodeHandle) view.View {
	r.s.Flush()
	r.s.UpdateTransforms()
	r.svc.Update(dt, dt)
	return r.svc.Adjust(node, baseView())
}

func (r *rig) frame() view.View {
	return r.frameOn(r.cam)
}

func (r *rig) frames(n int) view.View {
	var v view.View
	for range n {
		v = r.frame()
	}
	return v
}

func near(a, b mathx.Vec2, eps float32) bool {
	return a.Sub(b).Len() <= eps
}

func TestNoVirtualCameraKeepsView(t *testing.T) {
	r := newRig(t)
	if v := r.frame(); v.Camera != baseView().Camera {
		t.Fatalf("view changed without virtual cameras: %+v", v.Camera)
	}
}

func TestPriorityTieIsStable(t *testing.T) {
	r := newRig(t)
	a := r.vcam("a", mathx.Vec2{X: 1}, 0)
	r.vcam("b", mathx.Vec2{X: 2}, 0)
	for range 100 {
		v := r.frame()
		if r.svc.Active("") != a || v.Camera.Position != (mathx.Vec2{X: 1}) {
			t.Fatalf("tie must keep the first camera, active %v at %v", r.svc.Active(""), v.Camera.Position)
		}
	}
}

func TestHigherPriorityWins(t *testing.T) {
	r := newRig(t)
	r.vcam("a", mathx.Vec2{X: 1}, 0)
	b := r.vcam("b", mathx.Vec2{X: 2}, 5)
	r.frame()
	if r.svc.Active("") != b {
		t.Fatal("the higher priority camera must be active")
	}
}

func TestBlendStartsFromTheCurrentView(t *testing.T) {
	r := newRig(t)
	r.vcam("a", mathx.Vec2{}, 0)
	b := r.vcam("b", mathx.Vec2{X: 10}, -1)
	c := r.vcam("c", mathx.Vec2{Y: 10}, -1)
	r.frame()
	k1, err := r.svc.BlendTo(b, 1, false)
	if err != nil || k1 == 0 {
		t.Fatalf("blend to b: %v %v", k1, err)
	}
	v := r.frames(30)
	mid := v.Camera.Position
	if mid.X <= 0.5 || mid.X >= 9.5 {
		t.Fatalf("halfway through the blend the view is at %v", mid)
	}
	k2, err := r.svc.BlendTo(c, 1, false)
	if err != nil || k2 == 0 {
		t.Fatalf("blend to c: %v %v", k2, err)
	}
	if !r.svc.BlendDone(k1) {
		t.Fatal("the replaced blend must end")
	}
	if v := r.frame(); !near(v.Camera.Position, mid, 0.5) {
		t.Fatalf("new blend jumped from %v to %v", mid, v.Camera.Position)
	}
	v = r.frames(70)
	if !near(v.Camera.Position, mathx.Vec2{Y: 10}, 1e-3) || !r.svc.BlendDone(k2) {
		t.Fatalf("blend to c ended at %v", v.Camera.Position)
	}
}

func TestBlendZoomIsLogarithmic(t *testing.T) {
	r := newRig(t)
	a := r.vcam("a", mathx.Vec2{}, 0)
	b := r.vcam("b", mathx.Vec2{}, -1)
	va, _ := scene.Get[camera.VirtualCamera2D](r.s, a)
	vb, _ := scene.Get[camera.VirtualCamera2D](r.s, b)
	va.Zoom, vb.Zoom = 16, 64
	vb.BlendCurve = curve.Line(0, 0, 1, 1)
	r.frame()
	if _, err := r.svc.BlendTo(b, 1, false); err != nil {
		t.Fatal(err)
	}
	v := r.frames(30)
	if z := v.Camera.Zoom; z < 31 || z > 33 {
		t.Fatalf("zoom halfway between 16 and 64 is %v, want 32", z)
	}
}

func TestDestroyedTargetEndsTheBlend(t *testing.T) {
	r := newRig(t)
	a := r.vcam("a", mathx.Vec2{}, 0)
	b := r.vcam("b", mathx.Vec2{X: 10}, -1)
	r.frame()
	k, _ := r.svc.BlendTo(b, 1, false)
	r.frames(10)
	r.s.Destroy(b)
	r.frame()
	if r.svc.Active("") != a || !r.svc.BlendDone(k) {
		t.Fatal("destroying the blend target must fall back and end the blend")
	}
}

func TestCutIsInstant(t *testing.T) {
	r := newRig(t)
	r.vcam("a", mathx.Vec2{}, 0)
	b := r.vcam("b", mathx.Vec2{X: 10}, -1)
	r.frame()
	if _, err := r.svc.BlendTo(b, 0, true); err != nil {
		t.Fatal(err)
	}
	if v := r.frame(); v.Camera.Position != (mathx.Vec2{X: 10}) {
		t.Fatalf("cut landed at %v", v.Camera.Position)
	}
	r.svc.Release("", 0)
	r.frame()
	if r.svc.Active("") == b {
		t.Fatal("Release must hand control back to priorities")
	}
}

func (r *rig) zone(name string, p mathx.Vec2, prio int32, target scene.NodeHandle) scene.NodeHandle {
	h := r.vcam(name, p, -1)
	z := scene.Add[camera.CameraZone2D](r.s, h)
	*z = camera.DefaultZone()
	z.Priority = prio
	z.Area = mathx.Rect{X: -2, Y: -2, W: 4, H: 4}
	z.Target.Set(r.s, target)
	return h
}

func TestOverlappingZones(t *testing.T) {
	r := newRig(t)
	base := r.vcam("base", mathx.Vec2{}, 0)
	tg := r.node("target", mathx.Vec2{})
	za := r.zone("za", mathx.Vec2{X: 10}, 10, tg)
	zb := r.zone("zb", mathx.Vec2{X: 11}, 20, tg)
	steps := []struct {
		x    float32
		want scene.NodeHandle
	}{{0, base}, {8.5, za}, {10, zb}, {8.5, za}, {0, base}}
	for _, st := range steps {
		r.s.SetPosition(tg, mathx.Vec2{X: st.x})
		r.frame()
		if got := r.svc.Active(""); got != st.want {
			t.Fatalf("target at %v: active %v, want %v", st.x, got, st.want)
		}
	}
}

func TestFollowDeadzone(t *testing.T) {
	r := newRig(t)
	tg := r.node("target", mathx.Vec2{X: 3, Y: 1})
	h := r.vcam("follow", mathx.Vec2{}, 0)
	f := scene.Add[camera.CameraFollow2D](r.s, h)
	*f = camera.DefaultFollow()
	f.Target.Set(r.s, tg)
	start := r.frame().Camera.Position
	if start != (mathx.Vec2{X: 3, Y: 1}) {
		t.Fatalf("the first frame snaps to the target, got %v", start)
	}
	r.s.SetPosition(tg, mathx.Vec2{X: 3.5, Y: 1.2})
	if v := r.frames(30); v.Camera.Position != start {
		t.Fatalf("camera moved inside the deadzone: %v", v.Camera.Position)
	}
	r.s.SetPosition(tg, mathx.Vec2{X: 8, Y: 1})
	if v := r.frames(120); v.Camera.Position.X <= 5 {
		t.Fatalf("camera did not follow out of the deadzone: %v", v.Camera.Position)
	}
}

func TestBoundsSmallerThanView(t *testing.T) {
	r := newRig(t)
	h := r.vcam("a", mathx.Vec2{X: 50}, 0)
	vc, _ := scene.Get[camera.VirtualCamera2D](r.s, h)
	vc.UseBounds, vc.Bounds = true, mathx.Rect{X: 0, Y: 0, W: 4, H: 2}
	for range 10 {
		if v := r.frame(); v.Camera.Position != (mathx.Vec2{X: 2, Y: 1}) {
			t.Fatalf("view must centre on small bounds, got %v", v.Camera.Position)
		}
	}
}

func TestTwoOutputs(t *testing.T) {
	r := newRig(t)
	r.vcam("main", mathx.Vec2{X: 1}, 0)
	m := r.vcam("mini", mathx.Vec2{X: 7}, 0)
	vc, _ := scene.Get[camera.VirtualCamera2D](r.s, m)
	vc.Output = "mini"
	cam2 := r.node("cam2", mathx.Vec2{})
	scene.Add[camera.CameraBrain2D](r.s, cam2).Output = "mini"
	if v := r.frame(); v.Camera.Position != (mathx.Vec2{X: 1}) {
		t.Fatalf("main output at %v", v.Camera.Position)
	}
	v := r.svc.Adjust(cam2, baseView())
	if v.Camera.Position != (mathx.Vec2{X: 7}) || r.svc.Active("mini") != m {
		t.Fatalf("mini output at %v", v.Camera.Position)
	}
}

func TestShakeIsDeterministicAndLeavesTheNode(t *testing.T) {
	a, b := newRig(t), newRig(t)
	ha := a.vcam("a", mathx.Vec2{}, 0)
	b.vcam("a", mathx.Vec2{}, 0)
	a.frame()
	b.frame()
	a.svc.Shake("", 1)
	b.svc.Shake("", 1)
	moved := false
	for range 30 {
		va, vb := a.frame(), b.frame()
		if va.Camera != vb.Camera {
			t.Fatalf("shake differs: %+v vs %+v", va.Camera, vb.Camera)
		}
		if va.Camera.Position != (mathx.Vec2{}) {
			moved = true
		}
	}
	if !moved {
		t.Fatal("shake did not move the view")
	}
	if a.s.Local(ha).Position != (mathx.Vec2{}) {
		t.Fatal("shake must not touch the node")
	}
}

func TestShakeAccessibility(t *testing.T) {
	r := newRig(t)
	r.vcam("a", mathx.Vec2{}, 0)
	r.svc.SetAccessibility(0, false)
	r.svc.Shake("", 1)
	prev := r.svc.Trauma("")
	for range 30 {
		if v := r.frame(); v.Camera.Position != (mathx.Vec2{}) || v.Camera.Rotation != 0 {
			t.Fatalf("shake with screen shake 0: %+v", v.Camera)
		}
	}
	if r.svc.Trauma("") >= prev {
		t.Fatal("trauma must still fade")
	}
}

func TestZeroAllocs(t *testing.T) {
	r := newRig(t)
	tg := r.node("target", mathx.Vec2{})
	for i := range 8 {
		h := r.vcam("vc", mathx.Vec2{X: float32(i)}, int32(i%3))
		f := scene.Add[camera.CameraFollow2D](r.s, h)
		*f = camera.DefaultFollow()
		f.Target.Set(r.s, tg)
		*scene.Add[camera.CameraShake2D](r.s, h) = camera.DefaultShake()
	}
	for i := range 4 {
		r.zone("zone", mathx.Vec2{X: float32(i * 5)}, 10, tg)
	}
	r.frames(10)
	r.svc.Shake("", 0.5)
	x := float32(0)
	allocs := testing.AllocsPerRun(200, func() {
		x += 0.05
		r.s.SetPosition(tg, mathx.Vec2{X: x})
		r.frame()
	})
	if allocs != 0 {
		t.Fatalf("%v allocations per frame", allocs)
	}
}

func BenchmarkBrainStep(b *testing.B) {
	r := newRig(b)
	tg := r.node("target", mathx.Vec2{})
	for i := range 8 {
		h := r.vcam("vc", mathx.Vec2{X: float32(i)}, int32(i%3))
		f := scene.Add[camera.CameraFollow2D](r.s, h)
		*f = camera.DefaultFollow()
		f.Target.Set(r.s, tg)
	}
	r.frames(10)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		r.s.SetPosition(tg, mathx.Vec2{X: float32(i%200) * 0.05})
		r.frame()
	}
}
