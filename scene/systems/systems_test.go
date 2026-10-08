package systems

import (
	"testing"

	"aego/asset"
	"aego/asset/loader"
	"aego/core/log"
	"aego/core/mathx"
	"aego/engine/res"
	"aego/gpu/null"
	"aego/render"
	"aego/scene"
	"aego/scene/components"
	"aego/schema"
	"aego/vfs"
)

func TestCamera3DExtractsMeshes(t *testing.T) {
	files, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	dev := null.New()
	rnd, err := render.New(dev, dev.Caps(), files, log.Nop(), render.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer rnd.Close()
	ld := loader.New(dev, rnd.Textures(), log.Nop())
	defer ld.Close()

	types := scene.NewTypes(schema.NewRegistry())
	if err := components.Register(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, ld, scene.Options{DefaultSpace: scene.Space3D})
	cam := s.Create3D(s.Root(), "Camera")
	*scene.Add[components.Camera3D](s, cam) = components.DefaultCamera3D()
	s.SetPosition3D(cam, mathx.Vec3{Z: 5})
	for i, z := range []float32{0, 20} {
		n := s.Create3D(s.Root(), "Cube")
		mr := scene.Add[components.MeshRenderer](s, n)
		*mr = components.DefaultMeshRenderer()
		mr.Mesh.SetAssetRefID(asset.BuiltinCube)
		s.SetPosition3D(n, mathx.Vec3{X: float32(i), Z: z})
	}
	s.Flush()

	r := New()
	r.Bind(s, rnd, ld)
	r.Prepare(1280, 720, 1)
	fr := dev.BeginFrame(1280, 720)
	rnd.BeginFrame(fr, 1280, 720, 1)
	r.Extract(rnd)
	st := rnd.EndFrame()
	dev.EndFrame()

	if rs := r.Stats(); rs.Views != 1 || rs.Meshes != 2 || rs.MissingAssets != 0 {
		t.Fatalf("runner stats = %+v", rs)
	}
	if st.MeshDraws != 1 || st.MeshCulled != 1 {
		t.Fatalf("render stats draws=%d culled=%d", st.MeshDraws, st.MeshCulled)
	}
}
