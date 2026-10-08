package script

import (
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/script/abi"
	"aego/script/ast"
	"aego/script/bytecode"
	"aego/script/checker"
	"aego/script/compiler"
	"aego/script/diag"
	"aego/script/parser"
	"aego/script/source"
)

const (
	ImporterID = "script"
	Group      = "scripts"
	Root       = "scripts"
)

type CompileError struct {
	Package     string
	Summary     string
	Text        string
	Diagnostics []diag.Diagnostic
}

func (e *CompileError) Error() string {
	return e.Summary
}

type Problem struct {
	Package string
	Path    string
	Line    int
	Col     int
	Code    string
	Error   bool
	Message string
	Text    string
}

type cached struct {
	key  [32]byte
	data []byte
}

type Importer struct {
	module   string
	surface  func() abi.Surface
	cache    map[string]cached
	mu       sync.Mutex
	problems []Problem
	rev      uint64
}

func New(module string, surface func() abi.Surface) *Importer {
	if module == "" {
		module = "game"
	}
	return &Importer{module: module, surface: surface, cache: map[string]cached{}}
}

func (*Importer) ID() string                  { return ImporterID }
func (*Importer) Version() int                { return 1 }
func (*Importer) GroupVersion() int           { return 1 }
func (*Importer) Extensions() []string        { return []string{".aes"} }
func (*Importer) Defaults() pipeline.Settings { return pipeline.Settings{} }

func (*Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	return &pipeline.Result{Kind: asset.ArtScript, Group: Group, Source: ctx.Source}, nil
}

func (i *Importer) Module() string {
	return i.module
}

func PackageOf(module, file string) string {
	rel := strings.TrimPrefix(strings.TrimPrefix(path.Dir(file), Root), "/")
	if rel == "" || rel == "." {
		return module
	}
	return module + "/" + rel
}

func (i *Importer) Problems() ([]Problem, uint64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.problems, i.rev
}

