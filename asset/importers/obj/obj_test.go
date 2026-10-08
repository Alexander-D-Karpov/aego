package obj

import (
	"testing"

	"aego/core/mathx"
)

const quad = `# quad
v 0 0 0
v 1 0 0
v 1 1 0
v 0 1 0
vt 0 0
vt 1 0
vt 1 1
vt 0 1
vn 0 0 1
f 1/1/1 2/2/1 3/3/1 4/4/1
`

func TestQuadFanUVAndScale(t *testing.T) {
	d, err := Parse([]byte(quad), Options{Scale: 2, FlipUV: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Vertices) != 4 || len(d.Indices) != 6 {
		t.Fatalf("vertices=%d indices=%d", len(d.Vertices), len(d.Indices))
	}
	if d.Vertices[2].Pos != (mathx.Vec3{X: 2, Y: 2}) {
		t.Fatalf("scaled pos = %v", d.Vertices[2].Pos)
	}
	if d.Vertices[0].UV != (mathx.Vec2{Y: 1}) {
		t.Fatalf("flipped uv = %v", d.Vertices[0].UV)
	}
	if d.Vertices[0].Normal != (mathx.Vec3{Z: 1}) {
		t.Fatalf("normal = %v", d.Vertices[0].Normal)
	}
	if d.Bounds.Max != (mathx.Vec3{X: 2, Y: 2}) {
		t.Fatalf("bounds = %+v", d.Bounds)
	}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSharedVerticesAndNegativeIndices(t *testing.T) {
	src := "v 0 0 0\nv 1 0 0\nv 1 1 0\nv 0 1 0\nf 1 2 3\nf -4 -2 -1\n"
	d, err := Parse([]byte(src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Vertices) != 4 || len(d.Indices) != 6 {
		t.Fatalf("vertices=%d indices=%d", len(d.Vertices), len(d.Indices))
	}
	if !d.Vertices[0].Normal.NearlyEqual(mathx.Vec3{Z: 1}, 1e-6) {
		t.Fatalf("generated normal = %v", d.Vertices[0].Normal)
	}
}

func TestBrokenFilesFail(t *testing.T) {
	for name, src := range map[string]string{
		"out of range": "v 0 0 0\nv 1 0 0\nv 0 1 0\nf 1 2 9\n",
		"bad number":   "v 1 x 2\n",
		"short face":   "v 0 0 0\nv 1 0 0\nf 1 2\n",
		"no faces":     "v 0 0 0\n",
		"bad index":    "v 0 0 0\nv 1 0 0\nv 0 1 0\nf a b c\n",
		"empty":        "",
	} {
		if _, err := Parse([]byte(src), Options{}); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}
