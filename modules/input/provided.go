package input

import (
	"slices"

	"aego/contracts/inputactions"
	"aego/core/errs"
	"aego/core/log"
	"aego/engine/module"
)

func (c *Config) clone() Config {
	x := *c
	x.Stack = slices.Clone(c.Stack)
	x.Actions = slices.Clone(c.Actions)
	x.Contexts = slices.Clone(c.Contexts)
	return x
}

func merge(c *Config, list []module.Provision) error {
	for _, pv := range list {
		set, ok := pv.Value.(inputactions.Set)
		if !ok {
			return errs.New(errs.EngineModule, "module "+pv.Owner+" provides input actions that are not an inputactions.Set", log.F("module", pv.Owner))
		}
		if err := mergeSet(c, set); err != nil {
			return errs.Wrap(err, errs.EngineModule, "input actions of module "+pv.Owner).With(log.F("module", pv.Owner))
		}
	}
	return c.Validate()
}

func mergeSet(c *Config, set inputactions.Set) error {
	for _, x := range set.Actions {
		if slices.ContainsFunc(c.Actions, func(a Action) bool { return a.Name == x.Name }) {
			continue
		}
		k, ok := KindByName(x.Type)
		if !ok {
			return bad("action " + x.Name + ": unknown type " + x.Type)
		}
		a := Action{Name: x.Name, Kind: k, Buffer: x.Buffer, Deadzone: x.Deadzone}
		if x.Interaction != "" {
			t, ok := TriggerByName(x.Interaction)
			if !ok {
				return bad("action " + x.Name + ": unknown interaction " + x.Interaction)
			}
			a.Interaction = Interaction{Trigger: t, Time: x.Time}
		}
		for _, s := range x.Bindings {
			b, err := ParseBinding(s)
			if err != nil {
				return err
			}
			a.Bindings = append(a.Bindings, b)
		}
		c.Actions = append(c.Actions, a)
	}
	for _, x := range set.Contexts {
		if !slices.ContainsFunc(c.Contexts, func(y Context) bool { return y.Name == x.Name }) {
			c.Contexts = append(c.Contexts, Context{Name: x.Name, Priority: x.Priority, Consume: x.Consume, Release: x.Release, Actions: slices.Clone(x.Actions)})
		}
		if x.Default && !slices.Contains(c.Stack, x.Name) {
			c.Stack = append(c.Stack, x.Name)
		}
	}
	return nil
}
