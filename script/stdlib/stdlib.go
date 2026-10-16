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
)

// Options configure the script library; Input is read by modules/scripting/stdlib.
type Options struct {
	Seed    uint64
	Testing bool
	Input   *input.AxisConfig
}

// Install registers the language library: math, log, rand, time and, with Testing, testing.
func Install(r *bind.Registry, opt Options) {
	InstallBase(r, opt)
	if opt.Testing {
		InstallTesting(r)
	}
}

func InstallBase(r *bind.Registry, opt Options) {
	installMath(r)
	installLog(r)
	installRand(r, opt.Seed)
	installTime(r)
}

func InstallTesting(r *bind.Registry) {
	installTesting(r)
}
