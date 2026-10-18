package runtime

import (
	"errors"
	"fmt"
	rtdebug "runtime/debug"

	"aego/contracts/signals"
	"aego/core/ids"
	"aego/core/log"
	"aego/engine"
	"aego/engine/recorder"
	"aego/scene"
	"aego/schema"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/signal"
	"aego/script/vm"
)

var (
	faultSignalSource = &vm.Fault{Code: vm.CodeNil, Message: "signal source is nil or gone"}
	errNoSignal       = errors.New("signal is not part of the linked bindings")
)

type hostHandle struct {
	name string
	sigs []abi.SignalDef
}

func (r *Runtime) linkHandles() {
	tab := r.opt.Bindings.Table()
	r.handles = map[uint64]hostHandle{}
	for _, h := range tab.HandleList() {
		r.handles[abi.HandleID(h.Name)] = hostHandle{name: h.Name, sigs: tab.HandleSignals(h.Name)}
	}
}

func (r *Runtime) paramSlots(ps []abi.TypeRef) int {
	tab := r.opt.Bindings.Table()
	n := 0
	for _, p := range ps {
		n += tab.Slots(p)
	}
	return n
}

func (r *Runtime) callGo(c signal.Conn, args []vm.Value) (ok bool) {
	defer func() {
		v := recover()
		if v == nil {
			return
		}
		ok = false
		r.log.Error("signal handler panicked and is disconnected", log.F("signal", c.Signal), log.F("panic", fmt.Sprint(v)))
		if r.opt.OnPanic != nil {
			r.opt.OnPanic("signal "+c.Signal, v, rtdebug.Stack())
		}
	}()
	c.Go(args)
	return true
}

func connectIntrinsic(kind uint8) vm.HostFunc {
	return func(m *vm.Machine, w []vm.Value) error {
		b, ok := m.Signals.(bridge)
		if !ok {
			return errNoScene
		}
		return b.r.connectScript(kind, w)
	}
}

func (r *Runtime) hostSource(kind uint8, v vm.Value, sig int) (signal.Source, abi.SignalDef, error) {
	if v.IsZero() {
		return signal.Source{}, abi.SignalDef{}, faultSignalSource
	}
	switch kind {
	case signal.KindComponent:
		idx := int(v.B)
		if idx >= len(r.comps) {
			return signal.Source{}, abi.SignalDef{}, errBadComponent
		}
		c := &r.comps[idx]
		if sig < 0 || sig >= len(c.sigs) {
			return signal.Source{}, abi.SignalDef{}, errNoSignal
		}
		h := bind.NodeOf(v)
		if r.scene == nil || !r.scene.IsAlive(h) {
			return signal.Source{}, abi.SignalDef{}, faultNodeGone
		}
		if _, ok := r.scene.GetByType(h, c.typ); !ok {
			return signal.Source{}, abi.SignalDef{}, faultComponentGone
		}
		return signal.Source{Kind: signal.KindComponent, A: v.A, B: uint64(c.typ)}, c.sigs[sig], nil
	case signal.KindHandle:
		hh, ok := r.handles[v.B]
		if !ok || sig < 0 || sig >= len(hh.sigs) {
			return signal.Source{}, abi.SignalDef{}, errNoSignal
		}
		return signal.Source{Kind: signal.KindHandle, A: v.A, B: v.B}, hh.sigs[sig], nil
	}
	return signal.Source{}, abi.SignalDef{}, errNoSignal
}

func (r *Runtime) handleAlive(src signal.Source) bool {
	if r.ctx.World == nil {
		return true
	}
	hub, ok := engine.ServiceOf[signals.Hub](r.ctx.World, signals.ServiceID)
	return !ok || hub.HandleAlive(src.B, src.A)
}

func (r *Runtime) connectScript(kind uint8, w []vm.Value) error {
	sig := int(w[2].Int())
	src, def, err := r.hostSource(kind, w[0], sig)
	if err != nil {
		return err
	}
	if src.Kind == signal.KindHandle && !r.handleAlive(src) {
		w[0] = vm.Value{}
		return nil
	}
	id := vm.InstanceID(w[1].B)
	in := r.live(id)
	if in == nil || bind.NodeValue(in.node).A != w[1].A {
		return errTargetGone
	}
	name, _ := r.m.Str(w[3])
	mi := in.typ.methodIndex(name)
	if mi < 0 {
		return errHandler
	}
	h := r.sig.Connect(signal.Conn{
		Src: src, Sig: sig, Dst: id, Method: mi,
		Signal: def.Name, Handler: name, Sign: abi.Signature{Params: def.Params},
	})
	w[0] = signal.Value(h)
	return nil
}

