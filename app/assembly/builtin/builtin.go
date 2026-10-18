package builtin

import (
	"aego/app/assembly"
	"aego/engine/base"
	"aego/engine/module"
	"aego/modules/accessibility"
	"aego/modules/audio"
	"aego/modules/debug"
	"aego/modules/display"
	inputmod "aego/modules/input"
	"aego/modules/physics"
	"aego/modules/render2d"
	"aego/modules/render3d"
	"aego/modules/scripting"
	settingsmod "aego/modules/settings"
	"aego/modules/tween"
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

func tweenEntry() assembly.Entry {
	return assembly.Entry{ID: tween.ID, Requires: []string{scripting.ID}, New: func(assembly.Options) module.Module { return tween.New() }}
}

func Default() assembly.Catalog {
	return assembly.Catalog{core(), inputEntry(), render2dEntry(), render3dEntry(), scriptEntry(), physicsEntry(), debugEntry(),
		displayEntry(), audioEntry(), accessibilityEntry(), settingsEntry(), tweenEntry()}
}
