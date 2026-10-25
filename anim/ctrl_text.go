package anim

import (
	"strconv"

	"aego/core/curve"
	"aego/core/ids"
	"aego/format/aetext"
)

const (
	CtrlHead    = "controller"
	CtrlVersion = 1
	AnyName     = "any"
	unresolved  = int32(-2)
)

var (
	paramNames  = []string{"bool", "int", "float", "trigger"}
	opNames     = []string{"set", "eq", "ne", "gt", "ge", "lt", "le"}
	policyNames = []string{"dominant", "both", "incoming"}
)

func (k ParamKind) String() string   { return nameOf(paramNames, uint8(k)) }
func (o Op) String() string          { return nameOf(opNames, uint8(o)) }
func (p EventPolicy) String() string { return nameOf(policyNames, uint8(p)) }

func DecodeGraph(n *aetext.Node) (*Graph, []GraphProblem, error) {
	if n == nil || n.Head != CtrlHead {
		return nil, nil, textErr(n, "the document is not an animation controller")
	}
	if err := strict(n, ctrlShape); err != nil {
		return nil, nil, err
	}
	if v, ok := n.Attr("version"); ok && v.AsInt() > CtrlVersion {
		return nil, nil, textErr(n, "the controller is written by a newer engine, version "+strconv.FormatInt(v.AsInt(), 10))
	}
	g := &Graph{Ease: curve.InOutQuad}
	for _, at := range [...]struct {
		name string
		pos  *[2]float32
		has  *bool
	}{{"entryPos", &g.EntryPos, &g.HasEntryPos}, {"anyPos", &g.AnyPos, &g.HasAnyPos}} {
		if v, ok := n.Attr(at.name); ok {
			if len(v.List) != 2 {
				return nil, nil, textErr(n, at.name+" needs two numbers")
			}
			*at.pos, *at.has = [2]float32{float32(v.List[0].AsFloat()), float32(v.List[1].AsFloat())}, true
		}
	}
	if v, ok := n.Attr("ease"); ok {
		e, known := curve.ParseEasing(word(v))
		if !known {
			return nil, nil, textErr(n, "unknown ease "+word(v))
		}
		g.Ease = e
	}
	for _, b := range n.Blocks {
		switch b.Head {
		case "param":
			p, err := decodeParam(b)
			if err != nil {
				return nil, nil, err
			}
			if g.ParamIndex(p.Name) >= 0 {
				return nil, nil, textErr(b, "parameter "+p.Name+" is declared twice")
			}
			g.Params = append(g.Params, p)
		case "state":
			s, err := decodeState(b)
			if err != nil {
				return nil, nil, err
			}
			if g.StateIndex(s.Name) >= 0 {
				return nil, nil, textErr(b, "state "+s.Name+" is declared twice")
			}
			g.States = append(g.States, s)
		case "transition":
		default:
			return nil, nil, textErr(b, "unknown block "+b.Head)
		}
	}
	var probs []GraphProblem
	if v, ok := n.Field("entry"); ok {
		if g.Entry = g.StateIndex(word(v)); g.Entry < 0 {
			probs = append(probs, GraphProblem{Kind: GraphState, Transition: -1, Name: word(v)})
		}
	}
	for _, b := range n.Blocks {
		if b.Head != "transition" {
			continue
		}
		t, err := g.decodeTransition(b, len(g.Transitions), &probs)
		if err != nil {
			return nil, nil, err
		}
		g.Transitions = append(g.Transitions, t)
	}
	return g, probs, nil
}

func decodeParam(b *aetext.Node) (Param, error) {
	p := Param{Name: b.Label}
	if err := strict(b, paramShape); err != nil {
		return p, err
	}
	if p.Name == "" {
		return p, textErr(b, "a parameter needs a name")
	}
	if _, ok := b.Attr("kind"); !ok {
		return p, textErr(b, "parameter "+p.Name+" needs kind")
	}
	k, err := attrName(b, "kind", paramNames)
	if err != nil {
		return p, err
	}
	p.Kind = ParamKind(k)
	v, ok := b.Attr("default")
	if !ok {
		return p, nil
	}
	switch {
	case p.Kind == ParamTrigger:
		return p, textErr(b, "trigger "+p.Name+" has no default")
	case p.Kind == ParamBool && v.Kind == aetext.VBool:
		if v.Bool {
			p.Default = 1
		}
	case p.Kind == ParamInt && v.Kind == aetext.VInt:
		p.Default = float64(v.Int)
	case p.Kind == ParamFloat && (v.Kind == aetext.VInt || v.Kind == aetext.VFloat):
		p.Default = v.AsFloat()
	default:
		return p, textErr(b, "the default of "+p.Name+" does not match its kind "+p.Kind.String())
	}
	return p, nil
}

