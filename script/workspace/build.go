package workspace

import (
	"maps"
	"sort"
	"strings"

	"aego/script/compiler"
	"aego/script/diag"
	"aego/script/source"
)

type BuildOptions struct {
	IncludeTests bool
}

func IsTest(path string) bool {
	return strings.HasSuffix(path, "_test"+Ext)
}

func (w *Workspace) Build() (*Generation, []diag.Diagnostic) {
	return w.BuildWith(BuildOptions{})
}

func (w *Workspace) BuildWith(opt BuildOptions) (*Generation, []diag.Diagnostic) {
	if opt.IncludeTests {
		return w.buildTests()
	}
	return w.commit()
}

func (w *Workspace) buildTests() (*Generation, []diag.Diagnostic) {
	w.bmu.Lock()
	defer w.bmu.Unlock()
	w.mu.Lock()
	in := w.inputs(false, true)
	tab, module, debug := w.table(), w.cfg.Module, w.cfg.Debug
	w.mu.Unlock()

	a, _ := in.analyze(tab, module)
	ds := append([]diag.Diagnostic(nil), a.Diagnostics...)
	g := newGeneration()
	g.Sources, g.ABIHash = in.sm, tab.Hash()
	copt := compiler.Options{Sources: in.sm, Debug: debug}
	for _, p := range a.Order {
		mod, cds := compiler.Compile(a, p, copt)
		if mod == nil {
			g.Failed[p] = cds
			continue
		}
		g.Modules[p] = mod
		g.Hashes[p] = compiler.InterfaceHash(a, p)
		g.Changed = append(g.Changed, p)
	}
	diag.Sort(ds)
	return g, ds
}

func blocked(pkg, why string) []diag.Diagnostic {
	return []diag.Diagnostic{diag.Errorf(diag.CompileBlocked, source.Span{}, "package %s %s", pkg, why)}
}

func (w *Workspace) commit() (*Generation, []diag.Diagnostic) {
	w.bmu.Lock()
	defer w.bmu.Unlock()
	w.mu.Lock()
	in := w.inputs(false, false)
	tab, module, debug := w.table(), w.cfg.Module, w.cfg.Debug
	prev := w.committed
	w.mu.Unlock()

	a, _ := in.analyze(tab, module)
	ds := append([]diag.Diagnostic(nil), a.Diagnostics...)
	abiHash := tab.Hash()

	broken := map[string]bool{}
	errsOf := map[string][]diag.Diagnostic{}
	for _, d := range a.Diagnostics {
		if d.Severity != diag.Error {
			continue
		}
		if pkg, ok := in.pkgOf[d.Primary.File]; ok {
			broken[pkg] = true
			errsOf[pkg] = append(errsOf[pkg], d)
		}
	}

	closure := map[string]map[string]bool{}
	var deps func(string) map[string]bool
	deps = func(p string) map[string]bool {
		if c, ok := closure[p]; ok {
			return c
		}
		c := map[string]bool{}
		closure[p] = c
		if cp := a.Packages[p]; cp != nil {
			for _, d := range cp.Imports {
				c[d] = true
				for x := range deps(d) {
					c[x] = true
				}
			}
		}
		return c
	}

	iface := make(map[string][32]byte, len(a.Order))
	for _, p := range a.Order {
		iface[p] = compiler.InterfaceHash(a, p)
	}
	opt := compiler.Options{Sources: in.sm, Debug: debug}
	next := map[string]built{}
	failed := map[string][]diag.Diagnostic{}
	for _, p := range a.Order {
		dh := map[string][32]byte{}
		for d := range deps(p) {
			dh[d] = iface[d]
		}
		key := compiler.CacheKey(in.srcs[p], dh, abiHash, opt)
		if old, ok := w.state[p]; ok && old.key == key {
			continue
		}
		if broken[p] {
			failed[p] = errsOf[p]
			continue
		}
		if dep := firstBroken(deps(p), broken); dep != "" {
			failed[p] = blocked(p, "depends on broken package "+dep)
			continue
		}
		mod, cds := compiler.Compile(a, p, opt)
		var extra []diag.Diagnostic
		for _, d := range cds {
			if d.Code != diag.CompileBlocked {
				extra = append(extra, d)
			}
		}
		ds = append(ds, extra...)
		if mod == nil {
			if len(extra) == 0 {
				extra = cds
			}
			failed[p] = extra
			continue
		}
		next[p] = built{key: key, iface: iface[p], mod: mod}
	}

	changedIface := func(p string) bool {
		old, ok := w.state[p]
		return !ok || old.iface != iface[p]
	}
	for again := true; again; {
		again = false
		for _, p := range sortedKeys(next) {
			for _, h := range sortedKeys(failed) {
				if _, live := prev.Modules[h]; !live {
					continue
				}
				if (deps(p)[h] && changedIface(h)) || (deps(h)[p] && changedIface(p)) {
					delete(next, p)
					failed[p] = blocked(p, "is held back until "+h+" compiles")
					again = true
					break
				}
			}
		}
	}

	var removed []string
	for p := range w.state {
		if _, ok := a.Packages[p]; !ok {
			removed = append(removed, p)
		}
	}
	diag.Sort(ds)

	w.mu.Lock()
	defer w.mu.Unlock()
	if len(next) == 0 && len(removed) == 0 {
		g := *prev
		g.Changed, g.Failed, g.Sources = nil, failed, in.sm
		w.committed = &g
		return &g, ds
	}
	g := &Generation{
		Modules: maps.Clone(prev.Modules),
		Hashes:  maps.Clone(prev.Hashes),
		ABIHash: abiHash,
		Failed:  failed,
		Sources: in.sm,
	}
	for p, b := range next {
		w.state[p] = b
		g.Modules[p] = b.mod
		g.Hashes[p] = b.key
		g.Changed = append(g.Changed, p)
	}
	for _, p := range removed {
		delete(w.state, p)
		delete(g.Modules, p)
		delete(g.Hashes, p)
		g.Changed = append(g.Changed, p)
	}
	sort.Strings(g.Changed)
	w.nextID++
	g.ID = w.nextID
	w.committed = g
	return g, ds
}

func firstBroken(set map[string]bool, broken map[string]bool) string {
	for _, d := range sortedKeys(set) {
		if broken[d] {
			return d
		}
	}
	return ""
}
