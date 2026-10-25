package agent

import (
	"errors"
	"strconv"
	"strings"

	"aego/anim"
	"aego/format/aetext"
	"aego/scene/mutate"
)

type textDoc interface {
	Apply(mutate.Tx) (*mutate.Result, error)
	DryRun(mutate.Tx) (*mutate.Result, error)
	Bytes() []byte
}

func loadAnim(src []byte, rel string) (textDoc, error) {
	d, err := mutate.LoadAnim(src, rel)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func loadCtrl(src []byte, rel string) (textDoc, error) {
	d, err := mutate.LoadCtrl(src, rel)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Session) textTx(a Args, label string, load func([]byte, string) (textDoc, error), ops ...mutate.Op) (*Output, error) {
	rel, err := s.rel(a.Str("file"))
	if err != nil {
		return nil, err
	}
	src, err := s.read(rel)
	if err != nil {
		return nil, err
	}
	d, err := load(src, rel)
	if err != nil {
		return nil, err
	}
	tx := mutate.Tx{Label: label, Ops: ops}
	var res *mutate.Result
	if a.Bool("dry-run") {
		res, err = d.DryRun(tx)
	} else {
		res, err = d.Apply(tx)
	}
	if err != nil {
		return nil, err
	}
	out := &Output{DryRun: res.DryRun, Changes: res.Changes, Problems: res.Problems}
	if res.DryRun {
		return out, nil
	}
	if out.Errors() {
		return out, errors.New("the result has errors, nothing was written")
	}
	out.Files, err = s.save(rel, src, d.Bytes())
	return out, err
}

func (s *Session) animDoc(a Args) (*mutate.AnimDocument, error) {
	rel, err := s.rel(a.Str("file"))
	if err != nil {
		return nil, err
	}
	src, err := s.read(rel)
	if err != nil {
		return nil, err
	}
	return mutate.LoadAnim(src, rel)
}

func (s *Session) ctrlDoc(a Args) (*mutate.CtrlDocument, error) {
	rel, err := s.rel(a.Str("file"))
	if err != nil {
		return nil, err
	}
	src, err := s.read(rel)
	if err != nil {
		return nil, err
	}
	return mutate.LoadCtrl(src, rel)
}

func animTarget(a Args) string {
	if t := a.Str("target"); t != "." {
		return t
	}
	return ""
}

func attrMap(n *aetext.Node) map[string]any {
	out := map[string]any{}
	for _, f := range n.Attrs {
		out[f.Name] = mutate.FormatValue(f.Value)
	}
	return out
}

func number(a Args, name string) (int, error) {
	n, err := a.Int(name, 0)
	if err != nil {
		return 0, err
	}
	if n < 1 {
		return 0, errors.New(name + " counts from 1")
	}
	return n - 1, nil
}

func optValue(a Args, name string) (aetext.Value, bool, error) {
	if !a.Has(name) {
		return aetext.Value{}, false, nil
	}
	v, err := mutate.ParseValue(a.Str(name))
	return v, err == nil, err
}

func animCommands() []*Command {
	file := Arg{Name: "file", Type: "string", Doc: ".aeanim file", Required: true, Pos: true}
	tgt := Arg{Name: "target", Type: "string", Doc: "node path from the player's node, . for the node itself", Required: true, Pos: true}
	prop := Arg{Name: "prop", Type: "string", Doc: "property, like SpriteRenderer.tint or transform.position", Required: true, Pos: true}
	at := Arg{Name: "time", Type: "string", Doc: "seconds", Required: true, Pos: true}
	val := Arg{Name: "value", Type: "string", Doc: "value as in the clip file: 0.5, [1, 2], #ff0000ff, asset(\"…\")", Required: true, Pos: true}
	index := Arg{Name: "index", Type: "string", Doc: "event number, from 1, in the order anim get lists them", Required: true, Pos: true}
	tx := func(build func(Args) ([]mutate.Op, error)) func(*Session, Args) (*Output, error) {
		return func(s *Session, a Args) (*Output, error) {
			ops, err := build(a)
			if err != nil {
				return nil, err
			}
			return s.textTx(a, "Animation", loadAnim, ops...)
		}
	}
	return []*Command{
		{Name: "anim.get", Doc: "length, loop, tracks with their keys, frames blocks and events of a clip", Args: []Arg{file}, Run: (*Session).animGet},
		{Name: "anim.set-key", Doc: "set a key; a new track needs --kind", Write: true,
			Args: []Arg{file, tgt, prop, at, val,
				{Name: "kind", Type: "string", Doc: "kind of a new track: float32, int32, bool, enum, vec2, vec3, vec4, color, rect, quat, assetRef"},
				{Name: "interp", Type: "string", Doc: "bezier, linear or step"},
				{Name: "tangent", Type: "string", Doc: "auto, linear or free"}},
			Run: tx(func(a Args) ([]mutate.Op, error) {
				v, err := mutate.ParseValue(a.Str("value"))
				if err != nil {
					return nil, err
				}
				op := mutate.AnimKeySet(animTarget(a), a.Str("prop"), a.Str("time"), v)
				op.Comp = a.Str("kind")
				extra := &aetext.Node{Head: "values"}
				for _, n := range []string{"interp", "tangent"} {
					if a.Has(n) {
						extra.Attrs = append(extra.Attrs, aetext.Field{Name: n, Value: aetext.Ident(a.Str(n))})
					}
				}
				if len(extra.Attrs) > 0 {
					op.Values = extra
				}
				return []mutate.Op{op}, nil
			})},
		{Name: "anim.remove-key", Doc: "remove a key; the last key removes its track", Write: true, Args: []Arg{file, tgt, prop, at},
			Run: tx(func(a Args) ([]mutate.Op, error) {
				return []mutate.Op{mutate.AnimKeyRemove(animTarget(a), a.Str("prop"), a.Str("time"))}, nil
			})},
		{Name: "anim.move-key", Doc: "move a key to another time", Write: true,
			Args: []Arg{file, tgt, prop, at, {Name: "to", Type: "string", Doc: "new time in seconds", Required: true, Pos: true}},
			Run: tx(func(a Args) ([]mutate.Op, error) {
				return []mutate.Op{mutate.AnimKeyMove(animTarget(a), a.Str("prop"), a.Str("time"), a.Str("to"))}, nil
			})},
		{Name: "anim.remove-track", Doc: "remove a track with all its keys", Write: true, Args: []Arg{file, tgt, prop},
			Run: tx(func(a Args) ([]mutate.Op, error) {
				return []mutate.Op{mutate.AnimTrackRemove(animTarget(a), a.Str("prop"))}, nil
			})},
		{Name: "anim.add-event", Doc: "add an event; the name must be declared in Project Settings → Animation", Write: true,
			Args: []Arg{file, at, {Name: "name", Type: "string", Doc: "event name", Required: true, Pos: true},
				{Name: "payload", Type: "string", Doc: "number or true/false"},
				{Name: "mode", Type: "string", Doc: "gameplay, presentation or preview"}},
			Run: tx(func(a Args) ([]mutate.Op, error) {
				op := mutate.AnimEventAdd(a.Str("time"), a.Str("name"))
				op.Text, op.Level = a.Str("payload"), a.Str("mode")
				return []mutate.Op{op}, nil
			})},
		{Name: "anim.remove-event", Doc: "remove an event", Write: true, Args: []Arg{file, index},
			Run: tx(func(a Args) ([]mutate.Op, error) {
				i, err := number(a, "index")
				if err != nil {
					return nil, err
				}
				return []mutate.Op{mutate.AnimEventRemove(i)}, nil
			})},
		{Name: "anim.move-event", Doc: "move an event to another time", Write: true, Args: []Arg{file, index, at},
			Run: tx(func(a Args) ([]mutate.Op, error) {
				i, err := number(a, "index")
				if err != nil {
					return nil, err
				}
				return []mutate.Op{mutate.AnimEventMove(i, a.Str("time"))}, nil
			})},
		{Name: "anim.set", Doc: "set length, loop or fps of a clip; no value clears loop or fps", Write: true,
			Args: []Arg{file, {Name: "attr", Type: "string", Doc: "length, loop or fps", Required: true, Pos: true},
				{Name: "value", Type: "string", Doc: "number, or once, loop, pingPong", Pos: true}},
			Run: tx(func(a Args) ([]mutate.Op, error) {
				v, ok, err := optValue(a, "value")
				if err != nil {
					return nil, err
				}
				if !ok {
					return []mutate.Op{mutate.AnimClipClear(a.Str("attr"))}, nil
				}
				return []mutate.Op{mutate.AnimClipSet(a.Str("attr"), v)}, nil
			})},
		{Name: "anim.expand-frames", Doc: "turn a frames block into sprite tracks with step keys, so its frames can be edited one by one", Write: true, Args: []Arg{file, tgt},
			Run: tx(func(a Args) ([]mutate.Op, error) {
				return []mutate.Op{mutate.AnimFramesExpand(animTarget(a))}, nil
			})},
	}
}

func (s *Session) animGet(a Args) (*Output, error) {
	d, err := s.animDoc(a)
	if err != nil {
		return nil, err
	}
	root := d.Root()
	v := attrMap(root)
	delete(v, "version")
	var tracks, frames, events []map[string]any
	for _, b := range root.Blocks {
		switch b.Head {
		case "track":
			t := attrMap(b)
			t["target"] = b.Label
			var keys []map[string]any
			for _, k := range b.Blocks {
				keys = append(keys, attrMap(k))
			}
			t["keys"] = keys
			tracks = append(tracks, t)
		case "frames":
			f := attrMap(b)
			f["target"] = b.Label
			if sv, ok := b.Field("sprites"); ok {
				f["sprites"] = len(sv.List)
			}
			frames = append(frames, f)
		case "event":
			e := attrMap(b)
			e["index"] = len(events) + 1
			events = append(events, e)
		}
	}
	v["tracks"], v["frames"], v["events"] = tracks, frames, events
	return &Output{Value: v}, nil
}

var condOps = []struct{ sym, op string }{{">=", "ge"}, {"<=", "le"}, {"!=", "ne"}, {"==", "eq"}, {">", "gt"}, {"<", "lt"}}

func condBlock(text string) (*aetext.Node, error) {
	text = strings.TrimSpace(text)
	for _, c := range condOps {
		name, rest, ok := strings.Cut(text, c.sym)
		if !ok {
			continue
		}
		v, err := mutate.ParseValue(strings.TrimSpace(rest))
		if err != nil {
			return nil, err
		}
		return &aetext.Node{Head: "cond", Label: strings.TrimSpace(name), Attrs: []aetext.Field{
			{Name: "op", Value: aetext.Ident(c.op)}, {Name: "value", Value: v}}}, nil
	}
	if text == "" {
		return nil, errors.New("an empty condition")
	}
	return &aetext.Node{Head: "cond", Label: text}, nil
}

func posValue(text string) (aetext.Value, error) {
	x, y, ok := strings.Cut(text, ",")
	fx, err1 := strconv.ParseFloat(strings.TrimSpace(x), 32)
	fy, err2 := strconv.ParseFloat(strings.TrimSpace(y), 32)
	if !ok || err1 != nil || err2 != nil {
		return aetext.Value{}, errors.New("a position is x,y")
	}
	return aetext.List(aetext.Float32(float32(fx)), aetext.Float32(float32(fy))), nil
}

func (s *Session) stateValue(attr, text string) (aetext.Value, error) {
	switch attr {
	case "clip":
		id, err := s.asset(text)
		if err != nil {
			return aetext.Value{}, err
		}
		return aetext.AssetRef(id.String()), nil
	case "pos":
		return posValue(text)
	case "loop", "events":
		return aetext.Ident(text), nil
	}
	return mutate.ParseValue(text)
}

func ctrlCommands() []*Command {
	file := Arg{Name: "file", Type: "string", Doc: ".aectrl file", Required: true, Pos: true}
	name := Arg{Name: "name", Type: "string", Doc: "state name", Required: true, Pos: true}
	param := Arg{Name: "name", Type: "string", Doc: "parameter name", Required: true, Pos: true}
	index := Arg{Name: "index", Type: "string", Doc: "transition number, from 1, as ctrl get lists them", Required: true, Pos: true}
	attr := Arg{Name: "attr", Type: "string", Doc: "attribute", Required: true, Pos: true}
	value := Arg{Name: "value", Type: "string", Doc: "new value; empty removes the attribute", Pos: true}
	tx := func(build func(*Session, Args) ([]mutate.Op, error)) func(*Session, Args) (*Output, error) {
		return func(s *Session, a Args) (*Output, error) {
			ops, err := build(s, a)
			if err != nil {
				return nil, err
			}
			return s.textTx(a, "Controller", loadCtrl, ops...)
		}
	}
	return []*Command{
		{Name: "ctrl.get", Doc: "parameters, states and transitions of a controller", Args: []Arg{file}, Run: (*Session).ctrlGet},
		{Name: "ctrl.add-state", Doc: "add a state", Write: true,
			Args: []Arg{file, name,
				{Name: "clip", Type: "string", Doc: "clip id or path"},
				{Name: "speed", Type: "string", Doc: "speed"},
				{Name: "loop", Type: "string", Doc: "once, loop or pingPong"},
				{Name: "pos", Type: "string", Doc: "position in the graph, x,y"}},
			Run: tx(func(s *Session, a Args) ([]mutate.Op, error) {
				body := &aetext.Node{}
				for _, k := range []string{"clip", "speed", "loop", "pos"} {
					if !a.Has(k) {
						continue
					}
					v, err := s.stateValue(k, a.Str(k))
					if err != nil {
						return nil, err
					}
					body.Attrs = append(body.Attrs, aetext.Field{Name: k, Value: v})
				}
				return []mutate.Op{mutate.CtrlPut("state", a.Str("name"), body)}, nil
			})},
		{Name: "ctrl.set-state", Doc: "set clip, speed, loop, events or pos of a state", Write: true, Args: []Arg{file, name, attr, value},
			Run: tx(func(s *Session, a Args) ([]mutate.Op, error) {
				if !a.Has("value") {
					return []mutate.Op{mutate.CtrlAttrClear("state", a.Str("name"), a.Str("attr"))}, nil
				}
				v, err := s.stateValue(a.Str("attr"), a.Str("value"))
				if err != nil {
					return nil, err
				}
				return []mutate.Op{mutate.CtrlAttr("state", a.Str("name"), a.Str("attr"), v)}, nil
			})},
		{Name: "ctrl.remove-state", Doc: "remove a state and the transitions into and out of it", Write: true, Args: []Arg{file, name},
			Run: tx(func(s *Session, a Args) ([]mutate.Op, error) {
				d, err := s.ctrlDoc(a)
				if err != nil {
					return nil, err
				}
				g, err := d.Graph()
				if err != nil {
					return nil, err
				}
				st := g.StateIndex(a.Str("name"))
				switch {
				case st < 0:
					return nil, errors.New("no state " + a.Str("name"))
				case st == g.Entry:
					return nil, errors.New("this is the entry state; set another entry first")
				}
				var ops []mutate.Op
				for i := len(g.Transitions) - 1; i >= 0; i-- {
					if t := g.Transitions[i]; t.From == st || t.To == st {
						ops = append(ops, mutate.CtrlRemove("transition", "").At(i))
					}
				}
				return append(ops, mutate.CtrlRemove("state", a.Str("name"))), nil
			})},
		{Name: "ctrl.add-param", Doc: "add a parameter", Write: true,
			Args: []Arg{file, param, {Name: "kind", Type: "string", Doc: "bool, int, float or trigger", Required: true, Pos: true},
				{Name: "default", Type: "string", Doc: "default value, not for triggers"}},
			Run: tx(func(s *Session, a Args) ([]mutate.Op, error) {
				body := &aetext.Node{Attrs: []aetext.Field{{Name: "kind", Value: aetext.Ident(a.Str("kind"))}}}
				v, ok, err := optValue(a, "default")
				if err != nil {
					return nil, err
				}
				if ok {
					body.Attrs = append(body.Attrs, aetext.Field{Name: "default", Value: v})
				}
				return []mutate.Op{mutate.CtrlPut("param", a.Str("name"), body)}, nil
			})},
		{Name: "ctrl.remove-param", Doc: "remove a parameter; conditions on it become problems", Write: true, Args: []Arg{file, param},
			Run: tx(func(s *Session, a Args) ([]mutate.Op, error) {
				return []mutate.Op{mutate.CtrlRemove("param", a.Str("name"))}, nil
			})},
		{Name: "ctrl.add-transition", Doc: "add a transition; from may be any", Write: true,
			Args: []Arg{file,
				{Name: "from", Type: "string", Doc: "source state or any", Required: true, Pos: true},
				{Name: "to", Type: "string", Doc: "target state", Required: true, Pos: true},
				{Name: "duration", Type: "string", Doc: "seconds of the blend"},
				{Name: "exit", Type: "string", Doc: "exit time in passes of the clip"},
				{Name: "interrupt", Type: "boolean", Doc: "may cut into a running blend"},
				{Name: "cond", Type: "string", Doc: "conditions separated by ;, like Speed>0.1;Grounded"}},
			Run: tx(func(s *Session, a Args) ([]mutate.Op, error) {
				body := &aetext.Node{Attrs: []aetext.Field{{Name: "to", Value: aetext.String(a.Str("to"))}}}
				for _, x := range [][2]string{{"duration", "duration"}, {"exit", "exitTime"}} {
					v, ok, err := optValue(a, x[0])
					if err != nil {
						return nil, err
					}
					if ok {
						body.Attrs = append(body.Attrs, aetext.Field{Name: x[1], Value: v})
					}
				}
				if a.Bool("interrupt") {
					body.Attrs = append(body.Attrs, aetext.Field{Name: "interrupt", Value: aetext.Bool(true)})
				}
				if a.Has("cond") {
					for _, c := range strings.Split(a.Str("cond"), ";") {
						cb, err := condBlock(c)
						if err != nil {
							return nil, err
						}
						body.Blocks = append(body.Blocks, cb)
					}
				}
				return []mutate.Op{mutate.CtrlPut("transition", a.Str("from"), body)}, nil
			})},
		{Name: "ctrl.set-transition", Doc: "set to, duration, exitTime or interrupt of a transition", Write: true, Args: []Arg{file, index, attr, value},
			Run: tx(func(s *Session, a Args) ([]mutate.Op, error) {
				i, err := number(a, "index")
				if err != nil {
					return nil, err
				}
				if !a.Has("value") {
					return []mutate.Op{mutate.CtrlAttrClear("transition", "", a.Str("attr")).At(i)}, nil
				}
				v, err := mutate.ParseValue(a.Str("value"))
				if a.Str("attr") == "to" {
					v, err = aetext.String(a.Str("value")), nil
				}
				if err != nil {
					return nil, err
				}
				return []mutate.Op{mutate.CtrlAttr("transition", "", a.Str("attr"), v).At(i)}, nil
			})},
		{Name: "ctrl.remove-transition", Doc: "remove a transition", Write: true, Args: []Arg{file, index},
			Run: tx(func(s *Session, a Args) ([]mutate.Op, error) {
				i, err := number(a, "index")
				if err != nil {
					return nil, err
				}
				return []mutate.Op{mutate.CtrlRemove("transition", "").At(i)}, nil
			})},
		{Name: "ctrl.entry", Doc: "set the state the animator starts in", Write: true, Args: []Arg{file, name},
			Run: tx(func(s *Session, a Args) ([]mutate.Op, error) {
				return []mutate.Op{mutate.CtrlRoot("entry", aetext.String(a.Str("name")))}, nil
			})},
	}
}

func stateName(g *anim.Graph, i int32) string {
	if i == anim.AnyState {
		return anim.AnyName
	}
	if i >= 0 && int(i) < len(g.States) {
		return g.States[i].Name
	}
	return ""
}

func condText(g *anim.Graph, c anim.Cond) string {
	name := ""
	if c.Param >= 0 && int(c.Param) < len(g.Params) {
		name = g.Params[c.Param].Name
	}
	if c.Op == anim.OpSet {
		return name
	}
	return name + " " + c.Op.String() + " " + strconv.FormatFloat(c.Value, 'g', -1, 64)
}

func (s *Session) ctrlGet(a Args) (*Output, error) {
	d, err := s.ctrlDoc(a)
	if err != nil {
		return nil, err
	}
	g, err := d.Graph()
	if err != nil {
		return nil, err
	}
	var params, states, trans []map[string]any
	for _, p := range g.Params {
		m := map[string]any{"name": p.Name, "kind": p.Kind.String()}
		if p.Kind != anim.ParamTrigger {
			m["default"] = p.Default
		}
		params = append(params, m)
	}
	for i, st := range g.States {
		m := map[string]any{"name": st.Name, "speed": st.Speed, "entry": int32(i) == g.Entry}
		if !st.Clip.IsZero() {
			m["clip"] = s.label(st.Clip)
		}
		if st.HasLoop {
			m["loop"] = st.Loop.String()
		}
		if st.Events != anim.EventsDominant {
			m["events"] = st.Events.String()
		}
		if st.HasPos {
			m["pos"] = st.Pos
		}
		states = append(states, m)
	}
	for i, t := range g.Transitions {
		m := map[string]any{"index": i + 1, "from": stateName(g, t.From), "to": stateName(g, t.To), "duration": t.Duration, "interrupt": t.Interrupt}
		if t.HasExit {
			m["exitTime"] = t.ExitTime
		}
		var conds []string
		for _, c := range t.Conds {
			conds = append(conds, condText(g, c))
		}
		m["conds"] = conds
		trans = append(trans, m)
	}
	out := &Output{Value: map[string]any{"params": params, "states": states, "transitions": trans}}
	out.Problems = d.Problems()
	return out, nil
}
