package render

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render/mesh"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/render/view"
)

func perspective(w, h int) view.View {
	eye := mathx.Mat4TRS(mathx.Vec3{Z: 5}, mathx.QuatIdentity(), mathx.Vec3{X: 1, Y: 1, Z: 1})
	v := view.PerspectiveView(eye, mathx.Deg2Rad(60), 0.1, 100, mathx.Rect{W: float32(w), H: float32(h)})
	v.TargetW, v.TargetH = w, h
	v.DoClear = true
	return v
}

func cube(t *testing.T, dev gpu.Device) *mesh.Mesh {
	t.Helper()
	m, err := mesh.Upload(dev, mesh.Cube(1), "cube")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPassOrderIncludes3D(t *testing.T) {
	r, _ := newTestRenderer(t, 64)
	if err := r.BeginFrame(nil, 64, 64, 1); err != nil {
		t.Fatal(err)
	}
	r.EndFrame()
	want := []string{"mesh_opaque", "mesh_blend", "sprites3d", "sprites", "debug"}
	if len(r.order) != len(want) {
		t.Fatalf("order = %v", r.order)
	}
	for i, id := range want {
		if r.order[i].ID != id {
			t.Fatalf("pass %d = %s, want %s", i, r.order[i].ID, id)
		}
	}
}

func TestMeshCullingAndDraws(t *testing.T) {
	r, dev := newTestRenderer(t, 64)
	c := cube(t, dev)
	fr := dev.BeginFrame(1280, 720)
	r.BeginFrame(fr, 1280, 720, 1)
	f := r.View(perspective(1280, 720))
	f.Mesh(mesh.Draw{Mesh: c, World: mathx.Mat4Identity(), Blend: sprite.BlendOpaque})
	f.Mesh(mesh.Draw{Mesh: c, World: mathx.Mat4Translate(mathx.Vec3{Z: 10}), Blend: sprite.BlendOpaque})
	f.Mesh(mesh.Draw{Mesh: c, World: mathx.Mat4Translate(mathx.Vec3{X: 1}), Color: mathx.Color{R: 1, G: 1, B: 1, A: 0.5}})
	f.End()
	s := r.EndFrame()
	dev.EndFrame()
	if s.MeshCulled != 1 || s.MeshDraws != 2 {
		t.Fatalf("culled=%d draws=%d", s.MeshCulled, s.MeshDraws)
	}
}

func TestSprite3DCulling(t *testing.T) {
	r, dev := newTestRenderer(t, 64)
	tex, _ := r.Textures().Upload(texture.Solid(4, 4, mathx.Color{R: 1, A: 1}), texture.UploadOptions{})
	reg := texture.Whole(tex, 4, 4)
	fr := dev.BeginFrame(1280, 720)
	r.BeginFrame(fr, 1280, 720, 1)
	f := r.View(perspective(1280, 720))
	f.Sprite3D(sprite.Draw3D{Region: reg, World: mathx.Mat4Identity(), Size: mathx.Vec2{X: 1, Y: 1}})
	f.Sprite3D(sprite.Draw3D{Region: reg, World: mathx.Mat4Translate(mathx.Vec3{Z: 20}), Size: mathx.Vec2{X: 1, Y: 1}})
	f.End()
	s := r.EndFrame()
	dev.EndFrame()
	if s.Sprites3D != 1 || s.Sprite3DCulled != 1 || s.Instances != 1 {
		t.Fatalf("stats = %+v", s)
	}
}

func TestMeshFrameDoesNotAllocate(t *testing.T) {
	r, dev := newTestRenderer(t, 64)
	c := cube(t, dev)
	draws := make([]mesh.Draw, 0, 2000)
	for i := 0; i < 2000; i++ {
		x, z := float32(i%50)-25, -float32(i/50)
		draws = append(draws, mesh.Draw{Mesh: c, World: mathx.Mat4Translate(mathx.Vec3{X: x, Z: z}), Blend: sprite.BlendOpaque})
	}
	v := perspective(1280, 720)
	frame := func() {
		fr := dev.BeginFrame(1280, 720)
		r.BeginFrame(fr, 1280, 720, 1)
		f := r.View(v)
		f.Meshes(draws)
		f.Debug().Line3(mathx.Vec3{}, mathx.Vec3{Y: 1}, mathx.Color{G: 1, A: 1})
		f.End()
		r.EndFrame()
		dev.EndFrame()
	}
	for i := 0; i < 5; i++ {
		frame()
	}
	if n := testing.AllocsPerRun(10, frame); n != 0 {
		t.Fatalf("mesh frame allocates %.1f objects", n)
	}
}

var _ = null.New
