package systems

import (
	"aego/core/log"
	"aego/core/mathx"
	"aego/modules/physics/components"
	"aego/modules/physics/world"
	"aego/scene"
)

const (
	ServiceID             = "physics.sim"
	Channel   log.Channel = "physics"
)

type bodyRec struct {
	node     scene.NodeHandle
	parent   scene.NodeHandle
	h        world.BodyHandle
	implicit bool
	cfg      components.Body2D
	local    scene.Transform2D
	world    mathx.Mat3
	prev     pose
	cur      pose
	excluded bool
	warned   bool
	mark     uint32
}

func (b *bodyRec) moves() bool {
	return !b.implicit && b.cfg.Type != components.Static
}

type shapeKey struct {
	node scene.NodeHandle
	kind components.Kind
}

type shapeRec struct {
	key     shapeKey
	owner   scene.NodeHandle
	body    world.BodyHandle
	h       world.ShapeHandle
	sig     colliderSig
	rel     mathx.Mat3
	built   bool
	bad     bool
	enabled bool
	mark    uint32
}

type jointKind uint8

const (
	jointDistance jointKind = iota
	jointRevolute
)

type jointKey struct {
	node scene.NodeHandle
	kind jointKind
}

type jointRec struct {
	key    jointKey
	h      world.JointHandle
	a, b   world.BodyHandle
	dist   components.DistanceJoint2D
	rev    components.RevoluteJoint2D
	built  bool
	failed bool
	mark   uint32
}

type Sim struct {
	w       *world.World
	s       *scene.Scene
	log     *log.ChannelLogger
	unsub   func()
	dirty   bool
	mark    uint32
	bodies  []*bodyRec
	nb      []*bodyRec
	byBody  map[scene.NodeHandle]*bodyRec
	shapes  []*shapeRec
	ns      []*shapeRec
	byShape map[shapeKey]*shapeRec
	joints  []*jointRec
	nj      []*jointRec
	byJoint map[jointKey]*jointRec
	diags   []Diag
	diagRev uint64
	col     components.Collider
	pts     []mathx.Vec2
	touch   []Touch
	layers  [32]string
	qhits   []world.RayHit
	qshapes []world.ShapeHandle
	qcasts  []world.CastHit
}

// Touch is a shape pair in contact; Fresh marks pairs that began this step.
type Touch struct {
	ShapeA world.ShapeHandle
	ShapeB world.ShapeHandle
	Fresh  bool
}

func New(s *scene.Scene, cfg world.Config, matrix [32]uint32, lg *log.Logger) (*Sim, error) {
	w, err := world.New(cfg)
	if err != nil {
		return nil, err
	}
	if err := w.SetLayerMatrix(matrix); err != nil {
		return nil, err
	}
	if lg == nil {
		lg = log.Nop()
	}
	m := &Sim{
		w:       w,
		s:       s,
		log:     lg.With(Channel),
		dirty:   true,
		byBody:  map[scene.NodeHandle]*bodyRec{},
		byShape: map[shapeKey]*shapeRec{},
		byJoint: map[jointKey]*jointRec{},
	}
	m.unsub = s.Events().Subscribe(m.onEvent)
	return m, nil
}

func (m *Sim) Close() {
	if m.unsub != nil {
		m.unsub()
		m.unsub = nil
	}
}

func (m *Sim) World() *world.World {
	return m.w
}

func (m *Sim) Scene() *scene.Scene {
	return m.s
}

func (m *Sim) Stats() world.Stats {
	return m.w.Stats()
}

func (m *Sim) BodyOf(h scene.NodeHandle) (world.BodyHandle, bool) {
	if b := m.byBody[h]; b != nil && !b.h.IsZero() {
		return b.h, true
	}
	return world.BodyHandle{}, false
}

func (m *Sim) onEvent(ev scene.Event) {
	switch ev.Kind {
	case scene.EvNodeDestroyed, scene.EvNodeReparented, scene.EvSpaceChanged:
		m.dirty = true
	case scene.EvComponentAdded, scene.EvComponentRemoved:
		if components.Is(ev.Type) {
			m.dirty = true
		}
	}
}

func (m *Sim) Sync() {
	m.s.UpdateTransforms()
	if m.dirty {
		m.reconcile()
	}
	for _, b := range m.bodies {
		m.syncBody(b)
		b.excluded = false
	}
	for _, sh := range m.shapes {
		m.buildShape(sh)
	}
	for _, sh := range m.shapes {
		m.enableShape(sh)
	}
	for _, b := range m.bodies {
		if b.excluded && !b.h.IsZero() {
			m.w.Sleep(b.h)
		}
	}
	for _, j := range m.joints {
		m.syncJoint(j)
	}
}

