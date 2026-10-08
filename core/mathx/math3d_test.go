package mathx

import "testing"

const eps3 = 1e-4

func TestQuatAxisAngleRotate(t *testing.T) {
	q := QuatAxisAngle(Vec3{Z: 1}, Pi/2)
	if got := q.Rotate(Vec3{X: 1}); !got.NearlyEqual(Vec3{Y: 1}, eps3) {
		t.Fatalf("rotate = %v", got)
	}
	if got := q.Inverse().Rotate(Vec3{Y: 1}); !got.NearlyEqual(Vec3{X: 1}, eps3) {
		t.Fatalf("inverse rotate = %v", got)
	}
}

func TestQuatEulerRoundTrip(t *testing.T) {
	cases := [][3]float32{{0, 0, 0}, {0.3, -0.4, 1.2}, {-1, 0.2, -2.5}, {2, 1, -0.5}}
	for _, c := range cases {
		x, y, z := QuatEuler(c[0], c[1], c[2]).Euler()
		if !NearlyEqual(x, c[0], eps3) || !NearlyEqual(y, c[1], eps3) || !NearlyEqual(z, c[2], eps3) {
			t.Fatalf("%v -> %v %v %v", c, x, y, z)
		}
	}
	q := QuatEuler(0.5, Pi/2, 0.3)
	x, y, z := q.Euler()
	if !QuatEuler(x, y, z).NearlySame(q, eps3) {
		t.Fatal("gimbal lock round trip changed the rotation")
	}
}

func TestQuatMat4MatchesRotate(t *testing.T) {
	q := QuatEuler(0.4, -0.7, 1.1)
	v := Vec3{X: 1, Y: -2, Z: 0.5}
	if !q.Mat4().TransformDir(v).NearlyEqual(q.Rotate(v), eps3) {
		t.Fatal("matrix and quaternion rotate differently")
	}
}

func TestQuatSlerp(t *testing.T) {
	a := QuatIdentity()
	b := QuatAxisAngle(Vec3{Z: 1}, Pi/2)
	if !a.Slerp(b, 0.5).NearlySame(QuatAxisAngle(Vec3{Z: 1}, Pi/4), eps3) {
		t.Fatal("slerp midpoint wrong")
	}
}

func TestQuatLookAt(t *testing.T) {
	f := Vec3{X: 1, Y: 0.5, Z: -1}
	q := QuatLookAt(f, Vec3{Y: 1})
	if !q.Forward().NearlyEqual(f.Normalized(), eps3) {
		t.Fatalf("forward = %v", q.Forward())
	}
	if q.Up().Y <= 0 {
		t.Fatal("up flipped")
	}
	if !QuatLookAt(Vec3{Z: -1}, Vec3{Y: 1}).NearlySame(QuatIdentity(), eps3) {
		t.Fatal("looking down -Z must be identity")
	}
	QuatLookAt(Vec3{Y: 1}, Vec3{Y: 1})
}

func TestMat4InverseAndDecompose(t *testing.T) {
	pos := Vec3{X: 1, Y: 2, Z: 3}
	rot := QuatEuler(0.3, 0.5, -0.2)
	scale := Vec3{X: 2, Y: 3, Z: 0.5}
	m := Mat4TRS(pos, rot, scale)
	p, q, s, ok := m.Decompose()
	if !ok || !p.NearlyEqual(pos, eps3) || !s.NearlyEqual(scale, eps3) || !q.NearlySame(rot, eps3) {
		t.Fatalf("decompose = %v %v %v", p, q, s)
	}
	flipped := Mat4TRS(pos, rot, Vec3{X: -2, Y: 3, Z: 1})
	p, q, s, ok = flipped.Decompose()
	if !ok || s.X >= 0 || !Mat4TRS(p, q, s).NearlyEqual(flipped, eps3) {
		t.Fatal("negative scale does not rebuild the same matrix")
	}
	if _, ok := (Mat4{}).Inverse(); ok {
		t.Fatal("singular matrix inverted")
	}
	if _, ok := Mat4TRS(pos, rot, Vec3{X: 1, Y: 0, Z: 1}).InverseAffine(); ok {
		t.Fatal("zero scale inverted")
	}
}

