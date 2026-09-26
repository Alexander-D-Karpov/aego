package scene

import (
	"reflect"

	"aego/asset"
	"aego/core/errs"
	"aego/core/handle"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/core/strs"
	"aego/schema"
)

const ServiceID = "scene"

type NodeHandle = handle.Handle[nodeData]

type Options struct {
	InitialNodes      int
	InitialComponents int
}

type Stats struct {
	Nodes         int
	Alive         int
	PendingOps    int
	Components    int
	MissingAssets uint32
	TransformNS   uint64
}

type opKind uint8

const (
	opDestroy opKind = iota
	opReparent
)

type structuralOp struct {
	kind   opKind
	node   NodeHandle
	parent NodeHandle
	index  int
}

type nameKey struct {
	parent NodeHandle
	name   strs.StringID
}

type Scene struct {
	types        *Types
	registry     *schema.Registry
	resolver     asset.Resolver
	nodes        *handle.Arena[nodeData]
	interner     *strs.Interner
	root         NodeHandle
	preOrder     []NodeHandle
	orderDirty   bool
	ops          []structuralOp
	pendingOps   int
	byID         map[ids.NodeID]NodeHandle
	byName       map[nameKey]NodeHandle
	storesByGo   map[reflect.Type]anyStore
	storesByID   map[schema.TypeID]anyStore
	storeList    []anyStore
	events       Events
	revision     uint64
	savedRev     uint64
	structureRev uint64
	stats        Stats
	scratch      []NodeHandle
}

func New(types *Types, res asset.Resolver, opt Options) *Scene {
	if opt.InitialNodes <= 0 {
		opt.InitialNodes = 1024
	}
	s := &Scene{
		types:      types,
		registry:   types.Schema(),
		resolver:   res,
		nodes:      handle.NewArena[nodeData](opt.InitialNodes),
		interner:   strs.NewInterner(),
		byID:       make(map[ids.NodeID]NodeHandle, opt.InitialNodes),
		byName:     make(map[nameKey]NodeHandle, opt.InitialNodes),
		storesByGo: map[reflect.Type]anyStore{},
		storesByID: map[schema.TypeID]anyStore{},
		ops:        make([]structuralOp, 0, 64),
		preOrder:   make([]NodeHandle, 0, opt.InitialNodes),
		scratch:    make([]NodeHandle, 0, 64),
	}
	h, n := s.nodes.Alloc()
	*n = nodeData{
		id:     ids.NewNodeID(),
		name:   s.interner.Intern("root"),
		local:  IdentityTransform(),
		world:  mathx.Mat3Identity(),
		layers: ^uint64(0),
		flags:  flagAlive | flagEnabled | flagEffective | flagWorldDirty,
	}
	s.root = h
	s.byID[n.id] = h
	s.orderDirty = true
	return s
}

func (s *Scene) Types() *Types {
	return s.types
}

func (s *Scene) Registry() *schema.Registry {
	return s.registry
}

func (s *Scene) Resolver() asset.Resolver {
	return s.resolver
}

func (s *Scene) Root() NodeHandle {
	return s.root
}

func (s *Scene) Events() *Events {
	return &s.events
}

func (s *Scene) Revision() uint64 {
	return s.revision
}

func (s *Scene) StructureRevision() uint64 {
	return s.structureRev
}

func (s *Scene) MarkSaved() {
	s.savedRev = s.revision
}

func (s *Scene) Dirty() bool {
	return s.revision != s.savedRev
}

func (s *Scene) Pending() int {
	return s.pendingOps
}

func (s *Scene) node(h NodeHandle) (*nodeData, bool) {
	n, ok := s.nodes.Get(h)
	if !ok || !n.has(flagAlive) {
		return nil, false
	}
	return n, true
}

func (s *Scene) IsAlive(h NodeHandle) bool {
	n, ok := s.node(h)
	return ok && !n.has(flagPendingDestroy)
}

func (s *Scene) queueStructural() {
	s.pendingOps++
}

func (s *Scene) touch() {
	s.revision++
}

