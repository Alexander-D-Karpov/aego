package inspector

import (
	"aego/editor/core/assetops"
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/document"
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

func (p *Panel) copyTexture(d *document.SceneDocument, td *schema.TypeDescriptor, fd *schema.FieldDescriptor) bool {
	if !textureField(fd) || p.d.Pipeline == nil {
		return false
	}
	v, mixed, err := d.Props.Common(p.nodes, td.ID, fd.ID)
	if err != nil || mixed || v.Asset.IsZero() {
		return false
	}
	if _, builtin := asset.LookupBuiltin(v.Asset); builtin {
		return false
	}
	if !p.d.Assets.CopyAssets([]ids.AssetID{v.Asset}) {
		return false
	}
	p.d.Notices.Notify(context.LevelInfo, "Copied "+assetState(p.d.Pipeline, v.Asset).label)
	return true
}

func (p *Panel) refFromFiles(fd *schema.FieldDescriptor) (ids.AssetID, bool) {
	files, ok := clip.Files(p.d.Clip.Clip)
	if !ok {
		return ids.AssetID{}, false
	}
	for _, f := range files {
		rel, ok := assetops.RelIn(p.d.Pipeline, f)
		if !ok {
			continue
		}
		id, ok := p.d.Pipeline.Lookup(rel)
		if !ok {
			continue
		}
		if e, ok := p.d.Pipeline.Entry(id); ok && context.AssetMatches(fd.AssetKind, e.Kind) {
			return id, true
		}
	}
	return ids.AssetID{}, false
}

func (p *Panel) pasteImage(d *document.SceneDocument, td *schema.TypeDescriptor, fd *schema.FieldDescriptor) error {
	if p.d.Pipeline == nil || len(p.nodes) == 0 {
		return nil
	}
	img, ok := clip.Image(p.d.Clip.Clip)
	if !ok {
		p.d.Notices.Notify(context.LevelInfo, "The clipboard has no image")
		return nil
	}
	s := d.Scene()
	stem := fileSafe(s.Name(s.Handle(p.nodes[0]))) + "-" + fileSafe(fd.Name)
	id, rel, err := p.d.Assets.ImportImage(pasteDir, stem, img)
	if err != nil {
		return err
	}
	if err := d.Props.Set(nil, p.nodes, td.ID, fd.ID, schema.Asset(id)); err != nil {
		return err
	}
	p.d.Notices.Notify(context.LevelInfo, "Pasted "+rel)
	return nil
}

func (p *Panel) pasteAsset(d *document.SceneDocument, td *schema.TypeDescriptor, fd *schema.FieldDescriptor) (bool, error) {
	if fd.Kind != schema.KindAssetRef || fd.Flags.Has(schema.FlagReadOnly) || p.d.Clip.Clip == nil || p.d.Pipeline == nil {
		return false, nil
	}
	if id, ok := p.refFromFiles(fd); ok {
		return true, d.Props.Set(nil, p.nodes, td.ID, fd.ID, schema.Asset(id))
	}
	if textureField(fd) && clip.HasImage(p.d.Clip.Clip) {
		return true, p.pasteImage(d, td, fd)
	}
	return false, nil
}
