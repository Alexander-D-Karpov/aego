package particles

import (
	"testing"

	"aego/core/log"
	"aego/core/mathx"
	"aego/engine/res"
	"aego/gpu/null"
	"aego/render"
	"aego/render/mesh"
	"aego/render/sprite"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/vfs"
)

func TestParticleDrawsMergeAndSplitAtMesh(t *testing.T) {
	s, a, front := testWorld(t, nil)
	front.Order = 2
	back := s.scene.Create(s.scene.Root(), "back")
	cfg := scene.Add[ParticleSystem2D](s.scene, back)
	*cfg = *front
	cfg.Order = 0
	s.scene.Flush()
	for _, h := range []scene.NodeHandle{a, back} {
		s.scene.SetPosition(h, mathx.V2(32, 32))
		s.Burst(h, 3)
	}
	files, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	dev := null.New()
	defer dev.Close()
	renderer, err := render.New(dev, dev.Caps(), files, log.Nop(), render.Options{MaxSprites: 32})
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()
	wall, err := mesh.Upload(dev, mesh.Cube(1), "wall")
	if err != nil {
		t.Fatal(err)
	}
	defer wall.Destroy(dev)
	r := frame.New()
	r.Bind(s.scene, renderer, nil)
	for _, wallOn := range []bool{false, true} {
		r.Prepare(64, 64, 1)
		r.AddView(view.ScreenView(64, 64, 1), 0)
		b := r.Bucket(0)
		// An ordinary sprite can share a draw with particles without sharing ownership of its upload bytes.
		b.Sprites = append(b.Sprites, sprite.Draw{Position: mathx.V2(30, 30), Size: mathx.V2(1, 1), Region: renderer.White(), Tint: mathx.White})
		if wallOn {
			b.Meshes = append(b.Meshes, mesh.Draw{Mesh: wall, World: mathx.Mat4Translate(mathx.Vec3{X: 32, Y: 32}), Texture: renderer.White(), Color: mathx.White, Is2D: true, DoubleSided: true, Order: 1})
		}
		s.extract(r)
		gf := dev.BeginFrame(64, 64)
		if err := renderer.BeginFrame(gf, 64, 64, 1); err != nil {
			t.Fatal(err)
		}
		r.Submit(renderer)
		actual := renderer.EndFrame()
		dev.EndFrame()
		got := s.Stats()
		if got.Submitted != 6 || got.InstanceUploads != 1 || got.UploadBytes != 6*sprite.InstanceStride {
			t.Fatalf("shared upload: %+v", got)
		}
		want := uint64(1)
		if wallOn {
			want = 2
		}
		if got.Draws != want || uint64(actual.Batches) != want {
			t.Fatalf("wall=%v particle draws %d total sprite batches %d, want %d", wallOn, got.Draws, actual.Batches, want)
		}
	}
}

func TestPreviewDistancePerCameraAndLastGood(t *testing.T) {
	p := NewPreview(nil, nil)
	c := DefaultParticleSystem2D()
	c.Space, c.SortMode, c.PlayOnStart, c.Rate = World, Distance, false, 0
	c.Lifetime, c.Speed = mathx.V2(2, 2), mathx.V2(0, 0)
	if err := p.Configure(c, 42); err != nil {
		t.Fatal(err)
	}
	for _, x := range []float32{12, 6, 6} {
		p.Step(0, mathx.Mat3TRS(mathx.V2(x, 0), 0, mathx.V2(1, 1)))
		p.Burst(1)
	}
	left := view.ScreenView(100, 100, 1)
	left.Camera.Position = mathx.V2(0, 0)
	right := left
	right.Camera.Position = mathx.V2(20, 0)
	a := p.e.pool.Particles()
	for _, test := range []struct {
		view view.View
		want [3]uint64
	}{{left, [3]uint64{a[0].ID, a[1].ID, a[2].ID}}, {right, [3]uint64{a[1].ID, a[2].ID, a[0].ID}}} {
		p.e.sortFor(test.view, mathx.Mat3Identity())
		for i, want := range test.want {
			if p.e.order[i].id != want {
				t.Fatalf("distance order: %+v", p.e.order)
			}
		}
	}
	c.Rate = -1
	if err := p.Configure(c, 999); err == nil {
		t.Fatal("invalid edit replaced working effect")
	}
	p.Burst(2)
	if p.Count() != 5 || p.e.cfg.Rate != 0 || p.seed != 42 {
		t.Fatal("last good was lost")
	}
	bounds, ok := p.Bounds(mathx.Mat3Identity())
	if !ok || bounds.Min.X > 6 || bounds.Max.X < 12 {
		t.Fatal("preview failed to fit detached world particles")
	}
	p.Close()
	p.Burst(8)
	if p.Count() != 0 || p.Configure(DefaultParticleSystem2D(), 2) == nil {
		t.Fatal("closed preview accepted new work")
	}
}
