package sceneview

import (
	"path"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/platform/clip"
	"aego/scene"
)

const (
	CommandPasteImage = "scene.paste_image"
	PastedDir         = "assets/pasted"
)

func (p *Panel) CanPasteImage() bool {
	return p.pool != nil && p.cur != nil && p.d.Templates != nil && p.d.Docs.Doc() != nil && p.d.Pipeline != nil && clip.HasImage(p.d.Files.Clipboard())
}

func (p *Panel) PasteImage() error {
	img, ok := clip.Image(p.d.Files.Clipboard())
	if !ok {
		return nil
	}
	if err := p.d.Files.EnsureDir(PastedDir); err != nil {
		return err
	}
	rel := p.d.Files.NextNumbered(PastedDir, "pasted", "-", ".png")
	id, err := p.d.Files.CreatePNG(rel, img)
	if err != nil {
		return err
	}
	e, ok := p.d.Pipeline.Entry(id)
	if !ok {
		return nil
	}
	ap := context.AssetPayload{ID: id, Kind: e.Kind, Name: e.Name}
	if p.viewMode == Mode3D {
		return p.paste3(ap, rel)
	}
	sp := scene.Space2D
	if !p.d.Templates.Accepts(ap.Kind, &sp) {
		p.d.Notices.Notify(context.LevelWarn, i18n.Note("editor.sceneview.saved_rel_no_template_turns", "Saved {rel}, no template turns it into a node", i18n.Str("rel", rel)))
		return nil
	}
	p.spawn(ap, p.pasteAt())
	return nil
}

func (p *Panel) pasteAt() mathx.Vec2 {
	at := p.d.Docs.Doc().View.Camera.Position
	if m := p.ui().Mouse(); p.rect.Contains(m) {
		at = p.worldAt(m)
	}
	if p.d.Tools.Snap {
		at = snapVec(at, p.d.Tools.SnapMove)
	}
	return at
}

func (p *Panel) paste3(ap context.AssetPayload, rel string) error {
	sp := scene.Space3D
	t, ok := p.d.Templates.ForAsset(ap.Kind, &sp, ap.ID, path.Base(ap.Name))
	if !ok {
		p.d.Notices.Notify(context.LevelWarn, i18n.Note("editor.sceneview.saved_rel_no_3d_template", "Saved {rel}, no 3D template turns it into a node", i18n.Str("rel", rel)))
		return nil
	}
	px := mathx.Vec2{X: p.rect.W * p.scale / 2, Y: p.rect.H * p.scale / 2}
	if m := p.ui().Mouse(); p.rect.Contains(m) {
		px = p.toPixels(m)
	}
	return p.d.Scenes.InstantiateAt3(t, p.c3.DropPoint(p.host3(), px, p.snapStep()))
}
