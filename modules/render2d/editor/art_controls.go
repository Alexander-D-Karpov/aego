package editor

import (
	"fmt"
	"math"

	"aego/art"
	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/ui/core"
	"aego/ui/widgets"
)

func artCell(row mathx.Rect, column, count int) mathx.Rect {
	w := (row.W - float32(count-1)*4) / float32(count)
	return mathx.Rect{X: row.X + float32(column)*(w+4), Y: row.Y, W: w, H: row.H}
}
func artButton(ui *core.Context, id string, r mathx.Rect, text string) bool {
	label := widgets.Elide(ui, text, max(0, r.W-8))
	clicked := widgets.ButtonRect(ui, ui.ID(id), r, label)
	if label != text {
		widgets.Tooltip(ui, ui.ID(id), text)
	}
	return clicked
}

func artToggle(ui *core.Context, id string, r mathx.Rect, text string, value *bool) bool {
	label := widgets.Elide(ui, text, max(0, r.W-8))
	clicked := widgets.ToggleButton(ui, ui.ID(id), r, label, value)
	if label != text {
		widgets.Tooltip(ui, ui.ID(id), text)
	}
	return clicked
}

// ColorField stores linear UI colors but displays and edits 8-bit sRGB
// numbers. Recover those exact displayed bytes for the Art document, including
// for data channels whose byte values must not be gamma-converted by Art Core.
func artColorBytes(c mathx.Color) art.Color {
	colorByte := func(v float32) uint8 {
		return uint8(math.Round(float64(mathx.Clamp(mathx.LinearToSRGB(mathx.Clamp(v, 0, 1)), 0, 1)) * 255))
	}
	return art.Color{R: colorByte(c.R), G: colorByte(c.G), B: colorByte(c.B), A: uint8(math.Round(float64(mathx.Clamp(c.A, 0, 1)) * 255))}
}

func artIconAction(ui *core.Context, name string, r mathx.Rect, icon, tip string, enabled bool) bool {
	id := ui.ID(name)
	clicked := false
	if enabled {
		clicked = widgets.IconButtonRect(ui, id, r, icon)
	} else {
		ui.MouseOver(id, r)
		ui.Box(r, ui.Theme.Radius.Button, ui.Theme.Widget, ui.Theme.Border)
		widgets.IconIn(ui, r, icon, ui.Theme.TextDisabled)
		tip += " · " + i18n.Tr("editor.art.read_only", "Read-only")
	}
	widgets.Tooltip(ui, id, tip)
	return clicked
}

type artPropertyParts uint8

const (
	artPartSurface artPropertyParts = 1 << iota
	artPartPalette
	artPartLayers
	artPartSelection
	artPartDocument
	artPartLegacyBrush
	artPartsAll = artPartSurface | artPartPalette | artPartLayers | artPartSelection | artPartDocument | artPartLegacyBrush
)

type artPartScroll struct {
	offset, content float32
	bar             widgets.ScrollState
}

func (p *artPanel) properties(ui *core.Context, r mathx.Rect, editable bool) {
	p.propertyParts(ui, r, editable, "", artPartsAll)
}