func (r *Runtime) signalDef(src signal.Source, sig int) (abi.SignalDef, bool) {
	switch src.Kind {
	case signal.KindComponent:
		i, ok := r.compByType[schema.TypeID(src.B)]
		if !ok || sig < 0 || sig >= len(r.comps[i].sigs) {
			return abi.SignalDef{}, false
		}
		h := bind.NodeOf(vm.Value{A: src.A})
		if r.scene == nil || !r.scene.IsAlive(h) {
			return abi.SignalDef{}, false
		}
		if _, has := r.scene.GetByType(h, r.comps[i].typ); !has {
			return abi.SignalDef{}, false
		}
		return r.comps[i].sigs[sig], true
	case signal.KindHandle:
		hh, ok := r.handles[src.B]
		if !ok || sig < 0 || sig >= len(hh.sigs) {
			return abi.SignalDef{}, false
		}
		return hh.sigs[sig], true
	}
	return abi.SignalDef{}, false
}

func (r *Runtime) WantsSignal(src signal.Source) bool {
	return r.sig != nil && r.sig.Has(src)
}

func (r *Runtime) SignalArgs(n int) []vm.Value {
	if need := r.argTop + n; need > len(r.argStack) {
		grown := make([]vm.Value, max(need, 2*len(r.argStack), 64))
		copy(grown, r.argStack[:r.argTop])
		r.argStack = grown
	}
	a := r.argStack[r.argTop : r.argTop+n]
	clear(a)
	r.argTop += n
	return a
}

func (r *Runtime) EmitHost(src signal.Source, sig int, args []vm.Value) {
	if r.sig != nil && r.m != nil {
		r.emits++
		r.recordHost(src, sig)
		if err := r.sig.Emit(src, sig, args, r.deliverFn); err != nil {
			r.log.Error("signal re-emits itself without end, the emit is dropped", log.F("err", err.Error()))
		}
	}
	r.argTop = max(r.argTop-len(args), 0)
}

func (r *Runtime) EndSource(src signal.Source) {
	if r.sig != nil {
		r.sig.DropSource(src)
	}
}

func (r *Runtime) ConnectHost(src signal.Source, sig int, fn signal.Handler) vm.Value {
	if r.sig == nil || fn == nil {
		return vm.Value{}
	}
	def, ok := r.signalDef(src, sig)
	if !ok {
		return vm.Value{}
	}
	h := r.sig.Connect(signal.Conn{Src: src, Sig: sig, Go: fn, Signal: def.Name, Handler: "go", Sign: abi.Signature{Params: def.Params}})
	return signal.Value(h)
}

func (r *Runtime) ConnectScriptHost(node scene.NodeHandle, script, name string, fn signal.Handler) vm.Value {
	if r.sig == nil || fn == nil {
		return vm.Value{}
	}
	id, ok := r.InstanceOf(node, script)
	in := r.live(id)
	if !ok || in == nil {
		return vm.Value{}
	}
	def := in.typ.def
	si := def.Signal(name)
	if si < 0 {
		return vm.Value{}
	}
	h := r.sig.Connect(signal.Conn{
		Src: signal.ScriptSource(id), Sig: si, Go: fn, Signal: name, Handler: "go",
		Sign: abi.Signature{Params: def.Signals[si].Params},
	})
	return signal.Value(h)
}

func (r *Runtime) DisconnectSignal(c vm.Value) {
	if r.sig != nil {
		r.sig.Disconnect(signal.Of(c))
	}
}

func (r *Runtime) SignalConnected(c vm.Value) bool {
	return r.sig != nil && r.sig.Connected(signal.Of(c))
}

func (r *Runtime) SetRecorder(rec *recorder.Recorder) {
	r.rec = rec
}

func (r *Runtime) recordHost(src signal.Source, sig int) {
	rec := r.rec
	if rec == nil || r.scene == nil {
		return
	}
	switch src.Kind {
	case signal.KindComponent:
		i, ok := r.compByType[schema.TypeID(src.B)]
		if !ok || sig >= len(r.comps[i].sigs) {
			return
		}
		var id ids.NodeID
		if h := bind.NodeOf(vm.Value{A: src.A}); r.scene.IsAlive(h) {
			id = r.scene.ID(h)
		}
		rec.Add(recorder.Signal, id, ids.NodeID{}, r.comps[i].sigs[sig].Name, shortName(r.comps[i].name))
	case signal.KindHandle:
		hh, ok := r.handles[src.B]
		if !ok || sig >= len(hh.sigs) {
			return
		}
		rec.Add(recorder.Signal, ids.NodeID{}, ids.NodeID{}, hh.sigs[sig].Name, shortName(hh.name))
	}
}
