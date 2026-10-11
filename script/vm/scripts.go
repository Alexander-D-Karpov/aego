package vm

import (
	"fmt"
	"strings"

	"aego/script/bytecode"
)

type Scripts interface {
	Find(node Value, script string) (Value, error)
	Instance(ref Value, script string) ([]Value, InstanceID, bool)
}

type Signals interface {
	Emit(src InstanceID, sig int, args []Value) error
	Connect(src, dst Value, c *Connect) (Value, error)
	Disconnect(c Value)
	Connected(c Value) bool
}

type Connect struct {
	Source     string
	Signal     int
	SignalName string
	Target     string
	Handler    *Func
}

type xfield struct {
	script string
	slot   int
	width  int
}

type xmethod struct {
	script string
	fn     *Func
}

type scriptLoc struct {
	mod *Module
	idx int
	def *bytecode.ScriptDef
}

func shortName(full string) string {
	return full[strings.LastIndexByte(full, '.')+1:]
}

func (l scriptLoc) method(name string) *Func {
	want := shortName(l.def.FullName) + "." + name
	for _, f := range l.mod.Funcs {
		if f.Script == l.idx && f.Name == want {
			return f
		}
	}
	return nil
}

func (l scriptLoc) lifecycle(f *Func) bool {
	for _, i := range [...]int32{l.def.Ready, l.def.FixedUpdate, l.def.Update, l.def.Reloaded} {
		if i >= 0 && int(i) < len(l.mod.Funcs) && l.mod.Funcs[i] == f {
			return true
		}
	}
	return false
}

func (l scriptLoc) callable(f *Func) bool {
	return f != nil && !f.Coroutine && !l.lifecycle(f)
}

func (mod *Module) ScriptMethod(script int, name string) *Func {
	if script < 0 || script >= len(mod.src.Scripts) {
		return nil
	}
	return scriptLoc{mod: mod, idx: script, def: &mod.src.Scripts[script]}.method(name)
}

func bindScripts(mods []*Module) error {
	index := map[string]scriptLoc{}
	for _, mod := range mods {
		for i := range mod.src.Scripts {
			d := &mod.src.Scripts[i]
			index[d.FullName] = scriptLoc{mod: mod, idx: i, def: d}
		}
	}
	for _, mod := range mods {
		src := mod.src
		target := func(name string) (scriptLoc, error) {
			l, ok := index[name]
			if !ok {
				return l, fmt.Errorf("%s refers to script %s, which is not loaded", mod.Package, name)
			}
			return l, nil
		}
		mod.xfields = make([]xfield, len(src.XFields))
		for i, x := range src.XFields {
			l, err := target(src.ScriptRefs[x.Ref])
			if err != nil {
				return err
			}
			fd := l.def.Field(x.Name)
			if fd == nil || fd.Type != x.Type || fd.Slots != x.Slots {
				return fmt.Errorf("%s uses field %s.%s with a stale layout", mod.Package, l.def.FullName, x.Name)
			}
			mod.xfields[i] = xfield{script: l.def.FullName, slot: int(fd.Slot), width: int(fd.Slots)}
		}
		mod.xmethods = make([]xmethod, len(src.XMethods))
		for i, x := range src.XMethods {
			l, err := target(src.ScriptRefs[x.Ref])
			if err != nil {
				return err
			}
			f := l.method(x.Name)
			if !l.callable(f) || f.Params != int(x.Params) || f.Results != int(x.Results) {
				return fmt.Errorf("%s calls %s.%s through a ScriptRef, but it is missing, a lifecycle method, a coroutine or has another signature", mod.Package, l.def.FullName, x.Name)
			}
			mod.xmethods[i] = xmethod{script: l.def.FullName, fn: f}
		}
		mod.connects = make([]Connect, len(src.Connects))
		for i, c := range src.Connects {
			sl, err := target(c.Source)
			if err != nil {
				return err
			}
			tl, err := target(c.Target)
			if err != nil {
				return err
			}
			sig := sl.def.Signal(c.Signal)
			if sig < 0 {
				return fmt.Errorf("%s connects to signal %s.%s, which does not exist", mod.Package, c.Source, c.Signal)
			}
			h := tl.method(c.Handler)
			if !tl.callable(h) || h.Results != 0 || h.Params != int(sl.def.Signals[sig].Slots) {
				return fmt.Errorf("%s connects %s.%s to %s.%s, which cannot handle it", mod.Package, c.Source, c.Signal, c.Target, c.Handler)
			}
			mod.connects[i] = Connect{Source: c.Source, Signal: sig, SignalName: c.Signal, Target: c.Target, Handler: h}
		}
	}
	return nil
}

func (m *Machine) Invoke(f *Func, self []Value, args []Value, owner InstanceID) *Fault {
	if f == nil || f.Coroutine || len(args) != f.Params {
		return &Fault{Code: CodeLink, Message: "handler is missing, is a coroutine or its argument count does not match"}
	}
	if len(m.frames) == 0 {
		_, fl := m.CallAs(f, self, args, owner)
		return fl
	}
	if len(m.frames) >= m.opt.MaxCallDepth {
		return &Fault{Code: CodeCallDepth, Message: "call depth exceeds " + fmt.Sprint(m.opt.MaxCallDepth)}
	}
	last := &m.frames[len(m.frames)-1]
	nb := last.base + last.f.Regs
	if need := nb + f.Regs; need > len(m.stack) {
		m.grow(need)
	}
	copy(m.stack[nb:], args)
	clear(m.stack[nb+len(args) : nb+f.Regs])
	floor := len(m.frames)
	m.frames = append(m.frames, frame{f: f, base: nb, ret: nb, self: self, owner: owner})
	m.stats.Calls++
	m.run(floor)
	fl := m.fault
	if fl != nil {
		m.fault = nil
		m.unwind(floor)
	}
	return fl
}