func (p *artPanel) propertyParts(ui *core.Context, r mathx.Rect, editable bool, panel string, parts artPropertyParts) {
	ui.Region(r)
	defer ui.PopCursor()
	ui.PushClip(r)
	defer ui.PopClip()
	cur, v, s := ui.Cursor(), p.view, p.model.Session
	info := s.Info()
	scroll, content, bar := &v.propertyScroll, &v.propertyContent, &v.propertyBar
	if panel != "" {
		if v.partScroll == nil {
			v.partScroll = make(map[string]*artPartScroll)
		}
		if v.partScroll[panel] == nil {
			v.partScroll[panel] = &artPartScroll{}
		}
		part := v.partScroll[panel]
		scroll, content, bar = &part.offset, &part.content, &part.bar
	}
	if ui.MouseOver(ui.ID("property-scroll"), r) {
		*scroll -= ui.ScrollY(ui.Theme.RowHeight * 3)
	}
	*scroll = mathx.Clamp(*scroll, 0, max(0, *content-r.H))
	if *content > r.H {
		cur.W = max(0, cur.W-ui.Theme.ScrollbarW)
	}
	cur.Y -= *scroll
	top := cur.Y
	defer func() {
		*content = cur.Y - top + ui.Theme.Padding*2
		track := mathx.Rect{X: r.X + r.W - ui.Theme.ScrollbarW, Y: r.Y, W: ui.Theme.ScrollbarW, H: r.H}
		widgets.ScrollBar(ui, ui.ID("property-scrollbar"), track, scroll, *content, r.H, bar)
	}()
	var row mathx.Rect
	separateTools := p.reg != nil && p.reg.PanelVisible("art.tools")
	if parts&artPartLegacyBrush != 0 && !separateTools {
		p.brushSettings(ui, r.W)
	}

	if parts&artPartSurface != 0 {
		channels := []art.Channel{art.Albedo, art.Normal, art.Height, art.Emission}
		for i, channel := range channels {
			if s.Channel == channel {
				v.surface = i
				break
			}
		}
		row = cur.Next(0)
		if widgets.Combo(ui, ui.ID("surface"), row, &v.surface, []string{i18n.Tr("editor.art.albedo", "Albedo"), i18n.Tr("editor.art.normal", "Normals"), i18n.Tr("editor.art.height", "Height"), i18n.Tr("editor.art.emission", "Emission")}, false) {
			p.report(s.Select("", "", channels[v.surface]))
			v.refresh = true
		}
	}
	if parts&artPartPalette != 0 {
		if panel == "art.palette" {
			p.paletteGrid(ui, info.Document.Palette)
		} else {
			row = cur.Next(0)
			if len(info.Document.Palette) > 0 {
				count := 8
				if r.W < 200 {
					count = 4
				}
				cells := count + 2
				pages := (len(info.Document.Palette) + count - 1) / count
				v.palettePage = min(v.palettePage, max(0, pages-1))
				if artButton(ui, "palette-prev", artCell(row, 0, cells), "‹") {
					v.palettePage = max(0, v.palettePage-1)
				}
				for j := 0; j < count; j++ {
					index := v.palettePage*count + j
					if index >= len(info.Document.Palette) {
						break
					}
					sw := info.Document.Palette[index].Color
					cell := artCell(row, j+1, cells)
					widgets.Swatch(ui, cell, mathx.ColorRGBA8(sw.R, sw.G, sw.B, sw.A), false)
					id := ui.ID("swatch-" + fmt.Sprint(index))
					if ui.Clicked(id, cell) {
						v.palette = index
						v.color = mathx.ColorRGBA8(sw.R, sw.G, sw.B, sw.A)
						v.rememberColor(sw)
					}
					if ui.RightClicked(id, cell) {
						v.backgroundPalette = index
						v.background = mathx.ColorRGBA8(sw.R, sw.G, sw.B, sw.A)
					}
					widgets.Tooltip(ui, id, i18n.Tr("editor.art.palette_pointer_hint", "Left click: foreground. Right click: background. X swaps them."))
					if v.palette == index {
						ui.Outline(cell, ui.Theme.Accent)
					}
				}
				if artButton(ui, "palette-next", artCell(row, cells-1, cells), "›") {
					v.palettePage = min(pages-1, v.palettePage+1)
				}
			}
		}
	}
	if parts&artPartLayers != 0 {
		row = cur.Next(0)
		addLayer, prevLayer, nextLayer := artCell(row, 0, 3), artCell(row, 1, 3), artCell(row, 2, 3)
		if r.W < 200 {
			addLayer = row
			row = cur.Next(0)
			prevLayer, nextLayer = artCell(row, 0, 2), artCell(row, 1, 2)
		}
		if artButton(ui, "add-layer", addLayer, i18n.Tr("editor.art.add_layer", "+ Layer")) && editable {
			p.execute(art.Operation{Kind: "add-layer", Name: i18n.Tr("editor.art.layer", "Layer"), LayerKind: art.Raster})
		}
		if artButton(ui, "layer-prev", prevLayer, "‹") {
			v.layerPage = max(0, v.layerPage-1)
		}
		if artButton(ui, "layer-next", nextLayer, "›") {
			v.layerPage++
		}
		// Short property panels scroll; pagination bounds widget work for documents
		// with hundreds of layers without hiding the controls below a small dock.
		rows := max(1, min(8, len(info.Document.Layers)))
		pages := (len(info.Document.Layers) + rows - 1) / rows
		v.layerPage = min(v.layerPage, max(0, pages-1))
		for i := v.layerPage * rows; i < min(len(info.Document.Layers), (v.layerPage+1)*rows); i++ {
			layer := info.Document.Layers[i]
			row = cur.Next(0)
			ui.PushID(string(layer.ID))
			thumb := mathx.Rect{X: row.X, Y: row.Y, W: row.H, H: row.H}
			p.thumbnail(ui, thumb, layer.ID, s.Frame)
			label := mathx.Rect{X: row.X + row.H + 3, Y: row.Y, W: max(0, row.W-row.H-63), H: row.H}
			controls := row
			if r.W < 200 {
				label.W = max(0, row.W-row.H-3)
				controls = cur.Next(0)
			}
			if artButton(ui, "select", label, layer.Name) {
				p.report(s.Select(layer.ID, "", ""))
				v.refresh = true
			}
			widgets.Tooltip(ui, ui.ID("select"), layer.Name)
			if layer.ID == s.Layer {
				ui.Outline(row, ui.Theme.Accent)
			}
			visibleIcon, visibleTip := "ui.hidden", i18n.Tr("editor.art.show_layer", "Show layer")
			if layer.Visible {
				visibleIcon, visibleTip = "anim.preview", i18n.Tr("editor.art.hide_layer", "Hide layer")
			}
			if artIconAction(ui, "visible", mathx.Rect{X: controls.X + controls.W - 56, Y: controls.Y, W: 26, H: controls.H}, visibleIcon, visibleTip, editable) {
				layer.Visible = !layer.Visible
				p.execute(art.Operation{Kind: "set-layer", LayerValue: &layer})
			}
			lockIcon, lockTip := "ui.unlock", i18n.Tr("editor.art.lock_layer", "Lock layer")
			if layer.Locked {
				lockIcon, lockTip = "ui.lock", i18n.Tr("editor.art.unlock_layer", "Unlock layer")
			}
			if artIconAction(ui, "locked", mathx.Rect{X: controls.X + controls.W - 26, Y: controls.Y, W: 26, H: controls.H}, lockIcon, lockTip, editable) {
				layer.Locked = !layer.Locked
				p.execute(art.Operation{Kind: "set-layer", LayerValue: &layer})
			}
			ui.PopID()
		}
	}
	var actions artActionGroups
	if parts&artPartLayers != 0 {
		actions |= artActionsLayer
	}
	if parts&artPartSelection != 0 {
		actions |= artActionsPixels
	}
	if parts&artPartPalette != 0 {
		actions |= artActionsPalette
	}
	if parts&artPartDocument != 0 {
		actions |= artActionsDocument
	}
	p.authoringActionsFor(ui, editable, actions)
	if parts&artPartLegacyBrush != 0 && !separateTools {
		p.brushPresentation(ui)
	}
}
