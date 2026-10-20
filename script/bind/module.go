package bind

import (
	"fmt"
	"reflect"

	"aego/script/abi"
	"aego/script/vm"
)

type Module struct {
	reg    *Registry
	path   string
	recv   abi.Kind
	method bool
	comp   string
	handle string
}

func (m *Module) Path() string {
	return m.path
}

func (m *Module) Registry() *Registry {
	return m.reg
}

func (m *Module) valid(name string, errs ...error) bool {
	for _, err := range errs {
		if err != nil {
			m.reg.fail(fmt.Errorf("%s.%s: %w", m.path, name, err))
			return false
		}
	}
	return true
}

func (m *Module) add(name string, params []abi.TypeRef, result abi.TypeRef, fn vm.HostFunc, opts []Option) {
	m.addFunc(name, params, result, fn, opts, 0)
}

func (m *Module) addRecv(name string, params []abi.TypeRef, result abi.TypeRef, fn vm.HostFunc, opts []Option) {
	m.addFunc(name, params, result, fn, opts, 1)
}

func (m *Module) addFunc(name string, params []abi.TypeRef, result abi.TypeRef, fn vm.HostFunc, opts []Option, recv int) {
	r := m.reg
	visible := len(params) - recv
	var err error
	switch {
	case m.comp != "" && (recv == 0 || len(params) == 0 || params[0].Kind != abi.NodeRef):
		err = fmt.Errorf("component method needs the %s component as its receiver", m.comp)
	case m.comp != "":
		params = append([]abi.TypeRef{abi.ComponentOf(m.comp)}, params[1:]...)
		err = r.table.AddComponentMethod(m.comp, name, abi.Signature{Params: params[1:], Result: result})
	case m.handle != "" && (len(params) == 0 || params[0].Kind != abi.Handle || params[0].Name != m.handle):
		err = fmt.Errorf("handle method needs a %s receiver as its first parameter", m.handle)
	case m.handle != "":
		visible--
		err = r.table.AddHandleMethod(m.handle, name, abi.Signature{Params: params[1:], Result: result})
	case !m.method:
		err = r.table.AddFunc(m.path, name, abi.Signature{Params: params, Result: result})
	case len(params) == 0 || params[0].Kind != m.recv:
		err = fmt.Errorf("method needs a %s receiver as its first parameter", m.recv)
	default:
		visible--
		err = r.table.AddMethod(m.recv, name, abi.Signature{Params: params[1:], Result: result})
	}
	if !m.valid(name, err) {
		return
	}
	r.hosts[key(m.path, name)] = host{sig: abi.Signature{Params: params, Result: result}, fn: fn}
	m.valid(name, r.describe(m.path, name, opts, visible))
}

func Func0[R any](m *Module, name string, f func(*Ctx) R, opts ...Option) {
	cr, er := codecOf[R](m.reg)
	if !m.valid(name, er) {
		return
	}
	m.add(name, nil, cr.ref, func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		cr.put(mc, w, f(x))
		return x.finish()
	}, opts)
}

func Func1[A, R any](m *Module, name string, f func(*Ctx, A) R, opts ...Option) {
	ca, ea := codecOf[A](m.reg)
	cr, er := codecOf[R](m.reg)
	if !m.valid(name, ea, er) {
		return
	}
	m.add(name, []abi.TypeRef{ca.ref}, cr.ref, func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		a := ca.get(mc, w)
		cr.put(mc, w, f(x, a))
		return x.finish()
	}, opts)
}

func Func2[A, B, R any](m *Module, name string, f func(*Ctx, A, B) R, opts ...Option) {
	ca, ea := codecOf[A](m.reg)
	cb, eb := codecOf[B](m.reg)
	cr, er := codecOf[R](m.reg)
	if !m.valid(name, ea, eb, er) {
		return
	}
	ob := ca.slots
	m.add(name, []abi.TypeRef{ca.ref, cb.ref}, cr.ref, func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		a, b := ca.get(mc, w), cb.get(mc, w[ob:])
		cr.put(mc, w, f(x, a, b))
		return x.finish()
	}, opts)
}

