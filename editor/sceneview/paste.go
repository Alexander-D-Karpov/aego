package sceneview

import (
	"path"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/assets"
	"aego/editor/context"
	"aego/editor/templates"
	"aego/platform/clip"
	"aego/scene"
)

const (
	CommandPasteImage = "scene.paste_image"
	PastedDir         = "assets/pasted"
)

func (p *Panel) CanPasteImage(ctx *context.Editor) bool {
	return p.ctx != nil && p.cur != nil && p.Templates != nil && ctx.Doc() != nil && ctx.Pipeline != nil && clip.HasImage(ctx.Clipboard)
}

func (p *Panel) PasteImage(ctx *context.Editor) error {
	img, ok := clip.Image(ctx.Clipboard)
	if !ok {
		return nil
	}
	if err := assets.EnsureDir(ctx, PastedDir); err != nil {
		return err
	}
	rel := assets.NextNumbered(ctx, PastedDir, "pasted", "-", ".png")
	id, err := assets.CreatePNG(ctx, rel, img)
	if err != nil {
		return err
	}
	e, ok := ctx.Pipeline.Entry(id)
	if !ok {
		return nil
	}
	ap := context.AssetPayload{ID: id, Kind: e.Kind, Name: e.Name}
	if p.viewMode == Mode3D {
		return p.paste3(ctx, ap, rel)
	}
	sp := scene.Space2D
	if !p.Templates.Accepts(ap.Kind, &sp) {
		ctx.Notify(context.LevelWarn, "Saved "+rel+", no template turns it into a node")
		return nil
	}
	p.spawn(ctx, ap, p.pasteAt(ctx))
	return nil
}

func (p *Panel) pasteAt(ctx *context.Editor) mathx.Vec2 {
	at := ctx.Doc().View.Camera.Position
	if m := ctx.UI.Mouse(); p.rect.Contains(m) {
		at = p.worldAt(m)
	}
	if ctx.Settings.Snap {
		at = snapVec(at, ctx.Settings.SnapMove)
	}
	return at
}

func (p *Panel) paste3(ctx *context.Editor, ap context.AssetPayload, rel string) error {
	sp := scene.Space3D
	t, ok := p.Templates.ForAsset(ap.Kind, &sp, ap.ID, path.Base(ap.Name))
	if !ok {
		ctx.Notify(context.LevelWarn, "Saved "+rel+", no 3D template turns it into a node")
		return nil
	}
	px := mathx.Vec2{X: p.rect.W * p.scale / 2, Y: p.rect.H * p.scale / 2}
	if m := ctx.UI.Mouse(); p.rect.Contains(m) {
		px = p.toPixels(m)
	}
	at := p.c3.DropPoint(p.host3(), px, p.snapStep(ctx))
	_, err := templates.InstantiateAt3(ctx, t, ids.NodeID{}, at)
	return err
}
