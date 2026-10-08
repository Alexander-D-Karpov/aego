package sdk

import "aego/script/bind"

type (
	Bindings  = bind.Module
	ScriptCtx = bind.Ctx
)

func ScriptModule(r *Registry, path string) *Bindings {
	if r.Script == nil {
		r.Script = bind.New()
	}
	return r.Script.Module(path)
}

func Func0[R any](m *Bindings, name string, f func(*ScriptCtx) R) {
	bind.Func0(m, name, f)
}

func Func1[A, R any](m *Bindings, name string, f func(*ScriptCtx, A) R) {
	bind.Func1(m, name, f)
}

func Func2[A, B, R any](m *Bindings, name string, f func(*ScriptCtx, A, B) R) {
	bind.Func2(m, name, f)
}

func Func3[A, B, C, R any](m *Bindings, name string, f func(*ScriptCtx, A, B, C) R) {
	bind.Func3(m, name, f)
}

func Proc0(m *Bindings, name string, f func(*ScriptCtx)) {
	bind.Proc0(m, name, f)
}

func Proc1[A any](m *Bindings, name string, f func(*ScriptCtx, A)) {
	bind.Proc1(m, name, f)
}

func Proc2[A, B any](m *Bindings, name string, f func(*ScriptCtx, A, B)) {
	bind.Proc2(m, name, f)
}

func Proc3[A, B, C any](m *Bindings, name string, f func(*ScriptCtx, A, B, C)) {
	bind.Proc3(m, name, f)
}

func ScriptConst[T any](m *Bindings, name string, v T) {
	bind.Const(m, name, v)
}
