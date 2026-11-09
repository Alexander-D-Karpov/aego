package editor

import (
	stdctx "context"
	"fmt"
	"slices"

	"aego/art"
	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/ui/core"
	"aego/ui/widgets"
)

// Each pane owns only scroll presentation. Tool values and all mutations stay
// on the active panel's canonical session and history.
func (p *artPanel) partRegion(ui *core.Context, r mathx.Rect, id string, draw func()) {
	ui.Region(r)
	defer ui.PopCursor()
	ui.PushClip(r)
	defer ui.PopClip()
	v := p.view
	if v.partScroll == nil {
		v.partScroll = make(map[string]*artPartScroll)
	}
	if v.partScroll[id] == nil {
		v.partScroll[id] = &artPartScroll{}
	}
	s := v.partScroll[id]
	cur := ui.Cursor()
	if ui.MouseOver(ui.ID("scroll"), r) {
		s.offset -= ui.ScrollY(ui.Theme.RowHeight * 3)
	}
	s.offset = mathx.Clamp(s.offset, 0, max(0, s.content-r.H))
	if s.content > r.H {
		cur.W = max(0, cur.W-ui.Theme.ScrollbarW)
	}
	cur.Y -= s.offset
	top := cur.Y
	draw()
	s.content = cur.Y - top + ui.Theme.Padding*2
	track := mathx.Rect{X: r.X + r.W - ui.Theme.ScrollbarW, Y: r.Y, W: ui.Theme.ScrollbarW, H: r.H}
	widgets.ScrollBar(ui, ui.ID("scrollbar"), track, &s.offset, s.content, r.H, &s.bar)
}

func (p *artPanel) colorPanel(ui *core.Context, r mathx.Rect, editable bool) {
	p.partRegion(ui, r, "art.color", func() {
		cur, v := ui.Cursor(), p.view
		if edit := widgets.ColorField(ui, ui.ID("foreground"), widgets.Field(ui, i18n.Tr("editor.art.foreground_color", "Foreground")), &v.color, false); edit.Changed || edit.End {
			p.setForegroundColor(v.color, edit.End)
		}
		if edit := widgets.ColorField(ui, ui.ID("background"), widgets.Field(ui, i18n.Tr("editor.art.background_color_short", "Background")), &v.background, false); edit.Changed {
			info := p.model.Session.Document.Info()
			if info.Canvas.Mode == art.Indexed && p.model.Session.Channel == art.Albedo {
				if index := artPaletteColorIndex(info.Palette, artColorBytes(v.background), v.backgroundPalette); index >= 0 {
					v.backgroundPalette = index
				}
			}
		}
		if artButton(ui, "swap-colors", cur.Next(0), i18n.Tr("editor.art.swap_colors", "Swap foreground / background")) {
			v.color, v.background = v.background, v.color
			v.palette, v.backgroundPalette = v.backgroundPalette, v.palette
			v.rememberColor(artColorBytes(v.color))
		}
		row := cur.Next(0)
		button := mathx.Rect{X: row.X, Y: row.Y, W: min(row.W, row.H), H: row.H}
		id := ui.ID("picker-eyedropper")
		if ui.Clicked(id, button) {
			p.toggleOneShotPicker()
		}
		bg := ui.WidgetColor(id)
		if v.pickOnce {
			bg = ui.Theme.Selection
		}
		ui.Box(button, ui.Theme.Radius.Button, bg, ui.Theme.Border)
		widgets.IconIn(ui, button, "tool.eyedropper", ui.Theme.Text)
		label := i18n.Tr("editor.art.pick_canvas_color", "Pick color from canvas")
		ui.TextIn(mathx.Rect{X: button.X + button.W + 4, Y: row.Y, W: max(0, row.W-button.W-4), H: row.H}, widgets.Elide(ui, label, max(0, row.W-button.W-4)), ui.Theme.TextDim, core.AlignLeft)
		widgets.Tooltip(ui, id, i18n.Tr("editor.art.pick_canvas_hint", "Pick a pixel from the active layer, then return to the previous tool. Click again or press Escape to cancel."))
		p.colorPicker(ui)
		if editable && len(p.model.Session.Document.Info().Palette) < 256 && artButton(ui, "add-foreground", cur.Next(0), i18n.Tr("editor.art.palette_add_foreground", "Add foreground to palette")) {
			p.addForegroundSwatch()
		}
		if p.model.Session.Document.Info().Canvas.Mode == art.Indexed && p.model.Session.Channel == art.Albedo {
			if artPaletteColorIndex(p.model.Session.Document.Info().Palette, artColorBytes(v.color), v.palette) < 0 {
				widgets.LabelWrapped(ui, i18n.Tr("editor.art.indexed_add_to_paint", "Indexed painting still uses the selected swatch. Add this color to the palette to paint with it."), false)
			}
		}
		p.recentColorGrid(ui)
	})
}

