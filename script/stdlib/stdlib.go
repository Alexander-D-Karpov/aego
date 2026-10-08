package stdlib

import (
	"aego/platform/input"
	"aego/script/bind"
)

const (
	ModMath    = "aego/math"
	ModLog     = "aego/log"
	ModRand    = "aego/rand"
	ModTime    = "aego/time"
	ModTesting = "aego/testing"
	ModScene   = "aego/scene"
	ModInput   = "aego/input"
)

type Options struct {
	Seed    uint64
	Testing bool
	Input   *input.AxisConfig
}

func Install(r *bind.Registry, opt Options) {
	installMath(r)
	installLog(r)
	installRand(r, opt.Seed)
	installTime(r)
	installScene(r)
	axes := input.DefaultAxes()
	if opt.Input != nil {
		axes = *opt.Input
	}
	installInput(r, axes)
	if opt.Testing {
		installTesting(r)
	}
}
