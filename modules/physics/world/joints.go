package world

const (
	angularSlop          = 2 * 3.14159265 / 180
	maxAngularCorrection = 8 * 3.14159265 / 180
	hugeLength           = 1e30
)

type JointKind uint8

const (
	JointDistance JointKind = iota
	JointRevolute
)

type JointDef interface {
	jointKind() JointKind
}

type DistanceJoint struct {
	A                BodyHandle
	B                BodyHandle
	LocalAnchorA     Vec2
	LocalAnchorB     Vec2
	Length           float32
	Min              float32
	Max              float32
	Stiffness        float32
	Damping          float32
	CollideConnected bool
}

type RevoluteJoint struct {
	A                BodyHandle
	B                BodyHandle
	LocalAnchorA     Vec2
	LocalAnchorB     Vec2
	EnableLimit      bool
	Lower            float32
	Upper            float32
	EnableMotor      bool
	Speed            float32
	MaxTorque        float32
	CollideConnected bool
}

func (DistanceJoint) jointKind() JointKind { return JointDistance }
func (RevoluteJoint) jointKind() JointKind { return JointRevolute }

type joint struct {
	kind        JointKind
	a, b        BodyHandle
	localA      Vec2
	localB      Vec2
	collide     bool
	mark        uint32
	length      float32
	minLen      float32
	maxLen      float32
	stiffness   float32
	damping     float32
	impulse     float32
	lowerImp    float32
	upperImp    float32
	refAngle    float32
	enableLimit bool
	enableMotor bool
	lower       float32
	upper       float32
	speed       float32
	maxTorque   float32
	pImpulse    Vec2
	motorImp    float32
	iA, iB      int32
	lcA, lcB    Vec2
	rA, rB      Vec2
	u           Vec2
	mass        float32
	softMass    float32
	gamma       float32
	biasCoef    float32
	axialMass   float32
	k11         float32
	k12         float32
	k22         float32
	mA, mB      float32
	iIA, iIB    float32
}

func (w *World) jointBodies(a, b BodyHandle, la, lb Vec2) (*body, *body, error) {
	ba, err := w.getBody(a)
	if err != nil {
		return nil, nil, err
	}
	bb, err := w.getBody(b)
	if err != nil {
		return nil, nil, err
	}
	if a == b {
		return nil, nil, bad("joint needs two different bodies")
	}
	if !finite2(la) || !finite2(lb) {
		return nil, nil, bad("joint anchors are not finite")
	}
	return ba, bb, nil
}

func (w *World) CreateJoint(def JointDef) (JointHandle, error) {
	var j joint
	var ba, bb *body
	var err error
	switch d := def.(type) {
	case DistanceJoint:
		if ba, bb, err = w.jointBodies(d.A, d.B, d.LocalAnchorA, d.LocalAnchorB); err != nil {
			return JointHandle{}, err
		}
		for _, f := range [...]float32{d.Length, d.Min, d.Max, d.Stiffness, d.Damping} {
			if !finite(f) || f < 0 {
				return JointHandle{}, bad("distance joint values must be non-negative and finite")
			}
		}
		length := d.Length
		if length == 0 {
			length = bb.xf.apply(d.LocalAnchorB).Sub(ba.xf.apply(d.LocalAnchorA)).Len()
		}
		if length < w.cfg.LinearSlop {
			return JointHandle{}, bad("distance joint length %v is below linear slop", length)
		}
		lo, hi := d.Min, d.Max
		if lo == 0 && hi == 0 {
			if d.Stiffness > 0 {
				hi = hugeLength
			} else {
				lo, hi = length, length
			}
		}
		if lo > hi || length < lo || length > hi {
			return JointHandle{}, bad("distance joint needs Min <= Length <= Max")
		}
		j = joint{kind: JointDistance, a: d.A, b: d.B, localA: d.LocalAnchorA, localB: d.LocalAnchorB, collide: d.CollideConnected,
			length: length, minLen: lo, maxLen: hi, stiffness: d.Stiffness, damping: d.Damping}
	case RevoluteJoint:
		if ba, bb, err = w.jointBodies(d.A, d.B, d.LocalAnchorA, d.LocalAnchorB); err != nil {
			return JointHandle{}, err
		}
		for _, f := range [...]float32{d.Lower, d.Upper, d.Speed, d.MaxTorque} {
			if !finite(f) {
				return JointHandle{}, bad("revolute joint values must be finite")
			}
		}
		if d.Lower > d.Upper || d.MaxTorque < 0 {
			return JointHandle{}, bad("revolute joint needs Lower <= Upper and MaxTorque >= 0")
		}
		j = joint{kind: JointRevolute, a: d.A, b: d.B, localA: d.LocalAnchorA, localB: d.LocalAnchorB, collide: d.CollideConnected,
			refAngle: bb.angle - ba.angle, enableLimit: d.EnableLimit, lower: d.Lower, upper: d.Upper,
			enableMotor: d.EnableMotor, speed: d.Speed, maxTorque: d.MaxTorque}
	default:
		return JointHandle{}, bad("unknown joint definition %T", def)
	}
	h, p := w.joints.alloc()
	*p = j
	ba.joints = append(ba.joints, h)
	bb.joints = append(bb.joints, h)
	if !j.collide {
		for k := len(ba.contacts) - 1; k >= 0; k-- {
			ci := ba.contacts[k]
			c := &w.contacts[ci]
			if c.bodyA == j.b || c.bodyB == j.b {
				w.destroyContact(ci, false)
			}
		}
	}
	w.wake(ba)
	w.wake(bb)
	return h, nil
}