// Keep grid controls usable at different dock widths, without stretching a
// single swatch or leaving most of a wide panel unused. All sizes are logical.
func artGridSize(width float32, count int, preferred, maximum float32) (int, float32) {
	if width <= 0 || count <= 0 || preferred <= 0 || maximum <= 0 {
		return 0, 0
	}
	columns := max(1, min(count, int((width+4)/(preferred+4))))
	cell := min(maximum, max(0, (width-float32(columns-1)*4)/float32(columns)))
	return columns, cell
}

// A picker selection changes view state, not document palette data. In indexed
// mode only select a matching existing swatch; never remap or overwrite it.
func (p *artPanel) setForegroundColor(color mathx.Color, remember bool) {
	v, session := p.view, p.model.Session
	v.color = color
	info := session.Document.Info()
	if info.Canvas.Mode == art.Indexed && session.Channel == art.Albedo {
		if index := artPaletteColorIndex(info.Palette, artColorBytes(color), v.palette); index >= 0 {
			v.palette, v.palettePage = index, index/8
		}
	}
	if remember {
		v.rememberColor(artColorBytes(color))
	}
}

func (v *artView) rememberColor(color art.Color) {
	if index := slices.Index(v.recentColors, color); index >= 0 {
		v.recentColors = slices.Delete(v.recentColors, index, index+1)
	}
	v.recentColors = append([]art.Color{color}, v.recentColors...)
	if len(v.recentColors) > 12 {
		v.recentColors = v.recentColors[:12]
	}
}

// Duplicate indexed entries are meaningful. Keep the current index when it
// already matches rather than silently switching to an identical swatch.
func artPaletteColorIndex(palette []art.Swatch, color art.Color, preferred int) int {
	if preferred >= 0 && preferred < len(palette) && palette[preferred].Color == color {
		return preferred
	}
	for i, swatch := range palette {
		if swatch.Color == color {
			return i
		}
	}
	return -1
}

func (p *artPanel) recentColorGrid(ui *core.Context) {
	v, cur, session := p.view, ui.Cursor(), p.model.Session
	if len(v.recentColors) == 0 {
		return
	}
	widgets.LabelDim(ui, i18n.Tr("editor.art.recent_colors", "Recent colors"))
	info := session.Document.Info()
	indexed := info.Canvas.Mode == art.Indexed && session.Channel == art.Albedo
	columns, width := artGridSize(cur.W, len(v.recentColors), max(24, ui.Theme.RowHeight*1.2), max(36, ui.Theme.RowHeight*1.8))
	if columns == 0 || width <= 0 {
		return
	}
	selected := -1
	var row mathx.Rect
	for i, color := range v.recentColors {
		if i%columns == 0 {
			row = cur.Next(width)
		}
		cell := mathx.Rect{X: row.X + float32(i%columns)*(width+4), Y: row.Y, W: min(width, max(0, row.W-float32(i%columns)*(width+4))), H: row.H}
		widgets.Swatch(ui, cell, mathx.ColorRGBA8(color.R, color.G, color.B, color.A), false)
		id := ui.ID("recent-color-" + fmt.Sprint(i))
		available := !indexed || artPaletteColorIndex(info.Palette, color, v.palette) >= 0
		if !available {
			shade := ui.Theme.Background
			shade.A = .6
			ui.Rect(cell, shade)
		}
		if ui.Clicked(id, cell) && available {
			selected = i
		}
		if !available {
			widgets.Tooltip(ui, id, i18n.Tr("editor.art.recent_indexed_unavailable", "Add this color to the palette before using it in indexed mode."))
		}
	}
	if selected >= 0 {
		color := v.recentColors[selected]
		p.setForegroundColor(mathx.ColorRGBA8(color.R, color.G, color.B, color.A), true)
	}
}

