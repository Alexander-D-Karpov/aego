package playground

import (
	"aego/app/assembly"
	"aego/engine/base"
	"aego/engine/module"
	"aego/modules/debug"
	inputmod "aego/modules/input"
	"aego/modules/physics"
	"aego/modules/render2d"
	"aego/modules/scripting"
	"aego/script/stdlib"
	"aego/script/vm"
)

func Catalog() assembly.Catalog {
	return assembly.Catalog{
		{ID: base.CoreID, New: func(assembly.Options) module.Module { return base.Core{} }},
		{ID: inputmod.ID, New: func(assembly.Options) module.Module { return inputmod.New() }},
		{ID: render2d.ID, New: func(assembly.Options) module.Module { return render2d.New() }},
		{ID: scripting.ID, New: func(o assembly.Options) module.Module {
			return scripting.New(stdlib.Options{Seed: o.Seed, Testing: o.Testing})
		}},
		{ID: physics.ID, New: func(assembly.Options) module.Module { return physics.New() }},
		{ID: debug.ID, New: func(assembly.Options) module.Module { return debug.New() }},
	}
}

const (
	SandboxBudget = 2_000_000
	SandboxHeap   = 64 << 20
	SandboxNodes  = 10_000
)

func SandboxVM() vm.Options {
	return vm.Options{Budget: SandboxBudget, MaxHeap: SandboxHeap}
}
