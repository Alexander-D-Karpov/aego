package mesh

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render/sprite"
	"aego/render/texture"
)

func outward(t *testing.T, name string, d *Data) {
	t.Helper()
	for i := 0; i < len(d.Indices); i += 3 {
		a := d.Vertices[d.Indices[i]].Pos
		b := d.Vertices[d.Indices[i+1]].Pos
		c := d.Vertices[d.Indices[i+2]].Pos
		n := b.Sub(a).Cross(c.Sub(a))
		if n.LenSq() < 1e-12 {
			continue
		}
		centroid := a.Add(b).Add(c).Scale(1.0 / 3)
		if n.Dot(centroid) <= 0 {
			t.Fatalf("%s: triangle %d faces inward", name, i/3)
		}
	}
}

func TestPrimitives(t *testing.T) {
	cube := Cube(2)
	if len(cube.Vertices) != 24 || len(cube.Indices) != 36 {
		t.Fatalf("cube = %d/%d", len(cube.Vertices), len(cube.Indices))
	}
	if cube.Bounds != (mathx.AABB3{Min: mathx.Vec3{X: -1, Y: -1, Z: -1}, Max: mathx.Vec3{X: 1, Y: 1, Z: 1}}) {
		t.Fatalf("cube bounds = %+v", cube.Bounds)
	}
	outward(t, "cube", cube)
	outward(t, "sphere", Sphere(1, 12, 16))
	outward(t, "cylinder", Cylinder(0.5, 2, 16))
	p := Plane(4, 2, 3)
	for i := 0; i < len(p.Indices); i += 3 {
		a, b, c := p.Vertices[p.Indices[i]].Pos, p.Vertices[p.Indices[i+1]].Pos, p.Vertices[p.Indices[i+2]].Pos
		if b.Sub(a).Cross(c.Sub(a)).Y <= 0 {
			t.Fatal("plane must face +Y")
		}
	}
	for _, d := range []*Data{cube, Sphere(1, 8, 8), Cylinder(1, 1, 8), p} {
		if err := d.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAEMSRoundTrip(t *testing.T) {
	src := Sphere(1.5, 6, 8)
	back, err := Decode(Encode(src))
	if err != nil {
		t.Fatal(err)
	}
	if back.Bounds != src.Bounds || len(back.Vertices) != len(src.Vertices) || len(back.Indices) != len(src.Indices) {
		t.Fatal("header lost")
	}
	for i := range src.Vertices {
		if back.Vertices[i] != src.Vertices[i] {
			t.Fatalf("vertex %d differs", i)
		}
	}
	bad := Encode(Cube(1))
	bad[len(bad)-4] = 200
	if _, err := Decode(bad); err == nil {
		t.Fatal("out of range index accepted")
	}
	if _, err := Decode([]byte("AEMS")); err == nil {
		t.Fatal("truncated artifact accepted")
	}
	if _, err := Decode(Encode(Cube(1))[:60]); err == nil {
		t.Fatal("short body accepted")
	}
}

func TestUploadAndDestroy(t *testing.T) {
	dev := null.New()
	m, err := Upload(dev, Cube(1), "cube")
	if err != nil {
		t.Fatal(err)
	}
	if m.Count != 36 || m.VB.IsZero() || m.IB.IsZero() {
		t.Fatalf("mesh = %+v", m)
	}
	m.Destroy(dev)
	if !m.VB.IsZero() {
		t.Fatal("destroy did not reset the handles")
	}
	if _, err := Upload(dev, &Data{}, "empty"); err == nil {
		t.Fatal("empty mesh uploaded")
	}
}

func TestBatcherOrdering(t *testing.T) {
	m := &Mesh{Count: 36, Bounds: Cube(1).Bounds}
	white := texture.Whole(gpu.TextureHandle{Index: 1, Gen: 1}, 1, 1)
	b := NewBatcher(16)
	cam := mathx.Vec3{Z: 10}
	push := func(z float32, blend sprite.BlendMode, ds bool) {
		d := Draw{Mesh: m, World: mathx.Mat4Translate(mathx.Vec3{Z: z}), Blend: blend, DoubleSided: ds}
		if !b.Push(&d, cam, white, gpu.SamplerHandle{}) {
			t.Fatal("push failed")
		}
	}
	push(0, sprite.BlendOpaque, false)
	push(5, sprite.BlendOpaque, false)
	push(8, sprite.BlendOpaque, true)
	push(-5, sprite.BlendOpaque, false)
	push(0, sprite.BlendPremultiplied, false)
	push(5, sprite.BlendPremultiplied, false)
	push(-5, sprite.BlendAdditive, false)
	b.Sort()
	op := b.Opaque()
	if len(op) != 4 || op[0].World.Translation().Z != 5 || op[1].World.Translation().Z != 0 || op[2].World.Translation().Z != -5 || !op[3].DoubleSided {
		t.Fatal("opaque must be front to back with double sided last")
	}
	tr := b.Transparent()
	if len(tr) != 3 || tr[0].World.Translation().Z != -5 || tr[2].World.Translation().Z != 5 {
		t.Fatal("transparent must be back to front")
	}
	if op[0].Texture != white {
		t.Fatal("missing texture must default to white")
	}
	small := NewBatcher(1)
	d := Draw{Mesh: m, World: mathx.Mat4Identity()}
	small.Push(&d, cam, white, gpu.SamplerHandle{})
	if small.Push(&d, cam, white, gpu.SamplerHandle{}) || small.Dropped() != 1 {
		t.Fatal("budget overflow not counted")
	}
}
