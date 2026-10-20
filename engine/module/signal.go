package module

import (
	"errors"
	"strconv"

	"aego/contracts/signals"
	"aego/engine"
	"aego/scene"
	"aego/schema"
	"aego/script/abi"
	"aego/script/bind"
)

type signalRef struct {
	typ   schema.TypeID
	index int32
}

var noSignal = signalRef{index: -1}

func (s signalRef) Index() int {
	return int(s.index)
}

func (s signalRef) target(w *engine.World, node scene.NodeHandle) (signals.Hub, signals.Source, bool) {
	if s.index < 0 || w == nil || node.IsZero() {
		return nil, signals.Source{}, false
	}
	h, ok := engine.ServiceOf[signals.Hub](w, signals.ServiceID)
	if !ok {
		return nil, signals.Source{}, false
	}
	src := signals.Source{Kind: signals.KindComponent, A: bind.NodeValue(node).A, B: uint64(s.typ)}
	return h, src, h.Wants(src)
}

type Signal0 struct {
	signalRef
}

func (s Signal0) Emit(w *engine.World, node scene.NodeHandle) {
	if h, src, ok := s.target(w, node); ok {
		h.Emit(src, int(s.index), h.Args(0))
	}
}

type Signal1[A any] struct {
	signalRef
	a bind.Encoder[A]
}

func (s Signal1[A]) Emit(w *engine.World, node scene.NodeHandle, a A) {
	h, src, ok := s.target(w, node)
	if !ok {
		return
	}
	args := h.Args(s.a.Slots())
	s.a.Put(args, a)
	h.Emit(src, int(s.index), args)
}

type Signal2[A, B any] struct {
	signalRef
	a bind.Encoder[A]
	b bind.Encoder[B]
}

func (s Signal2[A, B]) Emit(w *engine.World, node scene.NodeHandle, a A, b B) {
	h, src, ok := s.target(w, node)
	if !ok {
		return
	}
	na := s.a.Slots()
	args := h.Args(na + s.b.Slots())
	s.a.Put(args, a)
	s.b.Put(args[na:], b)
	h.Emit(src, int(s.index), args)
}

type Signal3[A, B, C any] struct {
	signalRef
	a bind.Encoder[A]
	b bind.Encoder[B]
	c bind.Encoder[C]
}

func (s Signal3[A, B, C]) Emit(w *engine.World, node scene.NodeHandle, a A, b B, c C) {
	h, src, ok := s.target(w, node)
	if !ok {
		return
	}
	na := s.a.Slots()
	nb := na + s.b.Slots()
	args := h.Args(nb + s.c.Slots())
	s.a.Put(args, a)
	s.b.Put(args[na:], b)
	s.c.Put(args[nb:], c)
	h.Emit(src, int(s.index), args)
}

func (b *ComponentBuilder[T]) bindings() *bind.Registry {
	if r := b.r.Bindings(); r != nil {
		return r
	}
	return bind.New()
}

func (b *ComponentBuilder[T]) signal(name, doc string, names []string, params ...abi.TypeRef) signalRef {
	d := b.edit()
	if d == nil {
		return noSignal
	}
	switch {
	case !abi.Exported(name):
		b.fail("signal " + name + " of " + d.FullName + " needs an exported name")
		return noSignal
	case len(names) > 0 && len(names) != len(params):
		b.fail("signal " + d.FullName + "." + name + " names " + strconv.Itoa(len(names)) + " parameters, it has " + strconv.Itoa(len(params)))
		return noSignal
	}
	for _, sd := range d.Signals {
		if sd.Name == name {
			b.fail("signal " + d.FullName + "." + name + " is declared twice")
			return noSignal
		}
	}
	for _, m := range d.ScriptMethods {
		if m.Name == name {
			b.fail("signal " + d.FullName + "." + name + " has the name of a script method")
			return noSignal
		}
	}
	ps := make([]string, len(params))
	for i, p := range params {
		if !abi.Plain(p) {
			b.fail("signal " + d.FullName + "." + name + " param " + strconv.Itoa(i) + " has type " + p.String() + ", which cannot be a signal parameter")
			return noSignal
		}
		ps[i] = p.String()
	}
	d.Signals = append(d.Signals, schema.SignalDesc{Name: name, Params: ps, Names: names, Doc: doc})
	return signalRef{typ: d.ID, index: int32(len(d.Signals) - 1)}
}

func (b *ComponentBuilder[T]) encoderFail(name string, err error) {
	b.fail("signal " + name + ": " + err.Error())
}

func (b *ComponentBuilder[T]) Signal0(name, doc string) Signal0 {
	return Signal0{signalRef: b.signal(name, doc, nil)}
}

func (b *ComponentBuilder[T]) Signal1[A any](name, doc string, names ...string) Signal1[A] {
	ea, err := bind.NewEncoder[A](b.bindings())
	if err != nil {
		b.encoderFail(name, err)
		return Signal1[A]{signalRef: noSignal}
	}
	return Signal1[A]{signalRef: b.signal(name, doc, names, ea.Type()), a: ea}
}

func (b *ComponentBuilder[T]) Signal2[A, B any](name, doc string, names ...string) Signal2[A, B] {
	reg := b.bindings()
	ea, err1 := bind.NewEncoder[A](reg)
	eb, err2 := bind.NewEncoder[B](reg)
	if err := errorsJoin(err1, err2); err != nil {
		b.encoderFail(name, err)
		return Signal2[A, B]{signalRef: noSignal}
	}
	return Signal2[A, B]{signalRef: b.signal(name, doc, names, ea.Type(), eb.Type()), a: ea, b: eb}
}

func (b *ComponentBuilder[T]) Signal3[A, B, C any](name, doc string, names ...string) Signal3[A, B, C] {
	reg := b.bindings()
	ea, err1 := bind.NewEncoder[A](reg)
	eb, err2 := bind.NewEncoder[B](reg)
	ec, err3 := bind.NewEncoder[C](reg)
	if err := errorsJoin(err1, err2, err3); err != nil {
		b.encoderFail(name, err)
		return Signal3[A, B, C]{signalRef: noSignal}
	}
	return Signal3[A, B, C]{signalRef: b.signal(name, doc, names, ea.Type(), eb.Type(), ec.Type()), a: ea, b: eb, c: ec}
}

func errorsJoin(errs ...error) error {
	return errors.Join(errs...)
}
