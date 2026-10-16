package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Config struct {
	Root       string
	Catalog    *Catalog // nil skips the semantic pass
	Exceptions string   // default: <Root>/tools/depcheck/exceptions.txt
}

type Violation struct {
	Variants []string
	Rule     string
	Where    string
	Detail   string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s: [%s] %s (%s)", v.Where, v.Rule, v.Detail, strings.Join(v.Variants, ","))
}

type collector struct {
	module string
	exc    []*exception
	byKey  map[string]*Violation
}

func (c *collector) add(variant, rule, where, detail string) {
	key := rule + "\x00" + where + "\x00" + detail
	if v, ok := c.byKey[key]; ok {
		for _, x := range v.Variants {
			if x == variant {
				return
			}
		}
		v.Variants = append(v.Variants, variant)
		return
	}
	c.byKey[key] = &Violation{Variants: []string{variant}, Rule: rule, Where: where, Detail: detail}
}

func (c *collector) list() []Violation {
	out := make([]Violation, 0, len(c.byKey))
	for _, v := range c.byKey {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Where != out[j].Where {
			return out[i].Where < out[j].Where
		}
		return out[i].Rule+out[i].Detail < out[j].Rule+out[j].Detail
	})
	return out
}

func Check(cfg Config) ([]Violation, error) {
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, err
	}
	module, err := modulePath(root)
	if err != nil {
		return nil, err
	}
	excFile := cfg.Exceptions
	if excFile == "" {
		excFile = filepath.Join(root, exceptionsFile)
	}
	exc, bad, err := loadExceptions(excFile)
	if err != nil {
		return nil, err
	}
	c := &collector{module: module, exc: exc, byKey: map[string]*Violation{}}
	for _, b := range bad {
		c.add("exceptions", "exception-syntax", b, "")
	}

	def, err := loadGraph(root, module, variantSpec{name: "default", patterns: []string{"./..."}})
	if err != nil {
		return nil, err
	}
	specs := []variantSpec{
		{name: "editor", tags: "aego_editor", patterns: []string{"./..."}},
		{name: "nonative", tags: "aego_nonative", patterns: []string{"./..."}},
		{name: "runtime", tags: "aego_runtime", patterns: modulePaths(module, runtimeRoots(def))},
	}
	if _, err := os.Stat(filepath.Join(root, "cmd", "aego-playground")); err == nil {
		specs = append(specs, variantSpec{name: "wasm", env: []string{"GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0"}, patterns: []string{"./cmd/aego-playground"}})
	}
	graphs := map[string]*graph{"default": def}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	for _, s := range specs {
		if len(s.patterns) == 0 {
			continue
		}
		wg.Add(1)
		go func(s variantSpec) {
			defer wg.Done()
			g, err := loadGraph(root, module, s)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			graphs[s.name] = g
		}(s)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	for _, p := range def.order {
		if _, ok := groupOf(p); !ok && p != "" {
			c.add("default", "no-group", p, "package path has no group prefix (see tools/depcheck/groups.go)")
		}
	}
	for _, name := range []string{"default", "editor", "nonative"} {
		if g := graphs[name]; g != nil {
			c.checkEdges(name, g, false)
		}
	}
	c.checkEdges("test", def, true)
	if g := graphs["editor"]; g != nil {
		c.checkEdges("test-editor", g, true)
	}
	if g := graphs["runtime"]; g != nil {
		roots := runtimeRoots(def)
		for _, p := range g.order {
			if allowed(runtimeForbidden, p) {
				c.reach("runtime", "runtime-reaches-dev", g, roots, p)
			}
		}
	}
	if g := graphs["wasm"]; g != nil {
		for _, p := range g.order {
			if !wasmOK(p) {
				c.reach("wasm", "wasm-reaches-forbidden", g, []string{"cmd/aego-playground"}, p)
			}
		}
	}
	variants := map[string]bool{"test": true, "test-editor": true}
	for name := range graphs {
		variants[name] = true
	}
	if cfg.Catalog != nil {
		variants["semantic"] = true
		for i := range cfg.Catalog.Modules {
			m := &cfg.Catalog.Modules[i]
			if m.codes, err = diagPrefixes(root, m.Dir); err != nil {
				return nil, err
			}
		}
		hits, err := scanLiterals(root, def, cfg.Catalog)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			if !excepted(exc, "semantic", h.file, h.lit) {
				c.add("semantic", "module-literal", h.pos, h.lit+" names module "+h.module)
			}
		}
	}
	src, err := checkSources(root)
	if err != nil {
		return nil, err
	}
	for _, s := range src {
		c.add("default", "sources", s, "")
	}
	for _, s := range staleExceptions(exc, variants) {
		c.add("exceptions", "exception-unused", s, "")
	}
	return c.list(), nil
}

