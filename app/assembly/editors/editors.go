package editors

import (
	"aego/app/assembly"
	"aego/modules/audio"
	audioeditor "aego/modules/audio/editor"
	"aego/modules/camera"
	cameraeditor "aego/modules/camera/editor"
	"aego/modules/debug"
	debugeditor "aego/modules/debug/editor"
	"aego/modules/gameplay"
	gameplayeditor "aego/modules/gameplay/editor"
	inputmod "aego/modules/input"
	inputeditor "aego/modules/input/editor"
	"aego/modules/physics"
	physicseditor "aego/modules/physics/editor"
	"aego/modules/render2d"
	render2deditor "aego/modules/render2d/editor"
	"aego/modules/render3d"
	render3deditor "aego/modules/render3d/editor"
)

func With(c assembly.Catalog) assembly.Catalog {
	return c.
		With(inputmod.ID, inputeditor.Hook).
		With(render2d.ID, render2deditor.Hook).
		With(render3d.ID, render3deditor.Hook).
		With(physics.ID, physicseditor.Hook).
		With(camera.ID, cameraeditor.Hook).
		With(gameplay.ID, gameplayeditor.Hook).
		With(audio.ID, audioeditor.Hook).
		With(debug.ID, debugeditor.Hook)
}
