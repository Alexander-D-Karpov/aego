package settings

import (
	"aego/engine/i18n"
	"aego/platform"
	"aego/schema"
	"slices"

	"aego/contracts/inputactions"
	"aego/contracts/scriptevents"
	"aego/engine"
	"aego/engine/module"
	es "aego/engine/settings"
	"aego/scene/frame"
	"aego/script/abi"
	"aego/script/vm"
)

const (
	ID           = "settings"
	ScriptModule = "aego/settings"
	ServiceView  = ID + ".view"
	SystemTick   = ID + ".tick"
	SystemDraw   = ID + ".draw"
	ActionToggle = "ToggleSettings"
	ContextView  = "Settings"
	saveDelay    = 0.5
	EventChanged = "OnSettingChanged"
	KeyLanguage  = ID + ".language"
)

// languages are the choices of the game's language setting; English and Russian name themselves.
var languages = []es.Choice{
	{Name: i18n.Auto, Label: "Automatic", Value: schema.Enum(0)},
	{Name: i18n.English, Label: "English", Value: schema.Enum(1)},
	{Name: i18n.Russian, Label: "Русский", Value: schema.Enum(2)},
}

type Module struct {
	st      *es.Store
	draft   *es.Tx
	now     float64
	touched float64
	unsub   func()
	ev      scriptevents.EventID
	worlds  []*engine.World
	arg     [1]vm.Value
	editor  bool
	locales []string
}

func New() *Module {
	return &Module{}
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Requires: []string{"input"}, Builtin: true, DisplayName: "Settings", DocsOrder: 110, DocsIcon: "cat.settings"}
}

func (m *Module) Register(r *module.Registry) error {
	registerDocs(r)
	r.Provide(inputactions.Kind, inputactions.Set{
		Actions:  []inputactions.Action{{Name: ActionToggle, Type: "button", Bindings: []string{"Key.F10"}}},
		Contexts: []inputactions.Context{{Name: ContextView, Priority: 1000, Consume: true, Actions: []string{ActionToggle}, Default: true}},
	})
	r.Setting(es.Desc{
		ID: KeyLanguage, Kind: schema.KindEnum, Default: schema.Enum(0), Title: "Language",
		Doc:      "Language of the settings and of the game's translated texts. Automatic follows the system.",
		Category: es.CategoryLanguage, Order: 10, Flags: es.User | es.Project, Choices: languages, Apply: m.applyLanguage,
	})
	m.ev = r.ScriptEvent(scriptevents.Event{
		Name:   EventChanged,
		Params: []abi.TypeRef{abi.T(abi.String)},
		Names:  []string{"id"},
		Doc:    "OnSettingChanged runs on every script with this method after a setting's effective value changed: Apply, Revert, an override or a confirmation timeout.",
	})
	r.System(engine.SystemDesc{ID: SystemTick, Phase: engine.PhaseFrameBegin, Scope: engine.ScopeFrame, System: tickSystem{m: m}})
	r.System(engine.SystemDesc{
		ID: SystemDraw, Phase: engine.PhaseRenderUI, Scope: engine.ScopePlay,
		After: []string{frame.SystemPresent}, Before: []string{frame.SystemOverlays}, System: drawSystem{},
	})
	if r.Bindings() != nil {
		installScript(r.ScriptModule(ScriptModule), m)
	}
	return nil
}

func (m *Module) Init(ctx *module.InitContext) error {
	m.st = ctx.Settings
	m.editor = ctx.Editor
	if ctx.Engine != nil {
		if l, ok := ctx.Engine.Window().(platform.Localer); ok {
			m.locales = l.PreferredLocales()
		}
	}
	if m.st != nil {
		m.unsub = m.st.Subscribe(0, m.changed)
		if k := m.st.Key(KeyLanguage); k != 0 {
			_ = m.applyLanguage(m.st.Value(k))
		}
	}
	return nil
}

// applyLanguage switches the game's texts; inside the editor the editor's own language stays.
func (m *Module) applyLanguage(v schema.Value) error {
	if m.editor {
		return nil
	}
	pref := i18n.Auto
	if i := int(v.I); i >= 0 && i < len(languages) {
		pref = languages[i].Name
	}
	b := i18n.Default()
	b.SetLanguage(i18n.Resolve(pref, m.locales, b.Languages()))
	return nil
}

func (m *Module) Shutdown() {
	if m.st != nil && m.st.Dirty() {
		_ = m.st.Save()
	}
	if m.unsub != nil {
		m.unsub()
		m.unsub = nil
	}
}

func (m *Module) AttachWorld(ctx *module.AttachContext) error {
	if !ctx.Play || ctx.World == nil || m.st == nil {
		return nil
	}
	ctx.World.SetView(ServiceView, NewView(m.st, ctx.World))
	m.worlds = append(m.worlds, ctx.World)
	return nil
}

func (m *Module) DetachWorld(ctx *module.AttachContext) {
	if !ctx.Play || ctx.World == nil {
		return
	}
	if v := Of(ctx.World); v != nil {
		v.Close()
	}
	ctx.World.RemoveService(ServiceView)
	m.worlds = slices.DeleteFunc(m.worlds, func(w *engine.World) bool { return w == ctx.World })
}

func (m *Module) tx() *es.Tx {
	if m.draft == nil || m.draft.Done() {
		m.draft = m.st.Begin()
	}
	return m.draft
}

func Of(w *engine.World) *View {
	v, _ := engine.ServiceOf[*View](w, ServiceView)
	return v
}

type tickSystem struct {
	m *Module
}

func (t tickSystem) Run(ctx *engine.Context) {
	m := t.m
	if m.st == nil {
		return
	}
	m.now = ctx.Engine.Platform().Now()
	m.st.Tick(m.now)
	if _, waiting := m.st.Pending(); !waiting && m.st.Dirty() && m.now-m.touched >= saveDelay {
		_ = m.st.Save()
	}
}

type drawSystem struct{}

func (drawSystem) Run(ctx *engine.Context) {
	v := Of(ctx.World)
	if v == nil {
		return
	}
	if r, ok := engine.ServiceOf[inputactions.Reader](ctx.World, inputactions.ReaderID); ok && r.Pressed(ActionToggle) {
		v.Toggle()
	}
	if !v.open {
		return
	}
	if fr := frame.Of(ctx); fr != nil && fr.Renderer() != nil {
		v.draw(ctx, fr)
	}
}

func (m *Module) changed(k es.Key) {
	m.touched = m.now
	if m.ev == scriptevents.NoEvent || len(m.worlds) == 0 {
		return
	}
	d := m.st.Desc(k)
	if d == nil {
		return
	}
	for _, w := range m.worlds {
		sink, ok := engine.ServiceOf[scriptevents.Sink](w, scriptevents.ServiceID)
		if !ok {
			continue
		}
		b, ok1 := sink.(scriptevents.Broadcaster)
		in, ok2 := sink.(scriptevents.Strings)
		if !ok1 || !ok2 {
			continue
		}
		m.arg[0] = in.Intern(d.ID)
		b.Broadcast(m.ev, m.arg[:])
	}
}
