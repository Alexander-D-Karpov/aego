package editor

import (
	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/engine/i18n"
	"aego/gpu"
	"aego/platform/input"
	"aego/render/texture"
	"aego/ui/widgets"
)

// Dock panels follow the focused art document. All edits use that document's
// canonical binding and history; panels never own a second document/session.
type artDocks struct {
	reg                *ext.Registry
	active             *artPanel
	preview            imageDisplay
	viewport           imageViewport
	pixels             *imagePixels
	owner              document.ID
	revision, device   uint64
	frame              art.ID
	isolation          art.ID
	channel            art.Channel
	draft              bool
	mode               int
	surface            imagePreview
	animationClipboard *art.AnimationClipboard
	assetPreview       *artDocks
}

func newArtDocks(r *ext.Registry) (*artDocks, error) {
	d := &artDocks{reg: r, assetPreview: &artDocks{reg: r, mode: 1}}
	for _, part := range []struct {
		id   string
		slot workspace.Slot
	}{
		{"art.tools", workspace.SlotLeft}, {"art.options", workspace.SlotLeft},
		{"art.properties", workspace.SlotRight}, {"art.palette", workspace.SlotRight},
		{"art.color", workspace.SlotRight}, {"art.layers", workspace.SlotRight},
		{"art.channels", workspace.SlotRight}, {"art.selection", workspace.SlotRight},
		{"art.info", workspace.SlotRight}, {"art.timeline", workspace.SlotBottom},
		{"art.preview", workspace.SlotCenter}, {"art.asset_preview", workspace.SlotRight},
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
			if d.surface.mat != nil || d.surface.pool != nil {
				d.surface.close(r.Editor())
			}
			d.pixels = nil
			d.owner = 0
			d.assetPreview.closePreview(r.Editor())
		} else if !r.PanelVisible("art.preview") {
			d.preview.close(r.Editor())
			if d.surface.mat != nil || d.surface.pool != nil {
				d.surface.close(r.Editor())
			}
		}
		if !r.PanelVisible("art.asset_preview") {
			d.assetPreview.closePreview(r.Editor())
		}
	})
	return d, nil
}

func (d *artDocks) closePreview(ed *context.Editor) {
	if d == nil {
		return
	}
	d.preview.close(ed)
	if d.surface.mat != nil || d.surface.pool != nil {
		d.surface.close(ed)
	}
	d.pixels, d.active = nil, nil
	d.owner = 0
	d.viewport.Ready = false
}

type artDockPanel struct {
	d  *artDocks
	id string
}

