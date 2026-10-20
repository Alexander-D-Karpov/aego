package run

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"aego/app/assembly"
	"aego/app/docstool"
	"aego/core/log"
	"aego/core/rng"
	"aego/project"
	"aego/script/bind"
	"aego/script/bytecode"
	"aego/script/diag"
	"aego/script/harness"
	"aego/script/vm"
	"aego/script/workspace"
	"aego/sdk"
)

const toolChannel log.Channel = "script"

func Tool(p *project.Project, args []string, stdout, stderr io.Writer, mods ...sdk.Module) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: --tool check | test [-run name] [-v] [-budget n] [-seed n] | docs export|check")
		return 2
	}
	switch args[0] {
	case "check":
		return toolCheck(p, stderr, mods)
	case "test":
		if WantsDeterminism(args[1:]) {
			return determinismTool(p, args[1:], stdout, stderr, mods)
		}
		return toolTest(p, args[1:], stdout, stderr, mods)
	case "docs":
		return docstool.Tool(p, args[1:], stdout, stderr, sdk.AdaptAll(mods)...)
	}
	fmt.Fprintf(stderr, "unknown tool %q\n", args[0])
	return 2
}

func toolWorkspace(p *project.Project, reg *bind.Registry, tests bool) (*workspace.Workspace, error) {
	return workspace.Open(workspace.Config{
		Root:     p.Root,
		Module:   p.Module,
		Scripts:  p.ScriptsDir(),
		Bindings: reg.Table(),
		Tests:    tests,
	})
}

func toolCheck(p *project.Project, stderr io.Writer, mods []sdk.Module) int {
	reg, err := ScriptBindings(p, assembly.Options{Seed: 1, Testing: true}, mods...)
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		return 2
	}
	ws, err := toolWorkspace(p, reg, true)
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		return 1
	}
	snap := ws.Snapshot()
	diag.Print(stderr, snap.Sources, snap.Diagnostics)
	if n := snap.Errors(); n > 0 {
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
	reg, err := ScriptBindings(p, assembly.Options{Seed: *seed, Testing: true}, mods...)
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		return 2
	}
	ws, err := toolWorkspace(p, reg, true)
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		return 1
	}
	g, ds := ws.BuildWith(workspace.BuildOptions{IncludeTests: true})
	diag.Print(stderr, g.Sources, ds)
	if diag.HasErrors(ds) {
		return 1
	}
	lg := log.New(log.NewTextSink(stderr))
	ctx := &bind.Ctx{Log: lg.With(toolChannel), Random: rng.New(*seed)}
	rep, err := harness.Run(g, reg, ctx, harness.Options{VM: vm.Options{Budget: *budget}, Filter: *filter})
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

func Dump(ws *workspace.Workspace, target, fn string, stdout, stderr io.Writer) int {
	g, ds := ws.Build()
	diag.Print(stderr, g.Sources, ds)
	pkg, script := resolveDump(g.Modules, target)
	if pkg == "" {
		fmt.Fprintf(stderr, "aego: no compiled package or script named %q\n", target)
		return 1
	}
	var buf bytes.Buffer
	bytecode.DisassembleSource(&buf, g.Modules[pkg], func(file string, line int) string {
		id, ok := g.Sources.Lookup(file)
		if !ok {
			return ""
		}
		return g.Sources.Line(id, line)
	})
	io.WriteString(stdout, filterFuncs(buf.String(), script, fn))
	return 0
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
		if strings.HasSuffix(p, "/"+target) {
			return p, ""
		}
	}
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
