package native

import (
	"errors"

	"aego/sdk"
)

type Settings struct {
	Strength  float32  `aego:"strength,min=0"`
	Direction sdk.Vec2 `aego:"direction"`
}

type Wind struct {
	Scale float32 `aego:"scale"`
}

type Module struct {
	ctx *sdk.InitContext
}

func (*Module) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "weather", Version: "0.1.0"}
}

func (m *Module) Register(r *sdk.Registry) error {
	if err := sdk.RegisterStruct[Settings](r, sdk.Name("weather.Settings")); err != nil {
		return err
	}
	sdk.ProjectSection(r, sdk.SectionDesc[Settings]{
		ID:      "weather",
		Title:   "Weather",
		Order:   400,
		Default: func() Settings { return Settings{Strength: 1, Direction: sdk.Vec2{X: 1}} },
		Validate: func(s *Settings) error {
			if s.Direction == (sdk.Vec2{}) {
				return errors.New("weather: direction must not be zero")
			}
			return nil
		},
	})
	err := sdk.Component[Wind](r, sdk.Name("weather.Wind")).
		Default(func() Wind { return Wind{Scale: 1} }).
		Validate(func(_ sdk.ValidateContext, w *Wind) []sdk.Diagnostic {
			if w.Scale != 0 {
				return nil
			}
			return []sdk.Diagnostic{{Code: "WTH001", Severity: sdk.SeverityWarning, Message: "weather.Wind: scale 0 makes the wind do nothing"}}
		}).
		Err()
	if err != nil {
		return err
	}
	r.System(sdk.SystemDesc{ID: "weather.blow", Phase: sdk.PhaseUpdate, System: blow{m: m}})
	registerEditor(r, m)
	return nil
}

func (m *Module) Init(ctx *sdk.InitContext) error {
	m.ctx = ctx
	return nil
}

// settings re-reads the section: Project Settings replaces it on commit.
func (m *Module) settings() *Settings {
	if m.ctx == nil {
		return nil
	}
	s, _ := sdk.Section[Settings](m.ctx, "weather")
	return s
}

type blow struct {
	m *Module
}

func (b blow) Run(ctx *sdk.Context) {
	s := sdk.SceneOf(ctx)
	st := b.m.settings()
	if s == nil || st == nil {
		return
	}
	l := st.Direction.Len()
	if l == 0 {
		return
	}
	d := st.Direction.Scale(st.Strength * float32(ctx.Dt) / l)
	sdk.Query1[Wind](s).Each(func(n sdk.NodeHandle, w *Wind) {
		s.SetPosition(n, s.Local(n).Position.Add(d.Scale(w.Scale)))
	})
}

func Modules() []sdk.Module {
	return []sdk.Module{&Module{}}
}
