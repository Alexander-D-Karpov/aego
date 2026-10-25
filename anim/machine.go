package anim

import "aego/core/curve"

const Frozen int32 = -1

const maxHops = 8

type ChangeKind uint8

const (
	ChangeEnter ChangeKind = iota
	ChangeExit
	ChangeFreeze
)

type Change struct {
	Kind  ChangeKind
	State int32
}

// Machine is the state of one animator in a Graph; Prev is a state or Frozen, a pose captured when a blend was interrupted.
type Machine struct {
	State    int32
	Time     float64
	Last     float64
	Prev     int32
	PrevTime float64
	PrevLast float64
	BlendT   float32
	BlendDur float32
	Blend    bool
	Started  bool
}

func ease(e curve.Easing, u float32) float32 {
	if e == curve.None {
		return u
	}
	return curve.Ease(e, u)
}

func (m *Machine) Weight(g *Graph) float32 {
	if !m.Blend || m.BlendDur <= 0 {
		return 1
	}
	return ease(g.Ease, min(max(m.BlendT/m.BlendDur, 0), 1))
}

func (m *Machine) WeightAt(g *Graph, blendT float32) float32 {
	if !m.Blend || m.BlendDur <= 0 {
		return 1
	}
	return ease(g.Ease, min(max(blendT/m.BlendDur, 0), 1))
}

func (m *Machine) Progress(g *Graph) float32 {
	if !m.Started || int(m.State) >= len(g.States) {
		return 0
	}
	l := g.States[m.State].Length
	if l <= 0 {
		return 0
	}
	return float32(m.Time / float64(l))
}

func (m *Machine) reached(g *Graph, exit float32) bool {
	l := g.States[m.State].Length
	return l <= 0 || m.Time >= float64(exit)*float64(l)
}

func (m *Machine) pick(g *Graph, params []float64) (int, bool) {
	for pass := range 2 {
		for i := range g.Transitions {
			t := &g.Transitions[i]
			anyState := t.From == AnyState
			switch {
			case (pass == 0) != anyState:
				continue
			case !anyState && t.From != m.State:
				continue
			case anyState && t.To == m.State:
				continue
			case m.Blend && !t.Interrupt:
				continue
			case t.HasExit && !m.reached(g, t.ExitTime):
				continue
			case !g.holds(t, params):
				continue
			}
			return i, true
		}
	}
	return -1, false
}

func (m *Machine) fire(to int32, dur float32, out []Change) []Change {
	interrupt := m.Blend
	if interrupt && m.Prev >= 0 {
		out = append(out, Change{Kind: ChangeExit, State: m.Prev})
	}
	switch {
	case dur <= 0:
		out = append(out, Change{Kind: ChangeExit, State: m.State})
		m.Blend, m.Prev, m.BlendT, m.BlendDur = false, Frozen, 0, 0
	case interrupt:
		out = append(out, Change{Kind: ChangeFreeze, State: m.State}, Change{Kind: ChangeExit, State: m.State})
		m.Prev, m.PrevTime, m.PrevLast = Frozen, 0, 0
		m.Blend, m.BlendT, m.BlendDur = true, 0, dur
	default:
		m.Prev, m.PrevTime, m.PrevLast = m.State, m.Time, m.Last
		m.Blend, m.BlendT, m.BlendDur = true, 0, dur
	}
	m.State, m.Time, m.Last = to, 0, 0
	return append(out, Change{Kind: ChangeEnter, State: to})
}

func (m *Machine) jump(g *Graph, to int32, dur float32, out []Change) []Change {
	if to < 0 || int(to) >= len(g.States) {
		return out
	}
	if !m.Started {
		*m = Machine{State: to, Prev: Frozen, Started: true}
		return append(out, Change{Kind: ChangeEnter, State: to})
	}
	return m.fire(to, dur, out)
}

func (m *Machine) Play(g *Graph, to int32, out []Change) []Change {
	return m.jump(g, to, 0, out)
}

func (m *Machine) CrossFade(g *Graph, to int32, dur float32, out []Change) []Change {
	return m.jump(g, to, max(dur, 0), out)
}

func (m *Machine) endBlend(out []Change) []Change {
	if m.Prev >= 0 {
		out = append(out, Change{Kind: ChangeExit, State: m.Prev})
	}
	m.Blend, m.Prev, m.BlendT, m.BlendDur = false, Frozen, 0, 0
	return out
}

func (m *Machine) Step(g *Graph, dt float32, params []float64, out []Change) []Change {
	if len(g.States) == 0 {
		return out
	}
	if dt != dt || dt < 0 {
		dt = 0
	}
	if !m.Started {
		e := g.Entry
		if e < 0 || int(e) >= len(g.States) {
			e = 0
		}
		out = m.jump(g, e, 0, out)
	}
	m.Last = m.Time
	m.Time += float64(dt) * float64(g.States[m.State].Speed)
	if m.Blend {
		m.PrevLast = m.PrevTime
		if m.Prev >= 0 {
			m.PrevTime += float64(dt) * float64(g.States[m.Prev].Speed)
		}
		m.BlendT += dt
		if m.BlendT >= m.BlendDur {
			out = m.endBlend(out)
		}
	}
	for range maxHops {
		i, ok := m.pick(g, params)
		if !ok {
			break
		}
		t := &g.Transitions[i]
		g.consume(t, params)
		out = m.fire(t.To, t.Duration, out)
		if t.Duration > 0 {
			break
		}
	}
	return out
}
