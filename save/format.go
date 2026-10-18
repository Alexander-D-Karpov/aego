package save

import (
	"time"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
)

const (
	FormatVersion = 1
	Ext           = ".aesave"
	PreviewExt    = ".png"
)

type Meta struct {
	Name     string
	Game     string
	Engine   string
	Scene    ids.AssetID
	Date     time.Time
	Playtime float64
	Objects  int
	Counter  uint64
}

type Section struct {
	ID      string
	Version int
	Node    *aetext.Node
}

type Doc struct {
	Meta      Meta
	Nodes     []*aetext.Node
	Spawned   []*aetext.Node
	Destroyed []ids.NodeID
	Global    []aetext.Field
	Sections  []Section
}

func (d *Doc) Section(id string) (*Section, bool) {
	for i := range d.Sections {
		if d.Sections[i].ID == id {
			return &d.Sections[i], true
		}
	}
	return nil, false
}

func metaNode(m *Meta) *aetext.Node {
	n := &aetext.Node{Head: "meta"}
	n.Fields = append(n.Fields,
		aetext.Field{Name: "name", Value: aetext.String(m.Name)},
		aetext.Field{Name: "game", Value: aetext.String(m.Game)},
		aetext.Field{Name: "engine", Value: aetext.String(m.Engine)},
		aetext.Field{Name: "scene", Value: aetext.AssetRef(m.Scene.String())},
		aetext.Field{Name: "date", Value: aetext.String(m.Date.UTC().Format(time.RFC3339))},
		aetext.Field{Name: "playtime", Value: aetext.Float64(m.Playtime)},
		aetext.Field{Name: "objects", Value: aetext.Int(int64(m.Objects))},
		aetext.Field{Name: "counter", Value: aetext.Int(int64(m.Counter))},
	)
	return n
}

func readMeta(n *aetext.Node, name string) (Meta, error) {
	var m Meta
	if n == nil {
		return m, errs.New(errs.SceneIOParse, "save has no meta block", log.F("path", name))
	}
	if v, ok := n.Field("name"); ok {
		m.Name = v.Str
	}
	if v, ok := n.Field("game"); ok {
		m.Game = v.Str
	}
	if v, ok := n.Field("engine"); ok {
		m.Engine = v.Str
	}
	if v, ok := n.Field("scene"); ok && v.Str != "" {
		id, err := ids.ParseAssetID(v.Str)
		if err != nil {
			return m, errs.Wrap(err, errs.SceneIOParse, "save names a malformed scene").With(log.F("path", name))
		}
		m.Scene = id
	}
	if v, ok := n.Field("date"); ok {
		if t, err := time.Parse(time.RFC3339, v.Str); err == nil {
			m.Date = t
		}
	}
	if v, ok := n.Field("playtime"); ok {
		m.Playtime = v.AsFloat()
	}
	if v, ok := n.Field("objects"); ok {
		m.Objects = int(v.AsInt())
	}
	if v, ok := n.Field("counter"); ok {
		m.Counter = uint64(v.AsInt())
	}
	return m, nil
}

func withVersion(attrs []aetext.Field, v int) []aetext.Field {
	out := make([]aetext.Field, 0, len(attrs)+1)
	out = append(out, aetext.Field{Name: "version", Value: aetext.Int(int64(v))})
	for _, a := range attrs {
		if a.Name != "version" {
			out = append(out, a)
		}
	}
	return out
}

func Encode(d *Doc) []byte {
	root := &aetext.Node{Head: "save"}
	root.Attrs = append(root.Attrs, aetext.Field{Name: "version", Value: aetext.Int(FormatVersion)})
	root.Blocks = append(root.Blocks, metaNode(&d.Meta))
	if len(d.Destroyed) > 0 {
		list := make([]aetext.Value, len(d.Destroyed))
		for i, id := range d.Destroyed {
			list[i] = aetext.NodeRef(id.String())
		}
		root.Fields = append(root.Fields, aetext.Field{Name: "destroyed", Value: aetext.List(list...)})
	}
	root.Blocks = append(root.Blocks, d.Nodes...)
	root.Blocks = append(root.Blocks, d.Spawned...)
	if len(d.Global) > 0 {
		root.Blocks = append(root.Blocks, &aetext.Node{Head: "global", Fields: d.Global})
	}
	for _, s := range d.Sections {
		n := &aetext.Node{Head: "section", Label: s.ID}
		if s.Node != nil {
			n.Attrs = s.Node.Attrs
			n.Fields = s.Node.Fields
			n.Blocks = s.Node.Blocks
		}
		n.Attrs = withVersion(n.Attrs, s.Version)
		root.Blocks = append(root.Blocks, n)
	}
	return aetext.Write(root)
}

func Decode(src []byte, name string) (*Doc, error) {
	root, err := aetext.Parse(src, name)
	if err != nil {
		return nil, errs.Wrap(err, errs.SceneIOParse, "cannot parse the save")
	}
	if root.Head != "save" {
		return nil, errs.New(errs.SceneIOParse, "not a save", log.F("path", name), log.F("head", root.Head))
	}
	if v, ok := root.Attr("version"); ok && int(v.AsInt()) > FormatVersion {
		return nil, errs.New(errs.SceneIOVersion, "save was written by a newer engine", log.F("path", name), log.F("version", v.AsInt()))
	}
	d := &Doc{}
	if d.Meta, err = readMeta(root.Block("meta", ""), name); err != nil {
		return nil, err
	}
	if v, ok := root.Field("destroyed"); ok {
		for _, e := range v.List {
			id, err := ids.ParseNodeID(e.Str)
			if err != nil {
				return nil, errs.Wrap(err, errs.SceneIOParse, "save lists a malformed destroyed node").With(log.F("path", name))
			}
			d.Destroyed = append(d.Destroyed, id)
		}
	}
	for _, b := range root.Blocks {
		switch b.Head {
		case "node":
			d.Nodes = append(d.Nodes, b)
		case "spawned":
			d.Spawned = append(d.Spawned, b)
		case "global":
			d.Global = append(d.Global, b.Fields...)
		case "section":
			v := 1
			if a, ok := b.Attr("version"); ok {
				v = int(a.AsInt())
			}
			d.Sections = append(d.Sections, Section{ID: b.Label, Version: v, Node: b})
		}
	}
	return d, nil
}
