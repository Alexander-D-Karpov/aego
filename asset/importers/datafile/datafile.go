package datafile

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

type Importer struct {
	kind asset.ArtifactKind
	id   string
	ext  string
	head string
}

func NewData() *Importer {
	return &Importer{kind: asset.ArtData, id: "data", ext: ".aedata", head: "data"}
}

func NewTable() *Importer {
	return &Importer{kind: asset.ArtTable, id: "table", ext: ".aetable", head: "table"}
}

func (i *Importer) ID() string                  { return i.id }
func (i *Importer) Version() int                { return 1 }
func (i *Importer) Extensions() []string        { return []string{i.ext} }
func (i *Importer) Defaults() pipeline.Settings { return pipeline.Settings{} }

func (i *Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	doc, err := aetext.Parse(ctx.Source, ctx.Path)
	if err != nil {
		return nil, errs.Wrap(err, errs.AssetImportFailed, "cannot parse the document")
	}
	if doc.Head != i.head {
		return nil, errs.New(errs.AssetImportFailed, "unexpected document root", log.F("path", ctx.Path), log.F("head", doc.Head), log.F("want", i.head))
	}
	if v, ok := doc.Attr("type"); !ok || v.Str == "" {
		return nil, errs.New(errs.AssetImportFailed, "the document needs a type attribute", log.F("path", ctx.Path))
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
		Kind:      i.kind,
		Deps:      list,
		Artifacts: []pipeline.Produced{{Kind: i.kind, Data: aetext.Write(doc)}},
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
