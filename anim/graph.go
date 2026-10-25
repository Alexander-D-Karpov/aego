package anim

import (
	"aego/core/curve"
	"aego/core/ids"
)

type ParamKind uint8

const (
	ParamBool ParamKind = iota
	ParamInt
	ParamFloat
	ParamTrigger
)

type Op uint8

const (
	OpSet Op = iota
	OpEq
	OpNe
	OpGt
	OpGe
	OpLt
	OpLe
)

type EventPolicy uint8

const (
	EventsDominant EventPolicy = iota
	EventsBoth
	EventsIncoming
)

const AnyState int32 = -1

type Param struct {
	Name    string
	Kind    ParamKind
	Default float64
}

type Cond struct {
	Param int32
	Op    Op
	Value float64
}

type State struct {
	Name    string
	Clip    ids.AssetID
	Speed   float32
	Loop    LoopMode
	HasLoop bool
	Events  EventPolicy
	Length  float32
	Pos     [2]float32
	HasPos  bool
}

type Transition struct {
	From      int32
	To        int32
	Duration  float32
	ExitTime  float32
	HasExit   bool
	Interrupt bool
	Conds     []Cond
}

type Graph struct {
	Params      []Param
	States      []State
	Transitions []Transition
	Entry       int32
	Ease        curve.Easing
	// EntryPos and AnyPos place the Entry and Any nodes in the editor; the runtime ignores them.
	EntryPos    [2]float32
	AnyPos      [2]float32
	HasEntryPos bool
	HasAnyPos   bool
}

func (g *Graph) ParamIndex(name string) int32 {
	for i := range g.Params {
		if g.Params[i].Name == name {
			return int32(i)
		}
	}
	return -1
}

func (g *Graph) StateIndex(name string) int32 {
	for i := range g.States {
		if g.States[i].Name == name {
			return int32(i)
		}
	}
	return -1
}

func (g *Graph) Defaults(dst []float64) []float64 {
	dst = dst[:0]
	for _, p := range g.Params {
		v := p.Default
		if p.Kind == ParamTrigger {
			v = 0
		}
		dst = append(dst, v)
	}
	return dst
}

func (g *Graph) Consume(params []float64) {
	for i := range g.Params {
		if g.Params[i].Kind == ParamTrigger && i < len(params) {
			params[i] = 0
		}
	}
}

func (g *Graph) consume(t *Transition, params []float64) {
	for _, c := range t.Conds {
		if c.Param >= 0 && int(c.Param) < len(g.Params) && int(c.Param) < len(params) && g.Params[c.Param].Kind == ParamTrigger {
			params[c.Param] = 0
		}
	}
}

func (g *Graph) holds(t *Transition, params []float64) bool {
	for _, c := range t.Conds {
		if c.Param < 0 || int(c.Param) >= len(params) {
			return false
		}
		v := params[c.Param]
		var ok bool
		switch c.Op {
		case OpSet:
			ok = v != 0
		case OpEq:
			ok = v == c.Value
		case OpNe:
			ok = v != c.Value
		case OpGt:
			ok = v > c.Value
		case OpGe:
			ok = v >= c.Value
		case OpLt:
			ok = v < c.Value
		case OpLe:
			ok = v <= c.Value
		}
		if !ok {
			return false
		}
	}
	return true
}

type GraphProblemKind uint8

const (
	GraphEmpty GraphProblemKind = iota
	GraphState
	GraphParam
	GraphSelf
	GraphLoop
)

type GraphProblem struct {
	Kind       GraphProblemKind
	Transition int
	Name       string
}

func (g *Graph) condOK(c Cond) bool {
	if c.Param < 0 || int(c.Param) >= len(g.Params) {
		return false
	}
	k := g.Params[c.Param].Kind
	switch c.Op {
	case OpSet:
		return k == ParamBool || k == ParamTrigger
	case OpEq, OpNe:
		return k != ParamTrigger
	case OpGt, OpGe, OpLt, OpLe:
		return k == ParamInt || k == ParamFloat
	}
	return false
}

