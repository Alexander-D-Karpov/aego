package sdk

import (
	"aego/contracts/scriptevents"
	"aego/engine"
	"aego/script/bind"
)

type (
	Bindings     = bind.Module
	ScriptCtx    = bind.Ctx
	ScriptOption = bind.Option
)

func ScriptDoc(s string) ScriptOption {
	return bind.Doc(s)
}

func ScriptParams(names ...string) ScriptOption {
	return bind.Params(names...)
}

func ScriptExample(s string) ScriptOption {
	return bind.Example(s)
}

func ScriptDescribe(m *Bindings, name string, opts ...ScriptOption) {
	bind.Describe(m, name, opts...)
}

func ScriptModule(r *Registry, path string) *Bindings {
	return r.ScriptModule(path)
}

func Func0[R any](m *Bindings, name string, f func(*ScriptCtx) R, opts ...ScriptOption) {
	bind.Func0(m, name, f, opts...)
}

func Func1[A, R any](m *Bindings, name string, f func(*ScriptCtx, A) R, opts ...ScriptOption) {
	bind.Func1(m, name, f, opts...)
}

func Func2[A, B, R any](m *Bindings, name string, f func(*ScriptCtx, A, B) R, opts ...ScriptOption) {
	bind.Func2(m, name, f, opts...)
}

func Func3[A, B, C, R any](m *Bindings, name string, f func(*ScriptCtx, A, B, C) R, opts ...ScriptOption) {
	bind.Func3(m, name, f, opts...)
}

func Proc0(m *Bindings, name string, f func(*ScriptCtx), opts ...ScriptOption) {
	bind.Proc0(m, name, f, opts...)
}

func Proc1[A any](m *Bindings, name string, f func(*ScriptCtx, A), opts ...ScriptOption) {
	bind.Proc1(m, name, f, opts...)
}

func Proc2[A, B any](m *Bindings, name string, f func(*ScriptCtx, A, B), opts ...ScriptOption) {
	bind.Proc2(m, name, f, opts...)
}

func Proc3[A, B, C any](m *Bindings, name string, f func(*ScriptCtx, A, B, C), opts ...ScriptOption) {
	bind.Proc3(m, name, f, opts...)
}

func ScriptConst[T any](m *Bindings, name string, v T, opts ...ScriptOption) {
	bind.Const(m, name, v, opts...)
}

type (
	Out[T any]   = bind.Out[T]
	ScriptStruct = bind.StructDesc
	ScriptEvent  = scriptevents.Event
	EventID      = scriptevents.EventID
	EventSink    = scriptevents.Sink
)

const EventSinkID = scriptevents.ServiceID

func Struct2Script[T any](m *Bindings, name string, opts ...ScriptOption) *ScriptStruct {
	return bind.Struct[T](m, name, opts...)
}

func (r *Registry) ScriptEvent(ev ScriptEvent) EventID {
	return r.r.ScriptEvent(ev)
}

func EventsOf(ctx *Context) (EventSink, bool) {
	if ctx.World == nil {
		return nil, false
	}
	return engine.ServiceOf[EventSink](ctx.World, EventSinkID)
}
