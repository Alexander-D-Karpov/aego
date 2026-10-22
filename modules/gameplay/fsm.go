package gameplay

import (
	"aego/contracts/scriptcall"
	"aego/engine"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
	"aego/script/vm"
)

const (
	NameStateMachine = "gameplay.StateMachine"
	CodeBadStates    = "GMP005"
	maxTransitions   = 8
)

type fsmHooks struct {
	enter, exit, fixed, update             scriptcall.Ref
	hasEnter, hasExit, hasFixed, hasUpdate bool
}

type fsmState struct {
	started bool
	gen     uint64
	hooks   []fsmHooks
	queued  int32
}

type StateMachine struct {
	States   []string `aego:"states"`
	Initial  string   `aego:"initial"`
	Current  string   `aego:"current,transient,readonly,category=Runtime"`
	Previous string   `aego:"previous,transient,readonly,category=Runtime"`
	Time     float32  `aego:"time,transient,readonly,category=Runtime"`
	Index    int32    `aego:"index,transient,readonly,category=Runtime"`

	st fsmState
}

func DefaultStateMachine() StateMachine {
	return StateMachine{States: []string{"Idle"}, Initial: "Idle"}
}

func (m *StateMachine) find(name string) int32 {
	for i, s := range m.States {
		if s == name {
			return int32(i)
		}
	}
	return -1
}

// Go queues a change; the system applies it after the running hook returns.
func (m *StateMachine) Go(state string) bool {
	i := m.find(state)
	if i < 0 {
		return false
	}
	m.st.queued = i + 1
	return true
}

func (m *StateMachine) Is(state string) bool {
	return m.st.started && m.Current == state
}

func validateFSM(_ module.ValidateContext, m *StateMachine) []problems.Diagnostic {
	bad := func(msg string) []problems.Diagnostic {
		return []problems.Diagnostic{{Code: CodeBadStates, Severity: problems.Warning, Message: "State Machine: " + msg}}
	}
	if len(m.States) == 0 {
		return bad("has no states")
	}
	for i, s := range m.States {
		if s == "" {
			return bad("a state has no name")
		}
		for _, o := range m.States[:i] {
			if o == s {
				return bad("state " + s + " is listed twice")
			}
		}
	}
	if m.Initial != "" && m.find(m.Initial) < 0 {
		return bad("Initial " + m.Initial + " is not one of the states")
	}
	return nil
}

func (s *Service) caller() scriptcall.Caller {
	if s.call == nil && s.world != nil {
		s.call, _ = engine.ServiceOf[scriptcall.Caller](s.world, scriptcall.ServiceID)
	}
	return s.call
}

// bindHooks looks the hooks up by name again after a hot reload changed the program generation.
func (s *Service) bindHooks(h scene.NodeHandle, m *StateMachine) {
	c := s.caller()
	if c == nil {
		return
	}
	st := &m.st
	gen := c.Generation()
	if st.gen == gen && len(st.hooks) == len(m.States) {
		return
	}
	st.gen = gen
	st.hooks = st.hooks[:0]
	for _, name := range m.States {
		var k fsmHooks
		k.enter, k.hasEnter = c.Method(h, "Enter"+name, 0)
		k.exit, k.hasExit = c.Method(h, "Exit"+name, 0)
		k.fixed, k.hasFixed = c.Method(h, "Fixed"+name, 1)
		k.update, k.hasUpdate = c.Method(h, "Update"+name, 1)
		st.hooks = append(st.hooks, k)
	}
}

func (s *Service) hook(m *StateMachine, i int32) *fsmHooks {
	if i >= 0 && int(i) < len(m.st.hooks) {
		return &m.st.hooks[i]
	}
	return nil
}

func (s *Service) run(ref *scriptcall.Ref, ok bool) {
	if ok && s.call != nil {
		s.call.Call(ref, nil)
	}
}

func (s *Service) runDt(ref *scriptcall.Ref, ok bool, dt float32) {
	if ok && s.call != nil {
		s.args[0] = vm.Float(dt)
		s.call.Call(ref, s.args[:1])
	}
}

