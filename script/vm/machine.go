package vm

import (
	"errors"
	"fmt"
	"runtime/debug"
	"strconv"
	"time"

	"aego/script/bytecode"
	"aego/script/verify"
)

const (
	CodeBudget      = "AES6001"
	CodeNativePanic = "AES6002"
	CodeCallDepth   = "AES6003"
	CodeNil         = "AES6004"
	CodeIndex       = "AES6005"
	CodeDivZero     = "AES6006"
	CodeBadHandle   = "AES6007"
	CodeExplicit    = "AES6008"
	CodeLink        = "AES6009"
	CodeHost        = "AES6010"
)

type FrameInfo struct {
	Function string
	File     string
	Line     int
}

type Fault struct {
	Code    string
	Message string
	Detail  string
	Stack   []FrameInfo
}

func (f *Fault) Error() string {
	s := f.Code + ": " + f.Message
	if len(f.Stack) > 0 {
		top := f.Stack[0]
		s += " (" + top.Function + " at " + top.File + ":" + strconv.Itoa(top.Line) + ")"
	}
	return s
}

type HostFunc func(m *Machine, w []Value) error

type Components interface {
	GetField(ref Value, field int, dst *Value) error
	SetField(ref Value, field int, src Value) error
}

type Options struct {
	MaxCallDepth int
	Budget       uint64
	InitialStack int
	GCThreshold  int
}

func (o Options) withDefaults() Options {
	if o.MaxCallDepth <= 0 {
		o.MaxCallDepth = 256
	}
	if o.Budget == 0 {
		o.Budget = 100_000
	}
	if o.InitialStack <= 0 {
		o.InitialStack = 4096
	}
	if o.GCThreshold <= 0 {
		o.GCThreshold = 1 << 20
	}
	return o
}

type Stats struct {
	Instructions uint64
	Calls        uint64
	HostCalls    uint64
	Faults       uint64
	BudgetAborts uint64
	GCRuns       uint64
	GCTime       time.Duration
	HeapObjects  int
	HeapBytes    int
}

type Func struct {
	Name    string
	File    string
	Params  int
	Results int
	Regs    int
	Script  int
	code    []bytecode.Instr
	consts  []Value
	mod     *Module
	src     *bytecode.Function
}

type sliceType struct {
	width int
	mask  uint32
}

type Module struct {
	Package string
	Funcs   []*Func
	src     *bytecode.Module
	hosts   []HostFunc
	windows []int
	externs []*Func
	types   []sliceType
}

func (m *Module) Source() *bytecode.Module {
	return m.src
}

func (m *Module) Func(name string) *Func {
	for _, f := range m.Funcs {
		if f.Name == name {
			return f
		}
	}
	return nil
}

type frame struct {
	f    *Func
	base int
	pc   int
}

type Machine struct {
	opt      Options
	heap     Heap
	statics  []string
	interned map[string]Value
	stack    []Value
	frames   []frame
	self     []Value
	left     uint64
	fault    *Fault
	marker   Marker
	stats    Stats
	Env      any
	Comps    Components
}

var errPanic = errors.New("native binding panicked")

func New(opt Options) *Machine {
	opt = opt.withDefaults()
	return &Machine{
		opt:      opt,
		interned: map[string]Value{},
		stack:    make([]Value, opt.InitialStack),
		frames:   make([]frame, 0, opt.MaxCallDepth),
	}
}

func (m *Machine) Options() Options {
	return m.opt
}

func (m *Machine) Intern(s string) Value {
	if s == "" {
		return Value{}
	}
	if v, ok := m.interned[s]; ok {
		return v
	}
	v := Value{A: staticBit | uint64(len(m.statics))}
	m.statics = append(m.statics, s)
	m.interned[s] = v
	return v
}

func (m *Machine) NewString(s string) Value {
	if s == "" {
		return Value{}
	}
	return m.heap.newString(s)
}

func (m *Machine) Str(v Value) (string, bool) {
	switch {
	case v.A == 0:
		return "", true
	case v.A&staticBit != 0:
		if i := v.A &^ staticBit; i < uint64(len(m.statics)) {
			return m.statics[i], true
		}
		return "", false
	}
	o := m.heap.get(v.A)
	if o == nil || !o.str {
		return "", false
	}
	return o.s, true
}

