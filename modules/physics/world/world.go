package world

import (
	"aego/core/mathx"
)

type body struct {
	typ          BodyType
	xf           xform
	angle        float32
	localCenter  Vec2
	center       Vec2
	v            Vec2
	w            float32
	force        Vec2
	torque       float32
	mass         float32
	invMass      float32
	inertia      float32
	invI         float32
	massMode     MassMode
	massOverride float32
	gravityScale float32
	linDamp      float32
	angDamp      float32
	fixedRot     bool
	bullet       bool
	allowSleep   bool
	awake        bool
	listed       bool
	sleepTime    float32
	layer        uint8
	mask         uint32
	userData     uint64
	shapes       []ShapeHandle
	joints       []JointHandle
	contacts     []int32
	index        int32
	solver       int32
	island       int32
	mark         uint32
	dead         bool
}

type shape struct {
	body        BodyHandle
	g           geom
	trigger     bool
	friction    float32
	restitution float32
	density     float32
	oneSided    bool
	disabled    bool
	userData    uint64
	proxies     []int32
	dead        bool
}

type proxyInfo struct {
	shape ShapeHandle
	child int32
	tight AABB
	alive bool
	moved bool
}

type contact struct {
	shapeA      ShapeHandle
	shapeB      ShapeHandle
	bodyA       BodyHandle
	bodyB       BodyHandle
	childA      int32
	childB      int32
	proxyA      int32
	proxyB      int32
	key         uint64
	m           Manifold
	friction    float32
	restitution float32
	trigger     bool
	touching    bool
	alive       bool
	mark        uint32
	seen        uint32
}

type candidate struct {
	a, b   int32
	lo, hi uint64
}

type beginRec struct {
	c   int32
	rel Vec2
}

type Contact struct {
	ShapeA   ShapeHandle
	ShapeB   ShapeHandle
	BodyA    BodyHandle
	BodyB    BodyHandle
	Trigger  bool
	Touching bool
	Manifold Manifold
}

type World struct {
	cfg         Config
	bodies      pool[body]
	shapes      pool[shape]
	joints      pool[joint]
	tree        tree
	prox        []proxyInfo
	moveBuf     []int32
	pairs       pairMap
	touch       pairMap
	contacts    []contact
	freeC       []int32
	layers      [32]uint32
	events      []ContactEvent
	pending     []ContactEvent
	began       []beginRec
	newPairs    []candidate
	dyingB      []BodyHandle
	deadB       []BodyHandle
	dyingS      []ShapeHandle
	deadS       []ShapeHandle
	queryProxy  int32
	pairFn      func(int32) bool
	inStep      bool
	mark        uint32
	seen        uint32
	awake       []int32
	cand        []int32
	isl         []int32
	islC        []int32
	islJ        []int32
	stack       []int32
	sb          []solverBody
	cc          []contactConstraint
	soft        softness
	staticSoft  softness
	liveBodies  int
	liveMovable int
	liveShapes  int
	touching    int
	manifolds   int
	points      int
	stats       Stats
	steps       uint64
	q           queryState
	forces      []ForceRecord
	lastForces  []ForceRecord
}

func New(cfg Config) (*World, error) {
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	w := &World{
		cfg:      cfg,
		bodies:   newPool[body](cfg.InitialBodies),
		shapes:   newPool[shape](cfg.InitialShapes),
		joints:   newPool[joint](16),
		tree:     newTree(cfg.InitialShapes * 2),
		prox:     make([]proxyInfo, 0, cfg.InitialShapes*2),
		moveBuf:  make([]int32, 0, cfg.InitialShapes),
		pairs:    newPairMap(cfg.InitialContacts),
		touch:    newPairMap(cfg.InitialContacts),
		contacts: make([]contact, 0, cfg.InitialContacts),
		events:   make([]ContactEvent, 0, 256),
		began:    make([]beginRec, 0, 256),
		newPairs: make([]candidate, 0, 256),
		awake:    make([]int32, 0, cfg.InitialBodies),
		cand:     make([]int32, 0, cfg.InitialContacts),
		isl:      make([]int32, 0, cfg.InitialBodies),
		islC:     make([]int32, 0, cfg.InitialContacts),
		stack:    make([]int32, 0, cfg.InitialBodies),
		sb:       make([]solverBody, 0, cfg.InitialBodies),
		cc:       make([]contactConstraint, 0, cfg.InitialContacts),
	}
	for i := range w.layers {
		w.layers[i] = ^uint32(0)
	}
	w.pairFn = w.pairQuery
	w.q.init(w)
	return w, nil
}

