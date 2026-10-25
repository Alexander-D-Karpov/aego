package projects

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/log"
	"aego/engine/res"
	"aego/project"
)

const (
	TemplateEmpty = "empty"
	Template2D    = "2d"
	Template25D   = "2.5d"
	Template3D    = "3d"
	templatesDir  = "templates"
)

type Template struct {
	ID          string
	Name        string
	Description string
	Icon        string
}

var Templates = []Template{
	{ID: TemplateEmpty, Name: "Empty", Description: "One scene with a 2D camera", Icon: "cat.node"},
	{ID: Template2D, Name: "2D", Description: "2D camera and a sprite", Icon: "cat.rendering"},
	{ID: Template25D, Name: "2.5D", Description: "Orthographic 3D camera with 2D sprites", Icon: "cat.camera"},
	{ID: Template3D, Name: "3D", Description: "Perspective camera, a plane and a cube", Icon: "cat.transform"},
}

type Options struct {
	Dir      string
	Template string
	Module   string
	Example  string
	Engine   string
	Native   bool
}

var skipCopy = map[string]bool{".aego": true, "exports": true, ".git": true}

var renameExt = map[string]bool{".aego": true, ".aescene": true, ".aeprefab": true, ".aes": true, ".go": true, ".md": true, ".aetext": true}

var replaceAego = regexp.MustCompile(`(?m)^(\s*replace\s+aego(?:\s+\S+)?\s*=>\s*)\S+`)

const nativeStarter = `package native

import "aego/sdk"

type Module struct{}

func (Module) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "game", Version: "0.1.0"}
}

func (Module) Register(r *sdk.Registry) error {
	return nil
}

func Modules() []sdk.Module {
	return []sdk.Module{Module{}}
}
`

func Create(opt Options) (*project.Project, error) {
	abs, created, err := prepare(opt.Dir)
	if err != nil {
		return nil, err
	}
	p, err := create(abs, opt)
	if err != nil && created {
		os.RemoveAll(abs)
	}
	return p, err
}

func create(abs string, opt Options) (*project.Project, error) {
	module, err := moduleFor(abs, opt.Module)
	if err != nil {
		return nil, err
	}
	if opt.Example != "" {
		return fromExample(abs, module, opt)
	}
	id := opt.Template
	if id == "" {
		id = Template2D
	}
	rep := strings.NewReplacer(
		"{{module}}", module,
		"{{template}}", id,
		"{{white}}", asset.BuiltinWhite.String(),
		"{{cube}}", asset.BuiltinCube.String(),
		"{{plane}}", asset.BuiltinPlane.String(),
	)
	if err := writeTemplate(id, abs, rep); err != nil {
		return nil, err
	}
	p, err := project.Load(abs)
	if err != nil {
		return nil, err
	}
	if opt.Native {
		if err := writeNative(p, opt.Engine); err != nil {
			return nil, err
		}
	}
	return p, finish(p)
}

func prepare(dir string) (string, bool, error) {
	if strings.TrimSpace(dir) == "" {
		return "", false, errs.New(errs.VFSBadPath, "project directory is empty")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false, errs.Wrap(err, errs.VFSBadPath, "cannot resolve the project directory")
	}
	entries, err := os.ReadDir(abs)
	switch {
	case os.IsNotExist(err):
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", false, errs.Wrap(err, errs.VFSIO, "cannot create the project directory")
		}
		return abs, true, nil
	case err != nil:
		return "", false, errs.Wrap(err, errs.VFSIO, "cannot read the project directory")
	case len(entries) > 0:
		return "", false, errs.New(errs.ProjectExists, "directory is not empty", log.F("dir", abs))
	}
	return abs, false, nil
}

func moduleFor(abs, given string) (string, error) {
	m := given
	if m == "" {
		m = project.ModuleName(filepath.Base(abs))
	}
	if !project.ValidModule(m) {
		return "", errs.New(errs.ProjectBadFile, "module must match [a-z0-9_/-]+", log.F("module", m))
	}
	if project.ReservedModule(m) {
		return "", errs.New(errs.ProjectBadFile, "module path aego is reserved for the engine", log.F("module", m))
	}
	return m, nil
}

func templateIDs() string {
	ids := make([]string, len(Templates))
	for i, t := range Templates {
		ids[i] = t.ID
	}
	return strings.Join(ids, ", ")
}

const PreviewFile = "preview.png"

func TemplatePreview(id string) ([]byte, error) {
	return fs.ReadFile(res.FS(), path.Join(templatesDir, id, PreviewFile))
}

func ExamplePreview(engine, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(engine, "examples", name, PreviewFile))
}

func TemplateFiles(id string) []string {
	root := path.Join(templatesDir, id)
	var out []string
	fs.WalkDir(res.FS(), root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if name := d.Name(); name == PreviewFile || strings.HasPrefix(name, ".") {
			return nil
		}
		out = append(out, strings.TrimPrefix(p, root+"/"))
		return nil
	})
	sort.Strings(out)
	return out
}

