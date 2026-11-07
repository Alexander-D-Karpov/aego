package edit

import (
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"aego/app/bootstrap"
	"aego/app/brand"
	"aego/app/run"
	"aego/app/supervise"
	"aego/app/updates"
	"aego/editor/core/session"
	"aego/editor/shell/launcher"
	"aego/engine/i18n"
	"aego/platform"
	"aego/project"
	"aego/sdk"
)

const (
	TabRecent = "recent"
	TabNew    = "new"
	NoGoNote  = "Go is not installed, so the native code of this project is not built and scenes that use its components may not load. Install Go from go.dev/dl and open the project again."
)

type Entry struct {
	Config
	Arg      string
	Launcher string
	Verb     string
	Mods     []sdk.Module
}

func Main(mods ...sdk.Module) {
	if len(os.Args) > 1 && os.Args[1] == "--tool" {
		os.Exit(tool(os.Args[2:], mods))
	}
	confirm, code, done := updates.Guard(os.Args)
	if done {
		os.Exit(code)
	}
	defer confirm()
	fs := flag.NewFlagSet("aego-editor", flag.ExitOnError)
	debug := fs.Bool("debug", false, "verbose logging and gpu debug output")
	hidden := fs.Bool("hidden", false, "create the window without showing it")
	headless := fs.Bool("headless", false, "run without a window")
	frames := fs.Int("frames", 0, "run a fixed number of frames and exit")
	cpu := fs.String("cpuprofile", "", "write a cpu profile to this file")
	profile := fs.String("profile", "", "player settings profile, default when empty")
	open := fs.String("open", "", "open this scene or prefab, relative to the project root")
	tab := fs.String("launcher", "", "open the launcher on its recent or new tab instead of a project")
	fs.Parse(os.Args[1:])
	err := Start(Entry{
		Config: Config{
			Debug: *debug, Hidden: *hidden, Headless: *headless, Frames: *frames, CPUProfile: *cpu, Profile: *profile,
			OpenPath: *open, Supervised: os.Getenv(session.EnvSupervised) == "1",
		},
		Arg: fs.Arg(0), Launcher: *tab, Mods: mods,
	})
	if errors.Is(err, ErrRestart) {
		confirm()
		os.Exit(session.ExitRestart)
	}
	if err != nil {
		fail(err)
	}
}

// tool runs a project tool in the project of the working directory, as the game binary's --tool does.
func tool(args []string, mods []sdk.Module) int {
	root, err := project.Find(".")
	if err != nil {
		os.Stderr.WriteString("aego-editor: " + err.Error() + "\n")
		return 1
	}
	p, err := project.Load(root)
	if err != nil {
		os.Stderr.WriteString("aego-editor: " + err.Error() + "\n")
		return 1
	}
	return run.Tool(p, args, os.Stdout, os.Stderr, mods...)
}

func fail(err error) {
	os.Stderr.WriteString("aego-editor: " + err.Error() + "\n")
	platform.Boot("fatal", err.Error())
	if !platform.ShowFatal("Aego", platform.FatalText("Aego", err.Error())) && !interactive() {
		platform.Alert("Aego", err.Error())
	}
	os.Exit(1)
}

func interactive() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	st, err := os.Stderr.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func goFound() bool {
	_, err := exec.LookPath("go")
	return err == nil
}

func hasProject(dir string) bool {
	_, err := project.Find(dir)
	return err == nil
}

func resolveArg(arg string) (dir, file string) {
	if arg == "" {
		return "", ""
	}
	st, err := os.Stat(arg)
	if err != nil || st.IsDir() {
		return arg, ""
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return filepath.Dir(arg), ""
	}
	if filepath.Base(abs) == project.FileName {
		return filepath.Dir(abs), ""
	}
	return filepath.Dir(abs), abs
}

func relInside(root, file string) (string, bool) {
	rel, err := filepath.Rel(root, file)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func Start(e Entry) error {
	defer platform.RecoverFatal("Aego")
	defer runAtExit()
	startBoot("edit")
	platform.SetAppIcon(brand.Window()...)
	markEntry(e.Verb)
	cfg := e.Config
	if cfg.Recent == "" {
		cfg.Recent, _ = launcher.RecentPath()
	}
	logs := attachLogs(&cfg)
	defer logs.close()
	dir, file := resolveArg(e.Arg)
	if e.Launcher != "" || (dir == "" && !hasProject(".")) {
		cfg.LauncherTab = e.Launcher
		root, entry, remote, err := launchWithEntry(cfg)
		if err != nil || root == "" {
			return err
		}
		dir, file = root, ""
		if remote {
			cfg.OpenPath = entry
			cfg.RemoteProject = true
		}
	}
	if dir == "" {
		dir = "."
	}
	root, err := project.Find(dir)
	if err != nil {
		return err
	}
	copy, managed, err := (launcherRemote{}).existingProject(root)
	if err != nil {
		return err
	}
	if managed {
		cfg.RemoteProject = true
		if file != "" {
			file, err = filepath.EvalSymlinks(file)
			if err != nil {
				return err
			}
		}
		root = copy.Directory
		if file == "" && cfg.OpenPath == "" {
			entry, ok := relInside(root, copy.Entry)
			if !ok {
				return errors.New("remote example entry is outside its project")
			}
			cfg.OpenPath = entry
		}
	}
	p, err := project.Load(root)
	if err != nil {
		return err
	}
	if cfg.RemoteProject && p.Native {
		return errors.New(i18n.Tr("launcher.remote.native_blocked", "Remote examples cannot start a native Go project."))
	}
	if file != "" {
		if rel, ok := relInside(p.Root, file); ok {
			cfg.OpenPath = rel
		}
	}
	platform.Boot("editor", p.Root)
	if bootstrap.NeedsNative(p) && !cfg.Headless && cfg.Frames == 0 && !cfg.Supervised {
		if goFound() {
			return supervise.Run(p, supervise.Options{
				Debug: cfg.Debug, Hidden: cfg.Hidden, CPUProfile: cfg.CPUProfile, Profile: cfg.Profile,
				Open: cfg.OpenPath, Stderr: logs.writer(os.Stderr),
			})
		}
		cfg.NativeNote = NoGoNote
		platform.Boot("native", NoGoNote)
	}
	return Run(p, cfg, e.Mods...)
}