func (w *World) Config() Config {
	return w.cfg
}

func (w *World) SetConfig(cfg Config) error {
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return err
	}
	cfg.InitialBodies, cfg.InitialShapes, cfg.InitialContacts = w.cfg.InitialBodies, w.cfg.InitialShapes, w.cfg.InitialContacts
	cfg.DebugVectors = w.cfg.DebugVectors
	w.cfg = cfg
	return nil
}

func (w *World) Gravity() Vec2 {
	return w.cfg.Gravity
}

func (w *World) SetGravity(g Vec2) error {
	if !finite2(g) {
		return bad("gravity is not finite")
	}
	w.cfg.Gravity = g
	return nil
}

func (w *World) margin() float32 {
	return 2 * w.cfg.ContactOffset
}

func (w *World) getBody(h BodyHandle) (*body, error) {
	b, ok := w.bodies.get(h)
	if !ok || b.dead {
		return nil, ErrHandle
	}
	return b, nil
}

func (w *World) getShape(h ShapeHandle) (*shape, error) {
	s, ok := w.shapes.get(h)
	if !ok || s.dead {
		return nil, ErrHandle
	}
	return s, nil
}

func (w *World) BodyAlive(h BodyHandle) bool {
	_, err := w.getBody(h)
	return err == nil
}

func (w *World) Alive(h ShapeHandle) bool {
	_, err := w.getShape(h)
	return err == nil
}

func validBodyDef(d BodyDef) error {
	switch {
	case d.Type > Dynamic:
		return bad("unknown body type %d", d.Type)
	case !finite2(d.Position) || !finite(d.Angle):
		return bad("body transform is not finite")
	case !finite2(d.LinearVelocity) || !finite(d.AngularVelocity):
		return bad("body velocity is not finite")
	case !finite(d.GravityScale):
		return bad("gravity scale is not finite")
	case !finite(d.LinearDamping) || d.LinearDamping < 0 || !finite(d.AngularDamping) || d.AngularDamping < 0:
		return bad("damping must be non-negative and finite")
	case !finite(d.Mass) || d.Mass < 0:
		return bad("mass must be non-negative and finite")
	case d.MassMode > MassOverride:
		return bad("unknown mass mode %d", d.MassMode)
	case d.MassMode == MassOverride && d.Type == Dynamic && d.Mass <= 0:
		return bad("mass override needs a positive mass")
	case d.Layer >= 32:
		return bad("layer %d is outside 0..31", d.Layer)
	}
	return nil
}

func validShapeDef(d ShapeDef) error {
	switch {
	case !finite(d.Friction) || d.Friction < 0:
		return bad("friction must be non-negative and finite")
	case !finite(d.Restitution) || d.Restitution < 0:
		return bad("restitution must be non-negative and finite")
	case !finite(d.Density) || d.Density < 0:
		return bad("density must be non-negative and finite")
	}
	return nil
}

