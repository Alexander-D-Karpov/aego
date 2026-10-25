// Package prefs holds the editor preferences of the user, shared by every project and the launcher (D358).
package prefs

import (
	"os"
	"path/filepath"

	"aego/core/errs"
	"aego/editor/core/document"
	"aego/format/aetext"
	"aego/ui/theme"
)

const (
	File    = "prefs.aetext"
	version = 1
)

type Prefs struct {
	Theme theme.Mode
}

func Default() *Prefs {
	return &Prefs{Theme: theme.ModeSystem}
}

func Load(dir string) (*Prefs, error) {
	p := Default()
	if dir == "" {
		return p, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, File))
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return p, errs.Wrap(err, errs.EditorDocument, "cannot read the editor preferences")
	}
	doc, err := aetext.Parse(data, File)
	if err != nil || doc.Head != "prefs" {
		return p, err
	}
	if v, ok := doc.Field("theme"); ok {
		name := v.Str
		if name == "" {
			name = v.Ident
		}
		p.Theme = theme.ParseMode(name)
	}
	return p, nil
}

func Save(dir string, p *Prefs) error {
	if dir == "" {
		return nil
	}
	root := &aetext.Node{Head: "prefs", Attrs: []aetext.Field{{Name: "version", Value: aetext.Int(version)}}}
	root.Fields = append(root.Fields, aetext.Field{Name: "theme", Value: aetext.Ident(p.Theme.String())})
	return document.WriteAtomic(filepath.Join(dir, File), aetext.Write(root))
}
