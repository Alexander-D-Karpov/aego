package editor

import (
	"fmt"
	"image"
	"math"

	"aego/art"
	"aego/asset"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/engine/i18n"
	"aego/gpu"
	"aego/platform/input"
	"aego/render/texture"
	"aego/ui/core"
	"aego/ui/widgets"
)

// Canvas presentation and input use the panel's canonical document and view;
// they do not own a separate editing session or history.
func (p *artPanel) Draw(f *services.Frame, area mathx.Rect) {
	ui, v := f.UI, p.view
	if f.Focused && p.docks != nil {
		p.docks.active = p
	}
	ui.PushID(p.ID())
	defer ui.PopID()
	ui.PushClip(area)
	defer ui.PopClip()
	ui.Region(area)
	defer ui.PopCursor()
	cur := ui.Cursor()
	if p.model == nil {
		if p.empty {
			widgets.LabelWrapped(ui, i18n.Tr("editor.art.empty_source_hint", "This file has no drawing. Initialize it, or create a separate drawing from the Create menu."), false)
			if artButton(ui, "initialize-empty", cur.Next(0), i18n.Tr("editor.art.initialize_empty", "Initialize empty drawing…")) {
				p.askInitializeEmpty()
			}
		} else if p.err != nil {
			widgets.LabelWrapped(ui, p.err.Error(), true)
		}
		return
	}
	s := p.model.Session
	info := s.Info()
	editable := p.doc.Capabilities.Has(document.EditSource) && !p.model.PendingRecovery
	if area.W < 300 || area.H < 220 {
		widgets.LabelWrapped(ui, i18n.Tr("editor.art.expand", "Enlarge this panel to display the canvas and controls."), false)
		return
	}
	if p.model.PendingRecovery {
		widgets.LabelWrapped(ui, i18n.Tr("editor.art.recovery_pending_edit", "Recover or discard the previous recovery before editing this drawing."), false)
		row := cur.Next(0)
		if artButton(ui, "recover-art", artCell(row, 0, 2), i18n.Tr("editor.art.recover", "Recover unsaved drawing")) {
			p.ctx.Ask(context.Question{Title: i18n.Tr("editor.art.recover", "Recover unsaved drawing"), Message: i18n.Tr("editor.art.recover_confirm", "Replace this open document with the recovered drawing? The source file is not overwritten."), Buttons: []string{i18n.Tr("editor.art.recover_action", "Recover"), i18n.Tr("editor.art.cancel", "Cancel")}, Then: func(choice int) {
				if choice == 0 {
					p.report(p.model.Recover())
					p.view.refresh = true
				}
			}})
		}
		if artButton(ui, "discard-recovery", artCell(row, 1, 2), i18n.Tr("editor.art.discard_recovery", "Discard recovery")) {
			p.ctx.Ask(context.Question{Title: i18n.Tr("editor.art.discard_recovery", "Discard recovery"), Message: i18n.Tr("editor.art.discard_recovery_confirm", "Permanently discard the recovery copy?"), Buttons: []string{i18n.Tr("editor.art.discard_recovery_action", "Discard"), i18n.Tr("editor.art.cancel", "Cancel")}, Then: func(choice int) {
				if choice == 0 {
					p.report(p.model.DiscardRecovery())
				}
			}})
		}
	}
	if p.doc.Transient {
		widgets.LabelWrapped(ui, i18n.Tr("editor.art.blank_hint", "Untitled drawing in memory. Save to create a project asset."), true)
	} else if p.model.Draft {
		widgets.LabelWrapped(ui, i18n.Tr("editor.art.png_draft_hint", "PNG is open in memory. Save to choose a linked editable source and publish the drawing; the PNG itself is not overwritten."), true)
	}
	row := cur.Next(0)
	// These are fixed-size actions, not buttons stretched across a large
	// canvas. The existing shared command/history route remains unchanged.
	buttonW := min(row.H, max(0, (row.W-16)/5))
	button := func(n int) mathx.Rect {
		return mathx.Rect{X: row.X + float32(n)*(buttonW+4), Y: row.Y, W: buttonW, H: row.H}
	}
	if artIconAction(ui, "save", button(0), "file.save", i18n.Tr("editor.art.save", "Save"), editable) {
		p.report(p.ctx.SaveAsset(p.doc))
	}
	if artIconAction(ui, "undo", button(1), "nav.back", i18n.Tr("editor.art.undo", "Undo"), editable) {
		p.report(p.model.SessionUndo())
		v.refresh = true
	}
	if artIconAction(ui, "redo", button(2), "nav.forward", i18n.Tr("editor.art.redo", "Redo"), editable) {
		p.report(p.model.SessionRedo())
		v.refresh = true
	}
	if artIconAction(ui, "fit", button(3), "view.zoom", i18n.Tr("editor.art.fit", "Fit"), true) {
		v.viewport.Ready = false
	}
	label := i18n.Tr("editor.art.deselect", "Deselect")
	x := row.X + 4*(buttonW+4)
	deselect := mathx.Rect{X: x, Y: row.Y, W: min(max(0, row.X+row.W-x), max(72, ui.TextSize(label).X+16)), H: row.H}
	if artButton(ui, "deselect", deselect, label) {
		p.deselect()
	}
	widgets.Tooltip(ui, ui.ID("deselect"), i18n.Tr("editor.art.deselect_shortcut", "Deselect (Ctrl/Cmd+D)"))
	if p.reg == nil || !p.reg.PanelVisible("art.tools") {
		row = cur.Next(0)
		tools := artToolOptions()
		if widgets.IconCombo(ui, ui.ID("tool"), artCell(row, 0, 3), &v.tool, tools) {
			v.pickOnce = false
		}
		p.brushSizeField(ui, "brush-size", artCell(row, 1, 3))
		if info.Document.Canvas.Mode != art.Indexed || s.Channel != art.Albedo {
			p.foregroundColorField(ui, artCell(row, 2, 3))
		} else {
			ui.TextIn(artCell(row, 2, 3), fmt.Sprintf("#%d", v.palette), ui.Theme.Text, core.AlignCenter)
		}
	}
	docked := p.usesArtDocks()
	narrow := area.W < 740 && !docked
	if narrow {
		row = cur.Next(0)
		widgets.Combo(ui, ui.ID("pane"), row, &v.pane, []string{i18n.Tr("editor.art.canvas", "Canvas"), i18n.Tr("editor.art.properties", "Tools / Layers"), i18n.Tr("editor.art.timeline", "Timeline")}, false)
	}
	// Navigation and actions never steal keyboard input from a text field.
	if f.Focused && !ui.WantsKeyboard() {
		if !ui.AnyPopup() {
			p.toolKeyboard(ui, editable)
		}
		if ui.KeyPressed(input.KeyEscape) && !ui.AnyPopup() {
			pending := info.Gesture || v.selecting || v.pickOnce
			v.cancelOneShotPicker()
			s.CancelGesture()
			v.construction = nil
			v.selecting = false
			v.selectionPoints = nil
			if !pending {
				p.deselect()
			}
			v.refresh = true
			ui.ReleaseMouse()
		}
		if ui.Primary() && editable {
			// The shell already dispatches shared document Undo/Redo/Save and
			// clipboard actions once, before drawing. Only hosts without its
			// command service need the local fallback. Deselect is Art-specific.
			if ui.KeyPressed(input.KeyD) {
				p.deselect()
			}
			if p.reg == nil || !p.reg.CommandsAvailable() {
				switch {
				case ui.KeyPressed(input.KeyZ):
					if ui.Mods().Has(input.ModShift) {
						p.report(p.model.SessionRedo())
					} else {
						p.report(p.model.SessionUndo())
					}
					v.refresh = true
				case ui.KeyPressed(input.KeyS):
					p.report(p.ctx.SaveAsset(p.doc))
				case ui.KeyPressed(input.KeyC):
					p.report(s.Copy())
				case ui.KeyPressed(input.KeyV):
					p.report(s.Paste(image.Point{}))
					v.refresh = true
				case ui.KeyPressed(input.KeyA):
					p.report(s.SetSelection("all", nil, 0))
					v.refresh = true
				}
			}
		}
	}
	// Finish a stroke even when the pointer leaves the panel or focus changes.
	if !f.Focused && (info.Gesture || v.selecting) {
		s.CancelGesture()
		v.construction = nil
		v.selecting = false
		v.selectionPoints = nil
		v.refresh = true
		ui.ReleaseMouse()
	}
	content := mathx.Rect{X: cur.X, Y: cur.Y, W: cur.W, H: max(0, area.Y+area.H-cur.Y-28)}
	if docked {
		p.canvas(f, content)
	} else if narrow {
		switch v.pane {
		case 0:
			p.canvas(f, content)
		case 1:
			p.properties(ui, content, editable)
		case 2:
			p.timeline(ui, content, editable)
		}
	} else {
		sideW := float32(240)
		bottomH := min(float32(180), max(float32(120), content.H*.3))
		canvas := mathx.Rect{X: content.X, Y: content.Y, W: content.W - sideW - 8, H: content.H - bottomH - 8}
		side := mathx.Rect{X: canvas.X + canvas.W + 8, Y: canvas.Y, W: sideW, H: canvas.H}
		timeline := mathx.Rect{X: content.X, Y: canvas.Y + canvas.H + 8, W: content.W, H: bottomH}
		p.canvas(f, canvas)
		p.properties(ui, side, editable)
		p.timeline(ui, timeline, editable)
	}
	status := mathx.Rect{X: area.X + 8, Y: area.Y + area.H - 24, W: area.W - 16, H: 20}
	text := i18n.Tr("editor.art.navigation", "Wheel: zoom · Space+drag or middle/right drag: pan · Esc: cancel gesture or clear selection")
	if v.canvasStatus != "" && (docked || !narrow || v.pane == 0) {
		layer, frame := "", 0
		for _, candidate := range info.Document.Layers {
			if candidate.ID == s.Layer {
				layer = candidate.Name
				break
			}
		}
		for index, candidate := range info.Document.Frames {
			if candidate.ID == s.Frame {
				frame = index + 1
				break
			}
		}
		text = v.canvasStatus + " · " + i18n.Tr("editor.art.status_context", "Layer: {layer} · Frame {frame}/{total} · {mode}", i18n.Str("layer", layer), i18n.Int("frame", int64(frame)), i18n.Int("total", int64(len(info.Document.Frames))), i18n.Str("mode", artModeLabel(info.Document.Canvas.Mode)))
	}
	if v.outlineSimplified && s.Selection != nil {
		text = i18n.Tr("editor.art.selection_outline_simplified", "Selection outline simplified") + " · " + text
	}
	if p.err != nil {
		text = p.err.Error()
	} else if p.doc.WorkingPreview.Error != "" {
		text = p.doc.WorkingPreview.Error
	}
	ui.TextIn(status, text, ui.Theme.TextDim, core.AlignLeft)
}

