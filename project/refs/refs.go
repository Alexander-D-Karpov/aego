package refs

import (
	"os"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/format/aetext"
	"aego/project"
	"aego/scene/mutate"
)

var (
	vAsset = aetext.AssetRef("").Kind
	vNode  = aetext.NodeRef("").Kind
)

type Source interface {
	Assets() []pipeline.AssetInfo
	Entry(id ids.AssetID) (asset.Entry, bool)
	PathOf(id ids.AssetID) (string, bool)
	Lookup(path string) (ids.AssetID, bool)
	Grave(id ids.AssetID) (pipeline.Grave, bool)
	Abs(rel string) string
}

type Ref struct {
	Asset   ids.AssetID
	Path    string
	Kind    asset.ArtifactKind
	Project bool
}

type Place struct {
	Node  ids.NodeID
	Path  string
	Owner string
	Field string
}

func (p Place) String() string {
	var parts []string
	for _, s := range []string{p.Path, p.Owner, p.Field} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " · ")
}

type Hit struct {
	Ref    Ref
	Places []Place
	Err    error
}

func Token(id ids.AssetID) []byte {
	return []byte(mutate.FormatValue(aetext.AssetRef(id.String())))
}

func Sort(list []Ref) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Project != list[j].Project {
			return list[i].Project
		}
		return list[i].Path < list[j].Path
	})
}

func Referrers(p *project.Project, src Source, target ids.AssetID) []Ref {
	var out []Ref
	if p != nil && !target.IsZero() && p.MainScene == target {
		out = append(out, Ref{Project: true, Path: project.FileName})
	}
	if src == nil {
		return out
	}
	for _, a := range src.Assets() {
		if a.ID == target {
			continue
		}
		if e, ok := src.Entry(a.ID); ok && slices.Contains(e.Deps, target) {
			out = append(out, Ref{Asset: a.ID, Path: a.Path, Kind: a.Kind})
		}
	}
	Sort(out)
	return out
}

func Places(data []byte, name string, target ids.AssetID) ([]Place, error) {
	doc, err := aetext.Parse(data, name)
	if err != nil {
		return nil, err
	}
	return textPlaces(doc, vAsset, target.String()), nil
}

func FileHit(src Source, r Ref, target ids.AssetID) Hit {
	h := Hit{Ref: r}
	if r.Project {
		h.Places = []Place{{Field: "mainScene"}}
		return h
	}
	data, err := os.ReadFile(src.Abs(r.Path))
	if err != nil {
		h.Err = err
		return h
	}
	h.Places, h.Err = Places(data, r.Path, target)
	return h
}

func Find(p *project.Project, src Source, target ids.AssetID) []Hit {
	var out []Hit
	for _, r := range Referrers(p, src, target) {
		out = append(out, FileHit(src, r, target))
	}
	return out
}

func textPlaces(doc *aetext.Node, kind aetext.ValueKind, target string) []Place {
	var out []Place
	var visit func(n *aetext.Node, pl Place)
	var value func(v aetext.Value, field string, pl Place)
	value = func(v aetext.Value, field string, pl Place) {
		switch {
		case v.Kind == kind && v.Str == target:
			pl.Field = field
			out = append(out, pl)
		case v.Kind == aetext.VList:
			for i, it := range v.List {
				value(it, field+"["+strconv.Itoa(i)+"]", pl)
			}
		case v.Kind == aetext.VStruct:
			for _, f := range v.Obj {
				value(f.Value, field+"."+f.Name, pl)
			}
		}
	}
	visit = func(n *aetext.Node, pl Place) {
		switch n.Head {
		case "node":
			if id, err := ids.ParseNodeID(n.Label); err == nil {
				pl.Node = id
			}
			name := n.Label
			if v, ok := n.Attr("name"); ok {
				name = v.Str
			}
			pl.Path = path.Join(pl.Path, name)
			pl.Owner = ""
		case "component", "script":
			pl.Owner = n.Label
		case "override":
			if v, ok := n.Attr("prop"); ok {
				pl.Owner = v.Str
			}
		}
		for _, f := range n.Attrs {
			value(f.Value, f.Name, pl)
		}
		for _, f := range n.Fields {
			value(f.Value, f.Name, pl)
		}
		for _, b := range n.Blocks {
			visit(b, pl)
		}
	}
	visit(doc, Place{})
	return out
}

