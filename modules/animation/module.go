package animation

import (
	"errors"

	"aego/asset"
	"aego/contracts/scriptevents"
	"aego/core/ids"
	"aego/core/log"
	"aego/engine"
	"aego/engine/module"
	"aego/script/abi"
	"aego/script/bind"
)

const (
	ID           = "animation"
	ServiceID    = ID + ".service"
	SystemFixed  = ID + ".fixed"
	SystemSample = ID + ".sample"
	ScriptModule = "aego/anim"
	EventName    = "OnAnimEvent"
	needsScripts = "script"
	scriptFixed  = "script.fixed"

	channel log.Channel = "animation"
)

const (
	CodeTrack    = "ANM001"
	CodeRange    = "ANM002"
	CodeEvent    = "ANM003"
	CodeConflict = "ANM004"
	CodeLost     = "ANM005"
	CodeBody     = "ANM006"
	CodeUnscaled = "ANM007"
	CodeReload   = "ANM008"
	CodeRoot     = "ANM009"
)

const (
	CodeCtrlParam = "ANC001"
	CodeCtrlState = "ANC002"
	CodeCtrlSelf  = "ANC003"
	CodeCtrlLoop  = "ANC005"
	CodeCtrlText  = "ANC006"
)

type signals struct {
	finished module.Signal1[int64]
	event    module.Signal2[int64, float32]
}

type Module struct {
	sig    signals
	asig   asignals
	ctrls  map[ids.AssetID]ctrlHit
	ev     scriptevents.EventID
	events []EventDesc
	names  map[string]int32
}

const (
	EventFootstep = "Footstep"
	EventHit      = "Hit"
)

var builtinEvents = []EventDesc{{Name: EventFootstep}, {Name: EventHit, Payload: PayloadFloat}}

func New() *Module {
	m := &Module{ev: scriptevents.NoEvent, names: map[string]int32{}, ctrls: map[ids.AssetID]ctrlHit{}}
	m.SetEvents(nil)
	return m
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Requires: []string{needsScripts}, Builtin: true,
		DisplayName: "Animation", DocsOrder: 55, DocsIcon: "play.start",
		Codes: []string{CodeTrack, CodeRange, CodeEvent, CodeConflict, CodeLost, CodeBody, CodeUnscaled, CodeReload, CodeRoot, CodeCtrlParam, CodeCtrlState, CodeCtrlSelf, CodeCtrlClip, CodeCtrlLoop, CodeCtrlText}}
}

func (*Module) IconDir() string { return "modules/animation/res/icons/src" }

func (m *Module) Register(r *module.Registry) error {
	registerDocs(r)
	r.ProjectSection(eventSection())
	if b := r.Bindings(); b != nil {
		if err := errors.Join(
			bind.AssetRefType[ClipRef](b, string(asset.ArtAnim)),
			bind.AssetRefType[ControllerRef](b, string(asset.ArtCtrl)),
		); err != nil {
			return err
		}
	}
	if err := errors.Join(registerTypes(r, m), registerAnimator(r, m)); err != nil {
		return err
	}
	m.ev = r.ScriptEvent(scriptevents.Event{Name: EventName,
		Params: []abi.TypeRef{abi.T(abi.Int), abi.T(abi.Float)}, Names: []string{"id", "payload"},
		Doc: "OnAnimEvent runs on the scripts of a node whose Animation Player reaches an event of its clip; anim.EventName(id) gives the name."})
	r.Validator(string(asset.ArtAnim), m.checkClip)
	r.Validator(string(asset.ArtCtrl), m.checkCtrl)
	r.System(engine.SystemDesc{ID: SystemFixed, Phase: engine.PhasePrePhysics, Scope: engine.ScopePlay, After: []string{scriptFixed, engine.AnchorIntents}, Before: []string{engine.AnchorMotion}, System: fixedSystem{}})
	r.System(engine.SystemDesc{ID: SystemSample, Phase: engine.PhaseAnimation, Scope: engine.ScopePlay, Pause: engine.PauseAlways, System: sampleSystem{}})
	if r.Bindings() != nil {
		installScript(r.ScriptModule(ScriptModule), m)
	}
	registerDev(r)
	return nil
}

