package sceneio

import (
	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/scene/prefab"
	"aego/schema"
)

type Library struct {
	reg   *schema.Registry
	res   asset.Resolver
	local map[ids.AssetID]*prefab.Node
	set   map[ids.AssetID]*prefab.Node
}

func NewLibrary(reg *schema.Registry, res asset.Resolver) *Library {
	return &Library{reg: reg, res: res, local: map[ids.AssetID]*prefab.Node{}, set: map[ids.AssetID]*prefab.Node{}}
}

func (b *Library) Set(id ids.AssetID, n *prefab.Node) {
	b.set[id] = n
	clear(b.local)
}

func (b *Library) Unset(id ids.AssetID) {
	delete(b.set, id)
	clear(b.local)
}

func (b *Library) Prefab(id ids.AssetID) (*prefab.Node, bool) {
	if n, ok := b.set[id]; ok {
		return n, n != nil
	}
	if n, ok := b.local[id]; ok {
		return n, n != nil
	}
	v, err := b.decode(id, "prefab.model", func() (any, error) { return b.parse(id) })
	n, _ := v.(*prefab.Node)
	if err != nil {
		n = nil
	}
	b.local[id] = n
	return n, n != nil
}

func (b *Library) Template(id ids.AssetID) (*Template, error) {
	v, err := b.decode(id, "prefab.template", func() (any, error) { return b.compile(id) })
	if err != nil {
		return nil, err
	}
	t, ok := v.(*Template)
	if !ok || t == nil {
		return nil, errs.New(errs.AssetLoadFailed, "prefab template is not available", log.F("prefab", id.String()))
	}
	return t, nil
}

func (b *Library) decode(id ids.AssetID, key string, build func() (any, error)) (any, error) {
	if d, ok := b.res.(asset.Decoder); ok && len(b.set) == 0 {
		return d.Decoded(id, key, build)
	}
	return build()
}

func (b *Library) parse(id ids.AssetID) (any, error) {
	if b.res == nil {
		return nil, errs.New(errs.AssetNotFound, "prefab is not available", log.F("prefab", id.String()))
	}
	data, ok := b.res.Prefab(id)
	if !ok {
		return nil, errs.New(errs.AssetNotFound, "prefab is not imported", log.F("prefab", id.String()))
	}
	return ParsePrefab(data, id.String(), b.reg)
}

func (b *Library) compile(id ids.AssetID) (any, error) {
	p, ok := b.Prefab(id)
	if !ok {
		return nil, errs.New(errs.AssetNotFound, "prefab is not loaded", log.F("prefab", id.String()))
	}
	tree, _, err := prefab.Expand(b, p, nil)
	if err != nil {
		return nil, err
	}
	w := &writer{reg: b.reg, lib: b}
	doc := &aetext.Node{Head: "prefab"}
	doc.Attrs = append(doc.Attrs, aetext.Field{Name: "version", Value: aetext.Int(FormatVersion)})
	doc.Blocks = append(doc.Blocks, w.modelNode(tree, keyID, nil))
	return &Template{Prefab: id, Doc: aetext.Write(doc)}, nil
}

func keyID(n *prefab.Node) ids.NodeID {
	return n.Key.ID()
}

func ParsePrefab(data []byte, name string, reg *schema.Registry) (*prefab.Node, error) {
	rep := &Report{}
	doc, err := parseDocument(data, name, rep)
	if err != nil {
		return nil, err
	}
	l := &loader{reg: reg, report: rep, name: name}
	for _, b := range doc.Blocks {
		if b.Head == "node" {
			return l.readModel(b)
		}
	}
	return nil, errs.New(errs.SceneIOParse, "prefab has no root node", log.F("path", name))
}