func (p *artPanel) addForegroundSwatch() {
	index := len(p.model.Session.Document.Info().Palette)
	swatch := art.Swatch{Color: artColorBytes(p.view.color)}
	_, err := p.model.Execute(stdctx.Background(), art.Operation{Kind: "palette-add", Swatch: &swatch})
	p.report(err)
	if err == nil {
		p.view.palette = index
		p.view.refresh = true
		p.view.rememberColor(swatch.Color)
	}
}

// The dedicated Palette panel uses every available row instead of forcing a
// one-line page selector. The selected palette index remains the same value
// used by indexed painting, and right-click selects the background index.
func (p *artPanel) paletteGrid(ui *core.Context, palette []art.Swatch) {
	cur, v := ui.Cursor(), p.view
	columns, width := artGridSize(cur.W, len(palette), max(24, ui.Theme.RowHeight*1.2), max(36, ui.Theme.RowHeight*1.8))
	if columns == 0 || width <= 0 {
		return
	}
	var row mathx.Rect
	for i, swatch := range palette {
		if i%columns == 0 {
			row = cur.Next(width)
		}
		cell := mathx.Rect{X: row.X + float32(i%columns)*(width+4), Y: row.Y, W: min(width, max(0, row.W-float32(i%columns)*(width+4))), H: row.H}
		c := swatch.Color
		widgets.Swatch(ui, cell, mathx.ColorRGBA8(c.R, c.G, c.B, c.A), false)
		id := ui.ID("palette-grid-" + fmt.Sprint(i))
		if ui.Clicked(id, cell) {
			v.palette, v.palettePage = i, i/8
			v.color = mathx.ColorRGBA8(c.R, c.G, c.B, c.A)
			v.rememberColor(c)
		}
		if ui.RightClicked(id, cell) {
			v.backgroundPalette = i
			v.background = mathx.ColorRGBA8(c.R, c.G, c.B, c.A)
		}
		if v.palette == i {
			ui.Outline(cell, ui.Theme.Accent)
		}
		widgets.Tooltip(ui, id, i18n.Tr("editor.art.palette_pointer_hint", "Left click: foreground. Right click: background. X swaps them."))
	}
}

func (p *artPanel) infoPanel(ui *core.Context, r mathx.Rect, editable bool) {
	p.partRegion(ui, r, "art.info", func() {
		cur, s := ui.Cursor(), p.model.Session
		info := s.Document.Info()
		widgets.LabelWrapped(ui, i18n.Tr("editor.art.canvas_size_info", "Canvas: {width} × {height}", i18n.Int("width", int64(info.Canvas.Width)), i18n.Int("height", int64(info.Canvas.Height))), false)
		widgets.LabelWrapped(ui, i18n.Tr("editor.art.color_mode_info", "Color mode: {mode}", i18n.Str("mode", artModeLabel(info.Canvas.Mode))), false)
		widgets.LabelWrapped(ui, i18n.Tr("editor.art.frame_count_info", "Frames: {count}", i18n.Int("count", int64(len(info.Frames)))), false)
		if artButton(ui, "pivot", cur.Next(0), i18n.Tr("editor.art.edit_pivot", "Frame pivot…")) && editable {
			p.askPivot()
		}
		p.authoringActionsFor(ui, editable, artActionsDocument)
	})
}

