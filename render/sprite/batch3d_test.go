package sprite

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render/texture"
)

func TestInstance3DBasis(t *testing.T) {
	reg := texture.Whole(gpu.TextureHandle{Index: 1, Gen: 1}, 4, 4)
	d := Draw3D{Region: reg, World: mathx.Mat4Identity(), Size: mathx.Vec2{X: 2, Y: 1}, Origin: mathx.Vec2{X: 0.5, Y: 0.5}}
	in := d.Instance()
	if !in.Pos.NearlyEqual(mathx.Vec3{X: -1, Y: 0.5}, 1e-5) || !in.Right.NearlyEqual(mathx.Vec3{X: 2}, 1e-5) || !in.Down.NearlyEqual(mathx.Vec3{Y: -1}, 1e-5) {
		t.Fatalf("instance = %+v", in)
	}
	if !in.Center().NearlyEqual(mathx.Vec3{}, 1e-5) {
		t.Fatalf("center = %v", in.Center())
	}
	lifted := Draw3D{Region: reg, World: mathx.Mat4Lift(mathx.Mat3TRS(mathx.Vec2{X: 3, Y: 4}, 0, mathx.Vec2{X: 1, Y: 1})), Size: mathx.Vec2{X: 1, Y: 1}}
	if p := lifted.Instance().Pos; !p.NearlyEqual(mathx.Vec3{X: 3, Y: -4}, 1e-5) {
		t.Fatalf("2D top-left (3, 4) must land at (3, -4, 0): %v", p)
	}
}

func TestBatcher3DSortsBackToFront(t *testing.T) {
	dev := null.New()
	b, err := NewBatcher3D(dev, 16)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	reg := texture.Whole(gpu.TextureHandle{Index: 1, Gen: 1}, 4, 4)
	cam := mathx.Vec3{Z: 10}
	push := func(z float32, layer int16) {
		d := Draw3D{Region: reg, World: mathx.Mat4Translate(mathx.Vec3{Z: z}), Size: mathx.Vec2{X: 1, Y: 1}, Origin: mathx.Vec2{X: 0.5, Y: 0.5}, Layer: layer}
		b.Push(d.Instance(), &d, gpu.SamplerHandle{Index: 1, Gen: 1}, cam)
	}
	push(0, 0)
	push(-5, 0)
	push(2, 0)
	push(9, -1)
	b.Sort()
	got := b.Instances()
	want := []float32{9, -5, 0, 2}
	for i, z := range want {
		if got[i].Pos.Z != z {
			t.Fatalf("order = %v", got)
		}
	}
	if batches := b.Build(); len(batches) != 1 || batches[0].Count != 4 {
		t.Fatalf("batches = %+v", batches)
	}
}
