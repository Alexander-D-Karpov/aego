package favorites

import (
	"path"
	"slices"

	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/editor/core/state"
	"aego/format/aetext"
)

const (
	CommandNode  = "favorites.toggle_node"
	CommandAsset = "favorites.toggle_asset"
	fileVersion  = 1
)

type Kind uint8

const (
	Asset Kind = iota
	Node
	Command
)

var kindNames = [...]string{"asset", "node", "command"}

type Item struct {
	Kind    Kind
	Asset   ids.AssetID
	Scene   string
	Node    ids.NodeID
	Command string
	Label   string
}

func (it Item) Same(o Item) bool {
	if it.Kind != o.Kind {
		return false
	}
	switch it.Kind {
	case Asset:
		return it.Asset == o.Asset
	case Node:
		return it.Scene == o.Scene && it.Node == o.Node
	}
	return it.Command == o.Command
}

type Store struct {
	root  string
	items []Item
	rev   uint64
	err   error
}

func Load(root string) *Store {
	s := &Store{root: root}
	if data, ok := state.LoadFavorites(root); ok {
		s.items, s.err = decode(data)
	}
	return s
}

func (s *Store) Err() error {
	return s.err
}

func (s *Store) Items() []Item {
	return s.items
}

func (s *Store) Revision() uint64 {
	return s.rev
}

func (s *Store) index(it Item) int {
	return slices.IndexFunc(s.items, it.Same)
}

func (s *Store) Has(it Item) bool {
	return s.index(it) >= 0
}

func (s *Store) Toggle(it Item) (bool, error) {
	if i := s.index(it); i >= 0 {
		s.items = slices.Delete(s.items, i, i+1)
		return false, s.save()
	}
	s.items = append(s.items, it)
	return true, s.save()
}

func (s *Store) Remove(it Item) error {
	i := s.index(it)
	if i < 0 {
		return nil
	}
	s.items = slices.Delete(s.items, i, i+1)
	return s.save()
}

func (s *Store) save() error {
	s.rev++
	if s.root == "" {
		return nil
	}
	return state.SaveFavorites(s.root, encode(s.items))
}

func encode(items []Item) []byte {
	root := &aetext.Node{Head: "favorites"}
	root.Attrs = append(root.Attrs, aetext.Field{Name: "version", Value: aetext.Int(fileVersion)})
	for _, it := range items {
		n := &aetext.Node{Head: "favorite"}
		n.Attrs = append(n.Attrs,
			aetext.Field{Name: "kind", Value: aetext.String(kindNames[it.Kind])},
			aetext.Field{Name: "label", Value: aetext.String(it.Label)})
		switch it.Kind {
		case Asset:
			n.Attrs = append(n.Attrs, aetext.Field{Name: "asset", Value: aetext.AssetRef(it.Asset.String())})
		case Node:
			n.Attrs = append(n.Attrs,
				aetext.Field{Name: "scene", Value: aetext.String(it.Scene)},
				aetext.Field{Name: "node", Value: aetext.NodeRef(it.Node.String())})
		case Command:
			n.Attrs = append(n.Attrs, aetext.Field{Name: "command", Value: aetext.String(it.Command)})
		}
		root.Blocks = append(root.Blocks, n)
	}
	return aetext.Write(root)
}

func decode(data []byte) ([]Item, error) {
	doc, err := aetext.Parse(data, state.FavoritesFile)
	if err != nil {
		return nil, err
	}
	if doc.Head != "favorites" {
		return nil, errs.New(errs.EditorDocument, "not a favorites file", log.F("head", doc.Head))
	}
	var out []Item
	doc.EachBlock("favorite", func(n *aetext.Node) {
		k, _ := n.Attr("kind")
		it := Item{Kind: Kind(slices.Index(kindNames[:], k.Str))}
		if v, ok := n.Attr("label"); ok {
			it.Label = v.Str
		}
		ok := true
		switch k.Str {
		case "asset":
			v, _ := n.Attr("asset")
			id, err := ids.ParseAssetID(v.Str)
			it.Asset, ok = id, err == nil
		case "node":
			sc, _ := n.Attr("scene")
			v, _ := n.Attr("node")
			id, err := ids.ParseNodeID(v.Str)
			it.Scene, it.Node, ok = sc.Str, id, err == nil && sc.Str != ""
		case "command":
			v, _ := n.Attr("command")
			it.Command, ok = v.Str, v.Str != ""
		default:
			ok = false
		}
		if ok {
			out = append(out, it)
		}
	})
	return out, nil
}

type Env struct {
	Pipeline      *pipeline.Pipeline
	Docs          services.Documents
	Selection     services.Selection
	Commands      *command.Registry
	Nav           services.Navigation
	ShowHierarchy func()
}

func Describe(e Env, it Item) (label, detail, icon string, ok bool) {
	switch it.Kind {
	case Asset:
		if e.Pipeline != nil {
			if rel, found := e.Pipeline.PathOf(it.Asset); found {
				return path.Base(rel), path.Dir(rel), "file.scene", true
			}
		}
		return it.Label, "missing asset", "file.unknown", false
	case Node:
		return it.Label, it.Scene, "cat.node", true
	}
	if e.Commands != nil {
		if d, found := e.Commands.Get(it.Command); found {
			return d.Title(), d.Category + "  " + d.KeysText(), "tool.select", true
		}
	}
	return it.Label, "unknown action", "tool.select", false
}

func Open(e Env, it Item) error {
	switch it.Kind {
	case Asset:
		rel, ok := "", false
		if e.Pipeline != nil {
			rel, ok = e.Pipeline.PathOf(it.Asset)
		}
		if !ok {
			return errs.New(errs.EditorCommand, it.Label+" is no longer in the project")
		}
		e.Nav.Open(context.Location{Path: rel, Offset: -1})
	case Node:
		d, err := e.Docs.Documents().Open(it.Scene)
		if err != nil {
			return err
		}
		s := d.Scene()
		if h := s.Handle(it.Node); h.IsZero() || !s.IsAlive(h) {
			return errs.New(errs.EditorCommand, it.Label+" is no longer in "+it.Scene)
		}
		e.Selection.Select(it.Node)
		if e.ShowHierarchy != nil {
			e.ShowHierarchy()
		}
	case Command:
		if e.Commands == nil || !e.Commands.Enabled(it.Command) {
			return errs.New(errs.EditorCommand, "this action is not available right now")
		}
		return e.Commands.Run(it.Command)
	}
	return nil
}