func (m *Sim) Step(dt float32) {
	for i := range m.touch {
		m.touch[i].Fresh = false
	}
	m.w.Step(dt)
	for _, e := range m.w.Events().Contacts {
		switch e.Kind {
		case world.BeginContact:
			m.touch = append(m.touch, Touch{ShapeA: e.ShapeA, ShapeB: e.ShapeB, Fresh: true})
		case world.EndContact:
			for i, t := range m.touch {
				if t.ShapeA == e.ShapeA && t.ShapeB == e.ShapeB {
					last := len(m.touch) - 1
					m.touch[i] = m.touch[last]
					m.touch = m.touch[:last]
					break
				}
			}
		}
	}
}

func (m *Sim) Touches() []Touch {
	return m.touch
}

func (m *Sim) SetLayers(names [32]string) {
	m.layers = names
}

func (m *Sim) Layers() [32]string {
	return m.layers
}

func (m *Sim) LayerIndex(name string) (int, bool) {
	for i, n := range m.layers {
		if n != "" && n == name {
			return i, true
		}
	}
	return -1, false
}

// Body is BodyOf after a pending reconcile, for callers outside the step.
func (m *Sim) Body(h scene.NodeHandle) (world.BodyHandle, bool) {
	if m.dirty {
		m.Sync()
	}
	return m.BodyOf(h)
}

func (m *Sim) reconcile() {
	m.dirty = false
	m.mark++
	s := m.s
	root := s.Root()
	m.nb, m.ns, m.nj = m.nb[:0], m.ns[:0], m.nj[:0]
	order := s.PreOrder()
	for _, h := range order {
		if h != root && s.IsAlive(h) && scene.Has[components.Body2D](s, h) {
			m.keepBody(h, false)
		}
	}
	for _, h := range order {
		if h == root || !s.IsAlive(h) {
			continue
		}
		for k := components.Kind(0); k < components.KindCount; k++ {
			if components.HasCollider(s, h, k) {
				m.keepShape(shapeKey{node: h, kind: k})
			}
		}
		if scene.Has[components.DistanceJoint2D](s, h) {
			m.keepJoint(jointKey{node: h, kind: jointDistance})
		}
		if scene.Has[components.RevoluteJoint2D](s, h) {
			m.keepJoint(jointKey{node: h, kind: jointRevolute})
		}
	}
	m.sweep()
}

func (m *Sim) sweep() {
	for _, j := range m.joints {
		if j.mark != m.mark {
			m.dropJoint(j)
			delete(m.byJoint, j.key)
			m.resolve(j.key.node, jointKeyName(j.key.kind))
		}
	}
	for _, sh := range m.shapes {
		if sh.mark != m.mark {
			m.dropShape(sh)
			delete(m.byShape, sh.key)
			m.resolve(sh.key.node, shapeKeyName(sh.key.kind))
		}
	}
	for _, b := range m.bodies {
		if b.mark != m.mark {
			m.dropBody(b)
			delete(m.byBody, b.node)
			m.forget(b.node)
		}
	}
	m.bodies, m.nb = m.nb, m.bodies[:0]
	m.shapes, m.ns = m.ns, m.shapes[:0]
	m.joints, m.nj = m.nj, m.joints[:0]
}

func (m *Sim) keepBody(h scene.NodeHandle, implicit bool) *bodyRec {
	b := m.byBody[h]
	if b != nil && b.mark == m.mark {
		return b
	}
	if b == nil {
		b = &bodyRec{node: h, implicit: implicit}
		m.byBody[h] = b
	} else if b.implicit != implicit {
		m.dropBody(b)
		b.implicit = implicit
	}
	b.mark = m.mark
	m.nb = append(m.nb, b)
	parent := m.s.Parent(h)
	if !b.h.IsZero() && b.moves() && !b.parent.IsZero() && parent != b.parent {
		m.reparent(b)
	}
	b.parent = parent
	if b.h.IsZero() {
		m.createBody(b, mathx.Vec2{}, 0)
	}
	return b
}

func (m *Sim) ownerOf(h scene.NodeHandle) scene.NodeHandle {
	root := m.s.Root()
	for x := h; !x.IsZero() && x != root; x = m.s.Parent(x) {
		if b := m.byBody[x]; b != nil && b.mark == m.mark && !b.implicit {
			return x
		}
	}
	return scene.NodeHandle{}
}

func (m *Sim) keepShape(k shapeKey) {
	var b *bodyRec
	if owner := m.ownerOf(k.node); owner.IsZero() {
		b = m.keepBody(k.node, true)
	} else {
		b = m.byBody[owner]
	}
	sh := m.byShape[k]
	if sh == nil {
		sh = &shapeRec{key: k}
		m.byShape[k] = sh
	}
	if sh.mark != m.mark {
		sh.mark = m.mark
		m.ns = append(m.ns, sh)
	}
	if sh.owner != b.node {
		m.dropShape(sh)
		sh.owner = b.node
	}
}

func (m *Sim) keepJoint(k jointKey) {
	j := m.byJoint[k]
	if j == nil {
		j = &jointRec{key: k}
		m.byJoint[k] = j
	}
	if j.mark != m.mark {
		j.mark = m.mark
		m.nj = append(m.nj, j)
	}
}
