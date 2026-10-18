package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"aego/app/bootstrap"
	"aego/app/docstool"
	"aego/app/edit"
	"aego/app/projects"
	"aego/app/run"
	"aego/app/supervise"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/editor/shell/launcher"
	"aego/engine"
	"aego/project"
	"aego/script/diag"
)

func main() {
	os.Exit(dispatch(os.Args[1:]))
}

func dispatch(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "new":
		return cmdNew(args[1:])
	case "run":
		return cmdRun(args[1:])
	case "edit":
		return cmdEdit(args[1:])
	case "build":
		return cmdBuild(args[1:])
	case "bootstrap":
		return cmdBootstrap(args[1:])
	case "import":
		return cmdImport(args[1:])
	case "check":
		return cmdCheck(args[1:])
	case "icons":
		return cmdIcons(args[1:])
	case "docs":
		return cmdDocs(args[1:])
	case "test":
		return cmdTest(args[1:])
	case "dump":
		return cmdDump(args[1:])
	case "help", "-h", "--help":
		usage()
		return 0
	}
	fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
	usage()
	return 2
}

func usage() {
	fmt.Fprint(os.Stderr, `aego <command> [flags]

  new <dir> [--template empty|2d|2.5d|3d] [--module m] [--example name] [--native]
                         create a project from a template or an engine example
  run [dir]              import assets and run the game
  edit [dir]             open the project in the editor; without a project opens the launcher
  build [dir]            build the native binary
  bootstrap [dir]        regenerate .aego/generated for the native module without building
  import [dir] [--force] reimport all assets
  check [dir] [--manifest file] [--explain] [--ensure-meta]  parse every document, audit .meta files and type-check scripts
  icons [dir]            check module icon files against the icons components and templates use
  test [dir] [-run name] [-v] [--determinism [--frames n] [--every n] [--seed s] [--scene file]]
                         run *_test.aes script tests headless, or run the game twice and compare state hashes
  dump script <pkg|Type> [--func name]  disassemble a script package with source lines
  docs export|check|build|serve  documentation registry, site and playground
`)
}

func openProject(dir string) (*project.Project, error) {
	if dir == "" {
		dir = "."
	}
	root, err := project.Find(dir)
	if err != nil {
		return nil, err
	}
	return project.Load(root)
}

func firstArg(args []string) (string, []string) {
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		return args[0], args[1:]
	}
	return "", args
}

func cmdNew(args []string) int {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	tmpl := fs.String("template", projects.Template2D, "empty, 2d, 2.5d or 3d")
	module := fs.String("module", "", "module path, defaults to the directory name")
	example := fs.String("example", "", "copy an engine example instead of a template")
	native := fs.Bool("native", false, "create a project with a native Go module")
	engineDir := fs.String("engine", "", "path to the aego source tree for native projects and examples")
	dir, rest := firstArg(args)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: aego new <dir> [--template empty|2d|2.5d|3d] [--module m] [--example name] [--native] [--engine dir]")
		return 2
	}
	opt := projects.Options{Dir: dir, Template: *tmpl, Module: *module, Example: *example, Native: *native}
	if *native || *example != "" {
		root, err := projects.EngineRoot(*engineDir)
		if err != nil {
			return fail(err)
		}
		opt.Engine = root
	}
	p, err := projects.Create(opt)
	if err != nil {
		return fail(err)
	}
	if err := projects.Verify(p, os.Stderr); err != nil {
		return fail(err)
	}
	what := "template " + *tmpl
	if *example != "" {
		what = "example " + *example
	}
	fmt.Printf("Created %s (%s). Run: aego edit %s\n", dir, what, dir)
	return 0
}