func (w *World) CreateBody(d BodyDef) (BodyHandle, error) {
	if err := validBodyDef(d); err != nil {
		return BodyHandle{}, err
	}
	h, b := w.bodies.alloc()
	*b = body{
		typ:          d.Type,
		xf:           xform{p: d.Position, q: makeRot(d.Angle)},
		angle:        d.Angle,
		center:       d.Position,
		massMode:     d.MassMode,
		massOverride: d.Mass,
		gravityScale: d.GravityScale,
		linDamp:      d.LinearDamping,
		angDamp:      d.AngularDamping,
		fixedRot:     d.FixedRotation,
		bullet:       d.Bullet,
		allowSleep:   d.AllowSleep,
		layer:        d.Layer,
		mask:         d.Mask,
		userData:     d.UserData,
		index:        int32(h.Index),
		solver:       -1,
	}
	w.liveBodies++
	if d.Type != Static {
		w.liveMovable++
		b.v, b.w = d.LinearVelocity, d.AngularVelocity
		if d.FixedRotation {
			b.w = 0
		}
		w.wake(b)
	}
	w.updateMass(b)
	return h, nil
}

func (w *World) DestroyBody(h BodyHandle) error {
	b, err := w.getBody(h)
	if err != nil {
		return err
	}
	for len(b.joints) > 0 {
		w.destroyJoint(b.joints[len(b.joints)-1])
	}
	for len(b.shapes) > 0 {
		w.removeShape(b.shapes[len(b.shapes)-1], b)
	}
	w.liveBodies--
	if b.typ != Static {
		w.liveMovable--
	}
	b.dead, b.awake = true, false
	w.dyingB = append(w.dyingB, h)
	return nil
}

func (w *World) AddShape(bh BodyHandle, d ShapeDef) (ShapeHandle, error) {
	b, err := w.getBody(bh)
	if err != nil {
		return ShapeHandle{}, err
	}
	if err := validShapeDef(d); err != nil {
		return ShapeHandle{}, err
	}
	g, err := buildGeom(d.Geometry, d.Local, w.cfg.LinearSlop)
	if err != nil {
		return ShapeHandle{}, err
	}
	sh, s := w.shapes.alloc()
	*s = shape{
		body:        bh,
		g:           g,
		trigger:     d.IsTrigger,
		friction:    d.Friction,
		restitution: d.Restitution,
		density:     d.Density,
		oneSided:    d.OneSided,
		userData:    d.UserData,
	}
	w.liveShapes++
	b.shapes = append(b.shapes, sh)
	w.createProxies(sh, s, b)
	w.updateMass(b)
	w.wake(b)
	return sh, nil
}

func (w *World) RemoveShape(h ShapeHandle) error {
	s, err := w.getShape(h)
	if err != nil {
		return err
	}
	b := &w.bodies.items[s.body.Index]
	w.removeShape(h, b)
	w.updateMass(b)
	w.wake(b)
	return nil
}

func (w *World) removeShape(sh ShapeHandle, b *body) {
	s := &w.shapes.items[sh.Index]
	w.dropShapeContacts(sh)
	w.destroyProxies(s)
	s.dead = true
	w.liveShapes--
	for i, x := range b.shapes {
		if x == sh {
			b.shapes = append(b.shapes[:i], b.shapes[i+1:]...)
			break
		}
	}
	w.dyingS = append(w.dyingS, sh)
}

func (w *World) SetShape(h ShapeHandle, d ShapeDef) error {
	s, err := w.getShape(h)
	if err != nil {
		return err
	}
	if err := validShapeDef(d); err != nil {
		return err
	}
	g, err := buildGeom(d.Geometry, d.Local, w.cfg.LinearSlop)
	if err != nil {
		return err
	}
	b := &w.bodies.items[s.body.Index]
	w.dropShapeContacts(h)
	w.destroyProxies(s)
	s.g, s.trigger, s.friction, s.restitution = g, d.IsTrigger, d.Friction, d.Restitution
	s.density, s.oneSided, s.userData = d.Density, d.OneSided, d.UserData
	if !s.disabled {
		w.createProxies(h, s, b)
	}
	w.updateMass(b)
	w.wake(b)
	return nil
}