func TestInverseVariants(t *testing.T) {
	trs := Mat4TRS(Vec3{X: -4, Y: 1.5, Z: 7}, QuatEuler(0.7, -1.1, 0.4), Vec3{X: 3, Y: 0.25, Z: 1.7})
	full, ok := trs.Inverse()
	if !ok || !trs.Mul(full).NearlyEqual(Mat4Identity(), 1e-5) {
		t.Fatal("Inverse failed on a non-uniform TRS")
	}
	aff, ok := trs.InverseAffine()
	if !ok || !trs.Mul(aff).NearlyEqual(Mat4Identity(), 1e-5) {
		t.Fatal("InverseAffine failed on a non-uniform TRS")
	}
	if !aff.NearlyEqual(full, 1e-5) {
		t.Fatal("affine and full inverse disagree")
	}
	vp := Mat4Perspective(Deg2Rad(60), 1.5, 0.1, 100).Mul(Mat4LookAt(Vec3{X: 3, Y: 2, Z: 5}, Vec3{}, Vec3{Y: 1}))
	inv, ok := vp.Inverse()
	if !ok || !vp.Mul(inv).NearlyEqual(Mat4Identity(), 1e-4) {
		t.Fatal("Inverse failed on Perspective×LookAt")
	}
}

func TestLookAtDegenerateUp(t *testing.T) {
	for _, target := range []Vec3{{Y: -3}, {Y: 3}, {X: 1e-7, Y: 3}} {
		m := Mat4LookAt(Vec3{}, target, Vec3{Y: 1})
		for i, v := range m {
			if v != v {
				t.Fatalf("NaN at %d looking at %v", i, target)
			}
		}
		if got := m.TransformPoint(target); !got.NearlyEqual(Vec3{Z: -target.Len()}, 1e-3) {
			t.Fatalf("target in view = %v", got)
		}
	}
}

func TestPerspectiveDegenerate(t *testing.T) {
	if Mat4Perspective(Deg2Rad(60), 1, 5, 5) != Mat4Identity() {
		t.Fatal("near == far must yield identity")
	}
	if Mat4Perspective(0, 1, 0.1, 100) != Mat4Identity() {
		t.Fatal("zero fov must yield identity")
	}
	m := Mat4Perspective(Deg2Rad(60), 0, 0.1, 100)
	if m[0] != m[5] {
		t.Fatal("zero aspect must behave as 1")
	}
}

func TestTransformPointHomogeneous(t *testing.T) {
	p := Mat4Perspective(Deg2Rad(90), 1, 1, 10)
	got := p.TransformPoint(Vec3{X: 2, Z: -2})
	if !NearlyEqual(got.X, 1, eps3) {
		t.Fatalf("perspective divide missing: %v", got)
	}
	if got := Mat4Translate(Vec3{X: 1}).TransformPoint(Vec3{Y: 2}); got != (Vec3{X: 1, Y: 2}) {
		t.Fatalf("affine point = %v", got)
	}
}

func TestPerspectiveAndLookAt(t *testing.T) {
	p := Mat4Perspective(Deg2Rad(60), 1.5, 0.1, 100)
	near := p.MulVec4(Vec4{Z: -0.1, W: 1})
	far := p.MulVec4(Vec4{Z: -100, W: 1})
	if !NearlyEqual(near.Z/near.W, -1, 1e-3) || !NearlyEqual(far.Z/far.W, 1, 1e-3) {
		t.Fatalf("near %v far %v", near.Z/near.W, far.Z/far.W)
	}
	eye := Vec3{Z: 5}
	v := Mat4LookAt(eye, Vec3{}, Vec3{Y: 1})
	if got := v.TransformPoint(Vec3{}); !got.NearlyEqual(Vec3{Z: -5}, eps3) {
		t.Fatalf("origin in view = %v", got)
	}
	if got := v.TransformPoint(Vec3{X: 1}); !got.NearlyEqual(Vec3{X: 1, Z: -5}, eps3) {
		t.Fatalf("+x in view = %v", got)
	}
	target := Vec3{X: 2, Y: 1, Z: -3}
	cam := Mat4TRS(eye, QuatLookAt(target.Sub(eye), Vec3{Y: 1}), Vec3{X: 1, Y: 1, Z: 1})
	inv, _ := cam.Inverse()
	if !inv.NearlyEqual(Mat4LookAt(eye, target, Vec3{Y: 1}), eps3) {
		t.Fatal("LookAt differs from inverse camera TRS")
	}
}