// In authoring workspaces the canvas must not recreate a panel the user has
// closed. The inline arrangement remains available to other workspaces and
// lightweight hosts that do not provide the dedicated Art docks.
func (p *artPanel) usesArtDocks() bool {
	if p.reg == nil {
		return false
	}
	preset := p.reg.CurrentWorkspace()
	for _, id := range []string{preset.ID, preset.Base} {
		if id == workspace.Art || id == workspace.Animation || id == workspace.VFX {
			return true
		}
	}
	return (p.reg.PanelVisible("art.properties") || p.reg.PanelVisible("art.layers")) && p.reg.PanelVisible("art.timeline")
}

func (p *artPanel) canvas(f *services.Frame, r mathx.Rect) {
	if r.W < 1 || r.H < 1 {
		return
	}
	ui, v, s := f.UI, p.view, p.model.Session
	if len(v.construction) != 0 && v.constructionTool != v.tool {
		s.CancelGesture()
		v.construction = nil
		v.refresh = true
		ui.ReleaseMouse()
	}
	info := s.Info()
	frame := s.Frame
	if v.playing {
		if candidate, err := s.Document.FrameAt(v.tag, int64((f.Now-v.playStart)*1000)); err == nil {
			frame = candidate
		} else {
			v.playing = false
			p.report(err)
		}
	}
	generation, lost := imageDevice(p.ctx)
	if lost {
		v.display.close(p.ctx)
		v.refresh = true
		return
	}
	if generation != v.device {
		v.display.close(p.ctx)
		v.device = generation
		v.refresh = true
	}
	if !v.viewport.Ready {
		v.viewport.fit(r, info.Document.Canvas.Width, info.Document.Canvas.Height)
	}
	// Space-drag is navigation, never a pencil stroke. Keep the capture until
	// mouse release even if Space is released first or the pointer leaves the
	// canvas. A gesture already in progress is not silently converted to pan.
	panID := ui.ID("art.pan")
	// A click may focus this canvas only after the current frame. Recognize
	// Space while hovering it even on that first click, so it cannot paint.
	spaceHeld := ui.Input() != nil && ui.Input().KeyDown(input.KeySpace)
	space := spaceHeld && !ui.WantsKeyboard()
	if !f.Focused && v.panDragging {
		if ui.Captured() == panID {
			ui.ReleaseMouse()
		}
		v.panDragging = false
	}
	if space && !v.panDragging && !info.Gesture && !v.selecting && len(v.construction) == 0 && ui.MouseOver(panID, r) && ui.MousePressed(input.MouseLeft) {
		v.panDragging = true
		ui.CaptureMouse(panID)
	}
	if v.panDragging {
		if ui.Captured() != panID {
			v.panDragging = false
		} else if ui.MouseDown(input.MouseLeft) {
			v.viewport.pan(ui.MouseDelta())
		} else {
			ui.ReleaseMouse()
			v.panDragging = false
		}
	}
	id := ui.ID("art.paint")
	pt := v.viewport.imagePoint(r, ui.Mouse())
	point := image.Pt(int(math.Floor(float64(pt.X))), int(math.Floor(float64(pt.Y))))
	brush := p.currentBrush()
	editable := p.doc.Capabilities.Has(document.EditSource) && !p.model.PendingRecovery
	if !v.playing && !spaceHeld && !v.panDragging && (v.tool == artEyedropper || editable) && ui.MouseOver(id, r) && ui.MousePressed(input.MouseLeft) {
		switch v.tool {
		case artCurve, artPolygon:
			p.constructionClick(ui, point, brush)
		case 2:
			p.execute(art.Operation{Kind: "fill", Points: []image.Point{point}, Brush: brush})
		case artRectSelect, artEllipseSelect, artLasso:
			v.selecting = true
			v.selectionStart = point
			v.selectionTool = v.tool
			v.selectionRevision = info.Document.Revision
			v.selectionLayer, v.selectionFrame, v.selectionChannel = s.Layer, s.Frame, s.Channel
			v.selectionPoints = append(v.selectionPoints[:0], point)
			ui.CaptureMouse(id)
		case artWand:
			p.report(s.SetSelection("wand", []image.Point{point}, 0))
			v.refresh = true
		case artEyedropper:
			if point.In(image.Rect(0, 0, info.Document.Canvas.Width, info.Document.Canvas.Height)) {
				paint, err := s.SamplePaint(point)
				if err == nil {
					v.color = mathx.ColorRGBA8(paint.Color.R, paint.Color.G, paint.Color.B, paint.Color.A)
					v.rememberColor(paint.Color)
					if paint.UseIndex {
						v.palette, v.palettePage = paint.Index, paint.Index/8
					}
					v.cancelOneShotPicker()
				}
				p.report(err)
			}
		default:
			names := []string{"pencil", "eraser", "", "line", "rectangle", "ellipse", "gradient"}
			if v.tool >= 0 && v.tool < len(names) {
				err := s.BeginGesture(names[v.tool], point, brush, v.filled)
				p.report(err)
				if err == nil {
					ui.CaptureMouse(id)
					v.refresh = true
				}
			}
		}
	}
	if len(v.construction) != 0 {
		if ui.MousePressed(input.MouseRight) || !s.Info().Gesture {
			s.CancelGesture()
			v.construction = nil
			v.refresh = true
			ui.ReleaseMouse()
		} else if f.Focused && !ui.WantsKeyboard() && ui.KeyPressed(input.KeyEnter) && v.tool == artPolygon && len(v.construction) >= 3 {
			p.finishConstruction(ui)
		} else if ui.MouseDelta() != (mathx.Vec2{}) {
			p.previewConstruction(point)
		}
	} else if s.Info().Gesture {
		if ui.MouseDown(input.MouseLeft) {
			if ui.MouseDelta() != (mathx.Vec2{}) {
				p.report(s.MoveGesture([]image.Point{point}))
				v.refresh = true
			}
		} else {
			p.report(s.EndGesture())
			ui.ReleaseMouse()
			v.refresh = true
		}
	}
	if v.selecting {
		if v.selectionTool == artLasso && (len(v.selectionPoints) == 0 || v.selectionPoints[len(v.selectionPoints)-1] != point) {
			if len(v.selectionPoints) >= 2048 {
				p.report(art.ErrLimit)
				v.selecting = false
				v.selectionPoints = nil
				ui.ReleaseMouse()
			} else {
				v.selectionPoints = append(v.selectionPoints, point)
			}
		}
		if v.selecting && !ui.MouseDown(input.MouseLeft) {
			if s.Document.Revision() != v.selectionRevision || s.Layer != v.selectionLayer || s.Frame != v.selectionFrame || s.Channel != v.selectionChannel {
				p.report(fmt.Errorf("%s", i18n.Tr("editor.art.selection_stale", "The drawing changed during selection; select again.")))
			} else {
				kind, points := "rectangle", []image.Point{v.selectionStart, point}
				if v.selectionTool == artEllipseSelect {
					kind = "ellipse"
				} else if v.selectionTool == artLasso {
					kind, points = "lasso", v.selectionPoints
				}
				p.report(s.SetSelection(kind, points, 0))
			}
			v.selecting = false
			v.selectionPoints = nil
			v.refresh = true
			ui.ReleaseMouse()
		}
	}
	info = s.Info()
	if v.refresh || v.revision != info.Document.Revision || v.shownFrame != frame || v.shownChannel != s.Channel || v.shownOnion != v.onion || v.shownIsolation != info.Isolation {
		pixels, err := s.Preview(frame, v.onion && !v.playing, true)
		if err != nil {
			p.report(err)
			return
		}
		v.pixels = &imagePixels{data: texture.Data{Width: pixels.Rect.Dx(), Height: pixels.Rect.Dy(), Format: gpu.TexSRGBA8, Pixels: pixels.Pix}, semantic: asset.TextureColor}
		v.revision = info.Document.Revision
		v.shownFrame = frame
		v.shownIsolation = info.Isolation
		v.shownChannel = s.Channel
		v.shownOnion = v.onion
		v.refresh = false
	}
	// Separate paint capture keeps Image Viewer's middle/right navigation from
	// interpreting a left stroke as a pan. It still owns texture presentation.
	var pivot *[2]float32
	if point, ok := info.Document.FramePivot(frame); ok {
		value := [2]float32{float32(point.X) / float32(info.Document.Canvas.Width), float32(point.Y) / float32(info.Document.Canvas.Height)}
		pivot = &value
	}
	v.canvasStatus = drawImageCanvas(p.ctx, r, v.pixels, &v.display, &v.viewport, imageRGBA, v.canvasBackground, v.grid, [4]int{}, pivot)
	if v.tiled && v.pixels != nil {
		region := v.display.region(p.ctx, v.pixels, imageRGBA)
		if !region.IsZero() {
			center := v.viewport.rect(r, v.pixels.data.Width, v.pixels.data.Height)
			ui.PushClip(r)
			for y := -1; y <= 1; y++ {
				for x := -1; x <= 1; x++ {
					if x != 0 || y != 0 {
						tile := center
						tile.X += float32(x) * center.W
						tile.Y += float32(y) * center.H
						ui.List().Texture(region, tile, mathx.White)
					}
				}
			}
			ui.Outline(center, ui.Theme.Accent)
			ui.PopClip()
		}
	}
	if len(v.construction) != 0 {
		if v.tool == artCurve {
			v.canvasStatus = i18n.Tr("editor.art.curve_hint", "Curve: click start, end, then two control points. Esc cancels.")
		} else {
			v.canvasStatus = i18n.Tr("editor.art.polygon_hint", "Polygon: click vertices, then Enter or the first vertex to finish. Esc cancels.")
		}
	}
	p.drawSelection(ui, r, point)
}
