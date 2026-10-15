package world

import (
	"slices"
	"time"

	"aego/core/mathx"
)

type solverBody struct {
	c            Vec2
	v            Vec2
	a            float32
	w            float32
	c0           Vec2
	a0           float32
	dq           Rot
	invMass      float32
	invI         float32
	force        Vec2
	torque       float32
	gravityScale float32
	linDamp      float32
	angDamp      float32
	dynamic      bool
}

type softness struct {
	biasRate     float32
	massScale    float32
	impulseScale float32
}

func makeSoft(hertz, zeta, h float32) softness {
	if hertz == 0 {
		return softness{massScale: 1}
	}
	omega := 2 * mathx.Pi * hertz
	a1 := 2*zeta + h*omega
	a2 := h * omega * a1
	a3 := 1 / (1 + a2)
	return softness{biasRate: omega / a1, massScale: a2 * a3, impulseScale: a3}
}

func (w *World) solve(h float32) {
	w.mark++
	slices.Sort(w.awake)
	items := w.bodies.items
	for k := 0; k < len(w.awake); k++ {
		i := w.awake[k]
		b := &items[i]
		if b.dead || !b.awake || b.mark == w.mark {
			continue
		}
		w.buildIsland(i, int32(w.stats.Islands))
		w.solveIsland(h)
		w.stats.Islands++
	}
}

func (w *World) buildIsland(seed, id int32) {
	items := w.bodies.items
	w.isl, w.islC, w.islJ = w.isl[:0], w.islC[:0], w.islJ[:0]
	st := append(w.stack[:0], seed)
	items[seed].mark = w.mark
	for len(st) > 0 {
		bi := st[len(st)-1]
		st = st[:len(st)-1]
		b := &items[bi]
		b.solver = int32(len(w.isl))
		w.isl = append(w.isl, bi)
		if b.typ == Static {
			continue
		}
		b.awake = true
		b.island = id
		w.listAwake(b)
		for _, ci := range b.contacts {
			c := &w.contacts[ci]
			if c.mark == w.mark || c.trigger || c.m.Count == 0 {
				continue
			}
			c.mark = w.mark
			w.islC = append(w.islC, ci)
			o := int32(c.bodyA.Index)
			if o == bi {
				o = int32(c.bodyB.Index)
			}
			if items[o].mark != w.mark {
				items[o].mark = w.mark
				st = append(st, o)
			}
		}
		for _, jh := range b.joints {
			j := &w.joints.items[jh.Index]
			if j.mark == w.mark {
				continue
			}
			j.mark = w.mark
			w.islJ = append(w.islJ, int32(jh.Index))
			o := int32(j.a.Index)
			if o == bi {
				o = int32(j.b.Index)
			}
			if items[o].mark != w.mark {
				items[o].mark = w.mark
				st = append(st, o)
			}
		}
	}
	w.stack = st[:0]
	for _, bi := range w.isl {
		if items[bi].typ == Static {
			items[bi].mark = 0
		}
	}
}

func (w *World) solveIsland(h float32) {
	cfg := &w.cfg
	items := w.bodies.items
	start := time.Now()
	sb := w.sb[:0]
	for _, bi := range w.isl {
		b := &items[bi]
		s := solverBody{c: b.center, a: b.angle, v: b.v, w: b.w, c0: b.center, a0: b.angle, dq: Rot{C: 1}}
		if b.typ == Dynamic {
			s.invMass, s.invI, s.dynamic = b.invMass, b.invI, true
			s.force, s.torque = b.force, b.torque
			s.gravityScale, s.linDamp, s.angDamp = b.gravityScale, b.linDamp, b.angDamp
		}
		sb = append(sb, s)
	}
	w.sb = sb
	n := cfg.Substeps
	hs := h / float32(n)
	invHs := 1 / hs
	hertz := min(cfg.ContactHertz, 0.25*float32(n)/h)
	w.soft = makeSoft(hertz, cfg.ContactDampingRatio, hs)
	w.staticSoft = makeSoft(2*hertz, cfg.ContactDampingRatio, hs)
	w.prepareContacts()
	w.prepareJoints(hs)
	maxV := cfg.MaxTranslationPerStep / h
	maxW := cfg.MaxRotationPerStep / h
	for i := 0; i < n; i++ {
		w.integrateVelocities(hs)
		w.warmStart()
		w.warmStartJoints()
		for it := 0; it < cfg.VelocityIterations; it++ {
			w.solveJointVelocities(hs)
			w.solveContacts(invHs, true)
		}
		w.integratePositions(hs, maxV, maxW)
		for it := 0; it < cfg.RelaxIterations; it++ {
			w.solveJointVelocities(hs)
			w.solveContacts(invHs, false)
		}
		if len(w.islJ) > 0 {
			w.solveJointPositions()
			w.refreshRotations()
		}
		w.accumulateImpulses()
	}
	w.applyRestitution()
	w.storeImpulses()
	mid := time.Now()
	w.stats.Time.Solve += mid.Sub(start)
	for i, bi := range w.isl {
		b := &items[bi]
		if b.typ == Static {
			continue
		}
		s := &w.sb[i]
		b.center, b.angle, b.v, b.w = s.c, s.a, s.v, s.w
		b.xf.q = makeRot(s.a)
		b.xf.p = s.c.Sub(b.xf.q.apply(b.localCenter))
	}
	w.sleepIsland(h)
	w.stats.Time.Integrate += time.Since(mid)
}

func (w *World) integrateVelocities(h float32) {
	g := w.cfg.Gravity
	for i := range w.sb {
		s := &w.sb[i]
		if !s.dynamic {
			continue
		}
		s.v = s.v.Add(g.Scale(s.gravityScale).Add(s.force.Scale(s.invMass)).Scale(h))
		s.w += h * s.invI * s.torque
		s.v = s.v.Scale(1 / (1 + h*s.linDamp))
		s.w *= 1 / (1 + h*s.angDamp)
	}
}

func (w *World) integratePositions(h, maxV, maxW float32) {
	for i := range w.sb {
		s := &w.sb[i]
		if l := s.v.LenSq(); l > maxV*maxV {
			s.v = s.v.Scale(maxV / sqrtf(l))
		}
		if s.w*s.w > maxW*maxW {
			s.w = mathx.Sign(s.w) * maxW
		}
		s.c = s.c.Add(s.v.Scale(h))
		s.a += h * s.w
		s.dq = makeRot(s.a - s.a0)
	}
}

func (w *World) refreshRotations() {
	for i := range w.sb {
		s := &w.sb[i]
		s.dq = makeRot(s.a - s.a0)
	}
}

func (w *World) sleepIsland(h float32) {
	cfg := &w.cfg
	items := w.bodies.items
	minSleep := float32(3.4e38)
	lin2 := cfg.SleepLinear * cfg.SleepLinear
	ang2 := cfg.SleepAngular * cfg.SleepAngular
	for _, bi := range w.isl {
		b := &items[bi]
		if b.typ == Static {
			continue
		}
		if !b.allowSleep || b.w*b.w > ang2 || b.v.LenSq() > lin2 {
			b.sleepTime = 0
			minSleep = 0
			continue
		}
		b.sleepTime += h
		minSleep = min(minSleep, b.sleepTime)
	}
	if minSleep < cfg.SleepTime {
		return
	}
	for _, bi := range w.isl {
		b := &items[bi]
		if b.typ == Static {
			continue
		}
		b.awake, b.sleepTime = false, 0
		b.v, b.w, b.force, b.torque = Vec2{}, 0, Vec2{}, 0
	}
}
