package propclaim

import (
	"aego/scene"
	"aego/scene/propref"
)

const ServiceID = "scene.claims"

type Owner uint8

const NoOwner Owner = 0

type Services interface {
	ServiceAny(id string) (any, bool)
	SetView(id string, v any)
}

type Key struct {
	node scene.NodeHandle
	prop uint64
}

func KeyOf(node scene.NodeHandle, r *propref.Ref) Key {
	return Key{node: node, prop: r.Hash()}
}

type entry struct {
	owner Owner
	ref   propref.Ref
	mark  uint32
}

// Slot remembers where a claim lives; it is valid while the table revision does not change.
type Slot struct {
	rev uint64
	ent *entry
}

func (s *Slot) set(rev uint64, e *entry) {
	if s != nil {
		s.rev, s.ent = rev, e
	}
}

type owner struct {
	name string
	mark uint32
	held int
	seen int
}

type Table struct {
	owners []owner
	m      map[Key][]entry
	rev    uint64
	total  int
}

func New() *Table {
	return &Table{owners: []owner{{}}, m: map[Key][]entry{}}
}

func Of(w Services) *Table {
	v, ok := w.ServiceAny(ServiceID)
	if !ok {
		return nil
	}
	t, _ := v.(*Table)
	return t
}

// Ensure sets the table as a view: claims are rebuilt by their owners, rewind keys skip it.
func Ensure(w Services) *Table {
	if t := Of(w); t != nil {
		return t
	}
	t := New()
	w.SetView(ServiceID, t)
	return t
}

func (t *Table) Register(name string) Owner {
	for i := 1; i < len(t.owners); i++ {
		if t.owners[i].name == name {
			return Owner(i)
		}
	}
	if len(t.owners) > 255 {
		return NoOwner
	}
	t.owners = append(t.owners, owner{name: name})
	return Owner(len(t.owners) - 1)
}

func (t *Table) Name(o Owner) string {
	if t == nil || int(o) >= len(t.owners) {
		return ""
	}
	return t.owners[o].name
}

func (t *Table) Revision() uint64 {
	if t == nil {
		return 0
	}
	return t.rev
}

func (t *Table) Begin(o Owner) {
	if t == nil || o == NoOwner {
		return
	}
	ow := &t.owners[o]
	ow.mark++
	ow.seen = 0
}

func (t *Table) Claim(o Owner, node scene.NodeHandle, r *propref.Ref) {
	t.ClaimAt(o, KeyOf(node, r), r, nil)
}

func (t *Table) ClaimAt(o Owner, k Key, r *propref.Ref, s *Slot) {
	if t == nil || o == NoOwner {
		return
	}
	ow := &t.owners[o]
	if s != nil && s.ent != nil && s.rev == t.rev {
		if e := s.ent; e.mark != ow.mark {
			e.mark = ow.mark
			ow.seen++
		}
		return
	}
	es := t.m[k]
	for i := range es {
		if e := &es[i]; e.owner == o && e.ref.Same(r) {
			if e.mark != ow.mark {
				e.mark = ow.mark
				ow.seen++
			}
			s.set(t.rev, e)
			return
		}
	}
	es = append(es, entry{owner: o, ref: *r, mark: ow.mark})
	t.m[k] = es
	ow.held++
	ow.seen++
	t.total++
	t.rev++
	s.set(t.rev, &es[len(es)-1])
}

func (t *Table) End(o Owner) {
	if t == nil || o == NoOwner {
		return
	}
	ow := &t.owners[o]
	if ow.seen == ow.held {
		return
	}
	for k, es := range t.m {
		out := es[:0]
		for _, e := range es {
			if e.owner == o && e.mark != ow.mark {
				ow.held--
				t.total--
				continue
			}
			out = append(out, e)
		}
		if len(out) == 0 {
			delete(t.m, k)
			continue
		}
		clear(es[len(out):])
		t.m[k] = out
	}
	t.rev++
}

func (t *Table) Drop(o Owner) {
	t.Begin(o)
	t.End(o)
}

func (t *Table) Held(node scene.NodeHandle, r *propref.Ref, except Owner) (Owner, bool) {
	return t.HeldKey(KeyOf(node, r), r, except)
}

func (t *Table) HeldKey(k Key, r *propref.Ref, except Owner) (Owner, bool) {
	if t == nil || t.total == 0 {
		return NoOwner, false
	}
	if except != NoOwner && int(except) < len(t.owners) && t.owners[except].held == t.total {
		return NoOwner, false
	}
	es := t.m[k]
	for i := range es {
		if e := &es[i]; e.owner != except && e.ref.Overlaps(r) {
			return e.owner, true
		}
	}
	return NoOwner, false
}
