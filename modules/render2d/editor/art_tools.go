package editor

import (
	"math"

	"aego/art"
	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

// The inline canvas, Tools dock and Tool Options edit the same view settings.
// Callers keep their existing widget IDs so focus and input capture are stable.
func (p *artPanel) brushSizeField(ui *core.Context, id string, r mathx.Rect) {
	widgets.DragNumber(ui, ui.ID(id), r, &p.view.size, widgets.NumberOpts{Int: true, HasMin: true, Min: 1, HasMax: true, Max: 64, Speed: .3})
}

func (p *artPanel) foregroundColorField(ui *core.Context, r mathx.Rect) {
	v := p.view
	if widgets.ColorField(ui, ui.ID("paint-color"), r, &v.color, false).End {
		v.rememberColor(artColorBytes(v.color))
	}
}

// Canvas and Tool Options use the same brush settings and palette indices.
func (p *artPanel) currentBrush() art.Brush {
	v, s := p.view, p.model.Session
	info := s.Document.Info()
	brush := art.Brush{Size: int(v.size), Round: v.round, PixelPerfect: v.perfect, MirrorX: v.mirrorX, MirrorY: v.mirrorY,
		Paint:      art.Paint{Color: artColorBytes(v.color)},
		Background: art.Paint{Color: artColorBytes(v.background)},
		Dither:     int(v.dither),
		Source:     []string{"solid", "stamp", "pattern"}[min(2, max(0, v.brushSource))],
	}
	if info.Canvas.Mode == art.Indexed && s.Channel == art.Albedo && len(info.Palette) > 0 {
		v.palette = min(max(0, v.palette), len(info.Palette)-1)
		v.backgroundPalette = min(max(0, v.backgroundPalette), len(info.Palette)-1)
		brush.Paint.UseIndex, brush.Paint.Index = true, v.palette
		brush.Background.UseIndex, brush.Background.Index = true, v.backgroundPalette
	}
	if v.tool == 1 {
		if v.eraserStrength == 0 {
			v.eraserStrength = 100
		}
		brush.EraseStrength = uint8(math.Round(float64(mathx.Clamp(float32(v.eraserStrength), 1, 100) * 255 / 100)))
		brush.Smudge = v.eraserMode == 1
		if brush.Smudge {
			// Smudge carries pixels from the preceding dab; a colored stamp,
			// dither and pixel-perfect corner removal do not define that action.
			brush.Source, brush.Dither, brush.PixelPerfect = "solid", 0, false
		}
	}
	return brush
}

func (p *artPanel) eraserControls(ui *core.Context) {
	v := p.view
	if v.eraserStrength == 0 {
		v.eraserStrength = 100
	}
	widgets.Combo(ui, ui.ID("eraser-mode"), widgets.Field(ui, i18n.Tr("editor.art.eraser_mode", "Eraser mode")), &v.eraserMode, []string{
		i18n.Tr("editor.art.eraser_erase", "Erase"), i18n.Tr("editor.art.eraser_smudge", "Smudge"),
	}, false)
	widgets.DragNumber(ui, ui.ID("eraser-strength"), widgets.Field(ui, i18n.Tr("editor.art.eraser_strength", "Strength (%)")), &v.eraserStrength, widgets.NumberOpts{Int: true, HasMin: true, Min: 1, HasMax: true, Max: 100, Speed: .3})
	if p.model.Session.Document.Info().Canvas.Mode == art.Indexed && p.model.Session.Channel == art.Albedo {
		widgets.LabelWrapped(ui, i18n.Tr("editor.art.eraser_indexed", "In indexed mode, partial results are limited to the existing palette."), true)
	}
}

func (p *artPanel) deselect() {
	if p.model == nil {
		return
	}
	p.report(p.model.Session.SetSelection("none", nil, 0))
	p.view.refresh = true
}

var artToolKeys = [...]input.Scancode{input.KeyB, input.KeyE, input.KeyG, input.KeyL, input.KeyR, input.KeyO, input.KeyD, input.KeyM, input.KeyW, input.KeyI, input.KeyUnknown, input.KeyQ, input.KeyC, input.KeyP}

// Picking through Color Picker is temporary; selecting a tool explicitly via
// the toolbar or keyboard cancels that temporary state instead of restoring it.
func (v *artView) chooseTool(tool int) {
	v.tool, v.pickOnce = tool, false
}

func (v *artView) cancelOneShotPicker() {
	if v.pickOnce {
		v.tool, v.pickOnce = v.pickReturnTool, false
	}
}

func (p *artPanel) toggleOneShotPicker() {
	v, s := p.view, p.model.Session
	if v.pickOnce {
		v.cancelOneShotPicker()
		return
	}
	if v.tool == artEyedropper || s.Info().Gesture || v.selecting || len(v.construction) != 0 {
		return
	}
	v.pickReturnTool, v.pickOnce, v.tool = v.tool, true, artEyedropper
}

func (p *artPanel) toolKeyboard(ui *core.Context, editable bool) {
	if ui.Primary() || ui.Mods().Has(input.ModAlt) {
		return
	}
	v, s := p.view, p.model.Session
	for index, key := range artToolKeys {
		if key != input.KeyUnknown && ui.KeyPressed(key) {
			v.chooseTool(index)
		}
	}
	if ui.KeyPressed(input.KeyX) {
		v.color, v.background = v.background, v.color
		v.palette, v.backgroundPalette = v.backgroundPalette, v.palette
		v.rememberColor(artColorBytes(v.color))
	}
	if ui.KeyPressed(input.KeyEnter) && len(v.construction) == 0 && !s.Info().Gesture {
		v.playing = !v.playing
		v.playStart = ui.Time
		v.refresh = true
	}
	if editable && !s.Info().Gesture {
		if ui.KeyPressed(input.KeyBackspace) || (ui.KeyPressed(input.KeyDelete) && (p.reg == nil || !p.reg.CommandsAvailable())) {
			p.execute(art.Operation{Kind: "clear"})
		}
		if ui.KeyPressed(input.KeyV) {
			p.askTransform(false)
		}
	}
}

func artToolOptions() []widgets.IconOption {
	return []widgets.IconOption{
		{Icon: "tool.pencil", Label: i18n.Tr("editor.art.pencil", "Pencil")},
		{Icon: "tool.eraser", Label: i18n.Tr("editor.art.eraser", "Eraser")},
		{Icon: "tool.fill", Label: i18n.Tr("editor.art.fill", "Fill")},
		{Icon: "tool.line", Label: i18n.Tr("editor.art.line", "Line")},
		{Icon: "tool.rectangle", Label: i18n.Tr("editor.art.rectangle", "Rectangle")},
		{Icon: "tool.ellipse", Label: i18n.Tr("editor.art.ellipse", "Ellipse")},
		{Icon: "tool.gradient", Label: i18n.Tr("editor.art.gradient", "Gradient")},
		{Icon: "tool.selection", Label: i18n.Tr("editor.art.selection", "Selection")},
		{Icon: "tool.wand", Label: i18n.Tr("editor.art.wand", "Magic wand")},
		{Icon: "tool.eyedropper", Label: i18n.Tr("editor.art.eyedropper", "Eyedropper (active layer)")},
		{Icon: "tool.select_ellipse", Label: i18n.Tr("editor.art.ellipse_selection", "Ellipse selection")},
		{Icon: "tool.lasso", Label: i18n.Tr("editor.art.lasso", "Lasso selection")},
		{Icon: "ui.curve", Label: i18n.Tr("editor.art.curve", "Curve")},
		{Icon: "comp.polygon", Label: i18n.Tr("editor.art.polygon", "Polygon")},
	}
}

// Brush controls operate on the same view/session whether displayed inline or
// in the dockable Tools pane. They never maintain separate art state.
func (p *artPanel) brushSettings(ui *core.Context, width float32) {
	cur, v, s := ui.Cursor(), p.view, p.model.Session
	if v.tool == 1 {
		p.eraserControls(ui)
	}
	narrow := width < 220
	var row mathx.Rect
	for i, toggle := range []struct {
		name, label string
		value       *bool
	}{
		{"perfect", i18n.Tr("editor.art.pixel_perfect", "Pixel perfect"), &v.perfect},
		{"round", i18n.Tr("editor.art.round", "Round brush"), &v.round},
		{"filled", i18n.Tr("editor.art.filled", "Filled shapes"), &v.filled},
		{"grid", i18n.Tr("editor.art.grid", "Pixel grid"), &v.grid},
		{"mirror-x", i18n.Tr("editor.art.mirror_x", "Mirror X"), &v.mirrorX},
		{"mirror-y", i18n.Tr("editor.art.mirror_y", "Mirror Y"), &v.mirrorY},
	} {
		if narrow || i%2 == 0 {
			row = cur.Next(0)
		}
		cell := row
		if !narrow {
			cell = artCell(row, i%2, 2)
		}
		artToggle(ui, toggle.name, cell, toggle.label, toggle.value)
	}
	row = cur.Next(0)
	mode, capture, clear := artCell(row, 0, 3), artCell(row, 1, 3), artCell(row, 2, 3)
	if narrow {
		mode = row
		row = cur.Next(0)
		capture, clear = artCell(row, 0, 2), artCell(row, 1, 2)
	}
	widgets.Combo(ui, ui.ID("brush-source"), mode, &v.brushSource, []string{i18n.Tr("editor.art.solid_brush", "Solid"), i18n.Tr("editor.art.custom_brush", "Custom"), i18n.Tr("editor.art.pattern_brush", "Pattern")}, false)
	if artButton(ui, "capture-brush", capture, i18n.Tr("editor.art.capture_brush", "Capture")) {
		if err := s.CaptureBrush(); err != nil {
			p.report(err)
		} else {
			p.report(s.SetSelection("none", nil, 0))
			v.tool, v.brushSource, v.refresh = 0, 1, true
		}
	}
	widgets.Tooltip(ui, ui.ID("capture-brush"), i18n.Tr("editor.art.capture_brush_hint", "Capture a custom brush from the selected pixels"))
	if artButton(ui, "clear-brush", clear, i18n.Tr("editor.art.clear_brush", "Clear")) {
		s.ClearBrush()
		v.brushSource = 0
	}
	widgets.Tooltip(ui, ui.ID("clear-brush"), i18n.Tr("editor.art.clear_brush_hint", "Clear the custom brush and return to a solid brush"))
}

func (p *artPanel) brushPresentation(ui *core.Context) {
	cur := ui.Cursor()
	row := cur.Next(0)
	widgets.ColorField(ui, ui.ID("background-color"), artCell(row, 0, 2), &p.view.background, false)
	widgets.Tooltip(ui, ui.ID("background-color"), i18n.Tr("editor.art.background_color", "Background color for gradient and dithering"))
	widgets.DragNumber(ui, ui.ID("brush-dither"), artCell(row, 1, 2), &p.view.dither, widgets.NumberOpts{Int: true, HasMin: true, Min: 0, HasMax: true, Max: 16, Speed: .1})
	widgets.Tooltip(ui, ui.ID("brush-dither"), i18n.Tr("editor.art.dither_level", "Dithering: 0 off, 1–16 foreground coverage"))
	widgets.Combo(ui, ui.ID("canvas-background"), cur.Next(0), &p.view.canvasBackground, []string{
		i18n.Tr("editor.image.checker", "Checkerboard"), i18n.Tr("editor.image.light_background", "Light background"), i18n.Tr("editor.image.dark_background", "Dark background"),
	}, false)
	artToggle(ui, "tiled-preview", cur.Next(0), i18n.Tr("editor.art.tiled_preview", "Tiled preview"), &p.view.tiled)
}

func artInlineToolFields(optionsVisible, colorVisible bool) (size, color bool) {
	return !optionsVisible, !colorVisible
}

func (p *artPanel) tools(ui *core.Context, r mathx.Rect) {
	ui.Region(r)
	defer ui.PopCursor()
	ui.PushClip(r)
	defer ui.PopClip()
	cur, v := ui.Cursor(), p.view
	if ui.MouseOver(ui.ID("tools-scroll"), r) {
		v.toolScroll -= ui.ScrollY(ui.Theme.RowHeight * 3)
	}
	v.toolScroll = mathx.Clamp(v.toolScroll, 0, max(0, v.toolContent-r.H))
	if v.toolContent > r.H {
		cur.W = max(0, cur.W-ui.Theme.ScrollbarW)
	}
	cur.Y -= v.toolScroll
	top := cur.Y
	defer func() {
		v.toolContent = cur.Y - top + ui.Theme.Padding*2
		track := mathx.Rect{X: r.X + r.W - ui.Theme.ScrollbarW, Y: r.Y, W: ui.Theme.ScrollbarW, H: r.H}
		widgets.ScrollBar(ui, ui.ID("tools-scrollbar"), track, &v.toolScroll, v.toolContent, r.H, &v.toolBar)
	}()
	columns, buttonWidth := artGridSize(cur.W, len(artToolKeys), max(32, ui.Theme.RowHeight*1.5), max(44, ui.Theme.RowHeight*2))
	if columns == 0 || buttonWidth <= 0 {
		return
	}
	var row mathx.Rect
	for i, tool := range artToolOptions() {
		if i%columns == 0 {
			row = cur.Next(buttonWidth)
		}
		cell := mathx.Rect{X: row.X + float32(i%columns)*(buttonWidth+4), Y: row.Y, W: buttonWidth, H: buttonWidth}
		id := ui.ID(tool.Icon + tool.Label)
		if ui.Clicked(id, cell) {
			v.chooseTool(i)
		}
		background := ui.WidgetColor(id)
		if v.tool == i {
			background = ui.Theme.Selection
		}
		ui.Box(cell, ui.Theme.Radius.Button, background, ui.Theme.Border)
		if cell.W >= 8 {
			size := min(float32(24), cell.W-8)
			icon := mathx.Rect{X: cell.X + (cell.W-size)/2, Y: cell.Y + (cell.H-size)/2, W: size, H: size}
			widgets.IconAt(ui, icon, tool.Icon, ui.Theme.Text)
		}
		tip := tool.Label
		if artToolKeys[i] != input.KeyUnknown {
			tip += " · " + artToolKeys[i].String()
		}
		widgets.Tooltip(ui, id, tip)
	}
	optionsVisible := p.reg != nil && p.reg.PanelVisible("art.options")
	colorVisible := p.reg != nil && p.reg.PanelVisible("art.color")
	showSize, showColor := artInlineToolFields(optionsVisible, colorVisible)
	if showSize && showColor && cur.W >= 220 {
		row = cur.Next(0)
		p.brushSizeField(ui, "brush-size", artCell(row, 0, 2))
		p.foregroundColorField(ui, artCell(row, 1, 2))
	} else {
		if showSize {
			p.brushSizeField(ui, "brush-size", cur.Next(0))
		}
		if showColor {
			p.foregroundColorField(ui, cur.Next(0))
		}
	}
	if !optionsVisible {
		p.brushSettings(ui, r.W)
		p.brushPresentation(ui)
	}
}
