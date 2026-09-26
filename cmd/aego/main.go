package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"aego/app/run"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/engine"
	"aego/engine/bootstrap"
	"aego/format/aetext"
	"aego/project"
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
	case "build":
		return cmdBuild(args[1:])
	case "import":
		return cmdImport(args[1:])
	case "check":
		return cmdCheck(args[1:])
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

  new <dir> [--native]   create a project
  run [dir]              import assets and run the game
  build [dir]            build the native binary
  import [dir] [--force] reimport all assets
  check [dir]            parse every document and report problems
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
	native := fs.Bool("native", false, "create a project with a native Go module")
	engine := fs.String("engine", "", "path to the aego source tree for native projects")
	dir, rest := firstArg(args)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: aego new <dir> [--native] [--engine <dir>]")
		return 2
	}
	var root string
	if *native {
		r, err := engineRoot(*engine)
		if err != nil {
			return fail(err)
		}
		root = r
	}
	p, err := project.Create(dir, filepath.Base(dir), *native)
	if err != nil {
		return fail(err)
	}
	if err := writeStarterScene(p); err != nil {
		return fail(err)
	}
	if *native {
		if err := writeNativeStarter(p, root); err != nil {
			return fail(err)
		}
	}
	fmt.Printf("created %s\n", p.Root)
	return 0
}

func transformBlock(x, y float32) *aetext.Node {
	n := &aetext.Node{Head: "transform"}
	n.Fields = append(n.Fields,
		aetext.Field{Name: "position", Value: aetext.List(aetext.Float32(x), aetext.Float32(y))},
		aetext.Field{Name: "rotation", Value: aetext.Float32(0)},
		aetext.Field{Name: "scale", Value: aetext.List(aetext.Float32(1), aetext.Float32(1))},
	)
	return n
}

const nativeStarter = `package native

import "aego/sdk"

type Module struct{}

func (Module) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "game", Version: "0.1.0"}
}

func (Module) Register(r *sdk.Registry) error {
	return nil
}
`

func writeNativeStarter(p *project.Project, engine string) error {
	if err := os.WriteFile(filepath.Join(p.NativeDir(), "module.go"), []byte(nativeStarter), 0o644); err != nil {
		return err
	}
	gomod := fmt.Sprintf("module %s\n\ngo 1.27\n\nrequire aego v0.0.0\n\nreplace aego => %s\n",
		p.Module, filepath.ToSlash(engine))
	return os.WriteFile(filepath.Join(p.Root, "go.mod"), []byte(gomod), 0o644)
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	headless := fs.Bool("headless", false, "run without a window")
	frames := fs.Int("frames", 0, "run a fixed number of frames")
	debug := fs.Bool("debug", false, "verbose logging and gpu debug output")
	scenePath := fs.String("scene", "", "load this scene file instead of the main scene")
	dir, rest := firstArg(args)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	p, err := openProject(dir)
	if err != nil {
		return fail(err)
	}
	if bootstrap.NeedsNative(p) {
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
	}
	if err := run.Run(p, cfg); err != nil {
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
	p, err := openProject(dir)
	if err != nil {
		return fail(err)
	}
	if *force {
		os.RemoveAll(filepath.Join(p.CacheDir(), "artifacts"))
		os.Remove(filepath.Join(p.CacheDir(), "index.aetext"))
	}
	imps, err := run.DefaultImporters()
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
	dir, _ := firstArg(args)
	p, err := openProject(dir)
	if err != nil {
		return fail(err)
	}
	problems := 0
	err = filepath.WalkDir(p.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		switch ext {
		case ".aescene", ".aeprefab", ".aego", ".meta":
		default:
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if _, err := aetext.Parse(data, path); err != nil {
			fmt.Fprintln(os.Stderr, err)
			problems++
		}
		if len(data) > 0 && data[len(data)-1] != '\n' {
			fmt.Fprintf(os.Stderr, "%s: missing final newline\n", path)
			problems++
		}
		return nil
	})
	if err != nil {
		return fail(err)
	}
	if problems > 0 {
		fmt.Fprintf(os.Stderr, "%d document(s) failed to parse\n", problems)
		return 1
	}
	fmt.Println("check: ok")
	return 0
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "aego:", err)
	return 1
}
