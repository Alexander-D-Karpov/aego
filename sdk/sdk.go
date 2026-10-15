package sdk

import (
	"io/fs"

	"aego/asset"
	"aego/asset/loader"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/core/rng"
	"aego/engine"
	"aego/engine/module"
	"aego/render"
	renderhost "aego/render/host"
	"aego/scene"
	"aego/schema"
)

const APIVersion = 2

type (
	Context    = engine.Context
	System     = engine.System
	SystemDesc = engine.SystemDesc
	Phase      = engine.Phase
	ModuleInfo = module.Info

	InitContext   = module.InitContext
	AttachContext = module.AttachContext
	TemplateDesc  = schema.TemplateDesc

	ComponentBuilder[T any] = module.ComponentBuilder[T]

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

	Random = rng.Source

	SystemScope = engine.SystemScope
)

const (
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
)

const (
	ScopePlay   = engine.ScopePlay
	ScopeEditor = engine.ScopeEditor
	ScopeWorld  = engine.ScopeWorld
)

type Registry struct {
	r *module.Registry
}

type Module interface {
	Info() ModuleInfo
	Register(*Registry) error
}

type adapted struct {
	m Module
}

func (a adapted) Info() module.Info {
	return a.m.Info()
}

func (a adapted) Register(r *module.Registry) error {
	return a.m.Register(&Registry{r: r})
}

func (a adapted) Init(ctx *module.InitContext) error {
	if in, ok := a.m.(module.Initer); ok {
		return in.Init(ctx)
	}
	return nil
}

func (a adapted) AttachWorld(ctx *module.AttachContext) error {
	if at, ok := a.m.(module.WorldAttacher); ok {
		return at.AttachWorld(ctx)
	}
	return nil
}

func (a adapted) DetachWorld(ctx *module.AttachContext) {
	if at, ok := a.m.(module.WorldAttacher); ok {
		at.DetachWorld(ctx)
	}
}

func (a adapted) Shutdown() {
	if sd, ok := a.m.(module.Shutdowner); ok {
		sd.Shutdown()
	}
}

func Adapt(m Module) module.Module {
	return adapted{m: m}
}

func AdaptAll(mods []Module) []module.Module {
	out := make([]module.Module, len(mods))
	for i, m := range mods {
		out[i] = Adapt(m)
	}
	return out
}

func (r *Registry) Schema() *schema.Registry {
	return r.r.Schema()
}

func (r *Registry) System(d SystemDesc) {
	r.r.System(d)
}

func (r *Registry) Service(id string, v any) {
	r.r.Service(id, v)
}

func (r *Registry) ScriptModule(path string) *Bindings {
	return r.r.ScriptModule(path)
}

func (r *Registry) Template(t TemplateDesc) {
	r.r.Template(t)
}

func (r *Registry) Icons(fsys fs.FS, prefix string) {
	r.r.Icons(fsys, prefix)
}

func (r *Registry) Docs(fsys fs.FS) {
	r.r.Docs(fsys)
}

func Component[T any](r *Registry, opts ...schema.Option) *ComponentBuilder[T] {
	return module.Component[T](r.r, opts...)
}

func Struct[T any](r *Registry, opts ...schema.Option) error {
	return module.Struct[T](r.r, opts...)
}

func Enum[T module.Integer](r *Registry, name string, values ...schema.EnumValue) error {
	return module.Enum[T](r.r, name, values...)
}

func RegisterComponent[T any](r *Registry, opts ...schema.Option) error {
	return Component[T](r, opts...).Err()
}

func RegisterStruct[T any](r *Registry, opts ...schema.Option) error {
	return Struct[T](r, opts...)
}

func RegisterEnum[T module.Integer](r *Registry, name string, values ...schema.EnumValue) error {
	return Enum[T](r, name, values...)
}

func RegisterSystem(r *Registry, d SystemDesc) error {
	r.System(d)
	return nil
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

func ChoicesFrom(field, section, list string) schema.Option {
	return schema.ChoicesFrom(field, section, list)
}

func Bitset(field, section, list string) schema.Option {
	return schema.Bitset(field, section, list)
}

func Runtime(fields ...string) schema.Option {
	return schema.Runtime(fields...)
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
	r, _ := engine.Service[*render.Renderer](ctx.Engine, renderhost.ServiceID)
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