func (s *Service) startFSM(h scene.NodeHandle, m *StateMachine) {
	m.st.started = true
	if len(m.States) == 0 {
		return
	}
	i := m.find(m.Initial)
	if i < 0 {
		i = 0
	}
	m.Index, m.Current, m.Previous, m.Time = i, m.States[i], "", 0
	if k := s.hook(m, i); k != nil {
		s.run(&k.enter, k.hasEnter)
	}
	s.applyQueued(h, m)
}

func (s *Service) applyQueued(h scene.NodeHandle, m *StateMachine) {
	st := &m.st
	for n := 0; n < maxTransitions && st.queued > 0; n++ {
		to := st.queued - 1
		st.queued = 0
		if to == m.Index || int(to) >= len(m.States) {
			continue
		}
		from := m.Index
		if k := s.hook(m, from); k != nil {
			s.run(&k.exit, k.hasExit)
		}
		m.Previous, m.Current, m.Index, m.Time = m.Current, m.States[to], to, 0
		if k := s.hook(m, to); k != nil {
			s.run(&k.enter, k.hasEnter)
		}
		s.record(h, "state", m.States[to])
		if s.world != nil && s.sig != nil {
			s.sig.changed.Emit(s.world, h, int64(from), int64(to))
		}
	}
}

func (s *Service) fsmFixed(h scene.NodeHandle, m *StateMachine) {
	if !m.st.started || !s.scene.EffectiveEnabled(h) {
		return
	}
	s.bindHooks(h, m)
	s.applyQueued(h, m)
	m.Time += s.dt
	if k := s.hook(m, m.Index); k != nil {
		s.runDt(&k.fixed, k.hasFixed, s.dt)
	}
	s.applyQueued(h, m)
}

func (s *Service) fsmUpdate(h scene.NodeHandle, m *StateMachine) {
	if !s.scene.EffectiveEnabled(h) {
		return
	}
	s.bindHooks(h, m)
	if !m.st.started {
		s.startFSM(h, m)
		return
	}
	s.applyQueued(h, m)
	if k := s.hook(m, m.Index); k != nil {
		s.runDt(&k.update, k.hasUpdate, s.fdt)
	}
	s.applyQueued(h, m)
}

func (s *Service) FSMFixed(dt float32) {
	s.dt = dt
	scene.Query1[StateMachine](s.scene).Each(s.fsmFixFn)
}

func (s *Service) FSMUpdate(dt float32) {
	s.fdt = dt
	scene.Query1[StateMachine](s.scene).Each(s.fsmUpdFn)
}

func registerFSM(r *module.Registry, sig *signals) error {
	b := module.Component[StateMachine](r, schema.Name(NameStateMachine),
		schema.Doc("Named states with Enter, Exit, Fixed and Update hooks found by name on the node's scripts: EnterRun(), ExitRun(), FixedRun(dt), UpdateRun(dt). Scripts change the state with Go."),
		schema.FieldDoc("states", "State names, also the suffix of the hook methods."),
		schema.FieldDoc("initial", "State entered when the node starts, after the scripts' Ready. Empty uses the first state."),
		schema.FieldDoc("current", "State now."),
		schema.FieldDoc("previous", "State before the last change."),
		schema.FieldDoc("time", "Seconds of fixed steps spent in the current state."),
		schema.FieldDoc("index", "Index of the current state in States."),
		schema.Runtime("current", "previous", "time", "index"),
	).Category(Category).Display("State Machine").Icon("gameplay.fsm").Default(DefaultStateMachine).Validate(validateFSM)
	b.Method1("Go", func(_ *bind.Ctx, m *StateMachine, state string) bool { return m.Go(state) },
		bind.Doc("Go switches to a state after the current hook returns; Exit of the old state and Enter of the new one run in between. Returns false for an unknown state."),
		bind.Params("state"))
	b.Method1("Is", func(_ *bind.Ctx, m *StateMachine, state string) bool { return m.Is(state) },
		bind.Doc("Is reports whether the machine is in a state."), bind.Params("state"))
	b.Method0("TimeIn", func(_ *bind.Ctx, m *StateMachine) float32 { return m.Time },
		bind.Doc("TimeIn returns the seconds spent in the current state."))
	sig.changed = b.Signal2[int64, int64]("Changed", "Changed fires after a state change with the indices of the old and the new state.", "from", "to")
	return b.Err()
}
