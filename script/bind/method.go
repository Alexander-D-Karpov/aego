package bind

import (
	"errors"
	"reflect"

	"aego/schema"
	"aego/script/abi"
	"aego/script/vm"
)

var errMethodModule = errors.New("component methods cannot live in a method module")

func missing[T any](name string) string {
	return name + ": the " + reflect.TypeFor[T]().Name() + " reference is nil, its node is destroyed or the component was removed"
}

func receiver[T any](mc *vm.Machine, w []vm.Value, typ schema.TypeID, miss string) (*Ctx, *T, error) {
	x, err := ctxOf(mc)
	if err != nil {
		return nil, nil, err
	}
	if x.Scene == nil {
		return nil, nil, errNoTarget
	}
	h := NodeOf(w[0])
	if !x.Scene.IsAlive(h) {
		return nil, nil, &vm.Fault{Code: vm.CodeNil, Message: miss}
	}
	p, ok := x.Scene.GetByType(h, typ)
	if !ok {
		return nil, nil, &vm.Fault{Code: vm.CodeNil, Message: miss}
	}
	x.Recv = h
	return x, (*T)(p), nil
}

func (m *Module) component(name string) bool {
	if m.method {
		m.valid(name, errMethodModule)
		return false
	}
	return true
}

func Guard(m *Module, fn func()) error {
	before := len(m.reg.errs)
	fn()
	if len(m.reg.errs) == before {
		return nil
	}
	err := errors.Join(m.reg.errs[before:]...)
	m.reg.errs = m.reg.errs[:before]
	return err
}

func nodeParams(rest ...abi.TypeRef) []abi.TypeRef {
	return append([]abi.TypeRef{abi.T(abi.NodeRef)}, rest...)
}

func Method0[T, R any](m *Module, name string, typ schema.TypeID, f func(*Ctx, *T) R, opts ...Option) {
	cr, er := codecOf[R](m.reg)
	if !m.component(name) || !m.valid(name, er) {
		return
	}
	miss := missing[T](name)
	m.addRecv(name, nodeParams(), cr.ref, func(mc *vm.Machine, w []vm.Value) error {
		x, p, err := receiver[T](mc, w, typ, miss)
		if p == nil {
			return err
		}
		cr.put(mc, w, f(x, p))
		return x.finish()
	}, opts)
}

func Method1[T, A, R any](m *Module, name string, typ schema.TypeID, f func(*Ctx, *T, A) R, opts ...Option) {
	ca, ea := codecOf[A](m.reg)
	cr, er := codecOf[R](m.reg)
	if !m.component(name) || !m.valid(name, ea, er) {
		return
	}
	miss := missing[T](name)
	m.addRecv(name, nodeParams(ca.ref), cr.ref, func(mc *vm.Machine, w []vm.Value) error {
		x, p, err := receiver[T](mc, w, typ, miss)
		if p == nil {
			return err
		}
		a := ca.get(mc, w[1:])
		cr.put(mc, w, f(x, p, a))
		return x.finish()
	}, opts)
}

func Method2[T, A, B, R any](m *Module, name string, typ schema.TypeID, f func(*Ctx, *T, A, B) R, opts ...Option) {
	ca, ea := codecOf[A](m.reg)
	cb, eb := codecOf[B](m.reg)
	cr, er := codecOf[R](m.reg)
	if !m.component(name) || !m.valid(name, ea, eb, er) {
		return
	}
	miss := missing[T](name)
	ob := 1 + ca.slots
	m.addRecv(name, nodeParams(ca.ref, cb.ref), cr.ref, func(mc *vm.Machine, w []vm.Value) error {
		x, p, err := receiver[T](mc, w, typ, miss)
		if p == nil {
			return err
		}
		a, b := ca.get(mc, w[1:]), cb.get(mc, w[ob:])
		cr.put(mc, w, f(x, p, a, b))
		return x.finish()
	}, opts)
}

func MethodProc0[T any](m *Module, name string, typ schema.TypeID, f func(*Ctx, *T), opts ...Option) {
	if !m.component(name) {
		return
	}
	miss := missing[T](name)
	m.addRecv(name, nodeParams(), abi.T(abi.Void), func(mc *vm.Machine, w []vm.Value) error {
		x, p, err := receiver[T](mc, w, typ, miss)
		if p == nil {
			return err
		}
		f(x, p)
		return x.finish()
	}, opts)
}

func MethodProc1[T, A any](m *Module, name string, typ schema.TypeID, f func(*Ctx, *T, A), opts ...Option) {
	ca, ea := codecOf[A](m.reg)
	if !m.component(name) || !m.valid(name, ea) {
		return
	}
	miss := missing[T](name)
	m.addRecv(name, nodeParams(ca.ref), abi.T(abi.Void), func(mc *vm.Machine, w []vm.Value) error {
		x, p, err := receiver[T](mc, w, typ, miss)
		if p == nil {
			return err
		}
		f(x, p, ca.get(mc, w[1:]))
		return x.finish()
	}, opts)
}

func MethodProc2[T, A, B any](m *Module, name string, typ schema.TypeID, f func(*Ctx, *T, A, B), opts ...Option) {
	ca, ea := codecOf[A](m.reg)
	cb, eb := codecOf[B](m.reg)
	if !m.component(name) || !m.valid(name, ea, eb) {
		return
	}
	miss := missing[T](name)
	ob := 1 + ca.slots
	m.addRecv(name, nodeParams(ca.ref, cb.ref), abi.T(abi.Void), func(mc *vm.Machine, w []vm.Value) error {
		x, p, err := receiver[T](mc, w, typ, miss)
		if p == nil {
			return err
		}
		f(x, p, ca.get(mc, w[1:]), cb.get(mc, w[ob:]))
		return x.finish()
	}, opts)
}