func (i *Importer) Report() string {
	ps, _ := i.Problems()
	var b strings.Builder
	for _, p := range ps {
		if p.Error {
			b.WriteString(p.Text)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (i *Importer) publish(ps []Problem) {
	i.mu.Lock()
	i.problems = ps
	i.rev++
	i.mu.Unlock()
}

func emptySurface() abi.Surface {
	t := abi.NewTable()
	t.Freeze()
	return t
}

func (i *Importer) surfaceOf() abi.Surface {
	if i.surface != nil {
		if s := i.surface(); s != nil {
			return s
		}
	}
	return emptySurface()
}

func problemOf(sm *source.Manager, pkg string, d diag.Diagnostic) Problem {
	line, col := sm.LineCol(d.Primary.File, d.Primary.Start)
	return Problem{
		Package: pkg,
		Path:    sm.Path(d.Primary.File),
		Line:    line,
		Col:     col,
		Code:    d.Code,
		Error:   d.Severity == diag.Error,
		Message: d.Message,
		Text:    diag.Format(sm, d),
	}
}

func (i *Importer) Finalize(ctx *pipeline.GroupContext, members []pipeline.GroupMember) (map[ids.AssetID]*pipeline.Result, []pipeline.Produced, error) {
	surface := i.surfaceOf()
	sm := source.NewManager()
	files := map[string][]*ast.File{}
	sources := map[string]map[string][]byte{}
	pkgOf := map[source.FileID]string{}
	owners := map[string][]ids.AssetID{}
	results := make(map[ids.AssetID]*pipeline.Result, len(members))
	var parsed []diag.Diagnostic
	for _, m := range members {
		results[m.ID] = &pipeline.Result{Kind: asset.ArtScript}
		if m.Result == nil || strings.HasSuffix(m.Path, "_test.aes") {
			continue
		}
		pkg := PackageOf(i.module, m.Path)
		id := sm.Add(m.Path, m.Result.Source)
		f, ds := parser.ParseFile(sm, id)
		parsed = append(parsed, ds...)
		files[pkg] = append(files[pkg], f)
		pkgOf[id] = pkg
		if sources[pkg] == nil {
			sources[pkg] = map[string][]byte{}
		}
		sources[pkg][m.Path] = m.Result.Source
		owners[pkg] = append(owners[pkg], m.ID)
	}
	if len(files) == 0 {
		i.publish(nil)
		return results, nil, nil
	}
	a := checker.Check(sm, files, surface, checker.Options{Module: i.module})
	a.Diagnostics = append(parsed, a.Diagnostics...)
	diag.Sort(a.Diagnostics)
	own := map[string][]diag.Diagnostic{}
	var problems []Problem
	for _, d := range a.Diagnostics {
		pkg := pkgOf[d.Primary.File]
		own[pkg] = append(own[pkg], d)
		if d.Severity != diag.Info {
			problems = append(problems, problemOf(sm, pkg, d))
		}
	}
	pkgs := make([]string, 0, len(files))
	for p := range files {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	abiHash := surface.Hash()
	var pages []pipeline.Produced
	for _, pkg := range pkgs {
		data, extra, err := i.compile(a, sm, pkg, sources[pkg], abiHash, own)
		for _, d := range extra {
			problems = append(problems, problemOf(sm, pkg, d))
		}
		if err != nil {
			for _, id := range owners[pkg] {
				results[id].Error = err
			}
			prev, ok := []byte(nil), false
			if ctx.Previous != nil {
				prev, ok = ctx.Previous(pkg)
			}
			if !ok {
				continue
			}
			data = prev
			ctx.Log.Warn("script package has errors, keeping the last good build", log.F("package", pkg))
		}
		pages = append(pages, pipeline.Produced{Kind: asset.ArtScript, Name: pkg, Data: data})
	}
	i.publish(problems)
	return results, pages, nil
}

func (i *Importer) compile(a *checker.Analysis, sm *source.Manager, pkg string, srcs map[string][]byte, abiHash [32]byte, own map[string][]diag.Diagnostic) ([]byte, []diag.Diagnostic, error) {
	if diag.HasErrors(own[pkg]) {
		return nil, nil, failure(sm, pkg, own[pkg])
	}
	if dep := brokenDep(a, own, pkg); dep != "" {
		s := "package " + pkg + " depends on broken package " + dep
		return nil, nil, &CompileError{Package: pkg, Summary: s, Text: s + "\n"}
	}
	deps := map[string][32]byte{}
	if cp := a.Packages[pkg]; cp != nil {
		for _, d := range cp.Imports {
			deps[d] = compiler.InterfaceHash(a, d)
		}
	}
	key := compiler.CacheKey(srcs, deps, abiHash, compiler.Options{})
	if c, ok := i.cache[pkg]; ok && c.key == key {
		return c.data, nil, nil
	}
	mod, ds := compiler.Compile(a, pkg, compiler.Options{Sources: sm})
	var extra []diag.Diagnostic
	for _, d := range ds {
		if d.Code != diag.CompileBlocked {
			extra = append(extra, d)
		}
	}
	if mod == nil {
		return nil, extra, failure(sm, pkg, ds)
	}
	data := bytecode.Encode(mod)
	i.cache[pkg] = cached{key: key, data: data}
	return data, extra, nil
}

func brokenDep(a *checker.Analysis, own map[string][]diag.Diagnostic, pkg string) string {
	seen := map[string]bool{}
	var walk func(p string) string
	walk = func(p string) string {
		if seen[p] {
			return ""
		}
		seen[p] = true
		cp := a.Packages[p]
		if cp == nil {
			return ""
		}
		for _, d := range cp.Imports {
			if diag.HasErrors(own[d]) {
				return d
			}
			if b := walk(d); b != "" {
				return b
			}
		}
		return ""
	}
	return walk(pkg)
}

func failure(sm *source.Manager, pkg string, ds []diag.Diagnostic) *CompileError {
	var b strings.Builder
	n := 0
	for _, d := range ds {
		if d.Severity == diag.Error {
			b.WriteString(diag.Format(sm, d))
			n++
		}
	}
	return &CompileError{
		Package:     pkg,
		Summary:     "package " + pkg + " has " + strconv.Itoa(n) + " error(s)",
		Text:        b.String(),
		Diagnostics: ds,
	}
}
