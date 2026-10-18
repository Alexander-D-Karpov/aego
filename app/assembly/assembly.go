package assembly

import (
	"errors"
	"slices"

	"aego/core/errs"
	"aego/core/log"
	"aego/engine/base"
	"aego/engine/module"
	"aego/scene"
	"aego/script/bind"
)

type Options struct {
	Seed    uint64
	Testing bool
}

type Entry struct {
	ID       string
	Requires []string
	New      func(Options) module.Module
	Editor   any
}

type Catalog []Entry

func NewTypes() (*scene.Types, error) {
	return base.NewTypes()
}

func (c Catalog) IDs() []string {
	out := make([]string, len(c))
	for i, e := range c {
		out[i] = e.ID
	}
	return out
}

func (c Catalog) With(id string, editor any) Catalog {
	out := slices.Clone(c)
	for i := range out {
		if out[i].ID == id {
			out[i].Editor = editor
		}
	}
	return out
}

func (c Catalog) Without(ids ...string) Catalog {
	out := make(Catalog, 0, len(c))
	for _, e := range c {
		if !slices.Contains(ids, e.ID) {
			out = append(out, e)
		}
	}
	return out
}

func (c Catalog) Validate() error {
	var all []error
	seen := map[string]bool{}
	for _, e := range c {
		switch {
		case e.ID == "":
			all = append(all, errs.New(errs.EngineModule, "catalog entry needs an id"))
		case e.New == nil:
			all = append(all, errs.New(errs.EngineModule, "catalog entry "+e.ID+" has no constructor", log.F("module", e.ID)))
		case seen[e.ID]:
			all = append(all, errs.New(errs.EngineModule, "module "+e.ID+" is listed twice in the catalog", log.F("module", e.ID)))
		}
		seen[e.ID] = true
	}
	return errors.Join(all...)
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

func (c Catalog) Modules(opt Options) ([]module.Module, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	out := make([]module.Module, 0, len(c))
	var all []error
	for _, e := range c {
		m := e.New(opt)
		info := m.Info()
		if info.ID != e.ID {
			all = append(all, errs.New(errs.EngineModule, "catalog entry "+e.ID+" builds module "+info.ID, log.F("module", e.ID)))
			continue
		}
		if !sameSet(info.Requires, e.Requires) {
			all = append(all, errs.New(errs.EngineModule, "catalog entry "+e.ID+" lists requirements that differ from the module", log.F("module", e.ID)))
		}
		if e.Editor != nil {
			m = hooked{inner: m, hook: e.Editor}
		}
		out = append(out, m)
	}
	return out, errors.Join(all...)
}

func (c Catalog) Load(types *scene.Types, script *bind.Registry, opt Options, user ...module.Module) (*module.Manifest, error) {
	mods, err := c.Modules(opt)
	if err != nil {
		return nil, err
	}
	return module.Load(types, script, append(mods, user...)...)
}
