package builtin

import (
	"aego/app/assembly"
	"aego/engine/base"
	"aego/engine/module"
	"aego/modules/accessibility"
	"aego/modules/animation"
	artmod "aego/modules/art"
	"aego/modules/audio"
	"aego/modules/camera"
	"aego/modules/dataasset"
	"aego/modules/debug"
	"aego/modules/display"
	"aego/modules/gameplay"
	inputmod "aego/modules/input"
	"aego/modules/particles"
	"aego/modules/physics"
	"aego/modules/render2d"
	"aego/modules/render3d"
	"aego/modules/savegame"
	"aego/modules/scripting"
	settingsmod "aego/modules/settings"
	"aego/modules/spawn"
	"aego/modules/tween"
	updatemod "aego/modules/update"
	"aego/script/stdlib"
)

func core() assembly.Entry {
	return assembly.Entry{ID: base.CoreID, New: func(assembly.Options) module.Module { return base.Core{} }}
}

func inputEntry() assembly.Entry {
	return assembly.Entry{ID: inputmod.ID, New: func(assembly.Options) module.Module { return inputmod.New() }}
}

func render2dEntry() assembly.Entry {
	return assembly.Entry{ID: render2d.ID, New: func(assembly.Options) module.Module { return render2d.New() }}
}

func particlesEntry() assembly.Entry {
	return assembly.Entry{ID: particles.ID, Requires: []string{render2d.ID}, New: func(assembly.Options) module.Module { return particles.New() }}
}

func render3dEntry() assembly.Entry {
	return assembly.Entry{ID: render3d.ID, New: func(assembly.Options) module.Module { return render3d.New() }}
}

func scriptEntry() assembly.Entry {
	return assembly.Entry{ID: scripting.ID, New: func(o assembly.Options) module.Module {
		return scripting.New(stdlib.Options{Seed: o.Seed, Testing: o.Testing})
	}}
}

func physicsEntry() assembly.Entry {
	return assembly.Entry{ID: physics.ID, New: func(assembly.Options) module.Module { return physics.New() }}
}

func debugEntry() assembly.Entry {
	return assembly.Entry{ID: debug.ID, New: func(assembly.Options) module.Module { return debug.New() }}
}

func displayEntry() assembly.Entry {
	return assembly.Entry{ID: display.ID, New: func(assembly.Options) module.Module { return display.New() }}
}

func audioEntry() assembly.Entry {
	return assembly.Entry{ID: audio.ID, New: func(assembly.Options) module.Module { return audio.New() }}
}

func accessibilityEntry() assembly.Entry {
	return assembly.Entry{ID: accessibility.ID, New: func(assembly.Options) module.Module { return accessibility.New() }}
}

func settingsEntry() assembly.Entry {
	return assembly.Entry{ID: settingsmod.ID, Requires: []string{inputmod.ID}, New: func(assembly.Options) module.Module { return settingsmod.New() }}
}

func animationEntry() assembly.Entry {
	return assembly.Entry{ID: animation.ID, Requires: []string{scripting.ID}, New: func(assembly.Options) module.Module { return animation.New() }}
}

func tweenEntry() assembly.Entry {
	return assembly.Entry{ID: tween.ID, Requires: []string{scripting.ID}, New: func(assembly.Options) module.Module { return tween.New() }}
}

func cameraEntry() assembly.Entry {
	return assembly.Entry{ID: camera.ID, Requires: []string{scripting.ID}, New: func(assembly.Options) module.Module { return camera.New() }}
}

func gameplayEntry() assembly.Entry {
	return assembly.Entry{ID: gameplay.ID, Requires: []string{physics.ID, scripting.ID}, New: func(assembly.Options) module.Module { return gameplay.New() }}
}

func dataEntry() assembly.Entry {
	return assembly.Entry{ID: dataasset.ID, New: func(assembly.Options) module.Module { return dataasset.New() }}
}

func spawnEntry() assembly.Entry {
	return assembly.Entry{ID: spawn.ID, New: func(assembly.Options) module.Module { return spawn.New() }}
}

func savegameEntry() assembly.Entry {
	return assembly.Entry{ID: savegame.ID, Requires: []string{scripting.ID, spawn.ID}, New: func(assembly.Options) module.Module { return savegame.New() }}
}

func artEntry() assembly.Entry {
	return assembly.Entry{ID: artmod.ID, New: func(assembly.Options) module.Module { return artmod.New() }}
}

func updateEntry() assembly.Entry {
	return assembly.Entry{ID: updatemod.ID, New: func(assembly.Options) module.Module { return updatemod.New() }}
}

func Default() assembly.Catalog {
	return assembly.Catalog{core(), dataEntry(), inputEntry(), render2dEntry(), render3dEntry(), scriptEntry(), physicsEntry(), debugEntry(),
		displayEntry(), audioEntry(), accessibilityEntry(), settingsEntry(), tweenEntry(), animationEntry(), cameraEntry(), gameplayEntry(), spawnEntry(), savegameEntry(), updateEntry(), artEntry(), particlesEntry()}
}
