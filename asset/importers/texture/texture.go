package texture

import (
	"bytes"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/log"
	"aego/format/aetext"
	"aego/gpu"
	rtex "aego/render/texture"
)

const Version = 1

type Importer struct{}

func New() *Importer {
	return &Importer{}
}

func (Importer) ID() string {
	return "texture"
}

func (Importer) Version() int {
	return Version
}

func (Importer) Extensions() []string {
	return []string{".png"}
}

func (Importer) Defaults() pipeline.Settings {
	return pipeline.Settings{
		"filter":  aetext.Ident("nearest"),
		"srgb":    aetext.Bool(true),
		"mips":    aetext.Bool(false),
		"atlas":   aetext.String(""),
		"padding": aetext.Int(2),
		"extrude": aetext.Bool(true),
	}
}

func filterOf(s pipeline.Settings) gpu.FilterMode {
	if s.String("filter", "nearest") == "linear" {
		return gpu.FilterLinear
	}
	return gpu.FilterNearest
}

func (i Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	s := ctx.Meta.Settings
	data, err := rtex.DecodePNG(bytes.NewReader(ctx.Source), s.Bool("srgb", true))
	if err != nil {
		return nil, errs.Wrap(err, errs.AssetImportFailed, "cannot decode the png").
			With(log.F("path", ctx.Path))
	}
	data.Premultiply()
	filter := filterOf(s)

	if group := s.String("atlas", ""); group != "" {
		return &pipeline.Result{
			Kind:      asset.ArtSpriteRegion,
			Group:     group,
			Raw:       data,
			RawFilter: int(filter),
		}, nil
	}

	tex := asset.EncodeTexture(&asset.TextureArtifact{
		Width:  data.Width,
		Height: data.Height,
		Format: data.Format,
		Filter: filter,
		Mips:   s.Bool("mips", false),
		Pixels: data.Pixels,
	})
	region := asset.EncodeRegion(&asset.RegionArtifact{
		Page:   ctx.ID,
		X:      0,
		Y:      0,
		W:      data.Width,
		H:      data.Height,
		PageW:  data.Width,
		PageH:  data.Height,
		Filter: filter,
	})
	return &pipeline.Result{
		Kind: asset.ArtTexture,
		Artifacts: []pipeline.Produced{
			{Kind: asset.ArtTexture, Data: tex},
			{Kind: asset.ArtSpriteRegion, Name: ctx.ID.String() + ".aereg", Data: region},
		},
	}, nil
}