func (w *World) DestroyJoint(h JointHandle) error {
	if !w.joints.valid(h) {
		return ErrHandle
	}
	w.destroyJoint(h)
	return nil
}

func removeJoint(list []JointHandle, h JointHandle) []JointHandle {
	for i, x := range list {
		if x == h {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
}

func (w *World) destroyJoint(h JointHandle) {
	j := &w.joints.items[h.Index]
	ba, bb := &w.bodies.items[j.a.Index], &w.bodies.items[j.b.Index]
	ba.joints = removeJoint(ba.joints, h)
	bb.joints = removeJoint(bb.joints, h)
	if !j.collide {
		for _, id := range ba.shapes {
			for _, pid := range w.shapes.items[id.Index].proxies {
				if p := &w.prox[pid]; p.alive && !p.moved {
					p.moved = true
					w.moveBuf = append(w.moveBuf, pid)
				}
			}
		}
	}
	w.wake(ba)
	w.wake(bb)
	w.joints.release(h)
}

func (w *World) JointAnchors(h JointHandle) (Vec2, Vec2, bool) {
	j, ok := w.joints.get(h)
	if !ok {
		return Vec2{}, Vec2{}, false
	}
	ba, bb := &w.bodies.items[j.a.Index], &w.bodies.items[j.b.Index]
	return ba.xf.apply(j.localA), bb.xf.apply(j.localB), true
}

func (w *World) JointKindOf(h JointHandle) (JointKind, bool) {
	j, ok := w.joints.get(h)
	if !ok {
		return 0, false
	}
	return j.kind, true
}

func (w *World) SetMotorSpeed(h JointHandle, speed float32) error {
	j, ok := w.joints.get(h)
	if !ok || j.kind != JointRevolute {
		return ErrHandle
	}
	if !finite(speed) {
		return bad("motor speed is not finite")
	}
	j.speed = speed
	w.wakeHandle(j.a)
	w.wakeHandle(j.b)
	return nil
}

func (w *World) RevoluteAngle(h JointHandle) (float32, bool) {
	j, ok := w.joints.get(h)
	if !ok || j.kind != JointRevolute {
		return 0, false
	}
	return w.bodies.items[j.b.Index].angle - w.bodies.items[j.a.Index].angle - j.refAngle, true
}

func applyJoint(j *joint, sA, sB *solverBody, P Vec2) {
	sA.v = sA.v.Sub(P.Scale(j.mA))
	sA.w -= j.iIA * j.rA.Cross(P)
	sB.v = sB.v.Add(P.Scale(j.mB))
	sB.w += j.iIB * j.rB.Cross(P)
}

func solve22(a11, a12, a22 float32, b Vec2) Vec2 {
	det := a11*a22 - a12*a12
	if det != 0 {
		det = 1 / det
	}
	return Vec2{X: det * (a22*b.X - a12*b.Y), Y: det * (a11*b.Y - a12*b.X)}
}

func (w *World) prepareJoints(h float32) {
	items := w.bodies.items
	for _, ji := range w.islJ {
		j := &w.joints.items[ji]
		ba, bb := &items[j.a.Index], &items[j.b.Index]
		j.iA, j.iB = ba.solver, bb.solver
		sA, sB := &w.sb[j.iA], &w.sb[j.iB]
		j.mA, j.mB, j.iIA, j.iIB = sA.invMass, sB.invMass, sA.invI, sB.invI
		j.lcA, j.lcB = ba.localCenter, bb.localCenter
		j.rA = makeRot(sA.a).apply(j.localA.Sub(j.lcA))
		j.rB = makeRot(sB.a).apply(j.localB.Sub(j.lcB))
		if j.kind == JointDistance {
			w.prepDistance(j, sA, sB, h)
		} else {
			prepRevolute(j)
		}
	}
}

func (w *World) prepDistance(j *joint, sA, sB *solverBody, h float32) {
	u := sB.c.Add(j.rB).Sub(sA.c).Sub(j.rA)
	if l := u.Len(); l > w.cfg.LinearSlop {
		j.u = u.Scale(1 / l)
	} else {
		j.u = Vec2{}
		j.impulse, j.lowerImp, j.upperImp = 0, 0, 0
	}
	crA, crB := j.rA.Cross(j.u), j.rB.Cross(j.u)
	inv := j.mA + j.iIA*crA*crA + j.mB + j.iIB*crB*crB
	j.mass = 0
	if inv != 0 {
		j.mass = 1 / inv
	}
	j.gamma, j.biasCoef, j.softMass = 0, 0, j.mass
	if j.stiffness > 0 && j.minLen < j.maxLen {
		if g := h * (j.damping + h*j.stiffness); g != 0 {
			j.gamma = 1 / g
		}
		j.biasCoef = h * j.stiffness * j.gamma
		inv += j.gamma
		j.softMass = 0
		if inv != 0 {
			j.softMass = 1 / inv
		}
	}
}

func prepRevolute(j *joint) {
	rA, rB := j.rA, j.rB
	j.k11 = j.mA + j.mB + rA.Y*rA.Y*j.iIA + rB.Y*rB.Y*j.iIB
	j.k12 = -rA.Y*rA.X*j.iIA - rB.Y*rB.X*j.iIB
	j.k22 = j.mA + j.mB + rA.X*rA.X*j.iIA + rB.X*rB.X*j.iIB
	fixed := j.iIA+j.iIB == 0
	j.axialMass = 0
	if !fixed {
		j.axialMass = 1 / (j.iIA + j.iIB)
	}
	if !j.enableLimit || fixed {
		j.lowerImp, j.upperImp = 0, 0
	}
	if !j.enableMotor || fixed {
		j.motorImp = 0
	}
}

func (w *World) warmStartJoints() {
	for _, ji := range w.islJ {
		j := &w.joints.items[ji]
		sA, sB := &w.sb[j.iA], &w.sb[j.iB]
		if j.kind == JointDistance {
			applyJoint(j, sA, sB, j.u.Scale(j.impulse+j.lowerImp-j.upperImp))
			continue
		}
		axial := j.motorImp + j.lowerImp - j.upperImp
		P := j.pImpulse
		sA.v = sA.v.Sub(P.Scale(j.mA))
		sA.w -= j.iIA * (j.rA.Cross(P) + axial)
		sB.v = sB.v.Add(P.Scale(j.mB))
		sB.w += j.iIB * (j.rB.Cross(P) + axial)
	}
}

func jointLength(j *joint, sA, sB *solverBody) float32 {
	return sB.c.Add(sB.dq.apply(j.rB)).Sub(sA.c).Sub(sA.dq.apply(j.rA)).Len()
}

func (w *World) solveJointVelocities(h float32) {
	invH := 1 / h
	for _, ji := range w.islJ {
		j := &w.joints.items[ji]
		sA, sB := &w.sb[j.iA], &w.sb[j.iB]
		if j.kind == JointDistance {
			solveDistance(j, sA, sB, invH)
		} else {
			solveRevolute(j, sA, sB, h, invH)
		}
	}
}

func solveDistance(j *joint, sA, sB *solverBody, invH float32) {
	rel := func() float32 {
		return j.u.Dot(pointVel(sB, j.rB).Sub(pointVel(sA, j.rA)))
	}
	if j.minLen >= j.maxLen {
		imp := -j.mass * rel()
		j.impulse += imp
		applyJoint(j, sA, sB, j.u.Scale(imp))
		return
	}
	l := jointLength(j, sA, sB)
	if j.stiffness > 0 {
		imp := -j.softMass * (rel() + (l-j.length)*j.biasCoef + j.gamma*j.impulse)
		j.impulse += imp
		applyJoint(j, sA, sB, j.u.Scale(imp))
	}
	{
		imp := -j.mass * (rel() + max(0, l-j.minLen)*invH)
		old := j.lowerImp
		j.lowerImp = max(0, old+imp)
		applyJoint(j, sA, sB, j.u.Scale(j.lowerImp-old))
	}
	{
		imp := -j.mass * (-rel() + max(0, j.maxLen-l)*invH)
		old := j.upperImp
		j.upperImp = max(0, old+imp)
		applyJoint(j, sA, sB, j.u.Scale(-(j.upperImp - old)))
	}
}

func solveRevolute(j *joint, sA, sB *solverBody, h, invH float32) {
	fixed := j.iIA+j.iIB == 0
	if j.enableMotor && !fixed {
		imp := -j.axialMass * (sB.w - sA.w - j.speed)
		old := j.motorImp
		lim := h * j.maxTorque
		j.motorImp = clampf(old+imp, -lim, lim)
		imp = j.motorImp - old
		sA.w -= j.iIA * imp
		sB.w += j.iIB * imp
	}
	if j.enableLimit && !fixed {
		angle := sB.a - sA.a - j.refAngle
		{
			imp := -j.axialMass * (sB.w - sA.w + max(angle-j.lower, 0)*invH)
			old := j.lowerImp
			j.lowerImp = max(old+imp, 0)
			imp = j.lowerImp - old
			sA.w -= j.iIA * imp
			sB.w += j.iIB * imp
		}
		{
			imp := -j.axialMass * (sA.w - sB.w + max(j.upper-angle, 0)*invH)
			old := j.upperImp
			j.upperImp = max(old+imp, 0)
			imp = j.upperImp - old
			sA.w += j.iIA * imp
			sB.w -= j.iIB * imp
		}
	}
	cdot := pointVel(sB, j.rB).Sub(pointVel(sA, j.rA))
	imp := solve22(j.k11, j.k12, j.k22, cdot.Neg())
	j.pImpulse = j.pImpulse.Add(imp)
	applyJoint(j, sA, sB, imp)
}

func (w *World) solveJointPositions() bool {
	ok := true
	for _, ji := range w.islJ {
		j := &w.joints.items[ji]
		sA, sB := &w.sb[j.iA], &w.sb[j.iB]
		var done bool
		if j.kind == JointDistance {
			done = w.posDistance(j, sA, sB)
		} else {
			done = w.posRevolute(j, sA, sB)
		}
		ok = ok && done
	}
	return ok
}

func (w *World) posDistance(j *joint, sA, sB *solverBody) bool {
	rA := makeRot(sA.a).apply(j.localA.Sub(j.lcA))
	rB := makeRot(sB.a).apply(j.localB.Sub(j.lcB))
	u, length := normalizeLen(sB.c.Add(rB).Sub(sA.c).Sub(rA))
	var C float32
	switch {
	case j.minLen == j.maxLen:
		C = length - j.minLen
	case length < j.minLen:
		C = length - j.minLen
	case j.maxLen < length:
		C = length - j.maxLen
	default:
		return true
	}
	P := u.Scale(-j.mass * C)
	sA.c = sA.c.Sub(P.Scale(j.mA))
	sA.a -= j.iIA * rA.Cross(P)
	sB.c = sB.c.Add(P.Scale(j.mB))
	sB.a += j.iIB * rB.Cross(P)
	return abs32(C) < w.cfg.LinearSlop
}

func (w *World) posRevolute(j *joint, sA, sB *solverBody) bool {
	var angErr float32
	if j.enableLimit && j.iIA+j.iIB != 0 {
		angle := sB.a - sA.a - j.refAngle
		var C float32
		switch {
		case abs32(j.upper-j.lower) < 2*angularSlop:
			C = clampf(angle-j.lower, -maxAngularCorrection, maxAngularCorrection)
		case angle <= j.lower:
			C = clampf(angle-j.lower+angularSlop, -maxAngularCorrection, 0)
		case angle >= j.upper:
			C = clampf(angle-j.upper-angularSlop, 0, maxAngularCorrection)
		}
		imp := -j.axialMass * C
		sA.a -= j.iIA * imp
		sB.a += j.iIB * imp
		angErr = abs32(C)
	}
	rA := makeRot(sA.a).apply(j.localA.Sub(j.lcA))
	rB := makeRot(sB.a).apply(j.localB.Sub(j.lcB))
	C := sB.c.Add(rB).Sub(sA.c).Sub(rA)
	posErr := C.Len()
	k11 := j.mA + j.mB + j.iIA*rA.Y*rA.Y + j.iIB*rB.Y*rB.Y
	k12 := -j.iIA*rA.X*rA.Y - j.iIB*rB.X*rB.Y
	k22 := j.mA + j.mB + j.iIA*rA.X*rA.X + j.iIB*rB.X*rB.X
	imp := solve22(k11, k12, k22, C).Neg()
	sA.c = sA.c.Sub(imp.Scale(j.mA))
	sA.a -= j.iIA * rA.Cross(imp)
	sB.c = sB.c.Add(imp.Scale(j.mB))
	sB.a += j.iIB * rB.Cross(imp)
	return posErr <= w.cfg.LinearSlop && angErr <= angularSlop
}
