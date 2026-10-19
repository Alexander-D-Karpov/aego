package savegame

import (
	"strings"

	"aego/contracts/persistence"
	"aego/contracts/scriptevents"
	"aego/core/log"
	"aego/engine"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/save"
	"aego/schema"
)

const (
	ID             = "save"
	SystemStep     = ID + ".step"
	ScriptModule   = "aego/save"
	EventSave      = "OnSave"
	EventLoad      = "OnLoad"
	MarkerName     = "save.Persist"
	SlotCheckpoint = "checkpoint"
	SlotAutosave   = "autosave"
)

type Persist struct {
	NoTransform bool `aego:"noTransform"`
	NoEnabled   bool `aego:"noEnabled"`
}

type Events struct {
	Save scriptevents.EventID
	Load scriptevents.EventID
}

type Result struct {
	Slot string
	Load bool
	Err  error
}

type Module struct {
	ev       Events
	sections []module.OwnedSection
	game     string
	eng      *engine.Engine
	log      *log.ChannelLogger
	global   aetext.Node
	pending  *save.Doc
	unknown  []save.Section
	playtime float64
	autosave float64
	last     Result
	rev      uint64
}

func New() *Module {
	return &Module{ev: Events{Save: scriptevents.NoEvent, Load: scriptevents.NoEvent}}
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Requires: []string{"script", "spawn"}, Builtin: true}
}

func (m *Module) Register(r *module.Registry) error {
	registerDocs(r)
	if err := module.Component[Persist](r, schema.Name(MarkerName),
		schema.Doc("Persist saves the node's transform and enabled flag in save games.")).
		Category("Engine").Display("Persist").Icon("comp.persist").Err(); err != nil {
		return err
	}
	m.ev.Save = r.ScriptEvent(scriptevents.Event{Name: EventSave,
		Doc: "OnSave runs on every script that has it while a game is saved; write values with save.Put*."})
	m.ev.Load = r.ScriptEvent(scriptevents.Event{Name: EventLoad,
		Doc: "OnLoad runs after Ready on every script that has it when a saved game is loaded; read values with save.Get*."})
	r.System(engine.SystemDesc{ID: SystemStep, Phase: engine.PhaseAnimation, Scope: engine.ScopePlay, Pause: engine.PauseAlways, System: step{}})
	if r.Bindings() != nil {
		installScript(r.ScriptModule(ScriptModule), m)
	}
	r.Finalize(func(r *module.Registry) error {
		m.sections = r.SaveSections()
		var games []string
		for _, info := range r.GameModules() {
			games = append(games, info.ID+"@"+info.Version)
		}
		m.game = strings.Join(games, ",")
		return nil
	})
	return nil
}

func (m *Module) Init(ctx *module.InitContext) error {
	m.eng = ctx.Engine
	lg := ctx.Log
	if lg == nil {
		lg = log.Nop()
	}
	m.log = lg.With(log.Channel(ID))
	return nil
}

func (m *Module) logger() *log.ChannelLogger {
	if m.log == nil {
		m.log = log.Nop().With(log.Channel(ID))
	}
	return m.log
}

func (m *Module) AttachWorld(ctx *module.AttachContext) error {
	if !ctx.Play || ctx.World == nil || ctx.Scene == nil {
		return nil
	}
	s := NewService(m, ctx.World, ctx.Scene)
	ctx.World.SetService(persistence.ServiceID, s)
	if doc := m.pending; doc != nil {
		m.pending = nil
		m.finish(Result{Slot: doc.Meta.Name, Load: true, Err: s.Apply(doc)})
	}
	return nil
}

func (*Module) DetachWorld(ctx *module.AttachContext) {
	if ctx.Play && ctx.World != nil {
		ctx.World.RemoveService(persistence.ServiceID)
	}
}

func (m *Module) finish(r Result) {
	m.last = r
	m.rev++
	what := "saved"
	if r.Load {
		what = "loaded"
	}
	if r.Err != nil {
		m.logger().Error("cannot "+strings.TrimSuffix(what, "d")+" the game", log.F("slot", r.Slot), log.F("err", r.Err.Error()))
		return
	}
	m.logger().Info("game "+what, log.F("slot", r.Slot))
}

func (m *Module) Last() (Result, uint64) {
	return m.last, m.rev
}

func (m *Module) Playtime() float64 {
	return m.playtime
}

func (m *Module) Global(key string) (aetext.Value, bool) {
	return m.global.Field(key)
}

func (m *Module) SetGlobal(key string, v aetext.Value) {
	m.global.SetField(key, v)
}

func (m *Module) store() *save.Store {
	if m.eng == nil {
		return nil
	}
	st, _ := engine.Service[*save.Store](m.eng, persistence.StoreID)
	return st
}

func (m *Module) encode(st *save.Store, slot string, doc *save.Doc) {
	run := func() {
		err := st.Write(slot, save.Encode(doc), nil)
		done := func() { m.finish(Result{Slot: slot, Err: err}) }
		if m.eng != nil {
			m.eng.Jobs().OnMain(done)
			return
		}
		done()
	}
	if m.eng != nil {
		m.eng.Jobs().Submit(run)
		return
	}
	run()
}

func Of(w *engine.World) *Service {
	s, _ := engine.ServiceOf[*Service](w, persistence.ServiceID)
	return s
}

type step struct{}

func (step) Run(ctx *engine.Context) {
	if s := Of(ctx.World); s != nil {
		s.Step(ctx.Dt, ctx.Unscaled)
	}
}
