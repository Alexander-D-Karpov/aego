package app

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/editor/document"
	"aego/editor/templates"
	"aego/scene"
)

const CommandNewScene3D = "file.new_scene_3d"

func (a *App) newScene3D() error {
	var res asset.Resolver
	if a.ctx.Loader != nil {
		res = a.ctx.Loader
	}
	s := scene.New(a.opt.Types, res, scene.Options{DefaultSpace: scene.Space3D})
	templates.BuildCamera3D(s, s.Root())
	templates.BuildShape3D(s, s.Root(), templates.Shapes3D[1])
	cube := templates.BuildShape3D(s, s.Root(), templates.Shapes3D[0])
	s.SetPosition3D(cube, mathx.Vec3{Y: 0.5})
	s.Flush()
	rel := a.ctx.Docs.UniquePath("scenes/untitled3d.aescene")
	d := a.ctx.Docs.New(rel, s)
	d.View.Mode = document.View3D
	a.ctx.Selection.Clear()
	return nil
}
