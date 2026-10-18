package input

import (
	"aego/engine/module"
	"aego/format/aetext"
)

const (
	Version      = 2
	ListActions  = "actions"
	ListContexts = "contexts"
)

func Section() module.SectionDesc[Config] {
	return module.SectionDesc[Config]{
		ID:       ID,
		Title:    "Input",
		Order:    200,
		Version:  Version,
		Default:  Default,
		Decode:   Decode,
		Encode:   Encode,
		Validate: (*Config).Validate,
		Migrate:  Migrate,
		Names: func(c *Config, list string) []string {
			var out []string
			switch list {
			case ListActions:
				for _, a := range c.Actions {
					out = append(out, a.Name)
				}
			case ListContexts:
				for _, x := range c.Contexts {
					out = append(out, x.Name)
				}
			}
			return out
		},
	}
}

func nameOf(v aetext.Value) string {
	if v.Kind == aetext.VString {
		return v.Str
	}
	return v.Ident
}

func namesOf(v aetext.Value) []string {
	out := make([]string, 0, len(v.List))
	for _, x := range v.List {
		out = append(out, nameOf(x))
	}
	return out
}

func idents(list []string) aetext.Value {
	vals := make([]aetext.Value, len(list))
	for i, s := range list {
		vals[i] = aetext.Ident(s)
	}
	return aetext.List(vals...)
}

func Decode(n *aetext.Node, c *Config) error {
	*c = Config{Players: 1, Deadzone: DefaultDeadzone}
	for _, f := range n.Fields {
		switch f.Name {
		case "players":
			c.Players = int(f.Value.AsInt())
		case "deadzone":
			c.Deadzone = float32(f.Value.AsFloat())
		case "stack":
			c.Stack = namesOf(f.Value)
		case "migrated":
			c.Migrated = f.Value.Bool
		default:
			return bad("unknown setting " + f.Name)
		}
	}
	for _, b := range n.Blocks {
		switch b.Head {
		case "action":
			a, err := decodeAction(b)
			if err != nil {
				return err
			}
			c.Actions = append(c.Actions, a)
		case "context":
			x, err := decodeContext(b)
			if err != nil {
				return err
			}
			c.Contexts = append(c.Contexts, x)
		default:
			return bad("unknown block " + b.Head)
		}
	}
	return nil
}

func decodeAction(n *aetext.Node) (Action, error) {
	var a Action
	for _, f := range n.Fields {
		v := f.Value
		switch f.Name {
		case "name":
			a.Name = nameOf(v)
		case "type":
			k, ok := KindByName(nameOf(v))
			if !ok {
				return a, bad("action " + a.Name + ": unknown type " + nameOf(v))
			}
			a.Kind = k
		case "bind":
			for _, x := range v.List {
				b, err := ParseBinding(nameOf(x))
				if err != nil {
					return a, err
				}
				a.Bindings = append(a.Bindings, b)
			}
		case "interaction":
			t, ok := TriggerByName(nameOf(v))
			if !ok {
				return a, bad("action " + a.Name + ": unknown interaction " + nameOf(v))
			}
			a.Interaction.Trigger = t
		case "time":
			a.Interaction.Time = float32(v.AsFloat())
		case "buffer":
			a.Buffer = float32(v.AsFloat())
		case "deadzone":
			a.Deadzone = float32(v.AsFloat())
		case "sensitivity":
			a.Sensitivity = float32(v.AsFloat())
		case "smoothing":
			a.Smoothing = float32(v.AsFloat())
		default:
			return a, bad("action " + a.Name + ": unknown field " + f.Name)
		}
	}
	return a, nil
}

func decodeContext(n *aetext.Node) (Context, error) {
	var x Context
	for _, f := range n.Fields {
		switch f.Name {
		case "name":
			x.Name = nameOf(f.Value)
		case "priority":
			x.Priority = int(f.Value.AsInt())
		case "consume":
			x.Consume = f.Value.Bool
		case "release":
			x.Release = f.Value.Bool
		case "actions":
			x.Actions = namesOf(f.Value)
		default:
			return x, bad("context " + x.Name + ": unknown field " + f.Name)
		}
	}
	return x, nil
}