func decodeState(b *aetext.Node) (State, error) {
	s := State{Name: b.Label, Speed: 1}
	if err := strict(b, stateShape); err != nil {
		return s, err
	}
	if s.Name == "" || s.Name == AnyName {
		return s, textErr(b, "a state needs a name other than "+AnyName)
	}
	if v, ok := b.Attr("clip"); ok {
		if v.Kind != aetext.VAsset {
			return s, textErr(b, "the clip of "+s.Name+" is not an asset reference")
		}
		id, err := ids.ParseAssetID(v.Str)
		if err != nil {
			return s, textErr(b, "the clip of "+s.Name+" is not an asset id")
		}
		s.Clip = id
	}
	if v, ok := b.Attr("speed"); ok {
		s.Speed = float32(v.AsFloat())
	}
	if v, ok := b.Attr("loop"); ok {
		l, known := ParseLoop(word(v))
		if !known {
			return s, textErr(b, "unknown loop "+word(v))
		}
		s.Loop, s.HasLoop = l, true
	}
	if v, ok := b.Attr("pos"); ok {
		p, ok := readLanes(v, 2)
		if !ok {
			return s, textErr(b, "the pos of "+s.Name+" is [x, y]")
		}
		s.Pos, s.HasPos = [2]float32{p[0], p[1]}, true
	}
	e, err := attrName(b, "events", policyNames)
	if err != nil {
		return s, err
	}
	s.Events = EventPolicy(e)
	return s, nil
}

func (g *Graph) decodeTransition(b *aetext.Node, i int, probs *[]GraphProblem) (Transition, error) {
	t := Transition{From: AnyState}
	if err := strict(b, transShape); err != nil {
		return t, err
	}
	if b.Label != AnyName {
		if t.From = g.StateIndex(b.Label); t.From < 0 {
			t.From = unresolved
			*probs = append(*probs, GraphProblem{Kind: GraphState, Transition: i, Name: b.Label})
		}
	}
	tv, ok := b.Attr("to")
	if !ok {
		return t, textErr(b, "the transition needs to")
	}
	if t.To = g.StateIndex(word(tv)); t.To < 0 {
		*probs = append(*probs, GraphProblem{Kind: GraphState, Transition: i, Name: word(tv)})
	}
	if v, ok := b.Attr("duration"); ok {
		t.Duration = float32(v.AsFloat())
	}
	if v, ok := b.Attr("exitTime"); ok {
		t.ExitTime, t.HasExit = float32(v.AsFloat()), true
	}
	if v, ok := b.Attr("interrupt"); ok {
		t.Interrupt = v.Bool
	}
	for _, cb := range b.Blocks {
		if cb.Head != "cond" {
			return t, textErr(cb, "a transition holds only cond blocks")
		}
		if err := strict(cb, condShape); err != nil {
			return t, err
		}
		c := Cond{Param: g.ParamIndex(cb.Label)}
		if c.Param < 0 {
			*probs = append(*probs, GraphProblem{Kind: GraphParam, Transition: i, Name: cb.Label})
		}
		if v, ok := cb.Attr("op"); ok {
			o, known := indexOf(opNames, word(v))
			if !known {
				return t, textErr(cb, "unknown op "+word(v))
			}
			c.Op = Op(o)
		}
		v, ok := cb.Attr("value")
		switch {
		case !ok && c.Op != OpSet:
			return t, textErr(cb, "the condition on "+cb.Label+" needs value")
		case !ok:
		case v.Kind == aetext.VBool:
			if v.Bool {
				c.Value = 1
			}
		case v.Kind == aetext.VInt || v.Kind == aetext.VFloat:
			c.Value = v.AsFloat()
		default:
			return t, textErr(cb, "the value of a condition is a number or true/false")
		}
		t.Conds = append(t.Conds, c)
	}
	return t, nil
}

