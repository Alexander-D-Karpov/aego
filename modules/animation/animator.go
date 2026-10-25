package animation

import (
	"aego/anim"
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine/module"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
)

const (
	NameAnimator       = ID + ".Animator"
	MaxParams          = 32
	CodeCtrlClip       = "ANC004"
	acmdJump     uint8 = 1
	acmdFade     uint8 = 2
)

type RootMotion uint8

const (
	RootNone RootMotion = iota
	RootToMotion
	RootToTransform
)

type ControllerRef struct {
	ID ids.AssetID
}

func (r *ControllerRef) AssetRefID() ids.AssetID      { return r.ID }
func (r *ControllerRef) SetAssetRefID(id ids.AssetID) { r.ID = id }
func (r *ControllerRef) AssetKind() string            { return string(asset.ArtCtrl) }
func (r ControllerRef) IsZero() bool                  { return r.ID.IsZero() }

type animatorState struct {
	m      anim.Machine
	params [MaxParams]float64
	ctrl   ids.AssetID
	target int32
	fade   float32
	cmd    uint8
	on     bool
	auto   bool
	slot   int32
}

type Animator struct {
	Controller   ControllerRef `aego:"controller"`
	PlayOnStart  bool          `aego:"playOnStart"`
	Speed        float32       `aego:"speed"`
	TimeDomain   TimeDomain    `aego:"timeDomain"`
	RootMotion   RootMotion    `aego:"rootMotion"`
	RootFallback bool          `aego:"rootFallback"`
	RootScale    mathx.Vec2    `aego:"rootScale"`
	State        string        `aego:"state,transient,readonly,category=Runtime"`
	Time         float32       `aego:"time,transient,readonly,category=Runtime"`
	Progress     float32       `aego:"progress,transient,readonly,category=Runtime"`
	Blending     bool          `aego:"blending,transient,readonly,category=Runtime"`

	st animatorState
}

func DefaultAnimator() Animator {
	return Animator{PlayOnStart: true, Speed: 1, RootScale: mathx.Vec2{X: 1, Y: 1}}
}

type asignals struct {
	entered module.Signal1[int64]
	exited  module.Signal1[int64]
	event   module.Signal2[int64, float32]
}

type ctrlHit struct {
	res asset.Resolver
	e   *ctrlEntry
	gen uint32
}

type ctrlEntry struct {
	g      *anim.Graph
	clips  []*anim.Clip
	params map[string]int32
	states map[string]int32
}

func (e *ctrlEntry) nParams() int {
	return min(len(e.g.Params), MaxParams)
}

func (e *ctrlEntry) reset(a *Animator) {
	a.st.ctrl = a.Controller.ID
	a.st.m = anim.Machine{}
	a.st.cmd = 0
	clear(a.st.params[:])
	for i := range e.nParams() {
		if p := &e.g.Params[i]; p.Kind != anim.ParamTrigger {
			a.st.params[i] = p.Default
		}
	}
}

func (e *ctrlEntry) prepare(a *Animator) {
	if a.st.ctrl != a.Controller.ID {
		e.reset(a)
	}
}

func (m *Module) ctrl(res asset.Resolver, id ids.AssetID) *ctrlEntry {
	if res == nil || id.IsZero() {
		return nil
	}
	gen := res.Generation()
	if h, ok := m.ctrls[id]; ok && h.res == res && h.gen == gen {
		return h.e
	}
	dec, ok := res.(asset.Decoder)
	src, ok2 := res.(asset.CtrlSource)
	if !ok || !ok2 {
		return nil
	}
	v, err := dec.Decoded(id, "anim.ctrl", func() (any, error) {
		data, ok := src.Controller(id)
		if !ok {
			return nil, errNoClip
		}
		g, err := anim.UnmarshalGraph(data)
		if err != nil {
			return nil, err
		}
		if len(g.Check(nil)) > 0 {
			return nil, errNoClip
		}
		return m.newCtrl(res, g), nil
	})
	if err != nil {
		return nil
	}
	e, _ := v.(*ctrlEntry)
	m.ctrls[id] = ctrlHit{res: res, e: e, gen: gen}
	return e
}

