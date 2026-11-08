package editor

import (
	"aego/art"
	"aego/asset"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/engine/i18n"
	"aego/gpu"
	"aego/render/texture"
	"aego/ui/widgets"
)

// Dock panels follow the focused art document. All edits use that document's
// canonical binding and history; panels never own a second document/session.
type artDocks struct {
	reg              *ext.Registry
	active           *artPanel
	preview          imageDisplay
	viewport         imageViewport
	pixels           *imagePixels
	owner            document.ID
	revision, device uint64
	frame            art.ID
	channel          art.Channel
	draft            bool
}

func newArtDocks(r *ext.Registry) (*artDocks, error) {
	d := &artDocks{reg: r}
	for _, part := range []struct {
		id   string
		slot workspace.Slot
	}{
		{"art.properties", workspace.SlotRight}, {"art.timeline", workspace.SlotBottom}, {"art.preview", workspace.SlotCenter},
	} {
		if err := r.Panel(workspace.Desc{Panel: &artDockPanel{d: d, id: part.id}, Slot: part.slot}); err != nil {
			return nil, err
		}
	}
	r.Tick(func() {
		if d.active == nil {
			return
		}
		if r.Editor().Docs.AssetDocument(d.active.doc.Asset) != d.active.doc {
			d.active = nil
			d.preview.close(r.Editor())
			d.pixels = nil
			d.owner = 0
		} else if !r.PanelVisible("art.preview") {
			d.preview.close(r.Editor())
		}
	})
	return d, nil
}

type artDockPanel struct {
	d  *artDocks
	id string
}

func (p *artDockPanel) ID() string    { return p.id }
func (p *artDockPanel) Scope() string { return p.id }
func (p *artDockPanel) Icon() string {
	switch p.id {
	case "art.timeline":
		return "panel.art_timeline"
	case "art.preview":
		return "anim.preview"
	default:
		return "panel.art_tools"
	}
}
func (p *artDockPanel) Title() string {
	switch p.id {
	case "art.timeline":
		return i18n.Tr("editor.art.asset_timeline", "Asset Timeline")
	case "art.preview":
		return i18n.Tr("editor.art.animation_preview", "Animation Preview")
	default:
		return i18n.Tr("editor.art.properties", "Tools / Layers")
	}
}
func (p *artDockPanel) Document() *document.AssetDocument {
	if p.d.active != nil {
		return p.d.active.doc
	}
	return nil
}
func (p *artDockPanel) Draw(f *services.Frame, area mathx.Rect) {
	ui := f.UI
	ui.PushID(p.id)
	defer ui.PopID()
	a := p.d.active
	if a == nil || a.model == nil {
		ui.Region(area)
		defer ui.PopCursor()
		widgets.LabelWrapped(ui, i18n.Tr("editor.art.open_hint", "Open an .aeart document in Assets to start drawing."), false)
		return
	}
	editable := a.doc.Capabilities.Has(document.EditSource) && !a.model.PendingRecovery
	switch p.id {
	case "art.properties":
		a.properties(ui, area, editable)
	case "art.timeline":
		a.timeline(ui, area, editable)
	default:
		p.d.drawPreview(f, area)
	}
}
func (d *artDocks) drawPreview(f *services.Frame, area mathx.Rect) {
	a := d.active
	s := a.model.Session
	info := s.Info()
	frame := s.Frame
	if a.view.playing {
		if v, err := s.Document.FrameAt(a.view.tag, int64((f.Now-a.view.playStart)*1000)); err == nil {
			frame = v
		}
	}
	device, lost := imageDevice(a.ctx)
	if lost {
		d.preview.close(a.ctx)
		return
	}
	if d.device != device {
		d.preview.close(a.ctx)
		d.device = device
	}
	if d.owner != a.doc.ID {
		d.preview.close(a.ctx)
		d.viewport.Ready = false
		d.pixels = nil
		d.owner = a.doc.ID
	}
	if d.pixels == nil || d.revision != info.Document.Revision || d.frame != frame || d.channel != s.Channel || info.Gesture || d.draft != info.Gesture {
		pixels, err := s.Preview(frame, false, false)
		if err != nil {
			a.report(err)
			return
		}
		d.pixels = &imagePixels{data: texture.Data{Width: pixels.Rect.Dx(), Height: pixels.Rect.Dy(), Format: gpu.TexSRGBA8, Pixels: pixels.Pix}, semantic: asset.TextureColor}
		d.revision, d.frame, d.channel = info.Document.Revision, frame, s.Channel
		d.draft = info.Gesture
	}
	if !d.viewport.Ready {
		d.viewport.fit(area, info.Document.Canvas.Width, info.Document.Canvas.Height)
	}
	drawImageCanvas(a.ctx, area, d.pixels, &d.preview, &d.viewport, imageRGBA, 0, false, [4]int{}, nil)
}
