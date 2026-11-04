// Package material imports .aemat source documents into versioned binary AEMT artifacts.
package material

import (
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
)

const Version = 2

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
	data := asset.EncodeMaterial(d)
	if _, err := asset.DecodeMaterial(data, ctx.Path); err != nil {
		return nil, errs.Wrap(err, errs.AssetImportFailed, "cannot prepare binary material")
	}
	return &pipeline.Result{
		Kind:      asset.ArtMaterial,
		Deps:      d.Deps(),
		Artifacts: []pipeline.Produced{{Kind: asset.ArtMaterial, Data: data}},
	}, nil
}
