package runtime

import (
	"cmp"
	"slices"
	"unsafe"

	"aego/core/rewind"
	"aego/scene"
	"aego/script/coroutine"
	"aego/script/signal"
	"aego/script/vm"
)

var _ rewind.Keyframer = (*Runtime)(nil)

type typeKey struct {
	vals []vm.Value
	free []int
	live int
}

type runtimeKey struct {
	types      []*scriptType
	vals       []typeKey
	inst       []instance
	instFree   []vm.InstanceID
	nodes      []scene.NodeHandle
	counts     []int32
	ids        []vm.InstanceID
	update     []vm.InstanceID
	always     []vm.InstanceID
	whenPaused []vm.InstanceID
	fixed      []vm.InstanceID
	ready      []vm.InstanceID
	spawn      []scene.NodeHandle
	evq        []queued
	evargs     []vm.Value
	machine    vm.State
	co         coroutine.State
	sig        signal.State
	elapsed    float64
	dirty      bool
	size       int
}

func (k *runtimeKey) Size() int {
	return k.size
}

func fitTypes(s []typeKey, n int) []typeKey {
	if cap(s) < n {
		s = append(s[:cap(s)], make([]typeKey, n-cap(s))...)
	}
	return s[:n]
}

func (r *Runtime) Capture(reuse rewind.Key) rewind.Key {
	k, _ := reuse.(*runtimeKey)
	if k == nil {
		k = &runtimeKey{}
	}
	vsize := int(unsafe.Sizeof(vm.Value{}))
	k.size = 0
	k.types = append(k.types[:0], r.types...)
	k.vals = fitTypes(k.vals, len(r.types))
	for i, t := range r.types {
		tk := &k.vals[i]
		tk.vals = append(tk.vals[:0], t.vals...)
		tk.free = append(tk.free[:0], t.free...)
		tk.live = t.live
		k.size += len(tk.vals)*vsize + 8*len(tk.free)
	}
	k.inst = append(k.inst[:0], r.inst...)
	k.instFree = append(k.instFree[:0], r.instFree...)
	k.nodes, k.counts, k.ids = k.nodes[:0], k.counts[:0], k.ids[:0]
	for h, list := range r.byNode {
		k.nodes = append(k.nodes, h)
		k.counts = append(k.counts, int32(len(list)))
		k.ids = append(k.ids, list...)
	}
	k.update = append(k.update[:0], r.update...)
	k.always = append(k.always[:0], r.always...)
	k.whenPaused = append(k.whenPaused[:0], r.whenPaused...)
	k.fixed = append(k.fixed[:0], r.fixed...)
	k.ready = append(k.ready[:0], r.ready...)
	k.spawn = append(k.spawn[:0], r.spawn...)
	k.evq = append(k.evq[:0], r.evq...)
	k.evargs = append(k.evargs[:0], r.evargs...)
	k.elapsed, k.dirty = r.elapsed, r.dirty
	if r.m != nil {
		r.m.Capture(&k.machine)
	}
	if r.co != nil {
		r.co.Capture(&k.co)
	}
	if r.sig != nil {
		r.sig.Capture(&k.sig)
	}
	k.size += int(unsafe.Sizeof(*k)) + len(k.inst)*int(unsafe.Sizeof(instance{})) +
		4*(len(k.instFree)+len(k.ids)+len(k.counts)+len(k.update)+len(k.always)+len(k.whenPaused)+len(k.fixed)+len(k.ready)) +
		(len(k.nodes)+len(k.spawn))*int(unsafe.Sizeof(scene.NodeHandle{})) +
		len(k.evq)*int(unsafe.Sizeof(queued{})) + len(k.evargs)*vsize +
		k.machine.Size() + k.co.Size() + k.sig.Size()
	return k
}

