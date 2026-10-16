package shell

const CommandNewScene3D = "file.new_scene_3d"

func (a *App) newScene3D() error {
	return a.newUnsaved("scenes/untitled3d.aescene", true)
}
