package playground

import (
	"embed"
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"aego/format/aetext"
	"aego/script/workspace"
)

//go:embed examples
var exampleFS embed.FS

const (
	examplesRoot = "examples"
	projectFile  = "project.aego"
	scenesDir    = "scenes"
	sceneExt     = ".aescene"
	ScenePrefix  = "scenes/"
)

var (
	coreBlocks = []string{"window", "defaults"}
	mainFile   = "main" + workspace.Ext
)

type sceneExample struct {
	name  string
	title string
	scene []byte
	files map[string]string
	main  string
}

type exampleProject struct {
	doc    *aetext.Node
	scenes []sceneExample
}

func loadProject() (*exampleProject, error) {
	p := &exampleProject{}
	raw, err := fs.ReadFile(exampleFS, path.Join(examplesRoot, projectFile))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return p, nil
	case err != nil:
		return nil, err
	}
	if p.doc, err = aetext.Parse(raw, projectFile); err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(exampleFS, path.Join(examplesRoot, scenesDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != sceneExt {
			continue
		}
		ex, err := loadScene(strings.TrimSuffix(e.Name(), sceneExt))
		if err != nil {
			return nil, err
		}
		p.scenes = append(p.scenes, ex)
	}
	return p, nil
}

func loadScene(name string) (sceneExample, error) {
	ex := sceneExample{name: name, title: titleOf(name), files: map[string]string{}}
	var err error
	if ex.scene, err = fs.ReadFile(exampleFS, path.Join(examplesRoot, scenesDir, name+sceneExt)); err != nil {
		return ex, err
	}
	root := path.Join(examplesRoot, ScriptsDir)
	err = fs.WalkDir(exampleFS, path.Join(root, name), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(p) != workspace.Ext || strings.HasSuffix(p, "_test"+workspace.Ext) {
			return err
		}
		data, err := fs.ReadFile(exampleFS, p)
		if err != nil {
			return err
		}
		ex.files[strings.TrimPrefix(p, root+"/")] = string(data)
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	ex.main = name + "/" + mainFile
	if _, ok := ex.files[ex.main]; !ok {
		keys := make([]string, 0, len(ex.files))
		for k := range ex.files {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		ex.main = ""
		if len(keys) > 0 {
			ex.main = keys[0]
		}
	}
	return ex, err
}

func titleOf(name string) string {
	s := strings.NewReplacer("-", " ", "_", " ").Replace(name)
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

func (p *exampleProject) block(head string) *aetext.Node {
	if p.doc == nil {
		return nil
	}
	return p.doc.Block(head, "")
}

func (p *exampleProject) find(name string) (*sceneExample, bool) {
	for i := range p.scenes {
		if p.scenes[i].name == name {
			return &p.scenes[i], true
		}
	}
	return nil, false
}