func (p *artDockPanel) ID() string    { return p.id }
func (p *artDockPanel) Scope() string { return p.id }
func (p *artDockPanel) Icon() string {
	switch p.id {
	case "art.tools":
		return "panel.art_tools"
	case "art.timeline":
		return "panel.art_timeline"
	case "art.preview", "art.asset_preview":
		return "anim.preview"
	case "art.selection":
		return "tool.selection"
	case "art.info", "art.options":
		return "panel.settings"
	case "art.color":
		return "tool.eyedropper"
	default:
		return "panel.art_tools"
	}
}
func (p *artDockPanel) Title() string {
	switch p.id {
	case "art.tools":
		return i18n.Tr("editor.art.tools", "Art Tools")
	case "art.timeline":
		return i18n.Tr("editor.art.panel_animation_timeline", "Animation Timeline")
	case "art.preview":
		return i18n.Tr("editor.art.animation_preview", "Animation Preview")
	case "art.asset_preview":
		return i18n.Tr("editor.art.panel_asset_preview", "Asset Preview in Aego")
	case "art.options":
		return i18n.Tr("editor.art.panel_tool_options", "Tool Options")
	case "art.palette":
		return i18n.Tr("editor.art.panel_palette", "Palette")
	case "art.color":
		return i18n.Tr("editor.art.panel_color", "Color Picker")
	case "art.layers":
		return i18n.Tr("editor.art.panel_layers", "Layers")
	case "art.channels":
		return i18n.Tr("editor.art.panel_channels", "Surface Channels")
	case "art.selection":
		return i18n.Tr("editor.art.panel_selection", "Selection & Transform")
	case "art.info":
		return i18n.Tr("editor.art.panel_info", "Art Properties")
	default:
		return i18n.Tr("editor.art.palette_layers", "Palette / Layers")
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
		widgets.LabelWrapped(ui, i18n.Tr("editor.art.open_hint", "Double-click a PNG or .aeart file in Assets to start drawing."), false)
		return
	}
	editable := a.doc.Capabilities.Has(document.EditSource) && !a.model.PendingRecovery
	switch p.id {
	case "art.tools":
		a.tools(ui, area)
	case "art.properties":
		a.properties(ui, area, editable)
	case "art.palette":
		a.propertyParts(ui, area, editable, p.id, artPartPalette)
	case "art.layers":
		a.propertyParts(ui, area, editable, p.id, artPartLayers)
	case "art.channels":
		a.propertyParts(ui, area, editable, p.id, artPartSurface)
	case "art.selection":
		a.propertyParts(ui, area, editable, p.id, artPartSelection)
	case "art.info":
		a.infoPanel(ui, area, editable)
	case "art.options":
		a.optionsPanel(ui, area, editable)
	case "art.color":
		if f.Focused && !ui.ModalActive() && !ui.AnyPopup() && !ui.WantsKeyboard() && ui.KeyPressed(input.KeyEscape) {
			a.view.cancelOneShotPicker()
		}
		a.colorPanel(ui, area, editable)
	case "art.timeline":
		a.timeline(ui, area, editable)
	case "art.asset_preview":
		if p.d.assetPreview == nil {
			p.d.assetPreview = &artDocks{reg: p.d.reg, mode: 1}
		}
		p.d.assetPreview.active = a
		p.d.assetPreview.drawPreview(f, area)
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
		if d.surface.mat != nil || d.surface.pool != nil {
			d.surface.close(a.ctx)
		}
		imagePreviewReason(f.UI, area, i18n.Tr("editor.art.preview_context_lost", "The graphics context was lost. Preview will resume when it is restored."))
		return
	}
	if d.device != device {
		d.preview.close(a.ctx)
		if d.surface.mat != nil || d.surface.pool != nil {
			d.surface.close(a.ctx)
		}
		d.device = device
	}
	if d.owner != a.doc.ID {
		d.preview.close(a.ctx)
		if d.surface.mat != nil || d.surface.pool != nil {
			d.surface.close(a.ctx)
		}
		d.viewport.Ready = false
		d.pixels = nil
		d.owner = a.doc.ID
	}
	ui := f.UI
	ui.Region(area)
	defer ui.PopCursor()
	cur := ui.Cursor()
	widgets.Combo(ui, ui.ID("preview-mode"), cur.Next(0), &d.mode, []string{i18n.Tr("editor.art.pixel_preview", "Pixel preview"), i18n.Tr("editor.image.surface", "Lit surface")}, false)
	if d.mode == 1 {
		d.drawSurface(f, area, frame)
		return
	}
	if d.surface.mat != nil || d.surface.pool != nil {
		d.surface.close(a.ctx)
	}
	area = mathx.Rect{X: cur.X, Y: cur.Y, W: cur.W, H: max(0, area.Y+area.H-cur.Y-ui.Theme.Padding)}
	if d.pixels == nil || d.revision != info.Document.Revision || d.frame != frame || d.channel != s.Channel || d.isolation != info.Isolation || info.Gesture || d.draft != info.Gesture {
		pixels, err := s.Preview(frame, false, false)
		if err != nil {
			a.report(err)
			return
		}
		d.pixels = &imagePixels{data: texture.Data{Width: pixels.Rect.Dx(), Height: pixels.Rect.Dy(), Format: gpu.TexSRGBA8, Pixels: pixels.Pix}, semantic: asset.TextureColor}
		d.revision, d.frame, d.channel = info.Document.Revision, frame, s.Channel
		d.isolation = info.Isolation
		d.draft = info.Gesture
	}
	if !d.viewport.Ready {
		d.viewport.fit(area, info.Document.Canvas.Width, info.Document.Canvas.Height)
	}
	drawImageCanvas(a.ctx, area, d.pixels, &d.preview, &d.viewport, imageRGBA, a.view.canvasBackground, false, [4]int{}, nil)
}

