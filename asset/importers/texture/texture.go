package texture

import (
	"bytes"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/gpu"
	rtex "aego/render/texture"
)

const Version = 6

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

func (Importer) Defaults() pipeline.Settings { return asset.TextureDefaults() }

func filterOf(s pipeline.Settings) gpu.FilterMode {
	if s.String("filter", "nearest") == "linear" {
		return gpu.FilterLinear
	}
	return gpu.FilterNearest
}

func (i Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	s := ctx.Meta.Settings
	for _, key := range []string{"rotate", "rotation", "allow_rotation"} {
		if v, ok := s[key]; ok && !((v.Kind == aetext.VBool && !v.Bool) || (v.Kind == aetext.VInt && v.Int == 0)) {
			return nil, errs.New(errs.AssetImportFailed, "atlas rotation is not supported").With(log.F("path", ctx.Path), log.F("setting", key))
		}
	}
	if s.Bool("trim", false) && s.String("atlas", "") == "" {
		return nil, errs.New(errs.AssetImportFailed, "sprite trim requires an atlas group").With(log.F("path", ctx.Path))
	}
	semantic, err := semanticOf(ctx.Path, s)
	if err != nil {
		return nil, err
	}
	role, _ := asset.TextureRoleOf(ctx.Path, s)
	srgb, err := asset.TextureSRGB(role, s)
	if err != nil {
		return nil, err
	}
	data, err := rtex.DecodePNG(bytes.NewReader(ctx.Source), srgb)
	if err != nil {
		return nil, errs.Wrap(err, errs.AssetImportFailed, "cannot decode the png").
			With(log.F("path", ctx.Path))
	}
	data.NonColor = semantic != asset.TextureColor
	if err := prepareSurfaceData(data, semantic, s, ctx.Meta.Subs); err != nil {
		return nil, err
	}
	if semantic == asset.TextureHeight && s.Bool("generate_normal", false) {
		semantic = asset.TextureNormal
	}
	data.Premultiply()
	filter := filterOf(s)
	links, err := surfaceBindings(ctx, data.Width, data.Height)
	if err != nil {
		return nil, err
	}

	if group := s.String("atlas", ""); group != "" && len(ctx.Meta.Subs) > 0 {
		return nil, errs.New(errs.AssetImportFailed, "a sliced sheet cannot join an atlas group yet").
			With(log.F("path", ctx.Path), log.F("atlas", group))
	}
	if group := s.String("atlas", ""); group != "" {
		if s.Bool("mips", false) {
			return nil, errs.New(errs.AssetImportFailed, "atlas mipmaps require per-region mip isolation; use mips=false or a standalone texture")
		}
		groupName := semanticName(semantic) + "/" + group
		if semantic == asset.TextureEmissionRGB && !srgb {
			groupName += "/linear"
		}
		return &pipeline.Result{
			Kind:        asset.ArtSpriteRegion,
			Group:       groupName,
			Raw:         data,
			RawFilter:   int(filter),
			RawSemantic: semantic,
			RawNormal:   links.normal, RawPivot: ctx.Meta.Pivot, RawHasPivot: ctx.Meta.HasPivot,
			RawEmission: links.emission,
			Deps:        links.deps,
		}, nil
	}

	tex := asset.EncodeTexture(&asset.TextureArtifact{
		Width:    data.Width,
		Height:   data.Height,
		Format:   data.Format,
		Filter:   filter,
		Mips:     s.Bool("mips", false),
		Semantic: semantic,
		Pixels:   data.Pixels,
	})
	region := asset.EncodeRegion(&asset.RegionArtifact{
		Page: ctx.ID, Pivot: ctx.Meta.Pivot, HasPivot: ctx.Meta.HasPivot,
		Normal:   links.normal,
		Emission: links.emission,
		X:        0,
		Y:        0,
		W:        data.Width,
		H:        data.Height,
		PageW:    data.Width,
		PageH:    data.Height,
		Filter:   filter,
	})
	res := &pipeline.Result{
		Kind: asset.ArtTexture,
		Deps: links.deps,
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
		normalID := links.normals[sa.ID]
		emissionID := links.emissions[sa.ID]
		cell := asset.EncodeRegion(&asset.RegionArtifact{Page: ctx.ID, Pivot: sa.Pivot, HasPivot: sa.HasPivot, Normal: normalID, Emission: emissionID, X: x, Y: y, W: w, H: h, PageW: data.Width, PageH: data.Height, Filter: filter})
		var cellDeps []ids.AssetID
		if !normalID.IsZero() {
			cellDeps = append(cellDeps, normalID)
		}
		if !emissionID.IsZero() {
			cellDeps = append(cellDeps, emissionID)
		}
		res.Subs = append(res.Subs, pipeline.SubResult{ID: sa.ID, Name: sa.Name, Kind: asset.ArtSpriteRegion,
			Deps:      cellDeps,
			Artifacts: []pipeline.Produced{{Kind: asset.ArtSpriteRegion, Data: cell}}})
	}
	return res, nil
}