func (m *Module) Init(ctx *module.InitContext) error {
	var list []EventDesc
	if s, ok := module.SectionValue[Settings](ctx.Sections, ID); ok {
		list = s.Events
	}
	m.SetEvents(list)
	return nil
}

func (m *Module) SetEvents(list []EventDesc) {
	m.events = append(m.events[:0], builtinEvents...)
	clear(m.names)
	for i, e := range m.events {
		m.names[e.Name] = int32(i)
	}
	for _, e := range list {
		if _, dup := m.names[e.Name]; dup {
			continue
		}
		m.names[e.Name] = int32(len(m.events))
		m.events = append(m.events, e)
	}
}

func (m *Module) EventID(name string) (int32, bool) {
	id, ok := m.names[name]
	return id, ok
}

func (m *Module) EventName(id int32) string {
	if id < 0 || int(id) >= len(m.events) {
		return ""
	}
	return m.events[id].Name
}

func (m *Module) AttachWorld(ctx *module.AttachContext) error {
	if !ctx.Play || ctx.World == nil || ctx.Scene == nil {
		return nil
	}
	s := NewService(ctx.Scene, ctx.World, m)
	if ctx.Log != nil {
		s.log = ctx.Log.With(channel)
	}
	ctx.World.SetService(ServiceID, s)
	return nil
}

func (m *Module) DetachWorld(ctx *module.AttachContext) {
	if !ctx.Play || ctx.World == nil {
		return
	}
	if s := Of(ctx.World); s != nil {
		s.Close()
	}
	ctx.World.RemoveService(ServiceID)
}

func Of(w *engine.World) *Service {
	if w == nil {
		return nil
	}
	s, _ := engine.ServiceOf[*Service](w, ServiceID)
	return s
}

type fixedSystem struct{}

func (fixedSystem) Run(ctx *engine.Context) {
	if s := Of(ctx.World); s != nil {
		s.Fixed(float32(ctx.Dt))
	}
}

type sampleSystem struct{}

func (sampleSystem) Run(ctx *engine.Context) {
	s := Of(ctx.World)
	if s == nil {
		return
	}
	var fixed float32
	if t := ctx.Time; t != nil {
		fixed = float32(t.FixedDelta())
	}
	s.Present(float32(ctx.Alpha), fixed, float32(ctx.Unscaled))
}

func installScript(m *bind.Module, mod *Module) {
	bind.Func1(m, "EventName", func(_ *bind.Ctx, id int64) string {
		return mod.EventName(int32(id))
	}, bind.Doc("EventName returns the name of an animation event, or an empty string for an unknown id."), bind.Params("id"),
		bind.Example("func OnAnimEvent(id int, payload float) {\n\tif anim.EventName(id) == \"Hit\" {"))
	bind.Func1(m, "EventID", func(_ *bind.Ctx, name string) int64 {
		if id, ok := mod.EventID(name); ok {
			return int64(id)
		}
		return -1
	}, bind.Doc("EventID returns the id of an event declared in Project Settings → Animation, or -1."), bind.Params("name"),
		bind.Example("hit := anim.EventID(\"Hit\")"))
	bind.Func2(m, "StateName", func(c *bind.Ctx, ref ControllerRef, i int64) string {
		if c.Scene == nil {
			return ""
		}
		e := mod.ctrl(c.Scene.Resolver(), ref.ID)
		if e == nil || i < 0 || int(i) >= len(e.g.States) {
			return ""
		}
		return e.g.States[i].Name
	}, bind.Doc("StateName returns the name of a state of a controller, or an empty string."), bind.Params("controller", "index"))
	bind.Func2(m, "ParamIndex", func(c *bind.Ctx, ref ControllerRef, name string) int64 {
		if c.Scene == nil {
			return -1
		}
		if e := mod.ctrl(c.Scene.Resolver(), ref.ID); e != nil {
			if i, ok := e.params[name]; ok {
				return int64(i)
			}
		}
		return -1
	}, bind.Doc("ParamIndex returns the index of a parameter of a controller, or -1."), bind.Params("controller", "name"))
}
