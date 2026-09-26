package project

import (
	"os"
	"path/filepath"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
)

const (
	FileName = "project.aego"
	Version  = 1
)

type WindowSettings struct {
	Width  int
	Height int
	Title  string
}

type Project struct {
	Root      string
	Name      string
	Module    string
	MainScene ids.AssetID
	Window    WindowSettings
	Native    bool
	Version   int
}

func (p *Project) AssetsDir() string {
	return filepath.Join(p.Root, "assets")
}

func (p *Project) ScenesDir() string {
	return filepath.Join(p.Root, "scenes")
}

func (p *Project) NativeDir() string {
	return filepath.Join(p.Root, "native")
}

func (p *Project) CacheDir() string {
	return filepath.Join(p.Root, ".aego", "cache")
}

func (p *Project) GeneratedDir() string {
	return filepath.Join(p.Root, ".aego", "generated")
}

func (p *Project) BuildDir() string {
	return filepath.Join(p.Root, ".aego", "build")
}

func (p *Project) FilePath() string {
	return filepath.Join(p.Root, FileName)
}

func Find(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", errs.Wrap(err, errs.VFSBadPath, "cannot resolve the start directory")
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, FileName)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errs.New(errs.ProjectNotFound, "no project.aego found in this directory or its parents",
				log.F("start", start))
		}
		dir = parent
	}
}

func Load(root string) (*Project, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, errs.Wrap(err, errs.VFSBadPath, "cannot resolve the project directory")
	}
	data, err := os.ReadFile(filepath.Join(abs, FileName))
	if err != nil {
		return nil, errs.Wrap(err, errs.ProjectNotFound, "cannot read project.aego").
			With(log.F("root", abs))
	}
	doc, err := aetext.Parse(data, FileName)
	if err != nil {
		return nil, err
	}
	if doc.Head != "project" {
		return nil, errs.New(errs.ProjectBadFile, "not a project file", log.F("head", doc.Head))
	}
	p := &Project{Root: abs, Version: Version, Window: WindowSettings{Width: 1280, Height: 720}}
	if v, ok := doc.Attr("version"); ok {
		p.Version = int(v.AsInt())
		if p.Version > Version {
			return nil, errs.New(errs.ProjectBadFile, "project was written by a newer engine",
				log.F("version", p.Version), log.F("supported", Version))
		}
	}
	if v, ok := doc.Field("name"); ok {
		p.Name = v.Str
	}
	if v, ok := doc.Field("module"); ok {
		p.Module = v.Str
	}
	if v, ok := doc.Field("native"); ok {
		p.Native = v.Bool
	}
	if v, ok := doc.Field("mainScene"); ok && v.Str != "" {
		id, err := ids.ParseAssetID(v.Str)
		if err != nil {
			return nil, errs.Wrap(err, errs.ProjectBadFile, "mainScene is not a valid asset id")
		}
		p.MainScene = id
	}
	if w := doc.Block("window", ""); w != nil {
		if v, ok := w.Field("width"); ok {
			p.Window.Width = int(v.AsInt())
		}
		if v, ok := w.Field("height"); ok {
			p.Window.Height = int(v.AsInt())
		}
		if v, ok := w.Field("title"); ok {
			p.Window.Title = v.Str
		}
	}
	if p.Name == "" {
		p.Name = filepath.Base(abs)
	}
	if p.Window.Title == "" {
		p.Window.Title = p.Name
	}
	if p.Window.Width <= 0 {
		p.Window.Width = 1280
	}
	if p.Window.Height <= 0 {
		p.Window.Height = 720
	}
	return p, nil
}

func (p *Project) Save() error {
	root := &aetext.Node{Head: "project"}
	root.Attrs = append(root.Attrs, aetext.Field{Name: "version", Value: aetext.Int(Version)})
	root.Fields = append(root.Fields,
		aetext.Field{Name: "name", Value: aetext.String(p.Name)},
		aetext.Field{Name: "module", Value: aetext.String(p.Module)},
		aetext.Field{Name: "mainScene", Value: aetext.AssetRef(p.MainScene.String())},
		aetext.Field{Name: "native", Value: aetext.Bool(p.Native)},
	)
	window := &aetext.Node{Head: "window"}
	window.Fields = append(window.Fields,
		aetext.Field{Name: "width", Value: aetext.Int(int64(p.Window.Width))},
		aetext.Field{Name: "height", Value: aetext.Int(int64(p.Window.Height))},
		aetext.Field{Name: "title", Value: aetext.String(p.Window.Title)},
	)
	root.Blocks = append(root.Blocks, window)
	if err := os.WriteFile(p.FilePath(), aetext.Write(root), 0o644); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write project.aego")
	}
	return nil
}

func Create(root, name string, native bool) (*Project, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, errs.Wrap(err, errs.VFSBadPath, "cannot resolve the project directory")
	}
	if _, err := os.Stat(filepath.Join(abs, FileName)); err == nil {
		return nil, errs.New(errs.ProjectExists, "a project already exists here", log.F("root", abs))
	}
	if name == "" {
		name = filepath.Base(abs)
	}
	p := &Project{
		Root:   abs,
		Name:   name,
		Module: moduleName(name),
		Native: native,
		Window: WindowSettings{Width: 1280, Height: 720, Title: name},
	}
	dirs := []string{
		p.AssetsDir(),
		p.ScenesDir(),
		p.CacheDir(),
		filepath.Join(p.Root, "exports"),
	}
	if native {
		dirs = append(dirs, p.NativeDir(), p.GeneratedDir())
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, errs.Wrap(err, errs.VFSIO, "cannot create the project layout")
		}
	}
	if err := p.Save(); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(abs, ".gitignore"), []byte(gitignore), 0o644); err != nil {
		return nil, errs.Wrap(err, errs.VFSIO, "cannot write .gitignore")
	}
	return p, nil
}

const gitignore = `.aego/
exports/
`

func moduleName(name string) string {
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'A' && c <= 'Z':
			out = append(out, c+32)
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			out = append(out, c)
		case c == ' ' || c == '_' || c == '-':
			if len(out) > 0 && out[len(out)-1] != '-' {
				out = append(out, '-')
			}
		}
	}
	if len(out) == 0 {
		return "game"
	}
	return string(out)
}
