// Package material imports .aemat files: the artifact is the normalized text, checked for syntax only; the resolver applies it to the shader.
package material

import (
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/format/aetext"
)

const Version = 1

type Importer struct{}

func New() *Importer { return &Importer{} }

func (*Importer) ID() string                  { return "material" }
func (*Importer) Version() int                { return Version }
func (*Importer) Extensions() []string        { return []string{".aemat"} }
func (*Importer) Defaults() pipeline.Settings { return pipeline.Settings{} }

func (*Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	d, err := asset.ParseMaterial(ctx.Source, ctx.Path)
	if err != nil {
		return nil, errs.Wrap(err, errs.AssetImportFailed, "cannot read the material")
	}
	n, _ := aetext.Parse(ctx.Source, ctx.Path)
	return &pipeline.Result{
		Kind:      asset.ArtMaterial,
		Deps:      d.Deps(),
		Artifacts: []pipeline.Produced{{Kind: asset.ArtMaterial, Data: aetext.Write(n)}},
	}, nil
}
