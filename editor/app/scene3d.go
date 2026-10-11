package app

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/editor/document"
	"aego/scene"
)

const CommandNewScene3D = "file.new_scene_3d"

func (a *App) newScene3D() error {
	var res asset.Resolver
	if a.ctx.Loader != nil {
		res = a.ctx.Loader
	}
	s := scene.New(a.opt.Types, res, scene.Options{DefaultSpace: scene.Space3D})
	a.templates.Spawn(s, s.Root(), "render3d.camera")
	a.templates.Spawn(s, s.Root(), "render3d.plane")
	cube := a.templates.Spawn(s, s.Root(), "render3d.cube")
	s.SetPosition3D(cube, mathx.Vec3{Y: 0.5})
	s.Flush()
	rel := a.ctx.Docs.UniquePath("scenes/untitled3d.aescene")
	d := a.ctx.Docs.New(rel, s)
	d.View.Mode = document.View3D
	a.ctx.Selection.Clear()
	return nil
}