func cmdBootstrap(args []string) int {
	dir, _ := firstArg(args)
	p, err := openProject(dir)
	if err != nil {
		return fail(err)
	}
	if !p.Native {
		fmt.Fprintln(os.Stderr, "project has native=false, nothing to bootstrap")
		return 1
	}
	for _, m := range []bootstrap.Mode{bootstrap.ModeRun, bootstrap.ModeManifest} {
		path, err := bootstrap.Generate(p, m)
		if err != nil {
			return fail(err)
		}
		fmt.Println(path)
	}
	return 0
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	headless := fs.Bool("headless", false, "run without a window")
	frames := fs.Int("frames", 0, "run a fixed number of frames")
	debug := fs.Bool("debug", false, "verbose logging and gpu debug output")
	scenePath := fs.String("scene", "", "load this scene file instead of the main scene")
	tracePath := fs.String("trace", "", "write a Chrome trace of the last 239 frames to this file on exit")
	settingsProfile := fs.String("profile", "", "player settings profile, default when empty")
	dir, rest := firstArg(args)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	p, err := openProject(dir)
	if err != nil {
		return fail(err)
	}
	if bootstrap.NeedsNative(p) {
		if *tracePath != "" {
			rest = absTraceArg(rest, *tracePath)
		}
		if *headless || *frames > 0 {
			binary, err := bootstrap.Build(p, bootstrap.HostTarget(), bootstrap.ModeRun, engine.Version, nil)
			if err != nil {
				return fail(err)
			}
			cmd := exec.Command(binary, rest...)
			cmd.Dir = p.Root
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return exitCode(cmd.Run())
		}
		return superviseNative(p, rest)
	}
	cfg := run.Config{
		Headless:  *headless,
		Frames:    *frames,
		Debug:     *debug,
		Watch:     *frames == 0,
		ScenePath: *scenePath,
		Trace:     *tracePath,
		Profile:   *settingsProfile,
	}
	if err := run.Run(p, cfg); err != nil {
		return fail(err)
	}
	return 0
}

func absTraceArg(args []string, path string) []string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return args
	}
	out := append([]string(nil), args...)
	for i, a := range out {
		switch {
		case (a == "-trace" || a == "--trace") && i+1 < len(out):
			out[i+1] = abs
		case strings.HasPrefix(a, "-trace=") || strings.HasPrefix(a, "--trace="):
			out[i] = "--trace=" + abs
		}
	}
	return out
}

func cmdEdit(args []string) int {
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	debug := fs.Bool("debug", false, "verbose logging and gpu debug output")
	hidden := fs.Bool("hidden", false, "create the window without showing it")
	headless := fs.Bool("headless", false, "run without a window")
	frames := fs.Int("frames", 0, "run a fixed number of frames and exit")
	cpu := fs.String("cpuprofile", "", "write a cpu profile to this file")
	settingsProfile := fs.String("profile", "", "player settings profile, default when empty")
	dir, rest := firstArg(args)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	recent, _ := launcher.RecentPath()
	if dir == "" {
		if _, err := project.Find("."); err != nil {
			root, err := edit.Launch(edit.Config{Debug: *debug, Hidden: *hidden, Headless: *headless, Frames: *frames, Recent: recent})
			if err != nil {
				return fail(err)
			}
			if root == "" {
				return 0
			}
			dir = root
		}
	}
	p, err := openProject(dir)
	if err != nil {
		return fail(err)
	}
	profile := *cpu
	if profile != "" {
		if abs, err := filepath.Abs(profile); err == nil {
			profile = abs
		}
	}
	if bootstrap.NeedsNative(p) && !*headless && *frames == 0 {
		opt := supervise.Options{Debug: *debug, Hidden: *hidden, CPUProfile: profile, Profile: *settingsProfile}
		if err := supervise.Run(p, opt); err != nil {
			return fail(err)
		}
		return 0
	}
	cfg := edit.Config{Debug: *debug, Hidden: *hidden, Headless: *headless, Frames: *frames, CPUProfile: profile, Profile: *settingsProfile, Recent: recent}
	if err := edit.Run(p, cfg); err != nil {
		return fail(err)
	}
	return 0
}