func (w *World) SetShapeEnabled(h ShapeHandle, on bool) error {
	s, err := w.getShape(h)
	if err != nil {
		return err
	}
	if s.disabled == !on {
		return nil
	}
	b := &w.bodies.items[s.body.Index]
	if on {
		s.disabled = false
		w.createProxies(h, s, b)
	} else {
		w.dropShapeContacts(h)
		w.destroyProxies(s)
		s.disabled = true
	}
	w.updateMass(b)
	w.wake(b)
	return nil
}

func (w *World) ShapeEnabled(h ShapeHandle) bool {
	s, err := w.getShape(h)
	return err == nil && !s.disabled
}

// SetBodyProps updates everything except type, pose and velocity.
func (w *World) SetBodyProps(h BodyHandle, d BodyDef) error {
	b, err := w.getBody(h)
	if err != nil {
		return err
	}
	d.Type = b.typ
	d.Position, d.Angle = b.xf.p, b.angle
	d.LinearVelocity, d.AngularVelocity = b.v, b.w
	if err := validBodyDef(d); err != nil {
		return err
	}
	b.gravityScale, b.linDamp, b.angDamp = d.GravityScale, d.LinearDamping, d.AngularDamping
	b.bullet, b.allowSleep, b.userData = d.Bullet, d.AllowSleep, d.UserData
	massChanged := b.massMode != d.MassMode || b.massOverride != d.Mass || b.fixedRot != d.FixedRotation
	b.massMode, b.massOverride, b.fixedRot = d.MassMode, d.Mass, d.FixedRotation
	if b.fixedRot {
		b.w = 0
	}
	if massChanged {
		w.updateMass(b)
	}
	if b.layer != d.Layer || b.mask != d.Mask {
		b.layer, b.mask = d.Layer, d.Mask
		w.refilter(h, false)
	}
	w.wake(b)
	return nil
}

func (w *World) SetBodyTransform(h BodyHandle, p Vec2, angle float32) error {
	b, err := w.getBody(h)
	if err != nil {
		return err
	}
	if !finite2(p) || !finite(angle) {
		return bad("body transform is not finite")
	}
	b.xf = xform{p: p, q: makeRot(angle)}
	b.angle = angle
	b.center = b.xf.apply(b.localCenter)
	w.wake(b)
	w.syncBody(b, 0)
	return nil
}

func (w *World) SetBodyFilter(h BodyHandle, layer uint8, mask uint32) error {
	b, err := w.getBody(h)
	if err != nil {
		return err
	}
	if layer >= 32 {
		return bad("layer %d is outside 0..31", layer)
	}
	b.layer, b.mask = layer, mask
	w.refilter(h, false)
	return nil
}

func (w *World) SetLayerMatrix(m [32]uint32) error {
	for i := 0; i < 32; i++ {
		for j := 0; j < 32; j++ {
			if (m[i]>>j)&1 != (m[j]>>i)&1 {
				return bad("layer matrix is not symmetric at %d,%d", i, j)
			}
		}
	}
	w.layers = m
	w.refilter(BodyHandle{}, true)
	return nil
}

func (w *World) LayerMatrix() [32]uint32 {
	return w.layers
}

func (w *World) refilter(only BodyHandle, all bool) {
	for i := range w.contacts {
		c := &w.contacts[i]
		if !c.alive || (!all && c.bodyA != only && c.bodyB != only) {
			continue
		}
		if !w.shouldCollide(c.shapeA, c.shapeB) {
			w.destroyContact(int32(i), true)
		}
	}
	for id := range w.prox {
		p := &w.prox[id]
		if !p.alive || p.moved {
			continue
		}
		if !all && w.shapes.items[p.shape.Index].body != only {
			continue
		}
		p.moved = true
		w.moveBuf = append(w.moveBuf, int32(id))
	}
}

func (w *World) wake(b *body) {
	if b.typ == Static || b.dead {
		return
	}
	b.awake = true
	b.sleepTime = 0
	w.listAwake(b)
}

func (w *World) listAwake(b *body) {
	if !b.listed {
		b.listed = true
		w.awake = append(w.awake, b.index)
	}
}

