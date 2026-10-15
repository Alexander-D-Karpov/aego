// Package stdlib holds the engine script modules aego/scene and aego/input.
package stdlib

import (
	"aego/platform/input"
	"aego/script/bind"
	langstd "aego/script/stdlib"
)

const (
	ModScene = "aego/scene"
	ModInput = "aego/input"
)

// Install registers the script library: math, log, rand, time, scene, input, testing.
func Install(r *bind.Registry, opt langstd.Options) {
	langstd.InstallBase(r, opt)
	installScene(r)
	axes := input.DefaultAxes()
	if opt.Input != nil {
		axes = *opt.Input
	}
	installInput(r, axes)
	if opt.Testing {
		langstd.InstallTesting(r)
	}
}
