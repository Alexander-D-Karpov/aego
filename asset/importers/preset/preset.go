package preset

import (
	"slices"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
)

const (
	Ext        = ".aepreset"
	scriptHead = "engine.Script"
)

type Importer struct{}

func New() *Importer {
	return &Importer{}
}

func (*Importer) ID() string                  { return "preset" }
func (*Importer) Version() int                { return 1 }
func (*Importer) Extensions() []string        { return []string{Ext} }
func (*Importer) Defaults() pipeline.Settings { return pipeline.Settings{} }

func Type(doc *aetext.Node) (string, bool, bool) {
	for _, n := range doc.Blocks {
		if n.Head != "node" {
			continue
		}
		for _, c := range n.Blocks {
			if c.Head != "component" {
				continue
			}
			if c.Label != scriptHead {
				return c.Label, false, true
			}
			for _, s := range c.Blocks {
				if s.Head == "script" {
					return s.Label, true, true
				}
			}
		}
		return "", false, false
	}
	return "", false, false
}

func (*Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	doc, err := aetext.Parse(ctx.Source, ctx.Path)
	if err != nil {
		return nil, errs.Wrap(err, errs.AssetImportFailed, "cannot parse the preset")
	}
	if doc.Head != "prefab" {
		return nil, errs.New(errs.AssetImportFailed, "unexpected preset root", log.F("path", ctx.Path), log.F("head", doc.Head))
	}
	if _, _, ok := Type(doc); !ok {
		return nil, errs.New(errs.AssetImportFailed, "the preset holds no component", log.F("path", ctx.Path))
	}
	deps := map[ids.AssetID]bool{}
	collect(doc, deps)
	delete(deps, ctx.ID)
	list := make([]ids.AssetID, 0, len(deps))
	for id := range deps {
		list = append(list, id)
	}
	slices.SortFunc(list, func(a, b ids.AssetID) int { return strings.Compare(a.String(), b.String()) })
	return &pipeline.Result{
		Kind:      asset.ArtPreset,
		Deps:      list,
		Artifacts: []pipeline.Produced{{Kind: asset.ArtPreset, Data: aetext.Write(doc)}},
	}, nil
}

func collect(n *aetext.Node, out map[ids.AssetID]bool) {
	for _, f := range n.Attrs {
		value(f.Value, out)
	}
	for _, f := range n.Fields {
		value(f.Value, out)
	}
	for _, b := range n.Blocks {
		collect(b, out)
	}
}

func value(v aetext.Value, out map[ids.AssetID]bool) {
	switch v.Kind {
	case aetext.VAsset:
		if id, err := ids.ParseAssetID(v.Str); err == nil && !id.IsZero() {
			out[id] = true
		}
	case aetext.VList:
		for _, item := range v.List {
			value(item, out)
		}
	case aetext.VStruct:
		for _, f := range v.Obj {
			value(f.Value, out)
		}
	}
}
