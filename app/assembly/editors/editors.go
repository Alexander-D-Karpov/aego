package editors

import (
	"aego/app/assembly"
	"aego/modules/debug"
	debugeditor "aego/modules/debug/editor"
	"aego/modules/physics"
	physicseditor "aego/modules/physics/editor"
	"aego/modules/render2d"
	render2deditor "aego/modules/render2d/editor"
	"aego/modules/render3d"
	render3deditor "aego/modules/render3d/editor"
)

func With(c assembly.Catalog) assembly.Catalog {
	return c.
		With(render2d.ID, render2deditor.Hook).
		With(render3d.ID, render3deditor.Hook).
		With(physics.ID, physicseditor.Hook).
		With(debug.ID, debugeditor.Hook)
}