func (g *Graph) stateName(i int32) string {
	if i == AnyState {
		return AnyName
	}
	if i >= 0 && int(i) < len(g.States) {
		return g.States[i].Name
	}
	return ""
}

func EncodeGraph(g *Graph) *aetext.Node {
	n := &aetext.Node{Head: CtrlHead, Attrs: []aetext.Field{field("version", aetext.Int(CtrlVersion))}}
	if g.Ease != curve.InOutQuad {
		n.Attrs = append(n.Attrs, field("ease", aetext.Ident(g.Ease.String())))
	}
	if g.HasEntryPos {
		n.Attrs = append(n.Attrs, field("entryPos", aetext.List(aetext.Float32(g.EntryPos[0]), aetext.Float32(g.EntryPos[1]))))
	}
	if g.HasAnyPos {
		n.Attrs = append(n.Attrs, field("anyPos", aetext.List(aetext.Float32(g.AnyPos[0]), aetext.Float32(g.AnyPos[1]))))
	}
	if g.Entry >= 0 && int(g.Entry) < len(g.States) {
		n.Fields = append(n.Fields, field("entry", aetext.String(g.States[g.Entry].Name)))
	}
	for _, p := range g.Params {
		b := &aetext.Node{Head: "param", Label: p.Name, Attrs: []aetext.Field{field("kind", aetext.Ident(p.Kind.String()))}}
		if p.Default != 0 && p.Kind != ParamTrigger {
			var v aetext.Value
			switch p.Kind {
			case ParamBool:
				v = aetext.Bool(true)
			case ParamInt:
				v = aetext.Int(int64(p.Default))
			default:
				v = aetext.Float64(p.Default)
			}
			b.Attrs = append(b.Attrs, field("default", v))
		}
		n.Blocks = append(n.Blocks, b)
	}
	for _, s := range g.States {
		b := &aetext.Node{Head: "state", Label: s.Name}
		if !s.Clip.IsZero() {
			b.Attrs = append(b.Attrs, field("clip", aetext.AssetRef(s.Clip.String())))
		}
		if s.Speed != 1 {
			b.Attrs = append(b.Attrs, field("speed", aetext.Float32(s.Speed)))
		}
		if s.HasLoop {
			b.Attrs = append(b.Attrs, field("loop", aetext.Ident(s.Loop.String())))
		}
		if s.Events != EventsDominant {
			b.Attrs = append(b.Attrs, field("events", aetext.Ident(s.Events.String())))
		}
		if s.HasPos {
			b.Attrs = append(b.Attrs, field("pos", aetext.List(aetext.Float32(s.Pos[0]), aetext.Float32(s.Pos[1]))))
		}
		n.Blocks = append(n.Blocks, b)
	}
	for i := range g.Transitions {
		t := &g.Transitions[i]
		b := &aetext.Node{Head: "transition", Label: g.stateName(t.From), Attrs: []aetext.Field{field("to", aetext.String(g.stateName(t.To)))}}
		if t.Duration > 0 {
			b.Attrs = append(b.Attrs, field("duration", aetext.Float32(t.Duration)))
		}
		if t.HasExit {
			b.Attrs = append(b.Attrs, field("exitTime", aetext.Float32(t.ExitTime)))
		}
		if t.Interrupt {
			b.Attrs = append(b.Attrs, field("interrupt", aetext.Bool(true)))
		}
		for _, c := range t.Conds {
			cb := &aetext.Node{Head: "cond"}
			kind := ParamFloat
			if c.Param >= 0 && int(c.Param) < len(g.Params) {
				cb.Label, kind = g.Params[c.Param].Name, g.Params[c.Param].Kind
			}
			if c.Op != OpSet {
				var v aetext.Value
				switch kind {
				case ParamBool:
					v = aetext.Bool(c.Value != 0)
				case ParamInt:
					v = aetext.Int(int64(c.Value))
				default:
					v = aetext.Float64(c.Value)
				}
				cb.Attrs = append(cb.Attrs, field("op", aetext.Ident(c.Op.String())), field("value", v))
			}
			b.Blocks = append(b.Blocks, cb)
		}
		n.Blocks = append(n.Blocks, b)
	}
	return n
}
