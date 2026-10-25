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

	if group := s.String("atlas", ""); group != "" && len(ctx.Meta.Subs) > 0 {
		return nil, errs.New(errs.AssetImportFailed, "a sliced sheet cannot join an atlas group yet").
			With(log.F("path", ctx.Path), log.F("atlas", group))
	}
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
	res := &pipeline.Result{
		Kind: asset.ArtTexture,
		Artifacts: []pipeline.Produced{
			{Kind: asset.ArtTexture, Data: tex},
			{Kind: asset.ArtSpriteRegion, Name: ctx.ID.String() + ".aereg", Data: region},
		},
	}
	for _, sa := range ctx.Meta.Subs {
		x, y := min(max(sa.Rect[0], 0), data.Width), min(max(sa.Rect[1], 0), data.Height)
		w, h := min(max(sa.Rect[2], 0), data.Width-x), min(max(sa.Rect[3], 0), data.Height-y)
		if w == 0 || h == 0 {
			continue
		}
		cell := asset.EncodeRegion(&asset.RegionArtifact{Page: ctx.ID, X: x, Y: y, W: w, H: h, PageW: data.Width, PageH: data.Height, Filter: filter})
		res.Subs = append(res.Subs, pipeline.SubResult{ID: sa.ID, Name: sa.Name, Kind: asset.ArtSpriteRegion,
			Artifacts: []pipeline.Produced{{Kind: asset.ArtSpriteRegion, Data: cell}}})
	}
	return res, nil
}
