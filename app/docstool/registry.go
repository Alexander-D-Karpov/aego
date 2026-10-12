package docstool

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"aego/engine/builtin"
	"aego/engine/docs"
	"aego/engine/module"
	"aego/project"
	"aego/script/bind"
	"aego/script/bind/manifest"
	"aego/script/diag"
	"aego/script/langsvc"
	scriptmodule "aego/script/module"
	"aego/script/stdlib"
	"aego/script/workspace"
)

func build(p *project.Project, mods []module.Module) (*docs.Registry, *workspace.Workspace, error) {
	types, err := builtin.NewTypes()
	if err != nil {
		return nil, nil, err
	}
	reg := bind.New()
	man, err := builtin.Load(types, reg, stdlib.Options{Seed: 1, Testing: true}, mods...)
	if err != nil {
		return nil, nil, err
	}
	if err := types.Schema().Freeze(); err != nil {
		return nil, nil, err
	}
	if err := scriptmodule.Freeze(reg, types.Schema()); err != nil {
		return nil, nil, err
	}
	bm, err := manifest.FromRegistry(reg, types.Schema())
	if err != nil {
		return nil, nil, err
	}
	src := docs.SourcesOf(bm, types.Schema(), man)
	var ws *workspace.Workspace
	if p != nil {
		ws, err = workspace.Open(workspace.Config{Root: p.Root, Module: p.Module, Scripts: p.ScriptsDir(), Bindings: reg.Table(), Docs: bm})
		if err != nil {
			return nil, nil, err
		}
		for _, d := range langsvc.New(ws, nil).ScriptDocs(ws.Snapshot()) {
			src.Scripts = append(src.Scripts, docs.ScriptSymbol(d))
		}
		src.Project = p.Name
	}
	r := docs.Assemble(src)
	bm.Keywords = r.Keywords()
	if ws != nil {
		ws.SetDocs(bm)
	}
	return r, ws, nil
}

func Registry(p *project.Project, mods []module.Module) (*docs.Registry, error) {
	r, _, err := build(p, mods)
	return r, err
}

func Title(p *project.Project) string {
	if p == nil {
		return "Aego"
	}
	return p.Name + " · Aego"
}

func WriteExport(reg *docs.Registry, title, path string, w io.Writer) error {
	data, err := json.Marshal(reg.Export(title))
	if err != nil {
		return err
	}
	if path == "" {
		_, err = w.Write(append(data, '\n'))
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func Check(reg *docs.Registry, strict bool, w io.Writer) int {
	errs, warns := 0, 0
	for _, p := range reg.Check(strict) {
		level := "warn "
		if p.Error {
			level = "error"
			errs++
		} else {
			warns++
		}
		fmt.Fprintf(w, "%s %s: %s\n", level, p.ID, p.Message)
	}
	fmt.Fprintf(w, "docs: %d error(s), %d warning(s)\n", errs, warns)
	if errs > 0 {
		return 1
	}
	return 0
}

func Explain(w io.Writer, reg *docs.Registry, ds []diag.Diagnostic) {
	seen := map[string]bool{}
	for _, d := range ds {
		if d.Code == "" || seen[d.Code] {
			continue
		}
		seen[d.Code] = true
		e, ok := reg.Get(d.Code)
		if !ok {
			continue
		}
		fmt.Fprintf(w, "\n%s\n", d.Code)
		for _, l := range strings.Split(docs.Plain(e.Body), "\n") {
			if l == "" {
				fmt.Fprintln(w)
				continue
			}
			fmt.Fprintln(w, "  "+l)
		}
	}
}
