package compiler

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"strings"

	"aego/script/abi"
	"aego/script/bytecode"
	"aego/script/checker"
	"aego/script/diag"
	"aego/script/source"
	"aego/script/types"
)

const (
	LangVersion uint16 = 1
	NodeSlot           = 0
)

type Options struct {
	Debug       bool
	LangVersion uint16
	Sources     *source.Manager
}

type unit struct {
	a       *checker.Analysis
	pkg     *checker.Package
	opt     Options
	mod     *bytecode.Module
	funcs   map[*checker.Func]int
	scripts map[*checker.ScriptType]int
	imports map[string]int
	externs map[string]int
	types   map[*types.Type]int
	diags   []diag.Diagnostic
}

func Compile(a *checker.Analysis, pkg string, opt Options) (*bytecode.Module, []diag.Diagnostic) {
	p := a.Packages[pkg]
	if p == nil {
		return nil, []diag.Diagnostic{diag.Errorf(diag.CompileBlocked, source.Span{}, "package %s was not checked", pkg)}
	}
	if hasErrors(a, p) {
		return nil, []diag.Diagnostic{diag.Errorf(diag.CompileBlocked, source.Span{}, "package %s has errors and was not compiled", pkg)}
	}
	if opt.LangVersion == 0 {
		opt.LangVersion = LangVersion
	}
	u := &unit{
		a: a, pkg: p, opt: opt, mod: bytecode.NewModule(pkg),
		funcs: map[*checker.Func]int{}, scripts: map[*checker.ScriptType]int{},
		imports: map[string]int{}, externs: map[string]int{}, types: map[*types.Type]int{},
	}
	u.mod.LangVersion = opt.LangVersion
	u.mod.Hash = InterfaceHash(a, pkg)
	if a.Surface != nil {
		u.mod.ABIHash = a.Surface.Hash()
	}
	var order []*checker.Func
	for _, f := range p.Funcs {
		u.funcs[f] = len(order)
		order = append(order, f)
	}
	for i, st := range p.Scripts {
		u.scripts[st] = i
		for _, f := range st.Funcs {
			u.funcs[f] = len(order)
			order = append(order, f)
		}
	}
	u.mod.Functions = make([]bytecode.Function, len(order))
	for _, t := range p.Enums {
		u.mod.Enums = append(u.mod.Enums, bytecode.EnumDef{Name: t.Qualified(), Members: append([]string(nil), t.Members...)})
	}
	for _, st := range p.Scripts {
		u.script(st)
	}
	for i, f := range order {
		u.function(f, i)
	}
	if diag.HasErrors(u.diags) {
		return nil, u.diags
	}
	return u.mod, u.diags
}

// hasErrors reports whether p or any package it depends on, transitively, has
// an error. A clean package built against a broken dependency would compile
// against a stale interface and fail only at link time; blocking it here lets
// the importer mark it Invalid with "depends on broken package X". The seen set
// guards against import cycles (which the resolver marks Cyclic but still
// appear in Imports).
func hasErrors(a *checker.Analysis, p *checker.Package) bool {
	seen := map[string]bool{}
	var walk func(pk *checker.Package) bool
	walk = func(pk *checker.Package) bool {
		if pk == nil || seen[pk.Path] {
			return false
		}
		seen[pk.Path] = true
		files := map[source.FileID]bool{}
		for _, f := range pk.Res.Files {
			files[f.ID] = true
		}
		for _, d := range a.Diagnostics {
			if d.Severity == diag.Error && files[d.Primary.File] {
				return true
			}
		}
		for _, dep := range pk.Imports {
			if walk(a.Packages[dep]) {
				return true
			}
		}
		return false
	}
	return walk(p)
}

func funcName(f *checker.Func) string {
	if f.Script != nil {
		return f.Script.Name + "." + f.Name
	}
	return f.Name
}

func paramSlots(f *checker.Func) int {
	n := 0
	for _, p := range f.Params {
		n += p.Type.Slots()
	}
	return n
}

func constValue(k *types.Const, t *types.Type) bytecode.Const {
	switch {
	case t.Kind == types.KFloat || t.Kind == types.KUntypedFloat:
		return bytecode.FloatK(float32(k.Float64()))
	case k.Kind == types.CString:
		return bytecode.StringK(k.Str)
	case k.Kind == types.CBool:
		if k.Bool {
			return bytecode.IntK(1)
		}
		return bytecode.IntK(0)
	}
	return bytecode.IntK(k.Int)
}

func (u *unit) index(f *checker.Func) int32 {
	if f == nil {
		return bytecode.NoScript
	}
	return int32(u.funcs[f])
}

