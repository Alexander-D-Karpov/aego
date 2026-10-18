package runtime

import (
	"aego/contracts/scriptevents"
	"aego/core/ids"
	"aego/core/log"
	"aego/engine"
	"aego/engine/recorder"
	"aego/scene"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/vm"
)

const eventQueue = 1024

type queued struct {
	node scene.NodeHandle
	ev   uint16
	off  int32
	n    int32
}

var (
	_ scriptevents.Sink        = (*Runtime)(nil)
	_ scriptevents.Listener    = (*Runtime)(nil)
	_ scriptevents.Strings     = (*Runtime)(nil)
	_ scriptevents.Broadcaster = (*Runtime)(nil)
)

func (t *scriptType) event(i int) *vm.Func {
	if i >= len(t.events) {
		return nil
	}
	return t.events[i]
}

func (r *Runtime) linkEvents() {
	tab := r.opt.Bindings.Table()
	list := tab.EventList()
	r.evIndex = make(map[string]int, len(list))
	r.evSlots = make([]int, len(list))
	r.evNames = make([]string, len(list))
	r.evRef = make([]bool, len(list))
	r.listen = make([]bool, len(list))
	for i, e := range list {
		r.evIndex[e.Name] = i
		r.evNames[i] = e.Name
		r.evRef[i] = len(e.Params) > 0 && e.Params[0].Kind == abi.NodeRef
		n := 0
		for _, p := range e.Params {
			n += tab.Slots(p)
		}
		r.evSlots[i] = n
	}
	if cap(r.evq) == 0 {
		r.evq = make([]queued, 0, eventQueue)
		r.evargs = make([]vm.Value, 0, eventQueue*4)
	}
}
func (r *Runtime) record(node scene.NodeHandle, i int, args []vm.Value, handled bool) {
	rec := r.rec
	if rec == nil || r.scene == nil {
		return
	}
	id := r.scene.ID(node)
	if !handled && !rec.Watching(id) {
		rec.Count(recorder.Event)
		return
	}
	var other ids.NodeID
	if r.evRef[i] && len(args) > 0 {
		if h := bind.NodeOf(args[0]); r.scene.IsAlive(h) {
			other = r.scene.ID(h)
		}
	}
	rec.Add(recorder.Event, id, other, r.evNames[i], "")
}

func (r *Runtime) relisten() {
	clear(r.listen)
	for _, t := range r.types {
		for i, f := range t.events {
			if f != nil && i < len(r.listen) {
				r.listen[i] = true
			}
		}
	}
}

func (r *Runtime) Listens(ev scriptevents.EventID) bool {
	i := int(ev)
	return i < len(r.listen) && r.listen[i]
}

func (r *Runtime) ListensOn(node scene.NodeHandle, ev scriptevents.EventID) bool {
	i := int(ev)
	if i >= len(r.listen) || !r.listen[i] {
		return false
	}
	for _, id := range r.byNode[node] {
		in := &r.inst[id-1]
		if in.state == Running && in.typ.event(i) != nil {
			return true
		}
	}
	return false
}

func (r *Runtime) Emit(node scene.NodeHandle, ev scriptevents.EventID, args []vm.Value) {
	i := int(ev)
	if r.m == nil || i >= len(r.listen) || len(args) != r.evSlots[i] {
		return
	}
	_, owned := r.byNode[node]
	r.record(node, i, args, owned && r.listen[i])
	if !owned || !r.listen[i] {
		return
	}
	if (len(r.evq) == cap(r.evq) || len(r.evargs)+len(args) > cap(r.evargs)) && !r.evWarned {
		r.evWarned = true
		r.log.Warn("script event queue grew past its preallocated size", log.F("events", len(r.evq)))
	}
	off := len(r.evargs)
	r.evargs = append(r.evargs, args...)
	r.evq = append(r.evq, queued{node: node, ev: uint16(ev), off: int32(off), n: int32(len(args))})
}

func (r *Runtime) Flush() {
	if r.m == nil || r.scene == nil || r.m.Busy() || len(r.evq) == 0 {
		return
	}
	n := len(r.evq)
	for k := 0; k < n; k++ {
		q := r.evq[k]
		args := r.evargs[q.off : q.off+q.n]
		for _, id := range r.byNode[q.node] {
			in := &r.inst[id-1]
			if in.state != Running {
				continue
			}
			if f := in.typ.event(int(q.ev)); f != nil {
				r.evCount++
				r.call(id, f, args)
			}
		}
	}
	r.dropEvents(n)
}

func (r *Runtime) dropEvents(n int) {
	rest := len(r.evq) - n
	if rest <= 0 {
		r.evq, r.evargs = r.evq[:0], r.evargs[:0]
		return
	}
	base := r.evq[n].off
	k := copy(r.evargs, r.evargs[base:])
	r.evargs = r.evargs[:k]
	copy(r.evq, r.evq[n:])
	r.evq = r.evq[:rest]
	for i := range r.evq {
		r.evq[i].off -= base
	}
}

func (r *Runtime) Pending() int {
	return len(r.evq)
}

func (r *Runtime) EventCalls() uint64 {
	return r.evCount
}

func (r *Runtime) DispatchEvents(ec *engine.Context) {
	if !r.begin(ec) {
		return
	}
	r.ctx.Dt = float32(ec.Dt)
	r.Flush()
	r.end()
}

func (r *Runtime) Intern(s string) vm.Value {
	if r.m == nil {
		return vm.Value{}
	}
	return r.m.Intern(s)
}

func (r *Runtime) Broadcast(ev scriptevents.EventID, args []vm.Value) {
	if r.scene == nil || !r.Listens(ev) {
		return
	}
	for _, h := range r.scene.PreOrder() {
		if r.ListensOn(h, ev) {
			r.Emit(h, ev, args)
		}
	}
}
