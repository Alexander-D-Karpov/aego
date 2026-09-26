package sdk

import (
	"aego/asset"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/core/rng"
	"aego/engine"
	"aego/render"
	rendermodule "aego/render/module"
	"aego/scene"
	"aego/schema"
)

const APIVersion = 1

type (
	Context    = engine.Context
	System     = engine.System
	SystemDesc = engine.SystemDesc
	Phase      = engine.Phase
	ModuleInfo = engine.ModuleInfo

	Scene       = scene.Scene
	NodeHandle  = scene.NodeHandle
	NodeRef     = scene.NodeRef
	Transform2D = scene.Transform2D

	Vec2  = mathx.Vec2
	Color = mathx.Color
	Rect  = mathx.Rect

	AssetID    = ids.AssetID
	NodeID     = ids.NodeID
	TextureRef = asset.Ref[asset.Texture]

	Importer = pipeline.Importer
	Settings = pipeline.Settings
	Random   = rng.Source

	SystemScope = engine.SystemScope
)

const (
	PhaseFrameBegin    = engine.PhaseFrameBegin
	PhaseInput         = engine.PhaseInput
	PhasePrePhysics    = engine.PhasePrePhysics
	PhaseFixedUpdate   = engine.PhasePhysics
	PhasePostPhysics   = engine.PhasePostPhysics
	PhasePreUpdate     = engine.PhasePreUpdate
	PhaseUpdate        = engine.PhaseUpdate
	PhasePostUpdate    = engine.PhasePostUpdate
	PhaseAnimation     = engine.PhaseAnimation
	PhaseRenderPrepare = engine.PhaseRenderPrepare
	PhaseRender        = engine.PhaseRender
	PhaseFrameEnd      = engine.PhaseFrameEnd
)

const (
	ScopePlay   = engine.ScopePlay
	ScopeEditor = engine.ScopeEditor
	ScopeWorld  = engine.ScopeWorld
)

type Registry struct {
	Engine    *engine.Registry
	Types     *scene.Types
	Importers *pipeline.ImporterRegistry
}

func (r *Registry) Schema() *schema.Registry {
	return r.Types.Schema()
}

type Module interface {
	Info() ModuleInfo
	Register(*Registry) error
}

func RegisterComponent[T any](r *Registry, opts ...schema.Option) error {
	return scene.RegisterComponent[T](r.Types, opts...)
}

func RegisterStruct[T any](r *Registry, opts ...schema.Option) error {
	_, err := schema.RegisterStruct[T](r.Types.Schema(), opts...)
	return err
}

func RegisterEnum[T ~int8 | ~int16 | ~int32 | ~int64 | ~int | ~uint8 | ~uint16 | ~uint32](r *Registry, name string, values ...schema.EnumValue) error {
	return schema.RegisterEnum[T](r.Types.Schema(), name, values...)
}

func RegisterSystem(r *Registry, d SystemDesc) error {
	return r.Engine.Systems().Register(d)
}

func RegisterImporter(r *Registry, imp Importer) error {
	return r.Importers.Register(imp)
}

func EnumValue(name string, value int64) schema.EnumValue {
	return schema.EnumValue{Name: name, Value: value}
}

func Name(s string) schema.Option {
	return schema.Name(s)
}

func FormerName(names ...string) schema.Option {
	return schema.FormerName(names...)
}

func Doc(s string) schema.Option {
	return schema.Doc(s)
}

func Version(v uint32) schema.Option {
	return schema.Version(v)
}

func SceneOf(ctx *Context) *Scene {
	if ctx.World != nil {
		if s, ok := engine.ServiceOf[*scene.Scene](ctx.World, scene.ServiceID); ok {
			return s
		}
	}
	s, _ := engine.Service[*scene.Scene](ctx.Engine, scene.ServiceID)
	return s
}

func RendererOf(ctx *Context) *render.Renderer {
	r, _ := engine.Service[*render.Renderer](ctx.Engine, rendermodule.ServiceID)
	return r
}

func AssetsOf(ctx *Context) asset.Resolver {
	l, _ := engine.Service[*loader.Loader](ctx.Engine, loader.ServiceID)
	return l
}

func Add[T any](s *Scene, h NodeHandle) *T {
	return scene.Add[T](s, h)
}

func Get[T any](s *Scene, h NodeHandle) (*T, bool) {
	return scene.Get[T](s, h)
}

func Has[T any](s *Scene, h NodeHandle) bool {
	return scene.Has[T](s, h)
}

func Remove[T any](s *Scene, h NodeHandle) {
	scene.Remove[T](s, h)
}

func Query1[A any](s *Scene) scene.Query1Of[A] {
	return scene.Query1[A](s)
}

func Query2[A, B any](s *Scene) scene.Query2Of[A, B] {
	return scene.Query2[A, B](s)
}
