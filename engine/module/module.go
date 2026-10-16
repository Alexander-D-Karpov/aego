package module

import (
	"aego/asset"
	"aego/core/log"
	"aego/engine"
	"aego/engine/problems"
	"aego/scene"
)

type Info struct {
	ID       string
	Version  string
	Requires []string
	Builtin  bool
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
