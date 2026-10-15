package gizmo

import (
	"math"
	"testing"

	"aego/core/mathx"
	"aego/render/view"
	"aego/scene"
)

func one() mathx.Vec3 {
	return mathx.Vec3{X: 1, Y: 1, Z: 1}
}

func testView() view.View {
	eye := mathx.Mat4TRS(mathx.Vec3{Z: 10}, mathx.QuatIdentity(), one())
	return view.PerspectiveView(eye, mathx.Deg2Rad(60), 0.1, 100, mathx.Rect{W: 800, H: 600})
}

func rayTo(p mathx.Vec3) mathx.Ray3 {
	o := mathx.Vec3{Z: 10}
	return mathx.Ray3{Origin: o, Dir: p.Sub(o).Normalized()}
}

func frame() Frame3 {
	return Frame3{View: testView(), Rotation: mathx.QuatIdentity()}
}

func TestPxToWorldScalesWithDistance(t *testing.T) {
	v := testView()
	near := PxToWorld(v, mathx.Vec3{}, 10)
	want := float32(10 * 2 * 10 * math.Tan(math.Pi/6) / 600)
	if mathx.Abs(near-want) > 1e-4 {
		t.Fatalf("px at 10 = %v, want %v", near, want)
	}
	if far := PxToWorld(v, mathx.Vec3{Z: -10}, 10); mathx.Abs(far-2*near) > 1e-4 {
		t.Fatalf("px at 20 = %v", far)
	}
}

func TestAxisDragAndSnap(t *testing.T) {
	d, ok := BeginDrag3(Translate3, Part3X, frame(), rayTo(mathx.Vec3{X: 0.5}))
	if !ok {
		t.Fatal("drag did not start")
	}
	delta, ok := d.Update(rayTo(mathx.Vec3{X: 2}), Snap3{})
	if !ok || !delta.Move.NearlyEqual(mathx.Vec3{X: 1.5}, 1e-3) {
		t.Fatalf("move = %v", delta.Move)
	}
	delta, _ = d.Update(rayTo(mathx.Vec3{X: 1.9}), Snap3{Enabled: true, Move: 1})
	if !delta.Move.NearlyEqual(mathx.Vec3{X: 1}, 1e-5) {
		t.Fatalf("snapped move = %v", delta.Move)
	}
}

func TestPlaneDrag(t *testing.T) {
	d, ok := BeginDrag3(Translate3, Part3XY, frame(), rayTo(mathx.Vec3{X: 0.3, Y: 0.3}))
	if !ok {
		t.Fatal("drag did not start")
	}
	delta, _ := d.Update(rayTo(mathx.Vec3{X: 1.3, Y: -0.7}), Snap3{})
	if !delta.Move.NearlyEqual(mathx.Vec3{X: 1, Y: -1}, 1e-3) {
		t.Fatalf("move = %v", delta.Move)
	}
}

func TestRotateQuarterTurnAndSnap(t *testing.T) {
	d, ok := BeginDrag3(Rotate3, Part3Z, frame(), rayTo(mathx.Vec3{X: 1}))
	if !ok {
		t.Fatal("drag did not start")
	}
	delta, _ := d.Update(rayTo(mathx.Vec3{Y: 1}), Snap3{})
	if got := delta.Rotate.Rotate(mathx.Vec3{X: 1}); !got.NearlyEqual(mathx.Vec3{Y: 1}, 1e-4) {
		t.Fatalf("rotated = %v", got)
	}
	a := mathx.Deg2Rad(50)
	delta, _ = d.Update(rayTo(mathx.Vec3{X: float32(math.Cos(float64(a))), Y: float32(math.Sin(float64(a)))}), Snap3{Enabled: true, Angle: 30})
	b := mathx.Deg2Rad(60)
	want := mathx.Vec3{X: float32(math.Cos(float64(b))), Y: float32(math.Sin(float64(b)))}
	if got := delta.Rotate.Rotate(mathx.Vec3{X: 1}); !got.NearlyEqual(want, 1e-4) {
		t.Fatalf("snapped = %v", got)
	}
}

func TestScaleAxis(t *testing.T) {
	d, ok := BeginDrag3(Scale3, Part3X, frame(), rayTo(mathx.Vec3{X: 1}))
	if !ok {
		t.Fatal("drag did not start")
	}
	delta, _ := d.Update(rayTo(mathx.Vec3{X: 2}), Snap3{})
	if !delta.Scale.NearlyEqual(mathx.Vec3{X: 2, Y: 1, Z: 1}, 1e-3) {
		t.Fatalf("scale = %v", delta.Scale)
	}
}

func TestHit3(t *testing.T) {
	f := frame()
	l := f.Length()
	cases := []struct {
		op   Op3
		at   mathx.Vec3
		want Part3
	}{
		{Translate3, mathx.Vec3{X: l * 0.7}, Part3X},
		{Translate3, mathx.Vec3{Y: l * 0.7}, Part3Y},
		{Translate3, mathx.Vec3{X: l * 0.3, Y: l * 0.3}, Part3XY},
		{Translate3, mathx.Vec3{}, Part3Center},
		{Rotate3, mathx.Vec3{X: l}, Part3Z},
		{Translate3, mathx.Vec3{X: l * 3, Y: l * 3}, Part3None},
	}
	for _, c := range cases {
		if got := Hit3(c.op, f, rayTo(c.at)); got != c.want {
			t.Fatalf("op %d at %v: got %d want %d", c.op, c.at, got, c.want)
		}
	}
}

func TestApplyDelta3UnderRotatedParent(t *testing.T) {
	pr := mathx.QuatAxisAngle(mathx.Vec3{Y: 1}, mathx.Pi/2)
	parent := mathx.Mat4TRS(mathx.Vec3{X: 5}, pr, one())
	local := scene.IdentityTransform3D()

	d := IdentityDelta3()
	d.Move = mathx.Vec3{X: 1}
	if got := ApplyDelta3(parent, local, mathx.Vec3{X: 5}, d); !got.Position.NearlyEqual(mathx.Vec3{Z: 1}, 1e-5) {
		t.Fatalf("local move = %v", got.Position)
	}

	d = IdentityDelta3()
	d.Rotate = mathx.QuatAxisAngle(mathx.Vec3{Y: 1}, mathx.Pi/2)
	got := ApplyDelta3(parent, local, mathx.Vec3{X: 5}, d)
	if !got.Position.NearlyEqual(mathx.Vec3{}, 1e-5) {
		t.Fatalf("rotation about own pivot moved the node: %v", got.Position)
	}
	if w := pr.Mul(got.Rotation).Rotate(mathx.Vec3{X: 1}); !w.NearlyEqual(mathx.Vec3{X: -1}, 1e-4) {
		t.Fatalf("world x after rotate = %v", w)
	}
}
