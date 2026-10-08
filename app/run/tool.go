package run

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"aego/core/log"
	"aego/core/rng"
	"aego/project"
	"aego/script/ast"
	"aego/script/bind"
	"aego/script/build"
	"aego/script/bytecode"
	"aego/script/checker"
	"aego/script/compiler"
	"aego/script/diag"
	"aego/script/harness"
	"aego/script/parser"
	"aego/script/source"
	"aego/script/stdlib"
	"aego/script/vm"
	"aego/sdk"
)

const toolChannel log.Channel = "script"

func ScriptSources(p *project.Project) (map[string][]build.File, error) {
	root := p.ScriptsDir()
	out := map[string][]build.File{}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return out, nil
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".aes" {
			return nil
		}
		dir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(p.Root, path)
		if err != nil {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		pkg := build.PackagePath(p.Module, dir)
		out[pkg] = append(out[pkg], build.File{Path: filepath.ToSlash(rel), Src: src})
		return nil
	})
	return out, err
}

func Tool(p *project.Project, args []string, stdout, stderr io.Writer, mods ...sdk.Module) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: --tool check | test [-run name] [-v] | dump <pkg|Type> [--func name]")
		return 2
	}
	switch args[0] {
	case "check":
		return toolCheck(p, stderr, mods)
	case "test":
		return toolTest(p, args[1:], stdout, stderr, mods)
	case "dump":
		return toolDump(p, args[1:], stdout, stderr, mods)
	}
	fmt.Fprintf(stderr, "unknown tool %q\n", args[0])
	return 2
}

func toolCheck(p *project.Project, stderr io.Writer, mods []sdk.Module) int {
	reg, err := ScriptBindings(p, stdlib.Options{Seed: 1, Testing: true}, mods...)
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		return 1
	}
	srcs, err := ScriptSources(p)
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		return 1
	}
	res := build.Build(srcs, reg.Surface(), build.Options{Tests: true})
	diag.Print(stderr, res.Sources, res.Diagnostics)
	if n, _ := diag.Count(res.Diagnostics); n > 0 {
		fmt.Fprintf(stderr, "scripts: %d error(s)\n", n)
		return 1
	}
	return 0
}

func toolTest(p *project.Project, args []string, stdout, stderr io.Writer, mods []sdk.Module) int {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.SetOutput(stderr)
	filter := flags.String("run", "", "only run tests whose name contains this")
	verbose := flags.Bool("v", false, "list passing tests too")
	budget := flags.Uint64("budget", 10_000_000, "instruction budget per test")
	seed := flags.Uint64("seed", 1, "seed for aego/rand")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	reg, err := ScriptBindings(p, stdlib.Options{Seed: *seed, Testing: true}, mods...)
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		return 1
	}
	srcs, err := ScriptSources(p)
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		return 1
	}
	res := build.Build(srcs, reg.Surface(), build.Options{Tests: true})
	diag.Print(stderr, res.Sources, res.Diagnostics)
	if res.HasErrors() {
		return 1
	}
	lg := log.New(log.NewTextSink(stderr))
	ctx := &bind.Ctx{Log: lg.With(toolChannel), Random: rng.New(*seed)}
	rep, err := harness.Run(res, reg, ctx, harness.Options{VM: vm.Options{Budget: *budget}, Filter: *filter})
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		return 1
	}
	rep.Print(stdout, *verbose)
	if rep.Failed > 0 {
		return 1
	}
	return 0
}

func compileScripts(p *project.Project, reg *bind.Registry) (*source.Manager, map[string]*bytecode.Module, []diag.Diagnostic, error) {
	srcs, err := ScriptSources(p)
	if err != nil {
		return nil, nil, nil, err
	}
	sm := source.NewManager()
	files := map[string][]*ast.File{}
	var ds []diag.Diagnostic
	for pkg, list := range srcs {
		for _, f := range list {
			if strings.HasSuffix(f.Path, "_test.aes") {
				continue
			}
			af, d := parser.ParseSource(sm, f.Path, f.Src)
			ds = append(ds, d...)
			files[pkg] = append(files[pkg], af)
		}
	}
	a := checker.Check(sm, files, reg.Surface(), checker.Options{Module: p.Module})
	a.Diagnostics = append(ds, a.Diagnostics...)
	all := append([]diag.Diagnostic(nil), a.Diagnostics...)
	mods := map[string]*bytecode.Module{}
	for pkg := range files {
		m, cds := compiler.Compile(a, pkg, compiler.Options{Sources: sm, Debug: true})
		for _, d := range cds {
			if d.Code != diag.CompileBlocked {
				all = append(all, d)
			}
		}
		if m != nil {
			mods[pkg] = m
		}
	}
	diag.Sort(all)
	return sm, mods, all, nil
}

func resolveDump(mods map[string]*bytecode.Module, target string) (string, string) {
	if _, ok := mods[target]; ok {
		return target, ""
	}
	pkgs := make([]string, 0, len(mods))
	for p := range mods {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	for _, p := range pkgs {
		for _, s := range mods[p].Scripts {
			if s.FullName == target || strings.HasSuffix(s.FullName, "."+target) {
				return p, s.FullName[strings.LastIndexByte(s.FullName, '.')+1:]
			}
		}
	}
	return "", ""
}

func filterFuncs(text, script, fn string) string {
	if script == "" && fn == "" {
		return text
	}
	blocks := strings.Split(text, "\nfunc ")
	out := []string{blocks[0]}
	for _, b := range blocks[1:] {
		fields := strings.Fields(b)
		if len(fields) < 2 {
			continue
		}
		name := fields[1]
		if script != "" && !strings.HasPrefix(name, script+".") {
			continue
		}
		if fn != "" && name != fn && !strings.HasSuffix(name, "."+fn) {
			continue
		}
		out = append(out, b)
	}
	return strings.Join(out, "\nfunc ")
}

func toolDump(p *project.Project, args []string, stdout, stderr io.Writer, mods []sdk.Module) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(stderr, "usage: dump <package|Type> [--func name]")
		return 2
	}
	target := args[0]
	flags := flag.NewFlagSet("dump", flag.ContinueOnError)
	flags.SetOutput(stderr)
	fn := flags.String("func", "", "only print this function")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	reg, err := ScriptBindings(p, stdlib.Options{Seed: 1}, mods...)
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		return 1
	}
	sm, compiled, ds, err := compileScripts(p, reg)
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		return 1
	}
	diag.Print(stderr, sm, ds)
	pkg, script := resolveDump(compiled, target)
	if pkg == "" {
		fmt.Fprintf(stderr, "aego: no compiled package or script named %q\n", target)
		return 1
	}
	var buf bytes.Buffer
	bytecode.DisassembleSource(&buf, compiled[pkg], func(file string, line int) string {
		id, ok := sm.Lookup(file)
		if !ok {
			return ""
		}
		return sm.Line(id, line)
	})
	io.WriteString(stdout, filterFuncs(buf.String(), script, *fn))
	return 0
}