func (r *Runtime) Restore(key rewind.Key) {
	k := key.(*runtimeKey)
	if !slices.Equal(k.types, r.types) {
		r.log.Error("script state cannot be restored after the scripts were reloaded")
		return
	}
	for i, t := range r.types {
		tk := &k.vals[i]
		t.vals = append(t.vals[:0], tk.vals...)
		t.free = append(t.free[:0], tk.free...)
		t.live = tk.live
	}
	r.inst = append(r.inst[:0], k.inst...)
	r.instFree = append(r.instFree[:0], k.instFree...)
	clear(r.byNode)
	buf := slices.Clone(k.ids)
	off := 0
	for i, h := range k.nodes {
		n := int(k.counts[i])
		r.byNode[h] = buf[off : off+n : off+n]
		off += n
	}
	r.update = append(r.update[:0], k.update...)
	r.always = append(r.always[:0], k.always...)
	r.whenPaused = append(r.whenPaused[:0], k.whenPaused...)
	r.fixed = append(r.fixed[:0], k.fixed...)
	r.ready = append(r.ready[:0], k.ready...)
	r.readyRun = r.readyRun[:0]
	r.spawn = append(r.spawn[:0], k.spawn...)
	r.evq = append(r.evq[:0], k.evq...)
	r.evargs = append(r.evargs[:0], k.evargs...)
	r.elapsed, r.dirty = k.elapsed, k.dirty
	r.argTop = 0
	if r.m != nil {
		r.m.Restore(&k.machine)
	}
	if r.co != nil {
		r.co.Restore(&k.co)
	}
	if r.sig != nil {
		r.sig.Restore(&k.sig)
	}
}

func hashInstances(h *rewind.Hasher, list []vm.InstanceID) {
	h.Int(len(list))
	for _, id := range list {
		h.U32(uint32(id))
	}
}

func hashNodes(h *rewind.Hasher, list []scene.NodeHandle) {
	h.Int(len(list))
	for _, n := range list {
		h.U32(n.Index)
		h.U32(n.Gen)
	}
}

func (r *Runtime) Hash(h *rewind.Hasher) {
	h.String("script")
	h.Int(len(r.inst))
	for i := range r.inst {
		in := &r.inst[i]
		h.U64(uint64(in.state))
		h.Int(in.base)
		h.U32(in.node.Index)
		h.U32(in.node.Gen)
		if in.state == dead || in.base < 0 {
			continue
		}
		h.String(in.typ.name)
		vm.HashValues(h, in.typ.vals[in.base:in.base+in.typ.slots])
	}
	hashInstances(h, r.instFree)
	for _, t := range r.types {
		h.String(t.name)
		h.Int(t.live)
		h.Int(len(t.free))
		for _, f := range t.free {
			h.Int(f)
		}
	}
	nodes := make([]scene.NodeHandle, 0, len(r.byNode))
	for n := range r.byNode {
		nodes = append(nodes, n)
	}
	slices.SortFunc(nodes, func(a, b scene.NodeHandle) int {
		if c := cmp.Compare(a.Index, b.Index); c != 0 {
			return c
		}
		return cmp.Compare(a.Gen, b.Gen)
	})
	for _, n := range nodes {
		h.U32(n.Index)
		h.U32(n.Gen)
		hashInstances(h, r.byNode[n])
	}
	for _, l := range [...][]vm.InstanceID{r.update, r.always, r.whenPaused, r.fixed, r.ready} {
		hashInstances(h, l)
	}
	hashNodes(h, r.spawn)
	h.Int(len(r.evq))
	for _, q := range r.evq {
		h.U32(q.node.Index)
		h.U32(q.node.Gen)
		h.U32(uint32(q.ev))
		h.U32(uint32(q.off))
		h.U32(uint32(q.n))
	}
	vm.HashValues(h, r.evargs)
	h.F64(r.elapsed)
	h.Bool(r.dirty)
	if r.m != nil {
		r.m.Hash(h)
	}
	if r.co != nil {
		r.co.Hash(h)
	}
	if r.sig != nil {
		r.sig.Hash(h)
	}
}