func (s *Scene) emit(e Event) {
	s.events.dispatch(e)
}

func (s *Scene) Create(parent NodeHandle, name string) NodeHandle {
	return s.CreateWithID(parent, name, ids.NewNodeID())
}

func (s *Scene) CreateWithID(parent NodeHandle, name string, id ids.NodeID) NodeHandle {
	if parent.IsZero() {
		parent = s.root
	}
	if _, ok := s.node(parent); !ok {
		return NodeHandle{}
	}
	if id.IsZero() {
		id = ids.NewNodeID()
	}
	if existing, dup := s.byID[id]; dup {
		return existing
	}
	h, n := s.nodes.Alloc()
	*n = nodeData{
		id:     id,
		local:  IdentityTransform(),
		world:  mathx.Mat3Identity(),
		layers: 1,
		flags:  flagAlive | flagEnabled | flagWorldDirty,
	}
	s.byID[id] = h
	s.attach(h, parent, -1)
	s.SetName(h, name)
	s.orderDirty = true
	s.structureRev++
	s.touch()
	s.emit(Event{Kind: EvNodeCreated, Node: h, ID: id})
	return h
}

func (s *Scene) Destroy(h NodeHandle) {
	n, ok := s.node(h)
	if !ok || n.has(flagPendingDestroy) || h == s.root {
		return
	}
	s.markDestroy(h)
	s.ops = append(s.ops, structuralOp{kind: opDestroy, node: h})
	s.queueStructural()
	s.orderDirty = true
	s.structureRev++
	s.touch()
}

func (s *Scene) markDestroy(h NodeHandle) {
	n, ok := s.node(h)
	if !ok || n.has(flagPendingDestroy) {
		return
	}
	n.set(flagPendingDestroy, true)
	n.set(flagEffective, false)
	s.emit(Event{Kind: EvNodeDestroyed, Node: h, ID: n.id})
	for c := n.firstChild; !c.IsZero(); {
		child, ok := s.nodes.Get(c)
		if !ok {
			break
		}
		next := child.nextSib
		s.markDestroy(c)
		c = next
	}
}

func (s *Scene) Reparent(h, parent NodeHandle, index int) error {
	if h == s.root {
		return errs.New(errs.SceneBadReparent, "the root node cannot be reparented")
	}
	if _, ok := s.node(h); !ok {
		return errs.New(errs.SceneDeadNode, "node is not alive")
	}
	if parent.IsZero() {
		parent = s.root
	}
	if _, ok := s.node(parent); !ok {
		return errs.New(errs.SceneDeadNode, "new parent is not alive")
	}
	for p := parent; !p.IsZero(); {
		if p == h {
			return errs.New(errs.SceneBadReparent, "reparenting would create a cycle",
				log.F("node", s.Name(h)), log.F("parent", s.Name(parent)))
		}
		n, ok := s.nodes.Get(p)
		if !ok {
			break
		}
		p = n.parent
	}
	s.ops = append(s.ops, structuralOp{kind: opReparent, node: h, parent: parent, index: index})
	s.queueStructural()
	s.orderDirty = true
	s.touch()
	return nil
}

func (s *Scene) attach(h, parent NodeHandle, index int) {
	n, _ := s.nodes.Get(h)
	p, _ := s.nodes.Get(parent)
	n.parent = parent
	n.nextSib = NodeHandle{}
	n.prevSib = NodeHandle{}

	if p.firstChild.IsZero() || index == 0 {
		if !p.firstChild.IsZero() {
			first, _ := s.nodes.Get(p.firstChild)
			first.prevSib = h
			n.nextSib = p.firstChild
		} else {
			p.lastChild = h
		}
		p.firstChild = h
		p.childCount++
		return
	}
	if index < 0 || int(p.childCount) <= index {
		last, _ := s.nodes.Get(p.lastChild)
		last.nextSib = h
		n.prevSib = p.lastChild
		p.lastChild = h
		p.childCount++
		return
	}
	cur := p.firstChild
	for i := 0; i < index-1; i++ {
		c, _ := s.nodes.Get(cur)
		cur = c.nextSib
	}
	prev, _ := s.nodes.Get(cur)
	n.nextSib = prev.nextSib
	n.prevSib = cur
	if !prev.nextSib.IsZero() {
		next, _ := s.nodes.Get(prev.nextSib)
		next.prevSib = h
	} else {
		p.lastChild = h
	}
	prev.nextSib = h
	p.childCount++
}