func Known(src Source, id ids.AssetID) bool {
	if id.IsZero() {
		return true
	}
	if _, ok := asset.LookupBuiltin(id); ok {
		return true
	}
	if _, ok := src.PathOf(id); ok {
		return true
	}
	if _, buried := src.Grave(id); buried {
		return false
	}
	_, ok := src.Entry(id)
	return ok
}

type Missing struct {
	ID      ids.AssetID
	Grave   pipeline.Grave
	Buried  bool
	Refs    []Ref
	Suggest ids.AssetID
	Path    string
}

func (m Missing) Label() string {
	if m.Buried && m.Grave.Path != "" {
		return m.Grave.Path
	}
	return m.ID.String()
}

func MissingAssets(p *project.Project, src Source) []Missing {
	by := map[ids.AssetID]*Missing{}
	var order []ids.AssetID
	add := func(id ids.AssetID, r Ref) {
		m := by[id]
		if m == nil {
			m = &Missing{ID: id}
			by[id] = m
			order = append(order, id)
		}
		m.Refs = append(m.Refs, r)
	}
	if p != nil && !Known(src, p.MainScene) {
		add(p.MainScene, Ref{Project: true, Path: project.FileName})
	}
	assets := src.Assets()
	for _, a := range assets {
		e, ok := src.Entry(a.ID)
		if !ok {
			continue
		}
		for _, d := range e.Deps {
			if !Known(src, d) {
				add(d, Ref{Asset: a.ID, Path: a.Path, Kind: a.Kind})
			}
		}
	}
	out := make([]Missing, 0, len(order))
	for _, id := range order {
		m := by[id]
		if g, ok := src.Grave(id); ok {
			m.Grave, m.Buried = g, true
			base := strings.ToLower(path.Base(g.Path))
			for _, a := range assets {
				if a.Kind == g.Kind && strings.ToLower(path.Base(a.Path)) == base {
					m.Suggest, m.Path = a.ID, a.Path
					break
				}
			}
		}
		out = append(out, *m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Label() < out[j].Label() })
	return out
}

type MissingNode struct {
	Scene  string
	Node   ids.NodeID
	Path   string
	Owner  string
	Field  string
	Target ids.NodeID
}

func FileMissingNodes(src Source, rel string) []MissingNode {
	data, err := os.ReadFile(src.Abs(rel))
	if err != nil {
		return nil
	}
	doc, err := aetext.Parse(data, rel)
	if err != nil {
		return nil
	}
	return MissingNodesIn(doc, rel)
}

func MissingNodes(src Source) []MissingNode {
	var out []MissingNode
	for _, a := range src.Assets() {
		if a.Kind == asset.ArtScene || a.Kind == asset.ArtPrefab {
			out = append(out, FileMissingNodes(src, a.Path)...)
		}
	}
	return out
}

func MissingNodesIn(doc *aetext.Node, scene string) []MissingNode {
	defined := map[string]bool{}
	var collect func(n *aetext.Node)
	collect = func(n *aetext.Node) {
		if n.Head == "node" {
			defined[n.Label] = true
		}
		for _, b := range n.Blocks {
			collect(b)
		}
	}
	collect(doc)
	var refs []string
	var scan func(v aetext.Value)
	scan = func(v aetext.Value) {
		switch v.Kind {
		case vNode:
			refs = append(refs, v.Str)
		case aetext.VList:
			for _, it := range v.List {
				scan(it)
			}
		case aetext.VStruct:
			for _, f := range v.Obj {
				scan(f.Value)
			}
		}
	}
	var walk func(n *aetext.Node)
	walk = func(n *aetext.Node) {
		for _, f := range n.Fields {
			scan(f.Value)
		}
		for _, f := range n.Attrs {
			scan(f.Value)
		}
		for _, b := range n.Blocks {
			walk(b)
		}
	}
	walk(doc)
	var out []MissingNode
	seen := map[string]bool{}
	for _, id := range refs {
		if id == "" || defined[id] || seen[id] {
			continue
		}
		nid, err := ids.ParseNodeID(id)
		if err != nil || nid.IsZero() {
			continue
		}
		seen[id] = true
		for _, pl := range textPlaces(doc, vNode, id) {
			out = append(out, MissingNode{Scene: scene, Node: pl.Node, Path: pl.Path, Owner: pl.Owner, Field: pl.Field, Target: nid})
		}
	}
	return out
}