// checkEdges applies group rules, and layer rules for non-test edges, to every import in g.
func (c *collector) checkEdges(variant string, g *graph, tests bool) {
	for _, p := range g.order {
		from, ok := groupOf(p)
		if !ok {
			continue
		}
		info := g.pkgs[p]
		imps, files := info.imports, info.files
		if tests {
			imps, files = info.testImps, info.testFiles
		}
		for _, imp := range imps {
			to, ok := groupOf(imp)
			if !ok {
				continue
			}
			rule := groupRule(from, to)
			if rule == "" && !tests {
				rule = layerRule(p, imp)
			}
			if rule == "" {
				continue
			}
			for _, file := range importingFiles(info.dir, files, c.module+"/"+imp) {
				where := p
				if file != "" {
					where = p + "/" + file
				}
				if !excepted(c.exc, variant, where, imp) {
					c.add(variant, rule, where, "imports "+imp)
				}
			}
		}
		if !tests {
			for _, ext := range info.external {
				if rule := externalRule(p, ext); rule != "" && !excepted(c.exc, variant, p, ext) {
					c.add(variant, rule, p, "imports external "+ext)
				}
			}
		}
	}
}

func (c *collector) reach(variant, rule string, g *graph, roots []string, target string) {
	chain := g.path(roots, target)
	from := ""
	if len(chain) >= 2 {
		from = chain[len(chain)-2]
	}
	if excepted(c.exc, variant, from, target) {
		return
	}
	c.add(variant, rule, chain[0], "reaches "+target+" via "+strings.Join(chain, " -> "))
}

// importingFiles returns the files of a package that import path.
func importingFiles(dir string, files []string, path string) []string {
	var out []string
	fset := token.NewFileSet()
	for _, name := range files {
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			continue
		}
		for _, imp := range f.Imports {
			if strings.Trim(imp.Path.Value, `"`) == path {
				out = append(out, name)
				break
			}
		}
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

func wasmOK(p string) bool {
	if allowed(wasmDenied, p) {
		return false
	}
	if g, ok := groupOf(p); ok && (g.kind == kLow || g.kind == kKernel) {
		return true
	}
	return allowed(wasmExtra, p)
}

// runtimeRoots are the default graph's packages minus runtimeForbidden.
func runtimeRoots(def *graph) []string {
	var out []string
	for _, p := range def.order {
		if p != "" && !allowed(runtimeForbidden, p) {
			out = append(out, p)
		}
	}
	return out
}

func modulePaths(module string, pkgs []string) []string {
	out := make([]string, len(pkgs))
	for i, p := range pkgs {
		out[i] = module + "/" + p
	}
	return out
}

// RuntimePackages lists the import paths of the aego_runtime build.
func RuntimePackages(root string) ([]string, error) {
	module, err := modulePath(root)
	if err != nil {
		return nil, err
	}
	def, err := loadGraph(root, module, variantSpec{name: "default", patterns: []string{"./..."}})
	if err != nil {
		return nil, err
	}
	return modulePaths(module, runtimeRoots(def)), nil
}
