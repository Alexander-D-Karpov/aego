package runtime

import (
	"errors"

	"aego/scene"
	"aego/schema"
	"aego/script/bytecode"
	"aego/script/vm"
)

type move struct {
	typ  *scriptType
	base int
	kill bool
}

type Swap struct {
	r      *Runtime
	reg    *schema.Registry
	types  []*scriptType
	byName map[string]*scriptType
	descs  []*schema.TypeDescriptor
	moves  []move
	loaded bool
}

func (r *Runtime) Machine() *vm.Machine {
	return r.m
}

func (r *Runtime) Generation() uint64 {
	return r.gen
}

func (r *Runtime) BeginSwap(reg *schema.Registry) *Swap {
	if reg == nil {
		reg = r.opt.Schema
	}
	return &Swap{r: r, reg: reg}
}

func (s *Swap) LoadModules(mods []*bytecode.Module) error {
	r := s.r
	if r.m == nil {
		return errors.New("script runtime is not linked")
	}
	if r.m.Busy() {
		return errors.New("script machine is running a call")
	}
	loaded, err := load(r.m, r.opt.Bindings, mods)
	if err != nil {
		return err
	}
	if s.reg != nil {
		descs, err := Describe(mods, r.opt.Bindings.Surface())
		if err != nil {
			return err
		}
		if err := s.reg.CheckDynamic(descs); err != nil {
			return err
		}
		s.descs = descs
	}
	s.types, s.byName = buildTypes(r.m, loaded, mods, r.evIndex)
	s.loaded = true
	return nil
}

func (s *Swap) Migrate(fn func(oldType, newType string, src, dst []vm.Value) int) (migrated, reset, killed int, err error) {
	if !s.loaded {
		return 0, 0, 0, errors.New("new modules are not loaded")
	}
	r := s.r
	s.moves = make([]move, len(r.inst))
	for i := range r.inst {
		in := &r.inst[i]
		s.moves[i].base = -1
		if in.state == dead || in.base < 0 {
			continue
		}
		nt := s.byName[in.typ.name]
		if nt == nil {
			s.moves[i].kill = true
			killed++
			continue
		}
		base := len(nt.vals)
		nt.vals = append(nt.vals, nt.init...)
		dst := nt.vals[base : base+nt.slots]
		src := in.typ.vals[in.base : in.base+in.typ.slots]
		dst[0] = src[0]
		reset += fn(in.typ.name, nt.name, src, dst)
		s.moves[i] = move{typ: nt, base: base}
		migrated++
	}
	return migrated, reset, killed, nil
}

func (s *Swap) Commit(id uint64, changed func(name string) bool) ([]vm.InstanceID, error) {
	r := s.r
	if s.moves == nil {
		return nil, errors.New("instances are not migrated")
	}
	if s.reg != nil {
		if err := s.reg.SetDynamic(s.descs); err != nil {
			return nil, err
		}
	}
	var again []vm.InstanceID
	killed := false
	for i, mv := range s.moves {
		in := &r.inst[i]
		iid := vm.InstanceID(i + 1)
		switch {
		case mv.typ != nil:
			in.typ, in.base = mv.typ, mv.base
			mv.typ.live++
			if changed(mv.typ.name) {
				if in.state == Faulted {
					in.state = Running
				}
				if mv.typ.reloaded != nil && in.state == Running {
					again = append(again, iid)
				}
			}
		case mv.kill || (in.state == dead && in.base >= 0):
			in.state, in.base = dead, -1
			r.instFree = append(r.instFree, iid)
			killed = true
		}
	}
	if killed {
		for h, list := range r.byNode {
			n := 0
			for _, iid := range list {
				if r.inst[iid-1].state != dead {
					list[n] = iid
					n++
				}
			}
			if n == 0 {
				delete(r.byNode, h)
			} else {
				r.byNode[h] = list[:n]
			}
		}
	}
	r.types, r.byName = s.types, s.byName
	r.relisten()
	r.update = r.reschedule(r.update, func(t *scriptType) *vm.Func { return t.update }, true)
	r.fixed = r.reschedule(r.fixed, func(t *scriptType) *vm.Func { return t.fixed }, true)
	r.ready = r.reschedule(r.ready, func(t *scriptType) *vm.Func { return t.ready }, false)
	r.stats.Types = make([]TypeStats, len(r.types))
	r.gen = id
	r.dirty = false
	s.moves = nil
	return again, nil
}

func (s *Swap) Abort() {
	s.types, s.byName, s.descs, s.moves, s.loaded = nil, nil, nil, nil, false
}

func (r *Runtime) reschedule(list []vm.InstanceID, has func(*scriptType) *vm.Func, add bool) []vm.InstanceID {
	seen := make([]bool, len(r.inst)+1)
	live := func(iid vm.InstanceID) bool {
		in := &r.inst[iid-1]
		return in.state != dead && in.base >= 0 && has(in.typ) != nil
	}
	out := list[:0]
	for _, iid := range list {
		if !seen[iid] && live(iid) {
			seen[iid] = true
			out = append(out, iid)
		}
	}
	if add {
		for i := range r.inst {
			iid := vm.InstanceID(i + 1)
			if !seen[iid] && live(iid) {
				seen[iid] = true
				out = append(out, iid)
			}
		}
	}
	return out
}

func (r *Runtime) CallReloaded(ids []vm.InstanceID) {
	for _, iid := range ids {
		in := &r.inst[iid-1]
		if in.state == Running && in.typ.reloaded != nil {
			r.call(iid, in.typ.reloaded, nil)
		}
	}
	if r.m != nil {
		r.m.MaybeCollect(r)
	}
}

func (r *Runtime) Defer(fn func()) {
	r.dmu.Lock()
	r.deferred = append(r.deferred, fn)
	r.dmu.Unlock()
}

func (r *Runtime) RunDeferred() {
	if r.m == nil || r.m.Busy() {
		return
	}
	r.dmu.Lock()
	list := r.deferred
	r.deferred = nil
	r.dmu.Unlock()
	for _, fn := range list {
		fn()
	}
}

func (r *Runtime) InstanceOf(h scene.NodeHandle, typeName string) (vm.InstanceID, bool) {
	for _, iid := range r.byNode[h] {
		in := &r.inst[iid-1]
		if in.state != dead && in.base >= 0 && in.typ.name == typeName {
			return iid, true
		}
	}
	return vm.NoInstance, false
}

func (r *Runtime) Field(iid vm.InstanceID, name string) (vm.Value, bool) {
	if iid == vm.NoInstance || int(iid) > len(r.inst) {
		return vm.Value{}, false
	}
	in := &r.inst[iid-1]
	if in.base < 0 || in.typ.def == nil {
		return vm.Value{}, false
	}
	for _, f := range in.typ.def.Fields {
		if f.Name == name {
			return in.typ.vals[in.base+int(f.Slot)], true
		}
	}
	return vm.Value{}, false
}
