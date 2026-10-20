package pipeline

import (
	"os"
	"path/filepath"
	"slices"
	"time"

	"aego/asset"
	"aego/core/ids"
	"aego/format/aetext"
)

const (
	graveFile    = "graveyard.aetext"
	graveMax     = 2000
	graveVersion = 1
)

type Grave struct {
	ID   ids.AssetID
	Path string
	Kind asset.ArtifactKind
	When time.Time
}

func (p *Pipeline) bury(id ids.AssetID, path string) {
	g := Grave{ID: id, Path: path, When: time.Now()}
	if e, ok := p.index.Lookup(id); ok {
		g.Kind = e.Kind
	}
	p.graves = slices.DeleteFunc(p.graves, func(x Grave) bool { return x.ID == id })
	p.graves = append(p.graves, g)
	if over := len(p.graves) - graveMax; over > 0 {
		p.graves = slices.Delete(p.graves, 0, over)
	}
	p.gravesDirty = true
}

func (p *Pipeline) Graveyard() []Grave {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]Grave, 0, len(p.graves))
	for _, g := range p.graves {
		if _, alive := p.byID[g.ID]; !alive {
			out = append(out, g)
		}
	}
	return out
}

func (p *Pipeline) Grave(id ids.AssetID) (Grave, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if _, alive := p.byID[id]; alive {
		return Grave{}, false
	}
	for i := len(p.graves) - 1; i >= 0; i-- {
		if p.graves[i].ID == id {
			return p.graves[i], true
		}
	}
	return Grave{}, false
}

func (p *Pipeline) gravePath() string {
	return filepath.Join(p.project.CacheDir(), graveFile)
}

func (p *Pipeline) loadGraves() {
	data, err := os.ReadFile(p.gravePath())
	if err != nil {
		return
	}
	doc, err := aetext.Parse(data, graveFile)
	if err != nil || doc.Head != "graveyard" {
		return
	}
	doc.EachBlock("grave", func(n *aetext.Node) {
		id, err := ids.ParseAssetID(n.Label)
		if err != nil {
			return
		}
		g := Grave{ID: id}
		if v, ok := n.Attr("path"); ok {
			g.Path = v.Str
		}
		if v, ok := n.Attr("kind"); ok {
			g.Kind = asset.ArtifactKind(v.Str)
		}
		if v, ok := n.Attr("time"); ok {
			g.When = time.Unix(v.AsInt(), 0)
		}
		p.graves = append(p.graves, g)
	})
}

func (p *Pipeline) saveGraves() error {
	if p.shared || !p.gravesDirty {
		return nil
	}
	root := &aetext.Node{Head: "graveyard", Attrs: []aetext.Field{{Name: "version", Value: aetext.Int(graveVersion)}}}
	for _, g := range p.graves {
		root.Blocks = append(root.Blocks, &aetext.Node{Head: "grave", Label: g.ID.String(), Attrs: []aetext.Field{
			{Name: "path", Value: aetext.String(g.Path)},
			{Name: "kind", Value: aetext.String(string(g.Kind))},
			{Name: "time", Value: aetext.Int(g.When.Unix())},
		}})
	}
	path := p.gravePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, aetext.Write(root), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	p.gravesDirty = false
	return nil
}