func (w *World) ForEachBody(fn func(BodyHandle) bool) {
	for i := range w.bodies.items {
		if w.bodies.alive[i] && !w.bodies.items[i].dead && !fn(w.bodies.handleAt(i)) {
			return
		}
	}
}

func (w *World) wakeHandle(h BodyHandle) {
	if int(h.Index) < len(w.bodies.items) {
		w.wake(&w.bodies.items[h.Index])
	}
}

func (w *World) updateMass(b *body) {
	old := b.center
	b.mass, b.invMass, b.inertia, b.invI = 0, 0, 0, 0
	b.localCenter = Vec2{}
	if b.typ != Dynamic {
		b.center = b.xf.p
		return
	}
	var mass, rot float32
	var c Vec2
	for _, sh := range b.shapes {
		s := &w.shapes.items[sh.Index]
		if s.density == 0 || s.disabled {
			continue
		}
		md := s.g.mass(s.density)
		mass += md.mass
		c = c.Add(md.center.Scale(md.mass))
		rot += md.inertia + md.mass*md.center.Dot(md.center)
	}
	if mass > 0 {
		c = c.Scale(1 / mass)
	}
	if b.massMode == MassOverride && b.massOverride > 0 {
		if mass > 0 {
			rot *= b.massOverride / mass
		}
		mass = b.massOverride
	}
	if mass <= 0 {
		mass, rot = 1, 0
	}
	b.mass, b.invMass = mass, 1/mass
	if !b.fixedRot && rot > 0 {
		if in := rot - mass*c.Dot(c); in > 0 {
			b.inertia, b.invI = in, 1/in
		}
	}
	b.localCenter = c
	b.center = b.xf.apply(c)
	b.v = b.v.Add(crossSV(b.w, b.center.Sub(old)))
}

type ForceRecord struct {
	Body    BodyHandle
	Point   Vec2
	Vector  Vec2
	Impulse bool
}

func (w *World) record(h BodyHandle, p, v Vec2, impulse bool) {
	if w.cfg.DebugVectors {
		w.forces = append(w.forces, ForceRecord{Body: h, Point: p, Vector: v, Impulse: impulse})
	}
}

func (w *World) SetDebugVectors(on bool) {
	w.cfg.DebugVectors = on
	if !on {
		w.forces, w.lastForces = w.forces[:0], w.lastForces[:0]
	}
}

// Forces returns what was applied before the last Step, valid until the next.
func (w *World) Forces() []ForceRecord {
	return w.lastForces
}

func (w *World) dynamicBody(h BodyHandle) (*body, error) {
	b, err := w.getBody(h)
	if err != nil {
		return nil, err
	}
	if b.typ != Dynamic {
		return nil, bad("body is %s, forces apply only to dynamic bodies", b.typ)
	}
	return b, nil
}

func (w *World) ApplyForce(h BodyHandle, f Vec2) error {
	b, err := w.dynamicBody(h)
	if err != nil {
		return err
	}
	if !finite2(f) {
		return bad("force is not finite")
	}
	b.force = b.force.Add(f)
	w.record(h, b.center, f, false)
	w.wake(b)
	return nil
}

func (w *World) ApplyForceAtPoint(h BodyHandle, f, p Vec2) error {
	b, err := w.dynamicBody(h)
	if err != nil {
		return err
	}
	if !finite2(f) || !finite2(p) {
		return bad("force or point is not finite")
	}
	b.force = b.force.Add(f)
	b.torque += p.Sub(b.center).Cross(f)
	w.record(h, p, f, false)
	w.wake(b)
	return nil
}

func (w *World) ApplyImpulse(h BodyHandle, j Vec2) error {
	b, err := w.dynamicBody(h)
	if err != nil {
		return err
	}
	if !finite2(j) {
		return bad("impulse is not finite")
	}
	b.v = b.v.Add(j.Scale(b.invMass))
	w.record(h, b.center, j, true)
	w.wake(b)
	return nil
}

