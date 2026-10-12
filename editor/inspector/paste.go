package inspector

import (
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/editor/assets"
	"aego/editor/context"
	"aego/editor/document"
	"aego/platform/clip"
	"aego/schema"
)

const pasteDir = "assets"

func textureField(fd *schema.FieldDescriptor) bool {
	return fd.Kind == schema.KindAssetRef && context.AssetMatches(fd.AssetKind, asset.ArtTexture)
}

func fileSafe(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(s))
	s = strings.TrimLeft(s, ".")
	if s == "" {
		return "node"
	}
	return s
}

func (p *Panel) copyTexture(ctx *context.Editor, d *document.SceneDocument, td *schema.TypeDescriptor, fd *schema.FieldDescriptor) bool {
	if !textureField(fd) || ctx.Pipeline == nil {
		return false
	}
	v, mixed, err := d.Props.Common(p.nodes, td.ID, fd.ID)
	if err != nil || mixed || v.Asset.IsZero() {
		return false
	}
	if _, builtin := asset.LookupBuiltin(v.Asset); builtin {
		return false
	}
	if len(assets.CopyAssets(ctx, []ids.AssetID{v.Asset})) == 0 {
		return false
	}
	ctx.Notify(context.LevelInfo, "Copied "+assetName(ctx, v.Asset))
	return true
}

func (p *Panel) refFromFiles(ctx *context.Editor, fd *schema.FieldDescriptor) (ids.AssetID, bool) {
	files, ok := clip.Files(ctx.Clipboard)
	if !ok {
		return ids.AssetID{}, false
	}
	for _, f := range files {
		rel, ok := assets.ProjectRel(ctx, f)
		if !ok {
			continue
		}
		id, ok := ctx.Pipeline.Lookup(rel)
		if !ok {
			continue
		}
		if e, ok := ctx.Pipeline.Entry(id); ok && context.AssetMatches(fd.AssetKind, e.Kind) {
			return id, true
		}
	}
	return ids.AssetID{}, false
}

func (p *Panel) pasteImage(ctx *context.Editor, d *document.SceneDocument, td *schema.TypeDescriptor, fd *schema.FieldDescriptor) error {
	if ctx.Pipeline == nil || len(p.nodes) == 0 {
		return nil
	}
	img, ok := clip.Image(ctx.Clipboard)
	if !ok {
		ctx.Notify(context.LevelInfo, "The clipboard has no image")
		return nil
	}
	if err := assets.EnsureDir(ctx, pasteDir); err != nil {
		return err
	}
	s := d.Scene()
	stem := fileSafe(s.Name(s.Handle(p.nodes[0]))) + "-" + fileSafe(fd.Name)
	rel := assets.UniqueFile(ctx, pasteDir, stem, ".png")
	id, err := assets.CreatePNG(ctx, rel, img)
	if err != nil {
		return err
	}
	if err := d.Props.Set(nil, p.nodes, td.ID, fd.ID, schema.Asset(id)); err != nil {
		return err
	}
	ctx.Notify(context.LevelInfo, "Pasted "+rel)
	return nil
}

func (p *Panel) pasteAsset(ctx *context.Editor, d *document.SceneDocument, td *schema.TypeDescriptor, fd *schema.FieldDescriptor) (bool, error) {
	if fd.Kind != schema.KindAssetRef || fd.Flags.Has(schema.FlagReadOnly) || ctx.Clipboard == nil || ctx.Pipeline == nil {
		return false, nil
	}
	if id, ok := p.refFromFiles(ctx, fd); ok {
		return true, d.Props.Set(nil, p.nodes, td.ID, fd.ID, schema.Asset(id))
	}
	if textureField(fd) && clip.HasImage(ctx.Clipboard) {
		return true, p.pasteImage(ctx, d, td, fd)
	}
	return false, nil
}
