package mesh

import "testing"

func TestPrimitivesFaceOutwardAndFitBounds(t *testing.T) {
	cases := map[string]*Data{
		"cube":     Cube(1),
		"plane":    Plane(2, 2, 3),
		"sphere":   Sphere(0.5, 8, 12),
		"cylinder": Cylinder(0.5, 1, 12),
	}
	const eps = 1e-5
	for name, d := range cases {
		if len(d.Indices)%3 != 0 || len(d.Indices) == 0 {
			t.Fatalf("%s: index count %d", name, len(d.Indices))
		}
		for i := 0; i < len(d.Indices); i += 3 {
			ia, ib, ic := d.Indices[i], d.Indices[i+1], d.Indices[i+2]
			if int(max(ia, ib, ic)) >= len(d.Vertices) {
				t.Fatalf("%s: index out of range at %d", name, i)
			}
			a, b, c := d.Vertices[ia], d.Vertices[ib], d.Vertices[ic]
			n := b.Pos.Sub(a.Pos).Cross(c.Pos.Sub(a.Pos))
			if n.LenSq() < 1e-12 {
				continue
			}
			if n.Dot(a.Normal.Add(b.Normal).Add(c.Normal)) <= 0 {
				t.Fatalf("%s: triangle %d winds against its normals", name, i/3)
			}
		}
		for _, v := range d.Vertices {
			p, lo, hi := v.Pos, d.Bounds.Min, d.Bounds.Max
			if p.X < lo.X-eps || p.Y < lo.Y-eps || p.Z < lo.Z-eps || p.X > hi.X+eps || p.Y > hi.Y+eps || p.Z > hi.Z+eps {
				t.Fatalf("%s: vertex %v outside bounds %v", name, p, d.Bounds)
			}
			if l := v.Normal.Len(); l < 0.99 || l > 1.01 {
				t.Fatalf("%s: normal %v is not unit", name, v.Normal)
			}
		}
	}
}
