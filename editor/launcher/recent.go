package launcher

import (
	"os"
	"path/filepath"
	"time"

	"aego/editor/document"
	"aego/format/aetext"
	"aego/project"
)

const (
	MaxRecent     = 20
	recentDir     = "aego"
	recentFile    = "recent"
	recentVersion = 1
)

type Entry struct {
	Path    string
	Name    string
	Time    time.Time
	Missing bool
}

func RecentPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, recentDir, recentFile), nil
}

func LoadRecent(file string) ([]Entry, error) {
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	doc, err := aetext.Parse(data, recentFile)
	if err != nil {
		return nil, err
	}
	var out []Entry
	doc.EachBlock("project", func(n *aetext.Node) {
		if n.Label == "" {
			return
		}
		e := Entry{Path: n.Label}
		if v, ok := n.Attr("name"); ok {
			e.Name = v.Str
		}
		if v, ok := n.Attr("opened"); ok {
			e.Time = time.Unix(v.AsInt(), 0)
		}
		_, err := os.Stat(filepath.Join(e.Path, project.FileName))
		e.Missing = err != nil
		out = append(out, e)
	})
	if len(out) > MaxRecent {
		out = out[:MaxRecent]
	}
	return out, nil
}

func SaveRecent(file string, list []Entry) error {
	doc := &aetext.Node{Head: "recent"}
	doc.Attrs = append(doc.Attrs, aetext.Field{Name: "version", Value: aetext.Int(recentVersion)})
	for _, e := range list {
		n := &aetext.Node{Head: "project", Label: e.Path}
		n.Attrs = append(n.Attrs,
			aetext.Field{Name: "name", Value: aetext.String(e.Name)},
			aetext.Field{Name: "opened", Value: aetext.Int(e.Time.Unix())},
		)
		doc.Blocks = append(doc.Blocks, n)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return document.WriteAtomic(file, aetext.Write(doc))
}

func Touch(file, root, name string, now time.Time) error {
	list, _ := LoadRecent(file)
	next := []Entry{{Path: root, Name: name, Time: now}}
	for _, e := range list {
		if e.Path != root {
			next = append(next, e)
		}
	}
	if len(next) > MaxRecent {
		next = next[:MaxRecent]
	}
	return SaveRecent(file, next)
}

func Forget(file, root string) error {
	list, err := LoadRecent(file)
	if err != nil {
		return err
	}
	next := list[:0]
	for _, e := range list {
		if e.Path != root {
			next = append(next, e)
		}
	}
	return SaveRecent(file, next)
}