func (m *Module) newCtrl(res asset.Resolver, g *anim.Graph) *ctrlEntry {
	e := &ctrlEntry{g: g, clips: make([]*anim.Clip, len(g.States)), params: map[string]int32{}, states: map[string]int32{}}
	for i := range g.States {
		st := &g.States[i]
		e.states[st.Name] = int32(i)
		if c := m.clip(res, st.Clip); c != nil {
			e.clips[i] = c
			st.Length = c.Length
			if !st.HasLoop {
				st.Loop = c.Loop
			}
		}
	}
	for i, p := range g.Params {
		e.params[p.Name] = int32(i)
	}
	return e
}

func (m *Module) ctrlFor(c *bind.Ctx, a *Animator) *ctrlEntry {
	if c.Scene == nil {
		return nil
	}
	e := m.ctrl(c.Scene.Resolver(), a.Controller.ID)
	if e != nil {
		e.prepare(a)
	}
	return e
}

func (m *Module) param(c *bind.Ctx, a *Animator, name string) int32 {
	e := m.ctrlFor(c, a)
	if e == nil {
		c.Fail("the animator has no controller loaded")
		return -1
	}
	i, ok := e.params[name]
	if !ok || i >= MaxParams {
		c.Fail("the controller has no parameter " + name)
		return -1
	}
	return i
}

func (m *Module) stateOf(c *bind.Ctx, a *Animator, name string) int32 {
	e := m.ctrlFor(c, a)
	if e == nil {
		c.Fail("the animator has no controller loaded")
		return -1
	}
	i, ok := e.states[name]
	if !ok {
		c.Fail("the controller has no state " + name)
		return -1
	}
	return i
}