func Func3[A, B, C, R any](m *Module, name string, f func(*Ctx, A, B, C) R, opts ...Option) {
	ca, ea := codecOf[A](m.reg)
	cb, eb := codecOf[B](m.reg)
	cc, ec := codecOf[C](m.reg)
	cr, er := codecOf[R](m.reg)
	if !m.valid(name, ea, eb, ec, er) {
		return
	}
	ob, oc := ca.slots, ca.slots+cb.slots
	m.add(name, []abi.TypeRef{ca.ref, cb.ref, cc.ref}, cr.ref, func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		a, b, c := ca.get(mc, w), cb.get(mc, w[ob:]), cc.get(mc, w[oc:])
		cr.put(mc, w, f(x, a, b, c))
		return x.finish()
	}, opts)
}

func Proc0(m *Module, name string, f func(*Ctx), opts ...Option) {
	m.add(name, nil, abi.T(abi.Void), func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		f(x)
		return x.finish()
	}, opts)
}

func Proc1[A any](m *Module, name string, f func(*Ctx, A), opts ...Option) {
	ca, ea := codecOf[A](m.reg)
	if !m.valid(name, ea) {
		return
	}
	m.add(name, []abi.TypeRef{ca.ref}, abi.T(abi.Void), func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		f(x, ca.get(mc, w))
		return x.finish()
	}, opts)
}

func Proc2[A, B any](m *Module, name string, f func(*Ctx, A, B), opts ...Option) {
	ca, ea := codecOf[A](m.reg)
	cb, eb := codecOf[B](m.reg)
	if !m.valid(name, ea, eb) {
		return
	}
	ob := ca.slots
	m.add(name, []abi.TypeRef{ca.ref, cb.ref}, abi.T(abi.Void), func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		f(x, ca.get(mc, w), cb.get(mc, w[ob:]))
		return x.finish()
	}, opts)
}

func Proc3[A, B, C any](m *Module, name string, f func(*Ctx, A, B, C), opts ...Option) {
	ca, ea := codecOf[A](m.reg)
	cb, eb := codecOf[B](m.reg)
	cc, ec := codecOf[C](m.reg)
	if !m.valid(name, ea, eb, ec) {
		return
	}
	ob, oc := ca.slots, ca.slots+cb.slots
	m.add(name, []abi.TypeRef{ca.ref, cb.ref, cc.ref}, abi.T(abi.Void), func(mc *vm.Machine, w []vm.Value) error {
		x, err := ctxOf(mc)
		if err != nil {
			return err
		}
		f(x, ca.get(mc, w), cb.get(mc, w[ob:]), cc.get(mc, w[oc:]))
		return x.finish()
	}, opts)
}

func Const[T any](m *Module, name string, v T, opts ...Option) {
	c, err := codecOf[T](m.reg)
	if !m.valid(name, err) {
		return
	}
	if m.method {
		m.valid(name, fmt.Errorf("constants cannot live in a method module"))
		return
	}
	k := abi.Const{Type: c.ref}
	rv := reflect.ValueOf(v)
	switch c.ref.Kind {
	case abi.Bool:
		k.Bool = rv.Bool()
	case abi.Int, abi.Enum:
		if rv.CanInt() {
			k.Int = rv.Int()
		} else {
			k.Int = int64(rv.Uint())
		}
	case abi.Float:
		k.Float = rv.Float()
	case abi.String:
		k.Str = rv.String()
	default:
		m.valid(name, fmt.Errorf("constants of type %s are not allowed", c.ref))
		return
	}
	if m.valid(name, m.reg.table.AddConst(m.path, name, k)) {
		m.valid(name, m.reg.describe(m.path, name, opts, -1))
	}
}

func Member[T any](name string, v T) abi.EnumMember {
	rv := reflect.ValueOf(v)
	if rv.CanInt() {
		return abi.EnumMember{Name: name, Value: rv.Int()}
	}
	return abi.EnumMember{Name: name, Value: int64(rv.Uint())}
}

func Enum[T any](m *Module, name string, members ...abi.EnumMember) {
	rt := reflect.TypeFor[T]()
	switch {
	case m.method:
		m.valid(name, fmt.Errorf("enums cannot live in a method module"))
		return
	case rt.PkgPath() == "":
		m.valid(name, fmt.Errorf("enum %s must be a named Go type", rt))
		return
	}
	switch rt.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint8, reflect.Uint16, reflect.Uint32:
	default:
		m.valid(name, fmt.Errorf("enum %s must have an integer underlying type", rt))
		return
	}
	def := &abi.EnumDef{Module: m.path, Name: name, Members: members}
	if !m.valid(name, m.reg.table.AddEnum(def)) {
		return
	}
	m.reg.enums[rt] = def.Qualified()
	m.valid(name, m.reg.describe(m.path, name, nil, -1))
}
