package runtime

import (
	"errors"
	"slices"
	"strings"

	"aego/core/ids"
	"aego/engine"
	"aego/engine/recorder"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/coroutine"
	"aego/script/signal"
	"aego/script/vm"
)

var (
	errNodeGone   = errors.New("node reference is nil or destroyed")
	errSourceGone = errors.New("signal source is nil or its instance is gone")
	errTargetGone = errors.New("signal handler owner is nil or its instance is gone")
	errHandler    = errors.New("signal handler is not a method of the target script")
)

func shortName(full string) string {
	return full[strings.LastIndexByte(full, '.')+1:]
}

func (t *scriptType) methodIndex(name string) int {
	want := t.prefix + name
	for i, f := range t.methods {
		if f.Name == want {
			return i
		}
	}
	return -1
}

func (t *scriptType) lifecycle(f *vm.Func) bool {
	if f == t.ready || f == t.fixed || f == t.update || f == t.reloaded {
		return true
	}
	for _, e := range t.events {
		if e != nil && e == f {
			return true
		}
	}
	return false
}

func (r *Runtime) live(id vm.InstanceID) *instance {
	if id == vm.NoInstance || int(id) > len(r.inst) {
		return nil
	}
	in := &r.inst[id-1]
	if in.state == dead || in.base < 0 {
		return nil
	}
	return in
}

func (r *Runtime) instance(ref vm.Value, script string) (*instance, vm.InstanceID) {
	id := vm.InstanceID(ref.B)
	in := r.live(id)
	if in == nil || in.typ.name != script || bind.NodeValue(in.node).A != ref.A {
		return nil, vm.NoInstance
	}
	return in, id
}

func (r *Runtime) selfOf(id vm.InstanceID) ([]vm.Value, bool) {
	in := r.live(id)
	if in == nil || in.state != Running {
		return nil, false
	}
	r.ctx.Node, r.ctx.Self = in.node, id
	t := in.typ
	return t.vals[in.base : in.base+t.slots], true
}

func (r *Runtime) deliver(c signal.Conn, args []vm.Value) (bool, error) {
	if c.Go != nil {
		return r.callGo(c, args), nil
	}
	in := r.live(c.Dst)
	if in == nil {
		return false, nil
	}
	if in.state != Running {
		return true, nil
	}
	t := in.typ
	if c.Method < 0 || c.Method >= len(t.methods) {
		return false, nil
	}
	r.handlers++
	node, self := r.ctx.Node, r.ctx.Self
	r.ctx.Node, r.ctx.Self = in.node, c.Dst
	fl := r.m.Invoke(t.methods[c.Method], t.vals[in.base:in.base+t.slots], args, c.Dst)
	r.ctx.Node, r.ctx.Self = node, self
	if fl == nil {
		return true, nil
	}
	if fl.Code == vm.CodeCallDepth {
		return true, fl
	}
	r.fault(c.Dst, fl)
	return true, nil
}

func (r *Runtime) Coroutines(ec *engine.Context) {
	if !r.begin(ec) {
		return
	}
	r.ctx.Dt = float32(ec.Dt)
	r.co.Frame(float32(ec.Dt))
	r.end()
}

func (r *Runtime) CoStats() coroutine.Stats {
	if r.co == nil {
		return coroutine.Stats{}
	}
	return r.co.Stats()
}

func (r *Runtime) CancelChanged(changed func(name string) bool) int {
	if r.co == nil {
		return 0
	}
	return r.co.CancelTypes(func(id vm.InstanceID) bool {
		in := r.live(id)
		return in == nil || changed(in.typ.name)
	})
}

func (r *Runtime) RebindSignals() int {
	if r.sig == nil {
		return 0
	}
	return r.sig.Rebind(r.rebind)
}

func (r *Runtime) rebind(c *signal.Conn) bool {
	slots := 0
	if c.Src.Kind == signal.KindScript {
		src := r.live(c.Src.Instance())
		if src == nil {
			return false
		}
		def := src.typ.def
		si := def.Signal(c.Signal)
		if si < 0 || !(abi.Signature{Params: def.Signals[si].Params}).Equal(c.Sign) {
			return false
		}
		c.Sig, slots = si, int(def.Signals[si].Slots)
	} else {
		slots = r.paramSlots(c.Sign.Params)
	}
	if c.Go != nil {
		return true
	}
	dst := r.live(c.Dst)
	if dst == nil {
		return false
	}
	mi := dst.typ.methodIndex(c.Handler)
	if mi < 0 {
		return false
	}
	f := dst.typ.methods[mi]
	if f.Coroutine || dst.typ.lifecycle(f) || f.Results != 0 || f.Params != slots {
		return false
	}
	c.Method = mi
	return true
}

type bridge struct {
	r *Runtime
}

func (b bridge) Find(node vm.Value, script string) (vm.Value, error) {
	r := b.r
	h := bind.NodeOf(node)
	if r.scene == nil || !r.scene.IsAlive(h) {
		return vm.Value{}, errNodeGone
	}
	id, ok := r.InstanceOf(h, script)
	if !ok {
		return vm.Value{}, nil
	}
	return vm.Value{A: node.A, B: uint64(id)}, nil
}

func (b bridge) Instance(ref vm.Value, script string) ([]vm.Value, vm.InstanceID, bool) {
	in, id := b.r.instance(ref, script)
	if in == nil {
		return nil, vm.NoInstance, false
	}
	t := in.typ
	return t.vals[in.base : in.base+t.slots], id, true
}

func (b bridge) Emit(src vm.InstanceID, sig int, args []vm.Value) error {
	r := b.r
	r.emits++
	if in := r.live(src); in != nil && r.rec != nil && r.scene != nil {
		r.rec.Add(recorder.Signal, r.scene.ID(in.node), ids.NodeID{}, in.typ.def.Signals[sig].Name, shortName(in.typ.name))
	}
	return r.sig.Emit(signal.ScriptSource(src), sig, args, r.deliverFn)
}

func (b bridge) Connect(src, dst vm.Value, c *vm.Connect) (vm.Value, error) {
	r := b.r
	sin, sid := r.instance(src, c.Source)
	if sin == nil {
		return vm.Value{}, errSourceGone
	}
	din, did := r.instance(dst, c.Target)
	if din == nil {
		return vm.Value{}, errTargetGone
	}
	mi := slices.Index(din.typ.methods, c.Handler)
	if mi < 0 {
		return vm.Value{}, errHandler
	}
	h := r.sig.Connect(signal.Conn{
		Src:     signal.ScriptSource(sid),
		Sig:     c.Signal,
		Dst:     did,
		Method:  mi,
		Signal:  c.SignalName,
		Handler: shortName(c.Handler.Name),
		Sign:    abi.Signature{Params: sin.typ.def.Signals[c.Signal].Params},
	})
	return signal.Value(h), nil
}

func (b bridge) Disconnect(c vm.Value) {
	b.r.sig.Disconnect(signal.Of(c))
}

func (b bridge) Connected(c vm.Value) bool {
	return b.r.sig.Connected(signal.Of(c))
}

func (r *Runtime) CoroutinesUnscaled(ec *engine.Context) {
	if !r.begin(ec) {
		return
	}
	real := ec.Unscaled
	if ec.Time == nil {
		real = ec.Dt
	}
	r.co.Unscaled(float32(real))
	r.end()
}
