package stdlib

import (
	"aego/script/bind"
	langstd "aego/script/stdlib"
)

const ModScene = "aego/scene"

func Install(r *bind.Registry, opt langstd.Options) {
	langstd.InstallBase(r, opt)
	installScene(r)
	if opt.Testing {
		langstd.InstallTesting(r)
	}
}
