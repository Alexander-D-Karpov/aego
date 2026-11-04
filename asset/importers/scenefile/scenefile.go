package scenefile

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
	kind  asset.ArtifactKind
	id    string
	ext   string
	heads []string
}

func NewScene() *Importer {
	return &Importer{kind: asset.ArtScene, id: "scene", ext: ".aescene", heads: []string{"scene"}}
}

func NewPrefab() *Importer {
	return &Importer{kind: asset.ArtPrefab, id: "prefab", ext: ".aeprefab", heads: []string{"prefab", "scene"}}
}

func (i *Importer) ID() string {
	return i.id
}

func (i *Importer) Version() int {
	return 3
}

func (i *Importer) Extensions() []string {
	return []string{i.ext}
}

func (i *Importer) Defaults() pipeline.Settings {
	return pipeline.Settings{}
}

func (i *Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	doc, err := aetext.Parse(ctx.Source, ctx.Path)
	if err != nil {
		return nil, errs.Wrap(err, errs.AssetImportFailed, "cannot parse the document")
	}
	if !slices.Contains(i.heads, doc.Head) {
		return nil, errs.New(errs.AssetImportFailed, "unexpected document root",
			log.F("path", ctx.Path), log.F("head", doc.Head), log.F("want", strings.Join(i.heads, " or ")))
	}
	deps := map[ids.AssetID]bool{}
	collectDeps(doc, deps)
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

func collectDeps(n *aetext.Node, out map[ids.AssetID]bool) {
	for _, f := range n.Attrs {
		collectValue(f.Value, out)
	}
	for _, f := range n.Fields {
		collectValue(f.Value, out)
	}
	for _, b := range n.Blocks {
		collectDeps(b, out)
	}
}

func collectValue(v aetext.Value, out map[ids.AssetID]bool) {
	switch v.Kind {
	case aetext.VAsset:
		if id, err := ids.ParseAssetID(v.Str); err == nil && !id.IsZero() {
			out[id] = true
		}
	case aetext.VList:
		for _, item := range v.List {
			collectValue(item, out)
		}
	case aetext.VStruct:
		for _, f := range v.Obj {
			collectValue(f.Value, out)
		}
	}
}
