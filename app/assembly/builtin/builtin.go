package builtin

import (
	"aego/app/assembly"
	"aego/engine/base"
	"aego/engine/module"
	"aego/modules/debug"
	"aego/modules/physics"
	"aego/modules/render2d"
	"aego/modules/render3d"
	"aego/modules/scripting"
	"aego/script/stdlib"
)

func core() assembly.Entry {
	return assembly.Entry{ID: base.CoreID, New: func(assembly.Options) module.Module { return base.Core{} }}
}

func render2dEntry() assembly.Entry {
	return assembly.Entry{ID: render2d.ID, New: func(assembly.Options) module.Module { return render2d.New() }}
}

func render3dEntry() assembly.Entry {
	return assembly.Entry{ID: render3d.ID, New: func(assembly.Options) module.Module { return render3d.New() }}
}

func scriptEntry() assembly.Entry {
	return assembly.Entry{ID: scripting.ID, New: func(o assembly.Options) module.Module {
		return scripting.New(stdlib.Options{Seed: o.Seed, Testing: o.Testing, Input: o.Input})
	}}
}

func physicsEntry() assembly.Entry {
	return assembly.Entry{ID: physics.ID, New: func(assembly.Options) module.Module { return physics.New() }}
}

func debugEntry() assembly.Entry {
	return assembly.Entry{ID: debug.ID, New: func(assembly.Options) module.Module { return debug.New() }}
}

func Default() assembly.Catalog {
	return assembly.Catalog{core(), render2dEntry(), render3dEntry(), scriptEntry(), physicsEntry(), debugEntry()}
}