func (d *artDocks) drawSurface(f *services.Frame, area mathx.Rect, frame art.ID) {
	a, ui := d.active, f.UI
	cur := ui.Cursor()
	if a.model.Draft {
		content := mathx.Rect{X: cur.X, Y: cur.Y, W: cur.W, H: max(0, area.Y+area.H-cur.Y-ui.Theme.Padding)}
		imagePreviewReason(ui, content, i18n.Tr("editor.art.png_draft_preview", "Save the drawing before previewing it as a game asset."))
		return
	}
	d.surface.init()
	c := &d.surface.controls
	row := cur.Next(0)
	for i, toggle := range []struct {
		label string
		value *bool
	}{
		{i18n.Tr("editor.image.lit", "Lit"), &c.Lit}, {i18n.Tr("editor.art.normal", "Normals"), &c.Normal}, {i18n.Tr("editor.art.emission", "Emission"), &c.Emission}, {"Bloom", &c.Bloom},
	} {
		artToggle(ui, "preview-"+toggle.label, artCell(row, i, 4), toggle.label, toggle.value)
	}
	row = cur.Next(0)
	artToggle(ui, "preview-flip-x", artCell(row, 0, 3), i18n.Tr("editor.image.flip_x", "Flip X"), &c.FlipX)
	artToggle(ui, "preview-flip-y", artCell(row, 1, 3), i18n.Tr("editor.image.flip_y", "Flip Y"), &c.FlipY)
	widgets.DragFloat32(ui, ui.ID("preview-emission"), artCell(row, 2, 3), &c.EmissionStrength, widgets.NumberOpts{Speed: .05, Min: 0, HasMin: true, Max: 16, HasMax: true})
	widgets.Tooltip(ui, ui.ID("preview-emission"), i18n.Tr("editor.art.preview_emission_strength", "Preview emission intensity; source materials remain unchanged"))
	row = cur.Next(0)
	for i, field := range []struct {
		id, label string
		value     *float32
		maximum   float64
	}{
		{"light-height", i18n.Tr("editor.image.height", "Light height"), &c.Height, 10}, {"light-intensity", i18n.Tr("editor.image.intensity", "Light intensity"), &c.Intensity, 16}, {"light-ambient", i18n.Tr("editor.image.ambient", "Ambient intensity"), &c.Ambient, 4},
	} {
		widgets.DragFloat32(ui, ui.ID(field.id), artCell(row, i, 3), field.value, widgets.NumberOpts{Speed: .01, Min: 0, HasMin: true, Max: field.maximum, HasMax: true})
		widgets.Tooltip(ui, ui.ID(field.id), field.label)
	}
	if a.doc.WorkingPreview.Pending || a.model.Session.Info().Dirty && !a.ctx.WorkingSourceCurrent(a.doc, a.model.PreviewRevision()) {
		widgets.LabelWrapped(ui, i18n.Tr("editor.art.preparing_preview", "Preparing the latest authored frame…"), false)
	}
	content := mathx.Rect{X: cur.X, Y: cur.Y, W: cur.W, H: max(0, area.Y+area.H-cur.Y-ui.Theme.Padding)}
	if a.doc.WorkingPreview.Error != "" {
		imagePreviewReason(ui, content, a.doc.WorkingPreview.Error)
		return
	}
	surface := imageSurface{}
	info := a.model.Session.Document.Info()
	surface.Size = mathx.Vec2{X: float32(info.Canvas.Width), Y: float32(info.Canvas.Height)}.Scale(2 / float32(max(info.Canvas.Width, info.Canvas.Height)))
	if a.ctx.Loader == nil {
		surface.Reason = i18n.Tr("editor.image.preview_no_loader", "The surface preview needs a runtime asset loader and a graphics device.")
	} else {
		name := artsource.FrameName(frame, art.Albedo)
		id := artsource.StableID(a.doc.Asset, name)
		if a.ctx.Pipeline != nil {
			if prepared, ok := a.ctx.Pipeline.Lookup(a.doc.Path + pipeline.SubSep + name); ok {
				id = prepared
			}
		}
		var ok bool
		surface.Albedo, ok = a.ctx.Loader.Texture(id)
		if !ok {
			surface.Reason = i18n.Tr("editor.art.preparing_preview", "Preparing the latest authored frame…")
		} else {
			_, surface.Normal, _ = a.ctx.Loader.NormalFor(id)
			_, surface.Emission, _ = a.ctx.Loader.EmissionFor(id)
		}
	}
	d.surface.draw(a.ctx, content, surface, nil, [4]int{})
}
