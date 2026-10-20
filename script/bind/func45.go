package bind

import (
	"aego/script/abi"
	"aego/script/vm"
)

func Func4[A, B, C, D, R any](m *Module, name string, f func(*Ctx, A, B, C, D) R, opts ...Option) {
	ca, ea := codecOf[A](m.reg)
	cb, eb := codecOf[B](m.reg)
	cc, ec := codecOf[C](m.reg)
	cd, ed := codecOf[D](m.reg)
	cr, er := codecOf[R](m.reg)
	if !m.valid(name, ea, eb, ec, ed, er) {
		return
	}
	ob := ca.slots
	oc := ob + cb.slots
	od := oc + cc.slots
	m.add(name, []abi.TypeRef{ca.ref, cb.ref, cc.ref, cd.ref}, cr.ref, func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		a, b, c, d := ca.get(mc, w), cb.get(mc, w[ob:]), cc.get(mc, w[oc:]), cd.get(mc, w[od:])
		cr.put(mc, w, f(x, a, b, c, d))
		return x.finish()
	}, opts)
}

func Func5[A, B, C, D, E, R any](m *Module, name string, f func(*Ctx, A, B, C, D, E) R, opts ...Option) {
	ca, ea := codecOf[A](m.reg)
	cb, eb := codecOf[B](m.reg)
	cc, ec := codecOf[C](m.reg)
	cd, ed := codecOf[D](m.reg)
	ce, ee := codecOf[E](m.reg)
	cr, er := codecOf[R](m.reg)
	if !m.valid(name, ea, eb, ec, ed, ee, er) {
		return
	}
	ob := ca.slots
	oc := ob + cb.slots
	od := oc + cc.slots
	oe := od + cd.slots
	m.add(name, []abi.TypeRef{ca.ref, cb.ref, cc.ref, cd.ref, ce.ref}, cr.ref, func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		a, b, c := ca.get(mc, w), cb.get(mc, w[ob:]), cc.get(mc, w[oc:])
		d, e := cd.get(mc, w[od:]), ce.get(mc, w[oe:])
		cr.put(mc, w, f(x, a, b, c, d, e))
		return x.finish()
	}, opts)
}
