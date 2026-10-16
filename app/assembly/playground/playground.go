package playground

import (
	"aego/app/assembly"
	"aego/engine/base"
	"aego/engine/module"
	"aego/modules/render2d"
	"aego/modules/scripting"
	"aego/script/stdlib"
)

func Catalog() assembly.Catalog {
	return assembly.Catalog{
		{ID: base.CoreID, New: func(assembly.Options) module.Module { return base.Core{} }},
		{ID: render2d.ID, New: func(assembly.Options) module.Module { return render2d.New() }},
		{ID: scripting.ID, New: func(o assembly.Options) module.Module {
			return scripting.New(stdlib.Options{Seed: o.Seed, Testing: o.Testing, Input: o.Input})
		}},
	}
}