func field(name string, v aetext.Value) aetext.Field {
	return aetext.Field{Name: name, Value: v}
}

func Encode(c *Config, n *aetext.Node) {
	if c.Players > 1 {
		n.Fields = append(n.Fields, field("players", aetext.Int(int64(c.Players))))
	}
	if c.Deadzone != DefaultDeadzone {
		n.Fields = append(n.Fields, field("deadzone", aetext.Float32(c.Deadzone)))
	}
	if len(c.Stack) > 0 {
		n.Fields = append(n.Fields, field("stack", idents(c.Stack)))
	}
	for i := range c.Actions {
		n.Blocks = append(n.Blocks, encodeAction(&c.Actions[i]))
	}
	for i := range c.Contexts {
		x := &c.Contexts[i]
		b := &aetext.Node{Head: "context", Fields: []aetext.Field{field("name", aetext.Ident(x.Name))}}
		if x.Priority != 0 {
			b.Fields = append(b.Fields, field("priority", aetext.Int(int64(x.Priority))))
		}
		if x.Consume {
			b.Fields = append(b.Fields, field("consume", aetext.Bool(true)))
		}
		if x.Release {
			b.Fields = append(b.Fields, field("release", aetext.Bool(true)))
		}
		b.Fields = append(b.Fields, field("actions", idents(x.Actions)))
		n.Blocks = append(n.Blocks, b)
	}
}

func encodeAction(a *Action) *aetext.Node {
	b := &aetext.Node{Head: "action", Fields: []aetext.Field{
		field("name", aetext.Ident(a.Name)),
		field("type", aetext.Ident(a.Kind.String())),
	}}
	if len(a.Bindings) > 0 {
		vals := make([]aetext.Value, len(a.Bindings))
		for i := range a.Bindings {
			vals[i] = aetext.String(a.Bindings[i].String())
		}
		b.Fields = append(b.Fields, field("bind", aetext.List(vals...)))
	}
	if a.Interaction.Trigger != Press {
		b.Fields = append(b.Fields, field("interaction", aetext.Ident(a.Interaction.Trigger.String())))
	}
	for _, f := range []struct {
		name string
		v    float32
	}{{"time", a.Interaction.Time}, {"buffer", a.Buffer}, {"deadzone", a.Deadzone}, {"sensitivity", a.Sensitivity}, {"smoothing", a.Smoothing}} {
		if f.v != 0 {
			b.Fields = append(b.Fields, field(f.name, aetext.Float32(f.v)))
		}
	}
	return b
}

func Migrate(n *aetext.Node, from, to int) error {
	if from >= 2 {
		return nil
	}
	var acts []*aetext.Node
	var names []aetext.Value
	for _, b := range n.Blocks {
		if b.Head != "axes" {
			return bad("unknown block " + b.Head + " in a version 1 input section")
		}
		for _, ax := range b.Blocks {
			var binds []aetext.Value
			for _, pr := range [2][2]string{{"negative", "positive"}, {"altNegative", "altPositive"}} {
				neg, ok1 := ax.Field(pr[0])
				pos, ok2 := ax.Field(pr[1])
				if ok1 && ok2 {
					binds = append(binds, aetext.String("Key."+nameOf(neg)+"/Key."+nameOf(pos)))
				}
			}
			acts = append(acts, &aetext.Node{Head: "action", Fields: []aetext.Field{
				field("name", aetext.Ident(ax.Head)),
				field("type", aetext.Ident(Axis1D.String())),
				field("bind", aetext.List(binds...)),
			}})
			names = append(names, aetext.Ident(ax.Head))
		}
	}
	ctx := &aetext.Node{Head: "context", Fields: []aetext.Field{
		field("name", aetext.Ident(DefaultContext)),
		field("actions", aetext.List(names...)),
	}}
	n.Blocks = append(acts, ctx)
	n.Fields = append(n.Fields, field("stack", aetext.List(aetext.Ident(DefaultContext))), field("migrated", aetext.Bool(true)))
	return nil
}