func (u *unit) script(st *checker.ScriptType) {
	d := bytecode.ScriptDef{
		FullName: st.FullName, Former: st.Former, Hidden: st.Hidden, Slots: uint16(st.Slots + 1),
		Ready: u.index(st.Ready), FixedUpdate: u.index(st.FixedUpdate), Update: u.index(st.Update),
	}
	for _, f := range st.Fields {
		h := f.Hints
		fd := bytecode.FieldDef{
			Name: f.Name, Former: f.Former, Type: types.ToABI(f.Type).String(),
			Slot: uint16(f.Slot + 1), Slots: uint8(f.Type.Slots()), Export: f.Export,
			Hints: bytecode.ExportHints{
				Min: h.Min, Max: h.Max, Step: h.Step, HasMin: h.HasMin, HasMax: h.HasMax,
				Unit: h.Unit, Display: h.Display, Tip: h.Tip, Category: h.Category,
			},
		}
		if f.Default != nil {
			fd.HasDefault, fd.Default = true, constValue(f.Default, f.Type)
		}
		f.Type.HeapSlots(f.Slot+1, func(s int) { d.HeapSlots = append(d.HeapSlots, uint16(s)) })
		d.Fields = append(d.Fields, fd)
	}
	u.mod.Scripts = append(u.mod.Scripts, d)
}

func (u *unit) hostImport(module, name string, sig abi.Signature) int {
	key := module + "." + name + sig.String()
	if i, ok := u.imports[key]; ok {
		return i
	}
	i := len(u.mod.Imports)
	u.mod.Imports = append(u.mod.Imports, bytecode.HostImport{Module: module, Name: name, Sig: sig})
	u.imports[key] = i
	return i
}

func (u *unit) extern(f *checker.Func) int {
	key := f.Pkg + "." + f.Name
	if i, ok := u.externs[key]; ok {
		return i
	}
	i := len(u.mod.Externs)
	u.mod.Externs = append(u.mod.Externs, bytecode.Extern{Package: f.Pkg, Name: f.Name, Params: uint8(paramSlots(f)), Results: uint8(f.Result.Slots())})
	u.externs[key] = i
	return i
}

func (u *unit) typeIndex(t *types.Type, sp source.Span) int {
	if i, ok := u.types[t]; ok {
		return i
	}
	el := t.Elem
	// The GC mask is a uint32 (one bit per element slot), so a wider element
	// would lose roots; reject it instead of silently truncating.
	if el.Slots() > bytecode.MaxSliceElemSlots {
		u.diags = append(u.diags, diag.Errorf(diag.SliceTooWide, sp,
			"slice element %s is %d slots, the limit is %d", el, el.Slots(), bytecode.MaxSliceElemSlots))
	}
	var mask uint32
	el.HeapSlots(0, func(s int) {
		if s < 32 {
			mask |= 1 << uint(s)
		}
	})
	i := len(u.mod.Types)
	u.mod.Types = append(u.mod.Types, bytecode.TypeDef{Elem: types.ToABI(el).String(), Width: uint8(min(el.Slots(), 255)), Mask: mask})
	u.types[t] = i
	return i
}

func InterfaceHash(a *checker.Analysis, pkg string) [32]byte {
	var out [32]byte
	p := a.Packages[pkg]
	if p == nil {
		return out
	}
	var lines []string
	for _, o := range p.Consts {
		if o.Exported() && o.Const != nil && o.Type != nil {
			lines = append(lines, "c "+o.Name+" "+o.Type.String()+" "+o.Const.Key())
		}
	}
	for _, t := range p.Structs {
		if !abi.Exported(t.Local) {
			continue
		}
		var b strings.Builder
		b.WriteString("s " + t.Local)
		for _, f := range t.Fields {
			b.WriteString(" " + f.Name + ":" + types.ToABI(f.Type).String())
		}
		lines = append(lines, b.String())
	}
	for _, t := range p.Enums {
		if abi.Exported(t.Local) {
			lines = append(lines, "e "+t.Local+" "+strings.Join(t.Members, ","))
		}
	}
	for _, f := range p.Funcs {
		if !abi.Exported(f.Name) {
			continue
		}
		var b strings.Builder
		b.WriteString("f " + f.Name + "(")
		for i, prm := range f.Params {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(types.ToABI(prm.Type).String())
		}
		b.WriteString(")" + types.ToABI(f.Result).String())
		lines = append(lines, b.String())
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{'\n'})
	}
	copy(out[:], h.Sum(nil))
	return out
}

func CacheKey(sources map[string][]byte, deps map[string][32]byte, abiHash [32]byte, opt Options) [32]byte {
	lang := opt.LangVersion
	if lang == 0 {
		lang = LangVersion
	}
	h := sha256.New()
	var hdr [8]byte
	binary.LittleEndian.PutUint16(hdr[0:], bytecode.Version)
	binary.LittleEndian.PutUint16(hdr[2:], lang)
	if opt.Debug {
		hdr[4] = 1
	}
	h.Write(hdr[:])
	h.Write(abiHash[:])
	names := make([]string, 0, len(sources))
	for n := range sources {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		var l [8]byte
		binary.LittleEndian.PutUint64(l[:], uint64(len(sources[n])))
		h.Write([]byte(n))
		h.Write(l[:])
		h.Write(sources[n])
	}
	dnames := make([]string, 0, len(deps))
	for n := range deps {
		dnames = append(dnames, n)
	}
	sort.Strings(dnames)
	for _, n := range dnames {
		d := deps[n]
		h.Write([]byte(n))
		h.Write(d[:])
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}