func (g *Graph) Check(dst []GraphProblem) []GraphProblem {
	n := int32(len(g.States))
	if n == 0 {
		return append(dst, GraphProblem{Kind: GraphEmpty, Transition: -1})
	}
	if g.Entry < 0 || g.Entry >= n {
		dst = append(dst, GraphProblem{Kind: GraphState, Transition: -1})
	}
	for i := range g.Transitions {
		t := &g.Transitions[i]
		if t.To < 0 || t.To >= n || t.From < AnyState || t.From >= n {
			dst = append(dst, GraphProblem{Kind: GraphState, Transition: i})
			continue
		}
		if t.From == t.To && !t.HasExit {
			dst = append(dst, GraphProblem{Kind: GraphSelf, Transition: i})
		}
		for _, c := range t.Conds {
			if !g.condOK(c) {
				dst = append(dst, GraphProblem{Kind: GraphParam, Transition: i})
				break
			}
		}
	}
	if g.instantLoop() {
		dst = append(dst, GraphProblem{Kind: GraphLoop, Transition: -1})
	}
	return dst
}

func instant(t *Transition) bool {
	return t.Duration <= 0 && !t.HasExit && len(t.Conds) == 0
}

func (g *Graph) instantLoop() bool {
	n := len(g.States)
	color := make([]uint8, n)
	var visit func(s int) bool
	visit = func(s int) bool {
		color[s] = 1
		for i := range g.Transitions {
			t := &g.Transitions[i]
			if !instant(t) || t.To < 0 || int(t.To) >= n {
				continue
			}
			if t.From != int32(s) && !(t.From == AnyState && int(t.To) != s) {
				continue
			}
			switch color[t.To] {
			case 1:
				return true
			case 0:
				if visit(int(t.To)) {
					return true
				}
			}
		}
		color[s] = 2
		return false
	}
	for s := range n {
		if color[s] == 0 && visit(s) {
			return true
		}
	}
	return false
}

const (
	LayoutColumn float32 = 220
	LayoutRow    float32 = 110
)

// Layout places states without a position in columns by their distance from the entry, below the placed ones.
func (g *Graph) Layout() {
	n := len(g.States)
	if n == 0 {
		return
	}
	depth := make([]int32, n)
	for i := range depth {
		depth[i] = -1
	}
	start := g.Entry
	if start < 0 || int(start) >= n {
		start = 0
	}
	depth[start] = 0
	queue := []int32{start}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		for i := range g.Transitions {
			t := &g.Transitions[i]
			if t.From != s || t.To < 0 || int(t.To) >= n || depth[t.To] >= 0 {
				continue
			}
			depth[t.To] = depth[s] + 1
			queue = append(queue, t.To)
		}
	}
	var last int32
	var top float32
	placed := false
	for i := range g.States {
		last = max(last, depth[i])
		if g.States[i].HasPos {
			if !placed || g.States[i].Pos[1] > top {
				top = g.States[i].Pos[1]
			}
			placed = true
		}
	}
	base := float32(0)
	if placed {
		base = top + LayoutRow
	}
	rows := make([]int32, last+2)
	for i := range g.States {
		s := &g.States[i]
		if s.HasPos {
			continue
		}
		d := depth[i]
		if d < 0 {
			d = last + 1
		}
		s.Pos = [2]float32{float32(d) * LayoutColumn, base + float32(rows[d])*LayoutRow}
		s.HasPos = true
		rows[d]++
	}
	first := g.States[start].Pos
	if !g.HasEntryPos {
		g.EntryPos, g.HasEntryPos = [2]float32{first[0] - LayoutColumn*0.8, first[1]}, true
	}
	if !g.HasAnyPos {
		g.AnyPos, g.HasAnyPos = [2]float32{g.EntryPos[0], g.EntryPos[1] + LayoutRow}, true
	}
}