func cmdBuild(args []string) int {
	dir, _ := firstArg(args)
	p, err := openProject(dir)
	if err != nil {
		return fail(err)
	}
	if !p.Native {
		fmt.Fprintln(os.Stderr, "project has native=false, nothing to build")
		return 1
	}
	binary, err := bootstrap.Build(p, bootstrap.HostTarget(), bootstrap.ModeRun, engine.Version, nil)
	if err != nil {
		return fail(err)
	}
	fmt.Println(binary)
	return 0
}

func cmdImport(args []string) int {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	force := fs.Bool("force", false, "ignore the artifact cache")
	dir, rest := firstArg(args)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if dir == "" && fs.NArg() > 0 {
		dir = fs.Arg(0)
	}
	p, err := openProject(dir)
	if err != nil {
		return fail(err)
	}
	if *force {
		os.RemoveAll(filepath.Join(p.CacheDir(), "artifacts"))
		os.Remove(filepath.Join(p.CacheDir(), "index.aetext"))
	}
	ws, err := projects.OpenWorkspace(p, "", false)
	if err != nil {
		fmt.Fprintln(os.Stderr, "aego:", err)
		return 2
	}
	imps, err := run.Importers(ws)
	if err != nil {
		return fail(err)
	}
	pl, err := pipeline.Open(p, imps, log.New(log.NewTextSink(os.Stderr)))
	if err != nil {
		return fail(err)
	}
	defer pl.Close()
	added, removed, err := pl.Scan()
	if err != nil {
		return fail(err)
	}
	batch, err := pl.ImportAll()
	if err != nil {
		return fail(err)
	}
	fmt.Printf("scanned: %d new, %d missing\nimported: %d, failed: %d\n",
		len(added), len(removed), len(batch.Imported), len(batch.Failed))
	if len(batch.Failed) > 0 {
		for id, e := range batch.Failed {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", id, e)
		}
		return 1
	}
	return 0
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "", "check against this bindings manifest instead of building one")
	explain := fs.Bool("explain", false, "print the reference entry of every reported diagnostic code")
	ensure := fs.Bool("ensure-meta", false, "write missing .meta files instead of reporting them")
	dir, rest := firstArg(args)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	p, err := openProject(dir)
	if err != nil {
		return fail(err)
	}
	problems := projects.CheckDocuments(p, os.Stderr)
	problems += projects.CheckSections(p, os.Stderr)
	ws, err := projects.OpenWorkspace(p, *manifestPath, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "aego:", err)
		return 2
	}
	imps, err := run.Importers(ws)
	if err != nil {
		return fail(err)
	}
	mode := pipeline.MetaReadOnly
	if *ensure {
		mode = pipeline.MetaEnsure
	}
	issues, err := pipeline.Audit(p, imps, mode)
	if err != nil {
		return fail(err)
	}
	for _, is := range issues {
		fmt.Fprintf(os.Stderr, "%s: %s %s\n", is.Path, is.Code(), is.Message())
		problems++
	}
	problems += projects.CheckAssets(p, imps, os.Stderr)
	snap := ws.Snapshot()
	diag.Print(os.Stderr, snap.Sources, snap.Diagnostics)
	if *explain && len(snap.Diagnostics) > 0 {
		if reg, err := docstool.Registry(nil, nil); err != nil {
			fmt.Fprintln(os.Stderr, "aego: explain:", err)
		} else {
			docstool.Explain(os.Stderr, reg, snap.Diagnostics)
		}
	}
	if n := snap.Errors(); n > 0 {
		fmt.Fprintf(os.Stderr, "scripts: %d error(s)\n", n)
		problems++
	}
	if problems > 0 {
		fmt.Fprintf(os.Stderr, "check: %d problem(s)\n", problems)
		return 1
	}
	fmt.Println("check: ok")
	return 0
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "aego:", err)
	return 1
}