func (m *Machine) Load(src *bytecode.Module, resolve func(bytecode.HostImport) (HostFunc, bool)) (*Module, error) {
	if ds := verify.Module(src); len(ds) > 0 {
		return nil, fmt.Errorf("module %s failed verification: %s", src.Package, ds[0].Message)
	}
	mod := &Module{Package: src.Package, src: src, externs: make([]*Func, len(src.Externs))}
	for _, imp := range src.Imports {
		var h HostFunc
		ok := false
		if resolve != nil {
			h, ok = resolve(imp)
		}
		if !ok || h == nil {
			return nil, fmt.Errorf("module %s imports unknown host function %s.%s %s", src.Package, imp.Module, imp.Name, imp.Sig)
		}
		mod.hosts = append(mod.hosts, h)
		mod.windows = append(mod.windows, imp.Sig.Window())
	}
	for _, t := range src.Types {
		mod.types = append(mod.types, sliceType{width: int(t.Width), mask: t.Mask})
	}
	for i := range src.Functions {
		sf := &src.Functions[i]
		f := &Func{
			Name: sf.Name, File: sf.File, Params: int(sf.Params), Results: int(sf.Results),
			Regs: int(sf.Registers), Script: int(sf.Script), code: sf.Code, mod: mod, src: sf,
			consts: make([]Value, len(sf.Consts)),
		}
		for j, k := range sf.Consts {
			if k.Kind == bytecode.KString {
				f.consts[j] = m.Intern(k.Str)
			} else {
				f.consts[j] = Value{A: k.A, B: k.B}
			}
		}
		mod.Funcs = append(mod.Funcs, f)
	}
	return mod, nil
}

func Link(mods []*Module) error {
	byPkg := make(map[string]*Module, len(mods))
	for _, mod := range mods {
		byPkg[mod.Package] = mod
	}
	for _, mod := range mods {
		for i, e := range mod.src.Externs {
			target := byPkg[e.Package]
			if target == nil {
				return fmt.Errorf("%s calls %s.%s, but package %s is not loaded", mod.Package, e.Package, e.Name, e.Package)
			}
			f := target.Func(e.Name)
			if f == nil || f.Script >= 0 {
				return fmt.Errorf("%s calls %s.%s, which is not a package function", mod.Package, e.Package, e.Name)
			}
			if f.Params != int(e.Params) || f.Results != int(e.Results) {
				return fmt.Errorf("%s calls %s.%s with a stale signature", mod.Package, e.Package, e.Name)
			}
			mod.externs[i] = f
		}
	}
	return nil
}

func (m *Machine) Busy() bool {
	return len(m.frames) != 0
}

func (m *Machine) Stats() Stats {
	s := m.stats
	s.HeapObjects = m.heap.live
	s.HeapBytes = m.heap.bytes
	return s
}

func (m *Machine) grow(n int) {
	size := 2 * len(m.stack)
	if size < n {
		size = n
	}
	s := make([]Value, size)
	copy(s, m.stack)
	m.stack = s
}

func (m *Machine) Call(f *Func, self []Value, args []Value) ([]Value, *Fault) {
	if len(m.frames) != 0 {
		return nil, &Fault{Code: CodeLink, Message: "the machine is already running a call"}
	}
	if f == nil || len(args) != f.Params {
		return nil, &Fault{Code: CodeLink, Message: "function is missing or the argument count does not match"}
	}
	if f.Regs > len(m.stack) {
		m.grow(f.Regs)
	}
	copy(m.stack, args)
	clear(m.stack[len(args):f.Regs])
	m.self = self
	m.left = m.opt.Budget
	m.frames = append(m.frames, frame{f: f})
	m.stats.Calls++
	m.run()
	m.stats.Instructions += m.opt.Budget - m.left
	m.frames = m.frames[:0]
	m.self = nil
	if fl := m.fault; fl != nil {
		m.fault = nil
		return nil, fl
	}
	return m.stack[:f.Results], nil
}

func (m *Machine) raise(code, msg string) {
	if m.fault != nil {
		return
	}
	fl := &Fault{Code: code, Message: msg}
	for i := len(m.frames) - 1; i >= 0; i-- {
		fr := m.frames[i]
		pc := max(fr.pc-1, 0)
		fl.Stack = append(fl.Stack, FrameInfo{Function: fr.f.Name, File: fr.f.File, Line: fr.f.src.Line(pc)})
	}
	m.fault = fl
	m.stats.Faults++
}

func (m *Machine) trap(top, pc int, code, msg string) {
	m.frames[top].pc = pc
	m.raise(code, msg)
}

func (m *Machine) host(h HostFunc, w []Value) (err error) {
	defer func() {
		if r := recover(); r != nil {
			m.raise(CodeNativePanic, fmt.Sprint("native binding panicked: ", r))
			m.fault.Detail = string(debug.Stack())
			err = errPanic
		}
	}()
	return h(m, w)
}