func (w *World) ApplyImpulseAtPoint(h BodyHandle, j, p Vec2) error {
	b, err := w.dynamicBody(h)
	if err != nil {
		return err
	}
	if !finite2(j) || !finite2(p) {
		return bad("impulse or point is not finite")
	}
	b.v = b.v.Add(j.Scale(b.invMass))
	b.w += b.invI * p.Sub(b.center).Cross(j)
	w.record(h, p, j, true)
	w.wake(b)
	return nil
}

func (w *World) ApplyTorque(h BodyHandle, t float32) error {
	b, err := w.dynamicBody(h)
	if err != nil {
		return err
	}
	if !finite(t) {
		return bad("torque is not finite")
	}
	b.torque += t
	w.wake(b)
	return nil
}

func (w *World) SetLinearVelocity(h BodyHandle, v Vec2) error {
	b, err := w.getBody(h)
	if err != nil {
		return err
	}
	if !finite2(v) {
		return bad("velocity is not finite")
	}
	if b.typ == Static {
		return bad("static bodies have no velocity")
	}
	b.v = v
	if v.LenSq() > 0 {
		w.wake(b)
	}
	return nil
}

func (w *World) SetAngularVelocity(h BodyHandle, av float32) error {
	b, err := w.getBody(h)
	if err != nil {
		return err
	}
	if !finite(av) {
		return bad("angular velocity is not finite")
	}
	if b.typ == Static {
		return bad("static bodies have no velocity")
	}
	if b.fixedRot {
		av = 0
	}
	b.w = av
	if av != 0 {
		w.wake(b)
	}
	return nil
}

func (w *World) MoveKinematic(h BodyHandle, target Transform, dt float32) error {
	b, err := w.getBody(h)
	if err != nil {
		return err
	}
	if b.typ != Kinematic {
		return bad("MoveKinematic needs a kinematic body, got %s", b.typ)
	}
	if !finite2(target.Position) || !finite(target.Angle) || !finite(dt) || dt <= 0 {
		return bad("kinematic target or step is not valid")
	}
	b.v = target.Position.Sub(b.xf.p).Scale(1 / dt)
	b.w = (target.Angle - b.angle) / dt
	w.wake(b)
	return nil
}

func (w *World) WakeUp(h BodyHandle) error {
	b, err := w.getBody(h)
	if err != nil {
		return err
	}
	w.wake(b)
	return nil
}

func (w *World) Sleep(h BodyHandle) error {
	b, err := w.getBody(h)
	if err != nil {
		return err
	}
	if b.typ == Static {
		return nil
	}
	b.awake, b.sleepTime = false, 0
	b.v, b.w, b.force, b.torque = Vec2{}, 0, Vec2{}, 0
	return nil
}

func (w *World) BodyTransform(h BodyHandle) (Vec2, float32, bool) {
	b, err := w.getBody(h)
	if err != nil {
		return Vec2{}, 0, false
	}
	return b.xf.p, b.angle, true
}

func (w *World) BodyCenter(h BodyHandle) (Vec2, bool) {
	b, err := w.getBody(h)
	if err != nil {
		return Vec2{}, false
	}
	return b.center, true
}

func (w *World) BodyVelocity(h BodyHandle) (Vec2, float32, bool) {
	b, err := w.getBody(h)
	if err != nil {
		return Vec2{}, 0, false
	}
	return b.v, b.w, true
}

func (w *World) BodyAwake(h BodyHandle) bool {
	b, err := w.getBody(h)
	return err == nil && b.awake
}

func (w *World) BodyType(h BodyHandle) (BodyType, bool) {
	b, err := w.getBody(h)
	if err != nil {
		return 0, false
	}
	return b.typ, true
}

func (w *World) BodyMass(h BodyHandle) (mass, inertia float32, localCenter Vec2, ok bool) {
	b, err := w.getBody(h)
	if err != nil {
		return 0, 0, Vec2{}, false
	}
	return b.mass, b.inertia, b.localCenter, true
}

