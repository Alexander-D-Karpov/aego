package fakephys

import (
	"errors"

	"aego/contracts/debugdraw"
	"aego/contracts/inputactions"
	"aego/contracts/scriptevents"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/engine/settings"
	"aego/scene"
	"aego/schema"
)

const (
	ID          = "fakephys"
	Lists       = "layers"
	stateID     = ID + ".state"
	EventTick   = "OnFakeTick"
	ActionKick  = "FakeKick"
	ContextFake = "Fake"
)

type Settings struct {
	Gain float32 `aego:"gain"`
	Bad  bool    `aego:"bad"`
}

type Body struct {
	Size  float32 `aego:"size"`
	Layer int32   `aego:"layer"`
	Mask  int32   `aego:"mask"`
	Speed float32 `aego:"speed"`
}

type State struct {
	Ticks int
}

type Module struct {
	Gain        float32
	Changed     int
	Removed     int
	LastRemoved Body
	Attached    int
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: "1"}
}

func (m *Module) Register(r *module.Registry) error {
	if err := module.Struct[Settings](r, schema.Name("fakephys.Settings")); err != nil {
		return err
	}
	r.ProjectSection(module.SectionDesc[Settings]{
		ID:      ID,
		Title:   "Fake",
		Order:   900,
		Default: func() Settings { return Settings{Gain: 1} },
		Validate: func(s *Settings) error {
			if s.Bad {
				return errors.New("fakephys: bad settings")
			}
			return nil
		},
		Names: func(*Settings, string) []string { return []string{"A", "B"} },
	})
	r.System(engine.SystemDesc{ID: ID + ".tick", Phase: engine.PhasePhysics, Scope: engine.ScopePlay, System: tick{}})
	r.Template(schema.TemplateDesc{ID: ID + ".body", Name: "Fake Body", Menu: "Fake", Space: schema.Space2D, Setup: func(s *scene.Scene, h scene.NodeHandle) {
		*scene.Add[Body](s, h) = Body{Size: 1}
	}})
	r.Provide(inputactions.Kind, inputactions.Set{
		Actions:  []inputactions.Action{{Name: ActionKick, Type: "button", Bindings: []string{"Key.K", "Pad.Y"}}},
		Contexts: []inputactions.Context{{Name: ContextFake, Actions: []string{ActionKick}, Default: true}},
	})
	r.Setting(settings.Desc{
		ID: ID + ".layer", Kind: schema.KindString, Default: schema.String("A"), Title: "Fake Layer",
		Category: settings.CategoryGameplay, Flags: settings.User, ChoicesFrom: schema.HintSource{Section: ID, List: Lists},
	})
	r.ScriptEvent(scriptevents.Event{Name: EventTick})
	return module.Component[Body](r, schema.Name("fakephys.Body"),
		schema.ChoicesFrom("layer", ID, Lists),
		schema.Bitset("mask", ID, Lists),
		schema.Runtime("speed")).
		Category("Fake").
		Default(func() Body { return Body{Size: 1} }).
		OnAdd(func(ctx module.AddContext, b *Body) {
			if rc, ok := ctx.VisualRect(); ok {
				b.Size = rc.W
			}
		}).
		OnRemoving(func(_ module.AddContext, b *Body) {
			m.Removed++
			m.LastRemoved = *b
		}).
		OnChanged(func(*module.ChangeSet) {
			m.Changed++
		}).
		Validate(func(_ module.ValidateContext, b *Body) []problems.Diagnostic {
			if b.Size > 0 {
				return nil
			}
			return []problems.Diagnostic{{Code: "FAK001", Severity: problems.Error, Message: "fakephys.Body: size must be positive", BlocksPlay: true}}
		}).
		Err()
}

func (m *Module) Init(ctx *module.InitContext) error {
	if s, ok := module.SectionValue[Settings](ctx.Sections, ID); ok {
		m.Gain = s.Gain
	}
	return nil
}

func (m *Module) AttachWorld(ctx *module.AttachContext) error {
	if !ctx.Play || ctx.World == nil {
		return nil
	}
	m.Attached++
	ctx.World.SetService(stateID, &State{})
	return nil
}

func (m *Module) DetachWorld(ctx *module.AttachContext) {
	if !ctx.Play || ctx.World == nil {
		return
	}
	m.Attached--
	ctx.World.RemoveService(stateID)
}

func StateOf(w *engine.World) *State {
	s, _ := engine.ServiceOf[*State](w, stateID)
	return s
}

type tick struct{}

var tickStyle = debugdraw.Style{Color: mathx.Color{R: 1, A: 1}, Channel: ID + ".ticks"}

func (tick) Run(ctx *engine.Context) {
	s := StateOf(ctx.World)
	if s == nil {
		return
	}
	s.Ticks++
	if d, ok := engine.ServiceOf[debugdraw.Draw](ctx.World, debugdraw.ServiceID); ok {
		d.Cross(mathx.Vec2{}, 0.5, &tickStyle)
	}
}
