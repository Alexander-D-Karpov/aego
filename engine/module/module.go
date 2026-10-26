package module

import (
	"aego/asset"
	"aego/core/log"
	"aego/engine"
	"aego/engine/problems"
	"aego/engine/settings"
	"aego/scene"
)

type Info struct {
	ID               string
	Version          string
	Requires         []string
	Builtin          bool
	NonDeterministic string
	DisplayName      string
	DocsOrder        int
	DocsIcon         string
	DocsGroup        string
	Codes            []string
}

type Module interface {
	Info() Info
	Register(r *Registry) error
}

type Initer interface {
	Init(ctx *InitContext) error
}

type WorldAttacher interface {
	AttachWorld(ctx *AttachContext) error
	DetachWorld(ctx *AttachContext)
}

type Reconfigurer interface {
	Reconfigure(ctx *AttachContext) error
}

type Shutdowner interface {
	Shutdown()
}

type InitContext struct {
	Engine   *engine.Engine
	Assets   asset.Resolver
	Log      *log.Logger
	Editor   bool
	Sections *Sections
	Problems *problems.Service
	Settings *settings.Store
	// GameName and GameVersion are the project's module and version, for a module that talks to the game's publisher.
	GameName    string
	GameVersion string
	provided    []provision
	owner       string
}

func (c *InitContext) Setting(d settings.Desc) (settings.Key, error) {
	if c.Settings == nil {
		return 0, nil
	}
	d.Owner = c.owner
	return c.Settings.Register(d)
}

func (c *InitContext) Provided(kind string) []Provision {
	var out []Provision
	for _, p := range c.provided {
		if p.kind == kind {
			out = append(out, p.Provision)
		}
	}
	return out
}

func (c *InitContext) Section(id string) any {
	v, _ := c.Sections.Get(id)
	return v
}

type AttachContext struct {
	World  *engine.World
	Scene  *scene.Scene
	Log    *log.Logger
	Editor bool
	Play   bool
}

type Hook struct {
	Owner string
	Fn    any
}
