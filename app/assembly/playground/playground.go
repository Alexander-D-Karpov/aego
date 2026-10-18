package playground

import (
	"aego/app/assembly"
	"aego/engine/base"
	"aego/engine/module"
	inputmod "aego/modules/input"
	"aego/modules/render2d"
	"aego/modules/scripting"
	"aego/script/stdlib"
)

func Catalog() assembly.Catalog {
	return assembly.Catalog{
		{ID: base.CoreID, New: func(assembly.Options) module.Module { return base.Core{} }},
		{ID: inputmod.ID, New: func(assembly.Options) module.Module { return inputmod.New() }},
		{ID: render2d.ID, New: func(assembly.Options) module.Module { return render2d.New() }},
		{ID: scripting.ID, New: func(o assembly.Options) module.Module {
			return scripting.New(stdlib.Options{Seed: o.Seed, Testing: o.Testing})
		}},
	}
}