func (w *World) BodyUserData(h BodyHandle) (uint64, bool) {
	b, ok := w.bodies.get(h)
	if !ok {
		return 0, false
	}
	return b.userData, true
}

func (w *World) BodyShapes(h BodyHandle) []ShapeHandle {
	b, err := w.getBody(h)
	if err != nil {
		return nil
	}
	return b.shapes
}

func (w *World) ShapeBody(h ShapeHandle) (BodyHandle, bool) {
	s, ok := w.shapes.get(h)
	if !ok {
		return BodyHandle{}, false
	}
	return s.body, true
}

func (w *World) ShapeUserData(h ShapeHandle) (uint64, bool) {
	s, ok := w.shapes.get(h)
	if !ok {
		return 0, false
	}
	return s.userData, true
}

func (w *World) ShapeKind(h ShapeHandle) (GeometryKind, bool) {
	s, err := w.getShape(h)
	if err != nil {
		return 0, false
	}
	return s.g.kind, true
}

func (w *World) ShapeTrigger(h ShapeHandle) bool {
	s, err := w.getShape(h)
	return err == nil && s.trigger
}

func (w *World) ShapeAABB(h ShapeHandle) (AABB, bool) {
	s, err := w.getShape(h)
	if err != nil || len(s.proxies) == 0 {
		return AABB{}, false
	}
	box := w.prox[s.proxies[0]].tight
	for _, id := range s.proxies[1:] {
		box = boxUnion(box, w.prox[id].tight)
	}
	return box, true
}

func (w *World) Contacts(h BodyHandle, fn func(Contact) bool) {
	b, err := w.getBody(h)
	if err != nil {
		return
	}
	for _, ci := range b.contacts {
		c := &w.contacts[ci]
		if !c.alive || !c.touching {
			continue
		}
		if !fn(Contact{ShapeA: c.shapeA, ShapeB: c.shapeB, BodyA: c.bodyA, BodyB: c.bodyB, Trigger: c.trigger, Touching: true, Manifold: c.m}) {
			return
		}
	}
}

func (w *World) Events() Events {
	return Events{Contacts: w.events}
}

func (w *World) Stats() Stats {
	return w.stats
}

func sqrtf(v float32) float32 {
	return mathx.Sqrt(v)
}

func (w *World) ContactBetween(a, b ShapeHandle) (ContactEvent, bool) {
	s, err := w.getShape(a)
	if err != nil {
		return ContactEvent{}, false
	}
	bd := &w.bodies.items[s.body.Index]
	for _, ci := range bd.contacts {
		c := &w.contacts[ci]
		if !c.alive || !c.touching {
			continue
		}
		switch {
		case c.shapeA == a && c.shapeB == b:
			return w.event(c, &c.m, BeginContact), true
		case c.shapeA == b && c.shapeB == a:
			ev := w.event(c, &c.m, BeginContact)
			ev.ShapeA, ev.ShapeB = ev.ShapeB, ev.ShapeA
			ev.BodyA, ev.BodyB = ev.BodyB, ev.BodyA
			ev.Normal, ev.RelativeVelocity = ev.Normal.Neg(), ev.RelativeVelocity.Neg()
			return ev, true
		}
	}
	return ContactEvent{}, false
}

func (w *World) DebugTree(fn func(box AABB, height int32, leaf bool)) {
	for i := range w.tree.nodes {
		n := &w.tree.nodes[i]
		if n.height < 0 {
			continue
		}
		fn(n.box, n.height, n.leaf())
	}
}

func CheckShape(d ShapeDef, slop float32) error {
	if slop <= 0 {
		slop = DefaultConfig().LinearSlop
	}
	if err := validShapeDef(d); err != nil {
		return err
	}
	_, err := buildGeom(d.Geometry, d.Local, slop)
	return err
}