func flag(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func (s *Service) SetParam(h scene.NodeHandle, name string, v float64) bool {
	a, ok := scene.Get[Animator](s.scene, h)
	if !ok {
		return false
	}
	e := s.m.ctrl(s.scene.Resolver(), a.Controller.ID)
	if e == nil {
		return false
	}
	e.prepare(a)
	i, ok := e.params[name]
	if !ok || i >= MaxParams {
		return false
	}
	a.st.params[i] = v
	return true
}

// Param reads a parameter of the node's Animator, for the editor's Animator panel during Play.
func (s *Service) Param(h scene.NodeHandle, name string) (float64, bool) {
	a, ok := scene.Get[Animator](s.scene, h)
	if !ok {
		return 0, false
	}
	e := s.m.ctrl(s.scene.Resolver(), a.Controller.ID)
	if e == nil {
		return 0, false
	}
	e.prepare(a)
	i, ok := e.params[name]
	if !ok || i >= MaxParams {
		return 0, false
	}
	return a.st.params[i], true
}

func registerAnimator(r *module.Registry, m *Module) error {
	if err := module.Enum[RootMotion](r, ID+".RootMotion",
		schema.EnumValue{Name: "none", Value: int64(RootNone)},
		schema.EnumValue{Name: "toMotion", Value: int64(RootToMotion)},
		schema.EnumValue{Name: "toTransform", Value: int64(RootToTransform)}); err != nil {
		return err
	}
	b := module.Component[Animator](r, schema.Name(NameAnimator),
		schema.Doc("Plays the states of an animation controller and blends between them when its parameters change. The graph, the blends, root motion and gameplay events run on fixed steps, so replays match at any frame rate."),
		schema.FieldDoc("controller", "Controller with the states, parameters and transitions."),
		schema.FieldDoc("playOnStart", "Enters the entry state when Play starts."),
		schema.FieldDoc("speed", "Multiplies the speed of every state; 1 is normal speed."),
		schema.FieldDoc("timeDomain", "game follows game time and pauses with it; unscaled runs in real time while paused, for presentation only."),
		schema.FieldDoc("rootMotion", "What the root track moves: none ignores it, toMotion moves the node through Motion 2D so walls stop it, toTransform sets its position."),
		schema.FieldDoc("rootFallback", "toMotion moves the transform instead when the node has no Motion 2D."),
		schema.FieldDoc("rootScale", "Multiplies root motion in the node's parent space; set X to -1 while the character faces left."),
		schema.FieldDoc("state", "Current state."),
		schema.FieldDoc("time", "Seconds in the current state."),
		schema.FieldDoc("progress", "Time in the current state divided by the length of its clip; 1 is one full clip."),
		schema.FieldDoc("blending", "Whether a transition is blending."),
		schema.Runtime("state", "time", "progress", "blending"),
	).Category(Category).Display("Animator").Icon("play.start").Default(DefaultAnimator).Conflicts(NamePlayer)

	b.Proc2("SetBool", func(c *bind.Ctx, a *Animator, name string, v bool) {
		if i := m.param(c, a, name); i >= 0 {
			a.st.params[i] = flag(v)
		}
	}, bind.Doc("SetBool sets a bool parameter; transitions read it on the next fixed step."), bind.Params("name", "value"))
	b.Proc2("SetInt", func(c *bind.Ctx, a *Animator, name string, v int64) {
		if i := m.param(c, a, name); i >= 0 {
			a.st.params[i] = float64(v)
		}
	}, bind.Doc("SetInt sets an int parameter."), bind.Params("name", "value"))
	b.Proc2("SetFloat", func(c *bind.Ctx, a *Animator, name string, v float32) {
		if i := m.param(c, a, name); i >= 0 {
			a.st.params[i] = float64(v)
		}
	}, bind.Doc("SetFloat sets a float parameter."), bind.Params("name", "value"), bind.Example("anim.SetFloat(\"Speed\", math.Abs(motion.Velocity.X))"))
	b.Proc1("SetTrigger", func(c *bind.Ctx, a *Animator, name string) {
		if i := m.param(c, a, name); i >= 0 {
			a.st.params[i] = 1
		}
	}, bind.Doc("SetTrigger sets a trigger for the next fixed step; a transition that uses it, or the end of that step, clears it."), bind.Params("name"))
	b.Proc1("ResetTrigger", func(c *bind.Ctx, a *Animator, name string) {
		if i := m.param(c, a, name); i >= 0 {
			a.st.params[i] = 0
		}
	}, bind.Doc("ResetTrigger clears a trigger that has not fired yet."), bind.Params("name"))
	b.Method1("GetBool", func(c *bind.Ctx, a *Animator, name string) bool {
		i := m.param(c, a, name)
		return i >= 0 && a.st.params[i] != 0
	}, bind.Doc("GetBool reads a bool or trigger parameter."), bind.Params("name"))
	b.Method1("GetInt", func(c *bind.Ctx, a *Animator, name string) int64 {
		if i := m.param(c, a, name); i >= 0 {
			return int64(a.st.params[i])
		}
		return 0
	}, bind.Doc("GetInt reads an int parameter."), bind.Params("name"))
	b.Method1("GetFloat", func(c *bind.Ctx, a *Animator, name string) float32 {
		if i := m.param(c, a, name); i >= 0 {
			return float32(a.st.params[i])
		}
		return 0
	}, bind.Doc("GetFloat reads a float parameter."), bind.Params("name"))
	b.Proc1("Play", func(c *bind.Ctx, a *Animator, state string) {
		if i := m.stateOf(c, a, state); i >= 0 {
			a.st.cmd, a.st.target = acmdJump, i
		}
	}, bind.Doc("Play switches to a state on the next fixed step, without a blend."), bind.Params("state"))
	b.Proc2("CrossFade", func(c *bind.Ctx, a *Animator, state string, sec float32) {
		if i := m.stateOf(c, a, state); i >= 0 {
			a.st.cmd, a.st.target, a.st.fade = acmdFade, i, max(sec, 0)
		}
	}, bind.Doc("CrossFade blends into a state over some seconds, starting on the next fixed step."), bind.Params("state", "seconds"))
	b.Method1("IsState", func(c *bind.Ctx, a *Animator, state string) bool {
		e := m.ctrlFor(c, a)
		if e == nil || !a.st.m.Started {
			return false
		}
		i, ok := e.states[state]
		return ok && i == a.st.m.State
	}, bind.Doc("IsState reports whether the animator is in a state; during a blend that is the state it blends into."), bind.Params("state"))
	b.Method1("StateName", func(c *bind.Ctx, a *Animator, i int64) string {
		e := m.ctrlFor(c, a)
		if e == nil || i < 0 || int(i) >= len(e.g.States) {
			return ""
		}
		return e.g.States[i].Name
	}, bind.Doc("StateName returns the name of a state index, as StateEntered and StateExited report it."), bind.Params("index"))

	m.asig.entered = b.Signal1[int64]("StateEntered", "StateEntered runs when the animator enters a state, at the start of the transition.", "state")
	m.asig.exited = b.Signal1[int64]("StateExited", "StateExited runs when a state stops playing, at the end of the transition.", "state")
	m.asig.event = b.Signal2[int64, float32]("Event", "Event runs when a state reaches an event of its clip; during a blend the state's event policy decides which side reports.", "id", "payload")
	return b.Err()
}
