package export

import (
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"aego/app/assembly/builtin"
	"aego/app/bootstrap"
	"aego/app/export/icon"
	"aego/core/errs"
	"aego/core/log"
	"aego/project"
	"aego/runtime/bundle"
)

const Owner = "export"

type Options struct {
	Project   *project.Project
	Engine    string
	Target    string
	Profile   string
	Out       string
	CC        string
	CheckOnly bool
	Snapshot  *Snapshot
	Log       io.Writer
	Progress  func(float32, string)
}

type Target struct {
	OS   string
	Arch string
}

func (t Target) String() string {
	if t.Web() {
		return "web"
	}
	return t.OS + "/" + t.Arch
}

func (t Target) Web() bool {
	return t.OS == "js"
}

func (t Target) Host() bool {
	return t.OS == runtime.GOOS && t.Arch == runtime.GOARCH
}

func (t Target) Exe(name string) string {
	if t.OS == "windows" {
		return name + ".exe"
	}
	return name
}

func Targets() []string {
	return []string{"host", "linux", "windows", "darwin", "web"}
}

func ParseTarget(s string) (Target, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "host" {
		return Target{OS: runtime.GOOS, Arch: runtime.GOARCH}, nil
	}
	switch s {
	case "web", "js", "wasm", "js/wasm":
		return Target{OS: "js", Arch: "wasm"}, nil
	}
	osName, arch, _ := strings.Cut(s, "/")
	if arch == "" {
		arch = runtime.GOARCH
	}
	switch osName {
	case "macos":
		osName = "darwin"
	case "linux", "windows", "darwin":
	default:
		return Target{}, errs.New(errs.EngineConfig, "unknown export target "+s, log.F("targets", strings.Join(Targets(), ", ")))
	}
	switch arch {
	case "amd64", "arm64":
	default:
		return Target{}, errs.New(errs.EngineConfig, "unsupported architecture "+arch+", use amd64 or arm64")
	}
	return Target{OS: osName, Arch: arch}, nil
}

func ParseProfile(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "dev", bundle.Development:
		return bundle.Development, nil
	case bundle.Release:
		return bundle.Release, nil
	}
	return "", errs.New(errs.EngineConfig, "profile must be development or release", log.F("profile", s))
}

func DefaultOut(p *project.Project, t Target, profile string) string {
	if t.Web() {
		return filepath.Join(p.Root, "exports", "web-"+profile)
	}
	return filepath.Join(p.Root, "exports", t.OS+"-"+t.Arch+"-"+profile)
}

func ExeName(p *project.Project) string {
	return project.ModuleName(p.Name)
}

func AppName(p *project.Project) string {
	n := strings.TrimSpace(strings.NewReplacer("/", "-", ":", "-", "\\", "-").Replace(p.Name))
	if n == "" {
		return ExeName(p)
	}
	return n
}

func GameVersion(p *project.Project) string {
	if p.GameVersion != "" {
		return p.GameVersion
	}
	return "0.0.0"
}

type layout struct {
	root string
	bin  string
	data string
	app  string
	exe  string
}

func layoutFor(t Target, out string, p *project.Project) layout {
	if t.Web() {
		return layout{root: out, bin: out, data: filepath.Join(out, bundle.DataDir), exe: WebWasm}
	}
	exe := t.Exe(ExeName(p))
	if t.OS == "darwin" {
		app := filepath.Join(out, AppName(p)+".app")
		return layout{root: out, app: app, exe: exe,
			bin:  filepath.Join(app, "Contents", "MacOS"),
			data: filepath.Join(app, "Contents", "Resources", bundle.DataDir)}
	}
	return layout{root: out, bin: out, data: filepath.Join(out, bundle.DataDir), exe: exe}
}

func (l layout) binary() string {
	return filepath.Join(l.bin, l.exe)
}

type job struct {
	opt          Options
	p            *project.Project
	t            Target
	profile      string
	out          string
	l            layout
	native       bool
	snap         *Snapshot
	icon         image.Image
	rep          *Report
	start        time.Time
	goroot       string
	files        []webFile
	manifestSize int64
}

