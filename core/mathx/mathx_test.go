package mathx

import "testing"

const eps = 1e-4

func TestRotatedClockwiseInYDown(t *testing.T) {
	got := Vec2{X: 1}.Rotated(Pi / 2)
	if !got.NearlyEqual(Vec2{Y: 1}, eps) {
		t.Fatalf("rotated = %v", got)
	}
}

func TestTRSApply(t *testing.T) {
	m := Mat3TRS(Vec2{X: 10, Y: 20}, Pi/2, Vec2{X: 2, Y: 2})
	got := m.Apply(Vec2{X: 1})
	if !got.NearlyEqual(Vec2{X: 10, Y: 22}, eps) {
		t.Fatalf("apply = %v", got)
	}
	composed := Mat3TRS(Vec2{X: 10, Y: 20}, 0, Vec2{X: 1, Y: 1}).Mul(Mat3TRS(Vec2{}, Pi/2, Vec2{X: 2, Y: 2}))
	if !composed.Apply(Vec2{X: 1}).NearlyEqual(got, eps) {
		t.Fatalf("mul mismatch: %v", composed.Apply(Vec2{X: 1}))
	}
}

func TestInverse(t *testing.T) {
	m := Mat3TRS(Vec2{X: -3, Y: 7}, 0.7, Vec2{X: 1.5, Y: 0.5})
	inv, _ := m.Inverse()
	for _, p := range []Vec2{{X: 0, Y: 0}, {X: 1, Y: 2}, {X: -5, Y: 3.5}} {
		back := inv.Apply(m.Apply(p))
		if !back.NearlyEqual(p, eps) {
			t.Fatalf("roundtrip %v -> %v", p, back)
		}
	}
	id := m.Mul(inv)
	for i, v := range Mat3Identity() {
		if !NearlyEqual(id[i], v, eps) {
			t.Fatalf("m*inv not identity: %v", id)
		}
	}
}

func TestOrthoYDown(t *testing.T) {
	m := Mat4Ortho(0, 800, 600, 0, -1, 1)
	if got := m.Apply(Vec2{}); !got.NearlyEqual(Vec2{X: -1, Y: 1}, eps) {
		t.Fatalf("top-left -> %v", got)
	}
	if got := m.Apply(Vec2{X: 800, Y: 600}); !got.NearlyEqual(Vec2{X: 1, Y: -1}, eps) {
		t.Fatalf("bottom-right -> %v", got)
	}
	if got := m.Apply(Vec2{X: 400, Y: 300}); !got.NearlyEqual(Vec2{}, eps) {
		t.Fatalf("center -> %v", got)
	}
}

func TestMat4FromMat3(t *testing.T) {
	m3 := Mat3TRS(Vec2{X: 4, Y: -2}, 1.1, Vec2{X: 3, Y: 2})
	m4 := Mat4FromMat3(m3)
	p := Vec2{X: 0.3, Y: -0.9}
	if !m4.Apply(p).NearlyEqual(m3.Apply(p), eps) {
		t.Fatal("mat4 from mat3 differs")
	}
}

func TestAABB(t *testing.T) {
	a := Rect{X: 0, Y: 0, W: 10, H: 10}.ToAABB()
	b := Rect{X: 5, Y: 5, W: 10, H: 10}.ToAABB()
	c := Rect{X: 10, Y: 0, W: 5, H: 5}.ToAABB()
	if !a.Intersects(b) || a.Intersects(c) {
		t.Fatal("intersects wrong")
	}
	if !(Rect{W: 2, H: 2}).Contains(Vec2{X: 1, Y: 1}) {
		t.Fatal("contains wrong")
	}
}

func TestColor(t *testing.T) {
	c := ColorRGB8(255, 0, 128)
	if c.R != 1 || c.G != 0 || !NearlyEqual(c.B, 0.2158, 1e-3) {
		t.Fatalf("srgb decode: %v", c)
	}
	if got := (Color{R: 1, G: 0, B: 0, A: 1}).PackRGBA8(); got != 0xff0000ff {
		t.Fatalf("pack = %#x", got)
	}
	p := Color{R: 1, G: 1, B: 1, A: 0.5}.Premultiplied()
	if p.R != 0.5 {
		t.Fatalf("premultiplied: %v", p)
	}
}