func writeTemplate(id, dst string, rep *strings.Replacer) error {
	src := res.FS()
	root := path.Join(templatesDir, id)
	if _, err := fs.Stat(src, path.Join(root, project.FileName)); err != nil {
		return errs.New(errs.ProjectBadFile, "unknown project template "+id, log.F("templates", templateIDs()))
	}
	err := fs.WalkDir(src, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if rel == PreviewFile {
			return nil
		}
		data, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, []byte(rep.Replace(string(data))), 0o644)
	})
	if err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write the project template")
	}
	return nil
}

func finish(p *project.Project) error {
	if _, err := p.EnsureDirs(); err != nil {
		return err
	}
	for _, d := range []string{p.CacheDir(), filepath.Join(p.Root, "exports")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return errs.Wrap(err, errs.VFSIO, "cannot create the project layout")
		}
	}
	if err := sceneMetas(p); err != nil {
		return err
	}
	if err := scriptMetas(p); err != nil {
		return err
	}
	return p.Save()
}

func sceneMetas(p *project.Project) error {
	return filepath.WalkDir(p.ScenesDir(), func(file string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(file) != ".aescene" {
			return err
		}
		if _, err := os.Stat(file + pipeline.MetaSuffix); err == nil {
			return nil
		}
		meta := pipeline.NewMeta("scene", 1, nil)
		if err := os.WriteFile(file+pipeline.MetaSuffix, pipeline.WriteMeta(meta), 0o644); err != nil {
			return errs.Wrap(err, errs.VFSIO, "cannot write a scene sidecar")
		}
		if p.MainScene.IsZero() && filepath.Base(file) == "main.aescene" {
			p.MainScene = meta.ID
		}
		return nil
	})
}

// scriptMetas gives the template's scripts their sidecars, so a new project checks clean (META001).
func scriptMetas(p *project.Project) error {
	return filepath.WalkDir(p.ScriptsDir(), func(file string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(file) != ".aes" {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if _, err := os.Stat(file + pipeline.MetaSuffix); err == nil {
			return nil
		}
		if err := os.WriteFile(file+pipeline.MetaSuffix, pipeline.WriteMeta(pipeline.NewMeta("script", 1, nil)), 0o644); err != nil {
			return errs.Wrap(err, errs.VFSIO, "cannot write a script sidecar")
		}
		return nil
	})
}

func writeNative(p *project.Project, engine string) error {
	if engine == "" {
		return errs.New(errs.ProjectBadFile, "native projects need the engine source tree, pass --engine or set AEGO_ROOT")
	}
	if err := os.MkdirAll(p.NativeDir(), 0o755); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot create native/")
	}
	if err := os.WriteFile(filepath.Join(p.NativeDir(), "module.go"), []byte(nativeStarter), 0o644); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write native/module.go")
	}
	gomod := "module " + p.Module + "\n\ngo 1.27\n\nrequire aego v0.0.0\n\nreplace aego => " + filepath.ToSlash(engine) + "\n"
	if err := os.WriteFile(filepath.Join(p.Root, "go.mod"), []byte(gomod), 0o644); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write go.mod")
	}
	p.Native = true
	return nil
}

func fromExample(abs, module string, opt Options) (*project.Project, error) {
	if opt.Engine == "" {
		return nil, errs.New(errs.ProjectBadFile, "examples need the engine source tree, pass --engine or set AEGO_ROOT")
	}
	src := filepath.Join(opt.Engine, "examples", opt.Example)
	old, err := project.Load(src)
	if err != nil {
		return nil, errs.Wrap(err, errs.ProjectNotFound, "unknown example "+opt.Example).With(log.F("examples", strings.Join(Examples(opt.Engine), ", ")))
	}
	rename := renamer(old.Module, module)
	engine := []byte(filepath.ToSlash(opt.Engine))
	err = filepath.WalkDir(src, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, file)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel != "." && skipCopy[d.Name()] {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(abs, rel), 0o755)
		}
		if !d.Type().IsRegular() || strings.HasSuffix(d.Name(), "_test.go") || rel == PreviewFile {
			return nil
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if renameExt[filepath.Ext(file)] || d.Name() == "go.mod" {
			data = rename(data)
		}
		if d.Name() == "go.mod" {
			data = replaceAego.ReplaceAllFunc(data, func(m []byte) []byte {
				head := replaceAego.FindSubmatch(m)[1]
				return append(append([]byte{}, head...), engine...)
			})
		}
		return os.WriteFile(filepath.Join(abs, rel), data, 0o644)
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.VFSIO, "cannot copy the example")
	}
	p, err := project.Load(abs)
	if err != nil {
		return nil, err
	}
	p.Name = filepath.Base(abs)
	return p, finish(p)
}

func renamer(old, next string) func([]byte) []byte {
	if old == "" || old == next {
		return func(b []byte) []byte { return b }
	}
	re := regexp.MustCompile(`(?m)(^|[^A-Za-z0-9_./-]|"script/)` + regexp.QuoteMeta(old) + `([^A-Za-z0-9_-]|$)`)
	repl := []byte("${1}" + next + "${2}")
	return func(b []byte) []byte { return re.ReplaceAll(b, repl) }
}

func Examples(engine string) []string {
	entries, err := os.ReadDir(filepath.Join(engine, "examples"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(engine, "examples", e.Name(), project.FileName)); err == nil {
			out = append(out, e.Name())
		}
	}
	return out
}
