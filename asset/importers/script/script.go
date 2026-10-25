package script

import (
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/script/bytecode"
	"aego/script/diag"
	"aego/script/source"
	"aego/script/workspace"
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

type page struct {
	hash [32]byte
	data []byte
}

type Importer struct {
	ws       *workspace.Workspace
	pages    map[string]page
	mu       sync.Mutex
	problems []Problem
	rev      uint64
}

func New(ws *workspace.Workspace) *Importer {
	return &Importer{ws: ws, pages: map[string]page{}}
}

func (*Importer) ID() string                  { return ImporterID }
func (*Importer) Version() int                { return 1 }
func (*Importer) GroupVersion() int           { return 2 }
func (*Importer) Extensions() []string        { return []string{".aes"} }
func (*Importer) Defaults() pipeline.Settings { return pipeline.Settings{} }

func (*Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	if !strings.HasPrefix(ctx.Path, Root+"/") {
		return nil, errs.New(errs.AssetImportFailed, "scripts belong under "+Root+"/, so "+ctx.Path+" is not compiled; move it there", log.F("path", ctx.Path))
	}
	return &pipeline.Result{Kind: asset.ArtScript, Group: Group, Source: ctx.Source}, nil
}

func (i *Importer) Workspace() *workspace.Workspace {
	return i.ws
}

func (i *Importer) Module() string {
	if i.ws == nil {
		return "game"
	}
	return i.ws.Module()
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

func (i *Importer) encode(pkg string, g *workspace.Generation) []byte {
	h := g.Hashes[pkg]
	if p, ok := i.pages[pkg]; ok && p.hash == h {
		return p.data
	}
	data := bytecode.Encode(g.Modules[pkg])
	i.pages[pkg] = page{hash: h, data: data}
	return data
}

func (i *Importer) Finalize(ctx *pipeline.GroupContext, members []pipeline.GroupMember) (map[ids.AssetID]*pipeline.Result, []pipeline.Produced, error) {
	results := make(map[ids.AssetID]*pipeline.Result, len(members))
	files := make(map[string][]byte, len(members))
	owners := map[string][]ids.AssetID{}
	for _, m := range members {
		results[m.ID] = &pipeline.Result{Kind: asset.ArtScript}
		if m.Result == nil {
			continue
		}
		files[m.Path] = m.Result.Source
		if i.ws == nil || strings.HasSuffix(m.Path, "_test.aes") {
			continue
		}
		if pkg, ok := i.ws.PackageOf(m.Path); ok {
			owners[pkg] = append(owners[pkg], m.ID)
		}
	}
	if i.ws == nil {
		i.publish(nil)
		return results, nil, nil
	}
	i.ws.Sync(files)
	gen, ds := i.ws.Build()
	sm := gen.Sources
	var problems []Problem
	for _, d := range ds {
		if d.Severity == diag.Info {
			continue
		}
		pkg, _ := i.ws.PackageOf(sm.Path(d.Primary.File))
		problems = append(problems, problemOf(sm, pkg, d))
	}
	failed := make([]string, 0, len(gen.Failed))
	for pkg := range gen.Failed {
		failed = append(failed, pkg)
	}
	sort.Strings(failed)
	for _, pkg := range failed {
		fds := gen.Failed[pkg]
		for _, d := range fds {
			if !d.Primary.Valid() {
				problems = append(problems, problemOf(sm, pkg, d))
			}
		}
		err := failure(sm, pkg, fds)
		for _, id := range owners[pkg] {
			results[id].Error = err
		}
	}
	var pages []pipeline.Produced
	for pkg := range gen.Modules {
		pages = append(pages, pipeline.Produced{Kind: asset.ArtScript, Name: pkg, Data: i.encode(pkg, gen)})
	}
	for _, pkg := range failed {
		if _, ok := gen.Modules[pkg]; ok || ctx.Previous == nil {
			continue
		}
		if data, ok := ctx.Previous(pkg); ok {
			pages = append(pages, pipeline.Produced{Kind: asset.ArtScript, Name: pkg, Data: data})
			ctx.Log.Warn("script package has errors, keeping the last good build", log.F("package", pkg))
		}
	}
	sort.Slice(pages, func(a, b int) bool { return pages[a].Name < pages[b].Name })
	i.publish(problems)
	return results, pages, nil
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
