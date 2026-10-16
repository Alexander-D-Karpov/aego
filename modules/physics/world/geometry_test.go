package world

import (
	"errors"
	"testing"

	"aego/core/mathx"
)

func gridMass(c *convex, n int) (float64, Vec2, float64) {
	box := c.aabb(identityXf())
	size := box.Size()
	dx, dy := size.X/float32(n), size.Y/float32(n)
	cell := float64(dx * dy)
	var m, cx, cy, ixx float64
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			p := Vec2{X: box.Min.X + (float32(i)+0.5)*dx, Y: box.Min.Y + (float32(j)+0.5)*dy}
			if pointInConvex(c, p) {
				x, y := float64(p.X), float64(p.Y)
				m += cell
				cx += cell * x
				cy += cell * y
				ixx += cell * (x*x + y*y)
			}
		}
	}
	cx, cy = cx/m, cy/m
	return m, Vec2{X: float32(cx), Y: float32(cy)}, ixx - m*(cx*cx+cy*cy)
}

func TestMassMatchesIntegration(t *testing.T) {
	var pent Polygon
	for i := 0; i < 5; i++ {
		a := float32(i) * mathx.TwoPi / 5
		pent.Pts[i] = Vec2{X: 1.3 * mathx.Cos(a), Y: 0.9 * mathx.Sin(a)}
	}
	pent.N = 5
	cases := []struct {
		name string
		g    Geometry
		at   Transform
	}{
		{"circle", Circle{R: 0.7}, Transform{Position: Vec2{X: 1, Y: -2}}},
		{"box", Box{Half: Vec2{X: 1.5, Y: 0.4}}, Transform{Position: Vec2{X: 0.5, Y: 0.25}, Angle: 0.4}},
		{"capsule", Capsule{HalfLen: 1.2, R: 0.35}, Transform{Position: Vec2{X: -0.3, Y: 0.6}, Angle: 1.1}},
		{"polygon", pent, Transform{Position: Vec2{X: 2, Y: 1}, Angle: -0.3}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := gm(t, c.g, c.at)
			md := g.mass(2)
			m, center, inertia := gridMass(&g.c, 700)
			m, inertia = 2*m, 2*inertia
			if rel := (float64(md.mass) - m) / m; rel > 0.01 || rel < -0.01 {
				t.Fatalf("mass %v, integration %v", md.mass, m)
			}
			if !near2(md.center, center, 0.01) {
				t.Fatalf("center %v, integration %v", md.center, center)
			}
			if rel := (float64(md.inertia) - inertia) / inertia; rel > 0.02 || rel < -0.02 {
				t.Fatalf("inertia %v, integration %v", md.inertia, inertia)
			}
		})
	}
}

func TestCompositeBodyUsesSteiner(t *testing.T) {
	w := newTestWorld(t, nil)
	b := must(w.CreateBody(DefaultBodyDef()))
	for _, x := range []float32{-1, 1} {
		sd := DefaultShapeDef(Circle{R: 0.5})
		sd.Local.Position = Vec2{X: x}
		must(w.AddShape(b, sd))
	}
	m1 := mathx.Pi * 0.25
	wantI := 2 * (m1*0.25/2 + m1*1)
	mass, inertia, center, _ := w.BodyMass(b)
	if !near(mass, 2*m1, 1e-4) || !near(inertia, wantI, 1e-4) || !near2(center, Vec2{}, 1e-5) {
		t.Fatalf("mass %v inertia %v center %v, want %v %v", mass, inertia, center, 2*m1, wantI)
	}
}

func TestMassOverrideScalesDensity(t *testing.T) {
	w := newTestWorld(t, nil)
	d := DefaultBodyDef()
	d.MassMode, d.Mass = MassOverride, 8
	b := must(w.CreateBody(d))
	must(w.AddShape(b, DefaultShapeDef(Box{Half: Vec2{X: 1, Y: 1}})))
	mass, inertia, _, _ := w.BodyMass(b)
	if !near(mass, 8, 1e-5) || !near(inertia, 16.0/3, 1e-4) {
		t.Fatalf("mass %v inertia %v", mass, inertia)
	}
}

func signedArea(c *convex) float32 {
	var a float32
	for i := 0; i < c.count; i++ {
		a += c.v[i].Cross(c.v[(i+1)%c.count])
	}
	return a / 2
}

func TestPolygonValidation(t *testing.T) {
	cw := Polygon{N: 3}
	cw.Pts[0], cw.Pts[1], cw.Pts[2] = Vec2{X: 0, Y: 0}, Vec2{X: 0, Y: 1}, Vec2{X: 1, Y: 0}
	g := gm(t, cw, Transform{})
	if signedArea(&g.c) <= 0 {
		t.Fatal("clockwise polygon was not normalized")
	}
	for i := 0; i < g.c.count; i++ {
		if g.c.n[i].Dot(g.c.centroid.Sub(g.c.v[i])) >= 0 {
			t.Fatalf("normal %d points inwards", i)
		}
	}
	concave := Polygon{N: 4}
	concave.Pts[0], concave.Pts[1], concave.Pts[2], concave.Pts[3] = Vec2{}, Vec2{X: 2}, Vec2{X: 1, Y: 0.3}, Vec2{X: 1, Y: 2}
	bad := []Geometry{
		concave,
		Box{Half: Vec2{X: 0, Y: 1}},
		Circle{R: float32(mathx.Inf())},
		Capsule{HalfLen: 1, R: -1},
		Chain{Pts: []Vec2{{X: 0}}},
	}
	for _, b := range bad {
		if _, err := buildGeom(b, Transform{}, 0.005); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%T %v was accepted: %v", b, b, err)
		}
	}
}
