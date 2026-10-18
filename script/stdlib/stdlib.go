package stdlib

import "aego/script/bind"

const (
	ModMath    = "aego/math"
	ModLog     = "aego/log"
	ModRand    = "aego/rand"
	ModTime    = "aego/time"
	ModTesting = "aego/testing"
)

// Options configure the script library.
type Options struct {
	Seed    uint64
	Testing bool
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