func artModeLabel(mode art.Mode) string {
	switch mode {
	case art.RGBA:
		return "RGBA"
	case art.Grayscale:
		return i18n.Tr("editor.art.color_mode_gray", "Grayscale")
	case art.Indexed:
		return i18n.Tr("editor.art.color_mode_indexed", "Indexed")
	default:
		return string(mode)
	}
}

func (p *artPanel) optionsPanel(ui *core.Context, r mathx.Rect, editable bool) {
	p.partRegion(ui, r, "art.options", func() {
		cur, v, s := ui.Cursor(), p.view, p.model.Session
		tools := artToolOptions()
		if v.tool >= 0 && v.tool < len(tools) {
			widgets.LabelWrapped(ui, tools[v.tool].Label, false)
		}
		p.toolStrokePreview(ui)
		brush := v.tool == 0 || v.tool == 1 || v.tool == 3 || v.tool == 4 || v.tool == 5 || v.tool == artCurve || v.tool == artPolygon
		if brush {
			p.brushSizeField(ui, "size", widgets.Field(ui, i18n.Tr("editor.art.brush_size", "Brush size")))
			if v.tool == 1 {
				p.eraserControls(ui)
			}
			if v.tool == 0 {
				artToggle(ui, "perfect", cur.Next(0), i18n.Tr("editor.art.pixel_perfect", "Pixel perfect"), &v.perfect)
			}
			artToggle(ui, "round", cur.Next(0), i18n.Tr("editor.art.round", "Round brush"), &v.round)
			if v.tool == 4 || v.tool == 5 || v.tool == artPolygon {
				artToggle(ui, "filled", cur.Next(0), i18n.Tr("editor.art.filled", "Filled shapes"), &v.filled)
			}
			artToggle(ui, "mirror-x", cur.Next(0), i18n.Tr("editor.art.mirror_x", "Mirror X"), &v.mirrorX)
			artToggle(ui, "mirror-y", cur.Next(0), i18n.Tr("editor.art.mirror_y", "Mirror Y"), &v.mirrorY)
			if v.tool != 1 || v.eraserMode != 1 {
				widgets.Combo(ui, ui.ID("brush-source"), cur.Next(0), &v.brushSource, []string{i18n.Tr("editor.art.solid_brush", "Solid"), i18n.Tr("editor.art.custom_brush", "Custom"), i18n.Tr("editor.art.pattern_brush", "Pattern")}, false)
				row := cur.Next(0)
				if artButton(ui, "capture-brush", artCell(row, 0, 2), i18n.Tr("editor.art.capture_brush", "Capture")) {
					if err := s.CaptureBrush(); err != nil {
						p.report(err)
					} else {
						p.report(s.SetSelection("none", nil, 0))
						v.tool, v.brushSource, v.refresh = 0, 1, true
					}
				}
				if artButton(ui, "clear-brush", artCell(row, 1, 2), i18n.Tr("editor.art.clear_brush", "Clear")) {
					s.ClearBrush()
					v.brushSource = 0
				}
			}
		}
		if v.tool == 2 {
			widgets.LabelWrapped(ui, i18n.Tr("editor.art.fill_current_mode", "Contiguous fill on the active layer, with zero tolerance."), false)
		}
		if v.tool == 6 {
			widgets.ColorField(ui, ui.ID("background-color"), widgets.Field(ui, i18n.Tr("editor.art.background_color_short", "Background")), &v.background, false)
			widgets.DragNumber(ui, ui.ID("dither"), widgets.Field(ui, i18n.Tr("editor.art.dither", "Dither")), &v.dither, widgets.NumberOpts{Int: true, HasMin: true, Min: 0, HasMax: true, Max: 16, Speed: .1})
		}
		if v.tool == artRectSelect || v.tool == artEllipseSelect || v.tool == artLasso || v.tool == artWand {
			p.authoringActionsFor(ui, editable, artActionsPixels)
		}
	})
}
