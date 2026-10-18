package input

import (
	"aego/core/errs"
)

const (
	ID              = "input"
	ScriptModule    = "aego/input"
	MaxPlayers      = 4
	DefaultContext  = "Default"
	DefaultDeadzone = 0.2
)

type Kind uint8

const (
	Button Kind = iota
	Axis1D
	Axis2D
	Pointer
	PointerDelta
	kindCount
)

var kindNames = [kindCount]string{"button", "axis1", "axis2", "pointer", "pointerDelta"}

func (k Kind) String() string {
	if k < kindCount {
		return kindNames[k]
	}
	return ""
}

func KindByName(s string) (Kind, bool) {
	for i, n := range kindNames {
		if n == s {
			return Kind(i), true
		}
	}
	return 0, false
}

type Trigger uint8

const (
	Press Trigger = iota
	Release
	Hold
	Tap
	DoubleTap
	Toggle
	triggerCount
)

var triggerNames = [triggerCount]string{"press", "release", "hold", "tap", "doubleTap", "toggle"}

func (t Trigger) String() string {
	if t < triggerCount {
		return triggerNames[t]
	}
	return ""
}

func TriggerByName(s string) (Trigger, bool) {
	for i, n := range triggerNames {
		if n == s {
			return Trigger(i), true
		}
	}
	return 0, false
}

type Interaction struct {
	Trigger Trigger
	Time    float32
}

type Action struct {
	Name        string
	Kind        Kind
	Bindings    []Binding
	Interaction Interaction
	Buffer      float32
	Deadzone    float32
	Sensitivity float32
	Smoothing   float32
}

type Context struct {
	Name     string
	Priority int
	Consume  bool
	Release  bool
	Actions  []string
}

type Config struct {
	Players  int
	Deadzone float32
	Stack    []string
	Actions  []Action
	Contexts []Context
	Migrated bool
}

func mustBindings(list ...string) []Binding {
	out := make([]Binding, len(list))
	for i, s := range list {
		b, err := ParseBinding(s)
		if err != nil {
			panic(err)
		}
		out[i] = b
	}
	return out
}

func Default() Config {
	return Config{
		Players:  1,
		Deadzone: DefaultDeadzone,
		Stack:    []string{DefaultContext},
		Actions: []Action{
			{Name: "horizontal", Kind: Axis1D, Bindings: mustBindings("Key.A/Key.D", "Key.Left/Key.Right")},
			{Name: "vertical", Kind: Axis1D, Bindings: mustBindings("Key.S/Key.W", "Key.Down/Key.Up")},
		},
		Contexts: []Context{{Name: DefaultContext, Actions: []string{"horizontal", "vertical"}}},
	}
}

func bad(msg string) error {
	return errs.New(errs.ProjectBadFile, "input: "+msg)
}

func validName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

func (c *Config) Validate() error {
	if c.Players < 1 || c.Players > MaxPlayers {
		return bad("players must be from 1 to 4")
	}
	if c.Deadzone < 0 || c.Deadzone >= 1 {
		return bad("deadzone must be in [0, 1)")
	}
	acts := map[string]bool{}
	for i := range c.Actions {
		a := &c.Actions[i]
		switch {
		case !validName(a.Name):
			return bad("action name " + a.Name + " is not a valid identifier")
		case acts[a.Name]:
			return bad("action " + a.Name + " is declared twice")
		case a.Buffer < 0 || a.Deadzone < 0 || a.Deadzone >= 1 || a.Smoothing < 0 || a.Interaction.Time < 0:
			return bad("action " + a.Name + " has a negative or out-of-range setting")
		case (a.Kind == Pointer || a.Kind == PointerDelta) && len(a.Bindings) > 0:
			return bad("action " + a.Name + " of type " + a.Kind.String() + " takes no bindings")
		}
		for j := range a.Bindings {
			if !a.Bindings[j].fits(a.Kind) {
				return bad("action " + a.Name + ": binding " + a.Bindings[j].String() + " does not fit " + a.Kind.String())
			}
		}
		acts[a.Name] = true
	}
	ctxs := map[string]bool{}
	for _, x := range c.Contexts {
		switch {
		case !validName(x.Name):
			return bad("context name " + x.Name + " is not a valid identifier")
		case ctxs[x.Name]:
			return bad("context " + x.Name + " is declared twice")
		}
		for _, a := range x.Actions {
			if !acts[a] {
				return bad("context " + x.Name + " names the unknown action " + a)
			}
		}
		ctxs[x.Name] = true
	}
	for _, s := range c.Stack {
		if !ctxs[s] {
			return bad("stack names the unknown context " + s)
		}
	}
	return nil
}
