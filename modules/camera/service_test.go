package camera

import (
	"testing"

	"aego/core/mathx"
	"aego/render/view"
	"aego/scene"
	"aego/schema"
)

const step = float32(1) / 60

func newScene(t *testing.T) *scene.Scene {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := schema.RegisterEnum[TimeDomain](types.Schema(), "camera.TimeDomain",
		schema.EnumValue{Name: "unscaled", Value: int64(Unscaled)},
		schema.EnumValue{Name: "game", Value: int64(Game)},
	); err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{
		scene.RegisterComponent[VirtualCamera2D](types, schema.Name(NameVirtual)),
		scene.RegisterComponent[CameraFollow2D](types, schema.Name(NameFollow)),
		scene.RegisterComponent[CameraShake2D](types, schema.Name(NameShake)),
		scene.RegisterComponent[CameraZone2D](types, schema.Name(NameZone)),
		scene.RegisterComponent[CameraBrain2D](types, schema.Name(NameBrain)),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return scene.New(types, nil, scene.Options{})
}

func vcam(s *scene.Scene, name string, at mathx.Vec2, prio int32, blend float32) scene.NodeHandle {
	h := s.Create(s.Root(), name)
	s.SetPosition(h, at)
	vc := scene.Add[VirtualCamera2D](s, h)
	vc.Priority, vc.BlendTime = prio, blend
	return h
}

func baseView() view.View {
	return view.View{Viewport: mathx.Rect{W: 1, H: 1}, TargetW: 960, TargetH: 540, PixelScale: 1,
		Camera: view.Camera2D{Zoom: 32}, Scale: view.ScaleFitVirtual, Virtual: mathx.Vec2{X: 960, Y: 540}}
}

func frame(svc *Service, cam scene.NodeHandle) view.View {
	svc.Update(step, step)
	v := baseView()
	return svc.Adjust(cam, v)
}

func TestPriorityBlendAndRelease(t *testing.T) {
	s := newScene(t)
	cam := s.Create(s.Root(), "cam")
	wide := vcam(s, "wide", mathx.Vec2{}, 0, 0.5)
	close := vcam(s, "close", mathx.Vec2{X: 10}, -1, 0.5)
	s.Flush()
	s.UpdateTransforms()
	svc := NewService(s, nil)
	if v := frame(svc, cam); v.Camera.Position != (mathx.Vec2{}) || svc.Active("") != wide {
		t.Fatalf("wide camera drives the view: %v", v.Camera.Position)
	}
	vc, _ := scene.Get[VirtualCamera2D](s, close)
	vc.Priority = 5
	mid := frame(svc, cam)
	if svc.Active("") != close || mid.Camera.Position.X <= 0 || mid.Camera.Position.X >= 10 {
		t.Fatalf("switch blends: active %v, x %v", svc.Active(""), mid.Camera.Position.X)
	}
	var v view.View
	for range 40 {
		v = frame(svc, cam)
	}
	if v.Camera.Position.X != 10 {
		t.Fatalf("blend ends on the new camera: %v", v.Camera.Position.X)
	}
	if k, err := svc.BlendTo(wide, 0, true); err != nil || k != 0 {
		t.Fatalf("cut has no blend: %v %v", k, err)
	}
	if v = frame(svc, cam); svc.Active("") != wide || v.Camera.Position.X != 0 {
		t.Fatalf("cut overrides priority: %v", v.Camera.Position.X)
	}
	if svc.Release("", 0) == 0 || svc.Active("") != close {
		t.Fatal("release blends back to the highest priority")
	}
}

func TestZoneLiftsCamera(t *testing.T) {
	s := newScene(t)
	cam := s.Create(s.Root(), "cam")
	target := s.Create(s.Root(), "target")
	base := vcam(s, "base", mathx.Vec2{}, 0, 0)
	zn := vcam(s, "zone", mathx.Vec2{X: 20}, -1, 0)
	z := scene.Add[CameraZone2D](s, zn)
	*z = DefaultZone()
	z.Target = scene.RefTo(s, target)
	z.Zoom = 50
	s.Flush()
	s.UpdateTransforms()
	svc := NewService(s, nil)
	frame(svc, cam)
	if svc.Active("") != base {
		t.Fatal("outside the zone the base camera drives")
	}
	s.SetPosition(target, mathx.Vec2{X: 20})
	s.Flush()
	s.UpdateTransforms()
	if v := frame(svc, cam); svc.Active("") != zn || v.Camera.Zoom != 50 {
		t.Fatalf("inside the zone its camera drives with its zoom: %v", v.Camera.Zoom)
	}
	s.SetPosition(target, mathx.Vec2{X: 40})
	s.Flush()
	s.UpdateTransforms()
	if frame(svc, cam); svc.Active("") != base {
		t.Fatal("leaving the zone hands the view back")
	}
	if vc, _ := scene.Get[VirtualCamera2D](s, zn); vc.Priority != -1 {
		t.Fatal("zones never write camera fields")
	}
}

func TestOutputsAreIndependent(t *testing.T) {
	s := newScene(t)
	main := s.Create(s.Root(), "main")
	mini := s.Create(s.Root(), "mini")
	scene.Add[CameraBrain2D](s, mini).Output = "map"
	vcam(s, "a", mathx.Vec2{X: 1}, 0, 0)
	m := vcam(s, "m", mathx.Vec2{X: 7}, 0, 0)
	vm, _ := scene.Get[VirtualCamera2D](s, m)
	vm.Output = "map"
	s.Flush()
	s.UpdateTransforms()
	svc := NewService(s, nil)
	svc.Update(step, step)
	a, b := baseView(), baseView()
	a, b = svc.Adjust(main, a), svc.Adjust(mini, b)
	if a.Camera.Position.X != 1 || b.Camera.Position.X != 7 {
		t.Fatalf("each Camera 2D shows its output: %v %v", a.Camera.Position.X, b.Camera.Position.X)
	}
	svc.Shake("map", 2)
	if svc.Trauma("map") != 1 || svc.Trauma("") != 0 {
		t.Fatal("trauma is per output and capped at 1")
	}
}

func TestShakeFadesAndRespectsReducedMotion(t *testing.T) {
	s := newScene(t)
	cam := s.Create(s.Root(), "cam")
	vcam(s, "a", mathx.Vec2{}, 0, 0)
	s.Flush()
	s.UpdateTransforms()
	svc := NewService(s, nil)
	svc.Shake("", 0.6)
	v := frame(svc, cam)
	if v.Camera.Position == (mathx.Vec2{}) {
		t.Fatal("trauma moves the view")
	}
	if svc.Logical("").Position != (mathx.Vec2{}) {
		t.Fatal("the logical pose has no shake")
	}
	for range 30 {
		svc.Update(step, step)
	}
	if svc.Trauma("") != 0 {
		t.Fatalf("trauma fades by Decay per second: %v", svc.Trauma(""))
	}
	svc.Shake("", 1)
	svc.reduced = true
	v = baseView()
	svc.ticks++
	v = svc.Adjust(cam, v)
	if v.Camera.Position != (mathx.Vec2{}) {
		t.Fatal("reduced motion turns shake off")
	}
}

func TestFollowSnapsOnTeleport(t *testing.T) {
	s := newScene(t)
	cam := s.Create(s.Root(), "cam")
	target := s.Create(s.Root(), "target")
	h := vcam(s, "f", mathx.Vec2{}, 0, 0)
	f := scene.Add[CameraFollow2D](s, h)
	*f = DefaultFollow()
	f.Target = scene.RefTo(s, target)
	s.Flush()
	s.UpdateTransforms()
	svc := NewService(s, nil)
	frame(svc, cam)
	s.SetPosition(target, mathx.Vec2{X: 100})
	s.Flush()
	s.UpdateTransforms()
	if v := frame(svc, cam); v.Camera.Position.X != 100 {
		t.Fatalf("a jump over Snap Distance moves the camera at once: %v", v.Camera.Position.X)
	}
	s.SetPosition(target, mathx.Vec2{X: 103})
	s.Flush()
	s.UpdateTransforms()
	if v := frame(svc, cam); v.Camera.Position.X <= 100 || v.Camera.Position.X >= 103 {
		t.Fatalf("a short move is damped: %v", v.Camera.Position.X)
	}
}