func TestMat4LiftComposes(t *testing.T) {
	a := Mat3TRS(Vec2{X: 3, Y: 4}, 0.5, Vec2{X: 2, Y: 1})
	b := Mat3TRS(Vec2{X: -1, Y: 2}, -0.3, Vec2{X: 1, Y: 1})
	if !Mat4Lift(a).Mul(Mat4Lift(b)).NearlyEqual(Mat4Lift(a.Mul(b)), eps3) {
		t.Fatal("Lift is not a homomorphism")
	}
	p := Vec2{X: 1.5, Y: -2}
	w := a.Apply(p)
	got := Mat4Lift(a).TransformPoint(Vec3{X: p.X, Y: -p.Y})
	if !got.NearlyEqual(Vec3{X: w.X, Y: -w.Y}, eps3) {
		t.Fatalf("lift = %v, want %v", got, w)
	}
}

func TestAABB3Transformed(t *testing.T) {
	b := AABB3{Min: Vec3{X: -0.5, Y: -0.5, Z: -0.5}, Max: Vec3{X: 0.5, Y: 0.5, Z: 0.5}}
	r := b.Transformed(Mat4TRS(Vec3{X: 10}, QuatAxisAngle(Vec3{Z: 1}, Pi/4), Vec3{X: 1, Y: 1, Z: 1}))
	h := Sqrt(2) / 2
	if !r.Extents().NearlyEqual(Vec3{X: h, Y: h, Z: 0.5}, eps3) || !r.Center().NearlyEqual(Vec3{X: 10}, eps3) {
		t.Fatalf("transformed = %+v", r)
	}
}

func TestRayAABB(t *testing.T) {
	b := AABB3{Min: Vec3{X: -0.5, Y: -0.5, Z: -0.5}, Max: Vec3{X: 0.5, Y: 0.5, Z: 0.5}}
	if d, ok := (Ray3{Origin: Vec3{Z: 5}, Dir: Vec3{Z: -1}}).IntersectAABB(b); !ok || !NearlyEqual(d, 4.5, eps3) {
		t.Fatalf("hit = %v %v", d, ok)
	}
	if _, ok := (Ray3{Origin: Vec3{X: 3, Z: 5}, Dir: Vec3{Z: -1}}).IntersectAABB(b); ok {
		t.Fatal("miss reported as hit")
	}
	if _, ok := (Ray3{Origin: Vec3{Z: 5}, Dir: Vec3{Z: 1}}).IntersectAABB(b); ok {
		t.Fatal("box behind the ray reported as hit")
	}
	if d, ok := (Ray3{Dir: Vec3{X: 1}}).IntersectAABB(b); !ok || d != 0 {
		t.Fatal("origin inside must hit at 0")
	}
	pl := PlaneFromPointNormal(Vec3{}, Vec3{Y: 1})
	if d, ok := (Ray3{Origin: Vec3{Y: 2}, Dir: Vec3{Y: -1}}).IntersectPlane(pl); !ok || !NearlyEqual(d, 2, eps3) {
		t.Fatalf("plane hit = %v %v", d, ok)
	}
}

func TestFrustumContainsAABB(t *testing.T) {
	proj := Mat4Perspective(Deg2Rad(60), 1, 0.1, 100)
	view := Mat4LookAt(Vec3{Z: 5}, Vec3{}, Vec3{Y: 1})
	f := FrustumFromMat4(proj.Mul(view))
	box := func(c Vec3) AABB3 {
		return AABB3{Min: c.Sub(Vec3{X: 0.5, Y: 0.5, Z: 0.5}), Max: c.Add(Vec3{X: 0.5, Y: 0.5, Z: 0.5})}
	}
	if !f.ContainsAABB(box(Vec3{})) {
		t.Fatal("box in front of the camera culled")
	}
	if f.ContainsAABB(box(Vec3{Z: 7})) {
		t.Fatal("box behind the camera visible")
	}
	if f.ContainsAABB(box(Vec3{X: 50})) {
		t.Fatal("box far to the side visible")
	}
	if f.ContainsAABB(box(Vec3{Z: -200})) {
		t.Fatal("box past the far plane visible")
	}
}
