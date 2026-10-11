package render3d

import (
	"testing"

	"aego/asset"
	"aego/asset/loader"
	"aego/core/log"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/module"
	"aego/engine/res"
	"aego/gpu/null"
	"aego/render"
	"aego/scene"
	"aego/scene/frame"
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
	if err := frame.RegisterTypes(types); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Load(types, nil, New()); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, ld, scene.Options{DefaultSpace: scene.Space3D})
	cam := s.Create3D(s.Root(), "Camera")
	*scene.Add[Camera3D](s, cam) = DefaultCamera3D()
	s.SetPosition3D(cam, mathx.Vec3{Z: 5})
	for i, z := range []float32{0, 20} {
		n := s.Create3D(s.Root(), "Cube")
		mr := scene.Add[MeshRenderer](s, n)
		*mr = DefaultMeshRenderer()
		mr.Mesh.SetAssetRefID(asset.BuiltinCube)
		s.SetPosition3D(n, mathx.Vec3{X: float32(i), Z: z})
	}
	s.Flush()

	r := frame.New()
	r.Bind(s, rnd, ld)
	w := engine.NewWorld("test")
	w.SetService(frame.ServiceID, r)
	ctx := &engine.Context{World: w}
	r.Prepare(1280, 720, 1)
	cameras{}.Run(ctx)
	fr := dev.BeginFrame(1280, 720)
	rnd.BeginFrame(fr, 1280, 720, 1)
	extract{}.Run(ctx)
	r.Submit(rnd)
	st := rnd.EndFrame()
	dev.EndFrame()

	if rs := r.Stats(); rs.Views != 1 || rs.Meshes != 2 || rs.MissingAssets != 0 {
		t.Fatalf("runner stats = %+v", rs)
	}
	if st.MeshDraws != 1 || st.MeshCulled != 1 {
		t.Fatalf("render stats draws=%d culled=%d", st.MeshDraws, st.MeshCulled)
	}
}