func Export(opt Options) (*Report, error) {
	p := opt.Project
	if p == nil {
		return nil, errs.New(errs.ProjectNotFound, "export needs a project")
	}
	t, err := ParseTarget(opt.Target)
	if err != nil {
		return nil, err
	}
	profile, err := ParseProfile(opt.Profile)
	if err != nil {
		return nil, err
	}
	out := opt.Out
	if out == "" {
		out = DefaultOut(p, t, profile)
	}
	if out, err = filepath.Abs(out); err != nil {
		return nil, errs.Wrap(err, errs.VFSBadPath, "cannot resolve the output directory")
	}
	j := &job{opt: opt, p: p, t: t, profile: profile, out: out, l: layoutFor(t, out, p),
		native: bootstrap.NeedsNative(p), snap: opt.Snapshot, start: time.Now()}
	j.rep = &Report{Target: t.String(), Profile: profile, Out: out, Modules: j.modules()}
	err = j.run()
	j.rep.Millis = time.Since(j.start).Milliseconds()
	if err != nil {
		j.rep.Error = err.Error()
	}
	return j.rep, err
}

func (j *job) run() error {
	j.step(0.02, "checking the project")
	if j.snap == nil {
		snap, done, err := Fresh(j.p)
		if err != nil {
			return err
		}
		defer done()
		j.snap = snap
	}
	ds := Check(j.p, j.snap, j.profile, j.native)
	if j.t.Web() {
		ds = append(ds, CheckWeb(j.snap, j.native)...)
	}
	j.rep.setDiagnostics(ds)
	if j.rep.Blocked {
		return errs.New(errs.BuildFailed, fmt.Sprintf("export is blocked by %d problem(s)", Blocking(ds)))
	}
	if j.opt.CheckOnly {
		return nil
	}
	if j.p.Icon != "" {
		img, err := icon.Load(filepath.Join(j.p.Root, filepath.FromSlash(j.p.Icon)))
		if err != nil {
			return err
		}
		j.icon = img
	}
	if err := j.toolchain(); err != nil {
		return err
	}
	if err := j.prepareOut(); err != nil {
		return err
	}
	j.step(0.1, "building the runtime for "+j.t.String())
	if err := j.build(); err != nil {
		return err
	}
	j.step(0.7, "copying the game data")
	if err := j.data(); err != nil {
		return err
	}
	j.step(0.85, "packaging for "+j.t.String())
	if err := j.pack(); err != nil {
		return err
	}
	j.step(0.95, "writing the export manifest")
	if err := j.manifest(); err != nil {
		return err
	}
	j.step(1, "done")
	return nil
}

func (j *job) step(f float32, msg string) {
	if j.opt.Progress != nil {
		j.opt.Progress(f, msg)
	}
	if j.opt.Log != nil {
		fmt.Fprintf(j.opt.Log, "export: %s\n", msg)
	}
}

func (j *job) modules() []string {
	out := builtin.Runtime().IDs()
	if j.native {
		out = append(out, "native:"+j.p.Module)
	}
	return out
}

func inside(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (j *job) prepareOut() error {
	p := j.p
	if inside(j.out, p.Root) {
		return errs.New(errs.VFSBadPath, "the output directory must not contain the project", log.F("dir", j.out))
	}
	for _, d := range []string{p.AssetsDir(), p.ScenesDir(), p.ScriptsDir(), p.PrefabsDir(), p.NativeDir(), filepath.Join(p.Root, ".aego")} {
		if inside(d, j.out) {
			return errs.New(errs.VFSBadPath, "the output directory must not be inside a project source folder", log.F("dir", j.out))
		}
	}
	entries, err := os.ReadDir(j.out)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return errs.Wrap(err, errs.VFSIO, "cannot read the output directory").With(log.F("dir", j.out))
	case len(entries) > 0:
		if _, err := os.Stat(filepath.Join(j.out, bundle.ExportFile)); err != nil {
			return errs.New(errs.ProjectExists, "the output directory is not empty and holds no previous export", log.F("dir", j.out))
		}
		if err := os.RemoveAll(j.out); err != nil {
			return errs.Wrap(err, errs.VFSIO, "cannot clear the previous export").With(log.F("dir", j.out))
		}
	}
	if err := os.MkdirAll(j.l.data, 0o755); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot create the output directory").With(log.F("dir", j.out))
	}
	return nil
}
