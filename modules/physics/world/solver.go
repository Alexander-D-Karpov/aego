package world

type contactPoint struct {
	rA             Vec2
	rB             Vec2
	normalImpulse  float32
	tangentImpulse float32
	totalImpulse   float32
	normalMass     float32
	tangentMass    float32
	adjustedSep    float32
	relVel         float32
	maxImpulse     float32
}

type contactConstraint struct {
	points   [2]contactPoint
	normal   Vec2
	iA       int32
	iB       int32
	mA       float32
	mB       float32
	iIA      float32
	iIB      float32
	friction float32
	soft     softness
	count    int
	contact  int32
}

func pointVel(s *solverBody, r Vec2) Vec2 {
	return s.v.Add(crossSV(s.w, r))
}

func (w *World) prepareContacts() {
	items := w.bodies.items
	cc := w.cc[:0]
	for _, ci := range w.islC {
		c := &w.contacts[ci]
		iA, iB := items[c.bodyA.Index].solver, items[c.bodyB.Index].solver
		sA, sB := &w.sb[iA], &w.sb[iB]
		k := contactConstraint{
			normal: c.m.Normal, iA: iA, iB: iB,
			mA: sA.invMass, mB: sB.invMass, iIA: sA.invI, iIB: sB.invI,
			friction: c.friction, count: c.m.Count, contact: ci, soft: w.soft,
		}
		if k.mA == 0 || k.mB == 0 {
			k.soft = w.staticSoft
		}
		n := k.normal
		t := crossVS(n, 1)
		for j := 0; j < k.count; j++ {
			mp := &c.m.Points[j]
			p := &k.points[j]
			p.rA, p.rB = mp.AnchorA, mp.AnchorB
			p.normalImpulse, p.tangentImpulse = mp.NormalImpulse, mp.TangentImpulse
			rnA, rnB := p.rA.Cross(n), p.rB.Cross(n)
			if kn := k.mA + k.mB + k.iIA*rnA*rnA + k.iIB*rnB*rnB; kn > 0 {
				p.normalMass = 1 / kn
			}
			rtA, rtB := p.rA.Cross(t), p.rB.Cross(t)
			if kt := k.mA + k.mB + k.iIA*rtA*rtA + k.iIB*rtB*rtB; kt > 0 {
				p.tangentMass = 1 / kt
			}
			p.adjustedSep = mp.Separation - p.rB.Sub(p.rA).Dot(n)
			p.relVel = n.Dot(pointVel(sB, p.rB).Sub(pointVel(sA, p.rA)))
		}
		cc = append(cc, k)
	}
	w.cc = cc
}

func (w *World) warmStart() {
	for i := range w.cc {
		k := &w.cc[i]
		sA, sB := &w.sb[k.iA], &w.sb[k.iB]
		t := crossVS(k.normal, 1)
		for j := 0; j < k.count; j++ {
			p := &k.points[j]
			P := k.normal.Scale(p.normalImpulse).Add(t.Scale(p.tangentImpulse))
			sA.v = sA.v.Sub(P.Scale(k.mA))
			sA.w -= k.iIA * p.rA.Cross(P)
			sB.v = sB.v.Add(P.Scale(k.mB))
			sB.w += k.iIB * p.rB.Cross(P)
		}
	}
}

func (w *World) solveContacts(invH float32, useBias bool) {
	push := w.cfg.ContactPushVelocity
	for i := range w.cc {
		k := &w.cc[i]
		sA, sB := &w.sb[k.iA], &w.sb[k.iB]
		vA, wA, vB, wB := sA.v, sA.w, sB.v, sB.w
		mA, mB, iA, iB := k.mA, k.mB, k.iIA, k.iIB
		n := k.normal
		dp := sB.c.Sub(sB.c0).Sub(sA.c.Sub(sA.c0))
		for j := 0; j < k.count; j++ {
			p := &k.points[j]
			prA, prB := sA.dq.apply(p.rA), sB.dq.apply(p.rB)
			s := dp.Add(prB.Sub(prA)).Dot(n) + p.adjustedSep
			bias, mScale, iScale := float32(0), float32(1), float32(0)
			switch {
			case s > 0:
				bias = s * invH
			case useBias:
				bias = max(k.soft.biasRate*s, -push)
				mScale, iScale = k.soft.massScale, k.soft.impulseScale
			}
			vn := n.Dot(vB.Add(crossSV(wB, p.rB)).Sub(vA).Sub(crossSV(wA, p.rA)))
			imp := -p.normalMass*mScale*(vn+bias) - iScale*p.normalImpulse
			ni := max(p.normalImpulse+imp, 0)
			imp = ni - p.normalImpulse
			p.normalImpulse = ni
			p.maxImpulse = max(p.maxImpulse, ni)
			P := n.Scale(imp)
			vA = vA.Sub(P.Scale(mA))
			wA -= iA * p.rA.Cross(P)
			vB = vB.Add(P.Scale(mB))
			wB += iB * p.rB.Cross(P)
		}
		t := crossVS(n, 1)
		for j := 0; j < k.count; j++ {
			p := &k.points[j]
			vt := t.Dot(vB.Add(crossSV(wB, p.rB)).Sub(vA).Sub(crossSV(wA, p.rA)))
			maxF := k.friction * p.normalImpulse
			ni := clampf(p.tangentImpulse-p.tangentMass*vt, -maxF, maxF)
			lam := ni - p.tangentImpulse
			p.tangentImpulse = ni
			P := t.Scale(lam)
			vA = vA.Sub(P.Scale(mA))
			wA -= iA * p.rA.Cross(P)
			vB = vB.Add(P.Scale(mB))
			wB += iB * p.rB.Cross(P)
		}
		sA.v, sA.w, sB.v, sB.w = vA, wA, vB, wB
	}
}

func (w *World) accumulateImpulses() {
	for i := range w.cc {
		k := &w.cc[i]
		for j := 0; j < k.count; j++ {
			k.points[j].totalImpulse += k.points[j].normalImpulse
		}
	}
}

func (w *World) applyRestitution() {
	thr := w.cfg.RestitutionThreshold
	for i := range w.cc {
		k := &w.cc[i]
		e := w.contacts[k.contact].restitution
		if e == 0 {
			continue
		}
		sA, sB := &w.sb[k.iA], &w.sb[k.iB]
		n := k.normal
		for j := 0; j < k.count; j++ {
			p := &k.points[j]
			if p.relVel > -thr || p.maxImpulse == 0 {
				continue
			}
			vn := n.Dot(pointVel(sB, p.rB).Sub(pointVel(sA, p.rA)))
			ni := max(p.normalImpulse-p.normalMass*(vn+e*p.relVel), 0)
			lam := ni - p.normalImpulse
			p.normalImpulse = ni
			p.totalImpulse += lam
			P := n.Scale(lam)
			sA.v = sA.v.Sub(P.Scale(k.mA))
			sA.w -= k.iIA * p.rA.Cross(P)
			sB.v = sB.v.Add(P.Scale(k.mB))
			sB.w += k.iIB * p.rB.Cross(P)
		}
	}
}

func (w *World) storeImpulses() {
	for i := range w.cc {
		k := &w.cc[i]
		c := &w.contacts[k.contact]
		for j := 0; j < k.count && j < c.m.Count; j++ {
			mp := &c.m.Points[j]
			mp.NormalImpulse = k.points[j].normalImpulse
			mp.TangentImpulse = k.points[j].tangentImpulse
			mp.TotalNormalImpulse = k.points[j].totalImpulse
		}
	}
}