func (s *Scene) detach(h NodeHandle) {
	n, ok := s.nodes.Get(h)
	if !ok || n.parent.IsZero() {
		return
	}
	p, ok := s.nodes.Get(n.parent)
	if !ok {
		return
	}
	if !n.prevSib.IsZero() {
		prev, _ := s.nodes.Get(n.prevSib)
		prev.nextSib = n.nextSib
	} else {
		p.firstChild = n.nextSib
	}
	if !n.nextSib.IsZero() {
		next, _ := s.nodes.Get(n.nextSib)
		next.prevSib = n.prevSib
	} else {
		p.lastChild = n.prevSib
	}
	p.childCount--
	s.removeNameIndex(h, n)
	n.parent = NodeHandle{}
	n.nextSib = NodeHandle{}
	n.prevSib = NodeHandle{}
}

func (s *Scene) Flush() {
	if s.pendingOps == 0 && !s.orderDirty {
		return
	}
	for _, op := range s.ops {
		switch op.kind {
		case opReparent:
			n, ok := s.node(op.node)
			if !ok || n.has(flagPendingDestroy) {
				continue
			}
			name := n.name
			s.detach(op.node)
			s.attach(op.node, op.parent, op.index)
			n.name = name
			s.insertNameIndex(op.node, n)
			n.set(flagWorldDirty, true)
			s.emit(Event{Kind: EvNodeReparented, Node: op.node, ID: n.id})
		}
	}
	for _, st := range s.storeList {
		st.flush(s)
	}
	for _, op := range s.ops {
		if op.kind != opDestroy {
			continue
		}
		s.collectSubtree(op.node)
		for _, h := range s.scratch {
			s.finalizeDestroy(h)
		}
	}
	if len(s.ops) > 0 {
		s.structureRev++
	}
	s.ops = s.ops[:0]
	s.pendingOps = 0
	if s.orderDirty {
		s.rebuildOrder()
		s.emit(Event{Kind: EvHierarchyChanged})
	}
}

func (s *Scene) collectSubtree(h NodeHandle) {
	s.scratch = s.scratch[:0]
	s.appendSubtree(h)
}

func (s *Scene) appendSubtree(h NodeHandle) {
	n, ok := s.nodes.Get(h)
	if !ok {
		return
	}
	for c := n.firstChild; !c.IsZero(); {
		child, ok := s.nodes.Get(c)
		if !ok {
			break
		}
		next := child.nextSib
		s.appendSubtree(c)
		c = next
	}
	s.scratch = append(s.scratch, h)
}

func (s *Scene) finalizeDestroy(h NodeHandle) {
	n, ok := s.nodes.Get(h)
	if !ok {
		return
	}
	for _, st := range s.storeList {
		st.dropNode(h)
	}
	s.detach(h)
	delete(s.byID, n.id)
	n.set(flagAlive, false)
	s.nodes.Free(h)
}

func (s *Scene) rebuildOrder() {
	s.preOrder = s.preOrder[:0]
	s.appendOrder(s.root)
	s.orderDirty = false
}

func (s *Scene) appendOrder(h NodeHandle) {
	n, ok := s.nodes.Get(h)
	if !ok || !n.has(flagAlive) {
		return
	}
	s.preOrder = append(s.preOrder, h)
	for c := n.firstChild; !c.IsZero(); {
		child, ok := s.nodes.Get(c)
		if !ok {
			break
		}
		next := child.nextSib
		s.appendOrder(c)
		c = next
	}
}

func (s *Scene) PreOrder() []NodeHandle {
	if s.orderDirty {
		s.rebuildOrder()
	}
	return s.preOrder
}
