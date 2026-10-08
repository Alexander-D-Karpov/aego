package asset

import (
	"sort"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
)

const IndexVersion = 1

type Artifact struct {
	Kind ArtifactKind
	Name string
	Path string
	Hash string
	Size int64
}

type Entry struct {
	ID        ids.AssetID
	Kind      ArtifactKind
	Name      string
	Source    string
	Missing   bool
	Error     string
	Deps      []ids.AssetID
	Artifacts []Artifact
}

func (e *Entry) Artifact(kind ArtifactKind) (Artifact, bool) {
	for _, a := range e.Artifacts {
		if a.Kind == kind {
			return a, true
		}
	}
	return Artifact{}, false
}

type Index struct {
	Version int
	entries map[ids.AssetID]*Entry
	order   []ids.AssetID
}

func NewIndex() *Index {
	return &Index{Version: IndexVersion, entries: map[ids.AssetID]*Entry{}}
}

func (i *Index) Clone() *Index {
	out := &Index{
		Version: i.Version,
		entries: make(map[ids.AssetID]*Entry, len(i.entries)),
		order:   append([]ids.AssetID(nil), i.order...),
	}
	for id, e := range i.entries {
		cp := *e
		out.entries[id] = &cp
	}
	return out
}

func (i *Index) Len() int {
	return len(i.order)
}

func (i *Index) Lookup(id ids.AssetID) (*Entry, bool) {
	e, ok := i.entries[id]
	return e, ok
}

func (i *Index) Put(e Entry) {
	if _, exists := i.entries[e.ID]; !exists {
		i.order = append(i.order, e.ID)
	}
	copyEntry := e
	i.entries[e.ID] = &copyEntry
}

func (i *Index) Delete(id ids.AssetID) {
	if _, ok := i.entries[id]; !ok {
		return
	}
	delete(i.entries, id)
	for n, existing := range i.order {
		if existing == id {
			i.order = append(i.order[:n], i.order[n+1:]...)
			return
		}
	}
}

func (i *Index) Each(fn func(*Entry)) {
	for _, id := range i.order {
		if e, ok := i.entries[id]; ok {
			fn(e)
		}
	}
}

func (i *Index) Entries() []*Entry {
	out := make([]*Entry, 0, len(i.order))
	i.Each(func(e *Entry) { out = append(out, e) })
	return out
}

func (i *Index) Sort() {
	sort.Slice(i.order, func(a, b int) bool {
		return i.entries[i.order[a]].Name < i.entries[i.order[b]].Name
	})
}

func WriteIndex(i *Index) []byte {
	root := &aetext.Node{Head: "index"}
	root.Attrs = append(root.Attrs, aetext.Field{Name: "version", Value: aetext.Int(int64(i.Version))})
	i.Each(func(e *Entry) {
		n := &aetext.Node{Head: "asset", Label: e.ID.String()}
		n.Attrs = append(n.Attrs, aetext.Field{Name: "kind", Value: aetext.String(string(e.Kind))})
		n.Fields = append(n.Fields,
			aetext.Field{Name: "name", Value: aetext.String(e.Name)},
			aetext.Field{Name: "source", Value: aetext.String(e.Source)},
		)
		if e.Missing {
			n.Fields = append(n.Fields, aetext.Field{Name: "missing", Value: aetext.Bool(true)})
		}
		if e.Error != "" {
			n.Fields = append(n.Fields, aetext.Field{Name: "error", Value: aetext.String(e.Error)})
		}
		if len(e.Deps) > 0 {
			deps := make([]aetext.Value, len(e.Deps))
			for k, d := range e.Deps {
				deps[k] = aetext.AssetRef(d.String())
			}
			n.Fields = append(n.Fields, aetext.Field{Name: "deps", Value: aetext.List(deps...)})
		}
		for _, a := range e.Artifacts {
			an := &aetext.Node{Head: "artifact", Label: string(a.Kind)}
			if a.Name != "" {
				an.Fields = append(an.Fields, aetext.Field{Name: "name", Value: aetext.String(a.Name)})
			}
			an.Fields = append(an.Fields,
				aetext.Field{Name: "path", Value: aetext.String(a.Path)},
				aetext.Field{Name: "hash", Value: aetext.String(a.Hash)},
				aetext.Field{Name: "size", Value: aetext.Int(a.Size)},
			)
			n.Blocks = append(n.Blocks, an)
		}
		root.Blocks = append(root.Blocks, n)
	})
	return aetext.Write(root)
}

func ReadIndex(src []byte, name string) (*Index, error) {
	doc, err := aetext.Parse(src, name)
	if err != nil {
		return nil, err
	}
	if doc.Head != "index" {
		return nil, errs.New(errs.AssetBadIndex, "not an asset index", log.F("head", doc.Head))
	}
	idx := NewIndex()
	if v, ok := doc.Attr("version"); ok {
		idx.Version = int(v.AsInt())
	}
	if idx.Version > IndexVersion {
		return nil, errs.New(errs.AssetBadIndex, "index was written by a newer engine",
			log.F("version", idx.Version), log.F("supported", IndexVersion))
	}
	var parseErr error
	doc.EachBlock("asset", func(n *aetext.Node) {
		if parseErr != nil {
			return
		}
		id, err := ids.ParseAssetID(n.Label)
		if err != nil {
			parseErr = err
			return
		}
		e := Entry{ID: id}
		if v, ok := n.Attr("kind"); ok {
			e.Kind = ArtifactKind(v.Str)
		}
		if v, ok := n.Field("name"); ok {
			e.Name = v.Str
		}
		if v, ok := n.Field("source"); ok {
			e.Source = v.Str
		}
		if v, ok := n.Field("missing"); ok {
			e.Missing = v.Bool
		}
		if v, ok := n.Field("error"); ok {
			e.Error = v.Str
		}
		if v, ok := n.Field("deps"); ok {
			for _, d := range v.List {
				dep, err := ids.ParseAssetID(d.Str)
				if err != nil {
					parseErr = err
					return
				}
				e.Deps = append(e.Deps, dep)
			}
		}
		n.EachBlock("artifact", func(an *aetext.Node) {
			a := Artifact{Kind: ArtifactKind(an.Label)}
			if v, ok := an.Field("name"); ok {
				a.Name = v.Str
			}
			if v, ok := an.Field("path"); ok {
				a.Path = v.Str
			}
			if v, ok := an.Field("hash"); ok {
				a.Hash = v.Str
			}
			if v, ok := an.Field("size"); ok {
				a.Size = v.AsInt()
			}
			e.Artifacts = append(e.Artifacts, a)
		})
		idx.Put(e)
	})
	if parseErr != nil {
		return nil, parseErr
	}
	return idx, nil
}
