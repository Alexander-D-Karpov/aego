package module

import (
	"aego/asset"
	"aego/core/log"
	"aego/engine"
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

type Attacher interface {
	Attach(ctx *AttachContext) (func(), error)
}

type InitContext struct {
	Engine *engine.Engine
	Assets asset.Resolver
	Log    *log.Logger
	Editor bool
}

type AttachContext struct {
	World  *engine.World
	Scene  *scene.Scene
	Log    *log.Logger
	Editor bool
}

type Hook struct {
	Owner string
	Fn    any
}
