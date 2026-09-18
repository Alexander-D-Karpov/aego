package demos

import (
	"slices"

	"aego/engine"
	"aego/platform"
)

type Demo interface {
	engine.App
	Name() string
}

type Configurer interface {
	Configure(*engine.Config)
}

var Registry = map[string]func() Demo{}

func Register(name string, ctor func() Demo) {
	if _, dup := Registry[name]; dup {
		panic("demos: duplicate demo name " + name)
	}
	Registry[name] = ctor
}

func Names() []string {
	out := make([]string, 0, len(Registry))
	for n := range Registry {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

type Base struct{}

func (Base) Init(*engine.Engine) error { return nil }
func (Base) Event(platform.Event)      {}
func (Base) FixedUpdate(float64)       {}
func (Base) Update(float64)            {}
func (Base) Render(float64)            {}
func (Base) Shutdown()                 {}
