package anim

import (
	"math"
	"strconv"

	"aego/core/curve"
	"aego/core/ids"
)

const (
	ctrlMagic         = "AECT"
	CtrlBinaryVersion = 2
	minParam          = 13
	minState          = 35
	minTransition     = 22
	minCond           = 13
)

const (
	transExit uint8 = 1 << iota
	transInterrupt
)

func putF64(b []byte, f float64) []byte {
	return le.AppendUint64(b, math.Float64bits(f))
}

func (r *reader) f64() float64 {
	return math.Float64frombits(r.u64())
}

func boolByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}

func MarshalGraph(g *Graph) []byte {
	b := make([]byte, 0, 64+len(g.States)*minState+len(g.Transitions)*minTransition)
	b = append(b, ctrlMagic...)
	b = le.AppendUint32(b, CtrlBinaryVersion)
	b = append(b, byte(g.Ease))
	b = le.AppendUint32(b, uint32(g.Entry))
	b = le.AppendUint32(b, uint32(len(g.Params)))
	for _, p := range g.Params {
		b = putStr(b, p.Name)
		b = append(b, byte(p.Kind))
		b = putF64(b, p.Default)
	}
	b = le.AppendUint32(b, uint32(len(g.States)))
	for _, s := range g.States {
		b = putStr(b, s.Name)
		b = le.AppendUint64(b, s.Clip.Hi)
		b = le.AppendUint64(b, s.Clip.Lo)
		b = putF32(b, s.Speed)
		b = append(b, byte(s.Loop), boolByte(s.HasLoop), byte(s.Events))
		b = putF32(b, s.Pos[0])
		b = putF32(b, s.Pos[1])
	}
	b = le.AppendUint32(b, uint32(len(g.Transitions)))
	for i := range g.Transitions {
		t := &g.Transitions[i]
		var f uint8
		if t.HasExit {
			f |= transExit
		}
		if t.Interrupt {
			f |= transInterrupt
		}
		b = le.AppendUint32(b, uint32(t.From))
		b = le.AppendUint32(b, uint32(t.To))
		b = putF32(b, t.Duration)
		b = putF32(b, t.ExitTime)
		b = append(b, f)
		b = le.AppendUint32(b, uint32(len(t.Conds)))
		for _, c := range t.Conds {
			b = le.AppendUint32(b, uint32(c.Param))
			b = append(b, byte(c.Op))
			b = putF64(b, c.Value)
		}
	}
	return b
}

func UnmarshalGraph(data []byte) (*Graph, error) {
	if len(data) < 8 || string(data[:4]) != ctrlMagic {
		return nil, badArtifact("not an animation controller artifact")
	}
	r := reader{b: data[4:]}
	if v := r.u32(); v != CtrlBinaryVersion {
		return nil, badArtifact("unsupported animation controller artifact version " + strconv.FormatUint(uint64(v), 10))
	}
	g := &Graph{}
	g.Ease = curve.Easing(r.u8())
	g.Entry = int32(r.u32())
	g.Params = sized[Param](r.count(minParam))
	for i := range g.Params {
		p := &g.Params[i]
		p.Name = r.str()
		p.Kind = ParamKind(r.u8())
		p.Default = r.f64()
	}
	g.States = sized[State](r.count(minState))
	for i := range g.States {
		s := &g.States[i]
		s.Name = r.str()
		hi := r.u64()
		s.Clip = ids.AssetID{Hi: hi, Lo: r.u64()}
		s.Speed = r.f32()
		s.Loop = LoopMode(r.u8())
		s.HasLoop = r.u8() != 0
		s.Events = EventPolicy(r.u8())
		s.Pos = [2]float32{r.f32(), r.f32()}
		s.HasPos = true
	}
	g.Transitions = sized[Transition](r.count(minTransition))
	for i := range g.Transitions {
		t := &g.Transitions[i]
		t.From = int32(r.u32())
		t.To = int32(r.u32())
		t.Duration = r.f32()
		t.ExitTime = r.f32()
		f := r.u8()
		t.HasExit, t.Interrupt = f&transExit != 0, f&transInterrupt != 0
		t.Conds = sized[Cond](r.count(minCond))
		for j := range t.Conds {
			c := &t.Conds[j]
			c.Param = int32(r.u32())
			c.Op = Op(r.u8())
			c.Value = r.f64()
		}
	}
	if r.bad || len(r.b) != 0 {
		return nil, badArtifact("animation controller artifact is truncated or has trailing bytes")
	}
	for i := range g.Params {
		if g.Params[i].Kind > ParamTrigger {
			return nil, badArtifact("animation controller artifact has an unknown parameter kind")
		}
	}
	for i := range g.States {
		if s := &g.States[i]; s.Loop > LoopPingPong || s.Events > EventsIncoming {
			return nil, badArtifact("animation controller artifact has an unknown loop or event policy")
		}
	}
	return g, nil
}
