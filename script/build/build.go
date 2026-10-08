package build

import (
	"path"
	"path/filepath"
	"sort"
	"strings"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/bytecode"
	"aego/script/checker"
	"aego/script/compiler"
	"aego/script/diag"
	"aego/script/parser"
	"aego/script/source"
)

type File struct {
	Path string
	Src  []byte
}

type Options struct {
	Tests    bool
	Compiler compiler.Options
}

type Result struct {
	Sources     *source.Manager
	Analysis    *checker.Analysis
	Modules     map[string]*bytecode.Module
	Order       []string
	Invalid     []string
	Diagnostics []diag.Diagnostic
}

func (r *Result) HasErrors() bool {
	return diag.HasErrors(r.Diagnostics)
}

func IsTest(p string) bool {
	return strings.HasSuffix(p, "_test.aes")
}

func PackagePath(module, rel string) string {
	rel = path.Clean(filepath.ToSlash(rel))
	if rel == "." || rel == "" {
		return module
	}
	return module + "/" + rel
}

func Build(pkgs map[string][]File, surface abi.Surface, opt Options) *Result {
	if surface == nil {
		t := abi.NewTable()
		t.Freeze()
		surface = t
	}
	sm := source.NewManager()
	res := &Result{Sources: sm, Modules: map[string]*bytecode.Module{}}
	files := map[string][]*ast.File{}
	broken := map[string]bool{}
	names := make([]string, 0, len(pkgs))
	for p := range pkgs {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		list := append([]File(nil), pkgs[p]...)
		sort.Slice(list, func(i, j int) bool { return list[i].Path < list[j].Path })
		for _, f := range list {
			if !opt.Tests && IsTest(f.Path) {
				continue
			}
			af, ds := parser.ParseSource(sm, f.Path, f.Src)
			files[p] = append(files[p], af)
			res.Diagnostics = append(res.Diagnostics, ds...)
			if diag.HasErrors(ds) {
				broken[p] = true
			}
		}
		if len(files[p]) > 0 && surface.Module(p) {
			res.Diagnostics = append(res.Diagnostics, diag.Errorf(diag.ModuleConflict, files[p][0].Sp,
				"package %s has the same path as a native script module", p))
			broken[p] = true
		}
	}
	if len(files) == 0 {
		return res
	}
	a := checker.Check(sm, files, surface, checker.Options{})
	res.Analysis = a
	res.Diagnostics = append(res.Diagnostics, a.Diagnostics...)
	copt := opt.Compiler
	copt.Sources = sm
	for _, p := range a.Order {
		pkg := a.Packages[p]
		if pkg == nil {
			continue
		}
		for _, d := range pkg.Imports {
			if broken[d] {
				broken[p] = true
			}
		}
		if broken[p] {
			continue
		}
		m, ds := compiler.Compile(a, p, copt)
		for _, d := range ds {
			if d.Code != diag.CompileBlocked {
				res.Diagnostics = append(res.Diagnostics, d)
			}
		}
		if m == nil {
			broken[p] = true
			continue
		}
		res.Modules[p] = m
		res.Order = append(res.Order, p)
	}
	for p := range broken {
		if len(files[p]) > 0 {
			res.Invalid = append(res.Invalid, p)
		}
	}
	sort.Strings(res.Invalid)
	diag.Sort(res.Diagnostics)
	return res
}
