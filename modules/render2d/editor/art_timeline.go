package editor

import (
	stdctx "context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"aego/art"
	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

func artFrameIndex(frames []art.Frame, selected art.ID) int {
	for index, frame := range frames {
		if frame.ID == selected {
			return index
		}
	}
	return -1
}

// Navigating frames changes presentation only, never pixels or history. A
// stroke in progress still prevents selection through Session.Select.
func (p *artPanel) stepArtFrame(offset int) {
	s := p.model.Session
	frames := s.Document.Info().Frames
	index := artFrameIndex(frames, s.Frame)
	if index < 0 {
		return
	}
	target := min(max(0, index+offset), len(frames)-1)
	if target != index {
		if err := s.Select("", frames[target].ID, ""); err != nil {
			p.report(err)
			return
		}
	}
	p.view.playing, p.view.refresh = false, true
}

func timelineIndices(info art.Info, slot art.CellSlot) (int, int) {
	li, fi := -1, -1
	for i, l := range info.Layers {
		if l.ID == slot.Layer {
			li = i
			break
		}
	}
	for i, f := range info.Frames {
		if f.ID == slot.Frame {
			fi = i
			break
		}
	}
	return li, fi
}

func (p *artPanel) selectTimelineCell(slot art.CellSlot, extend, toggle bool) {
	s, v := p.model.Session, p.view
	info := s.Document.Info()
	li, fi := timelineIndices(info, slot)
	if li < 0 || fi < 0 || info.Layers[li].Kind == art.Group {
		return
	}
	if err := s.Select(slot.Layer, slot.Frame, slot.Channel); err != nil {
		p.report(err)
		return
	}
	v.playing, v.refresh = false, true
	if extend {
		al, af := timelineIndices(info, v.timelineAnchor)
		if al >= 0 && af >= 0 && v.timelineAnchor.Channel == slot.Channel {
			v.timelineCells = nil
			for l := min(al, li); l <= max(al, li); l++ {
				if info.Layers[l].Kind == art.Group {
					continue
				}
				for f := min(af, fi); f <= max(af, fi); f++ {
					v.timelineCells = append(v.timelineCells, art.CellSlot{Layer: info.Layers[l].ID, Frame: info.Frames[f].ID, Channel: slot.Channel})
				}
			}
			return
		}
	}
	if toggle {
		if index := slices.Index(v.timelineCells, slot); index >= 0 {
			v.timelineCells = slices.Delete(v.timelineCells, index, index+1)
		} else {
			v.timelineCells = append(v.timelineCells, slot)
		}
	} else {
		v.timelineCells = []art.CellSlot{slot}
	}
	v.timelineAnchor = slot
}

func (p *artPanel) timelineSelection() []art.CellSlot {
	info := p.model.Session.Document.Info()
	slots := make([]art.CellSlot, 0, len(p.view.timelineCells))
	for _, slot := range p.view.timelineCells {
		li, fi := timelineIndices(info, slot)
		if li >= 0 && fi >= 0 && info.Layers[li].Kind != art.Group && slot.Channel == p.model.Session.Channel {
			slots = append(slots, slot)
		}
	}
	if len(slots) == 0 {
		s := p.model.Session
		slot := art.CellSlot{Layer: s.Layer, Frame: s.Frame, Channel: s.Channel}
		li, fi := timelineIndices(info, slot)
		if li >= 0 && fi >= 0 && info.Layers[li].Kind != art.Group {
			slots = append(slots, slot)
		}
	}
	return slots
}

func (p *artPanel) askCelTransfer(copyCells, linked bool) {
	slots := p.timelineSelection()
	if len(slots) == 0 {
		return
	}
	title := i18n.Tr("editor.art.move_cels", "Move selected cels…")
	if copyCells {
		title = i18n.Tr("editor.art.copy_cels", "Copy selected cels…")
	}
	if linked {
		title = i18n.Tr("editor.art.link_cels", "Link selected cels…")
	}
	p.askArtValues(title,
		[]string{i18n.Tr("editor.art.frame_offset", "Frame offset"), i18n.Tr("editor.art.layer_offset", "Layer offset"), i18n.Tr("editor.art.overwrite_cels", "Replace occupied destination: yes / no")},
		[]string{"1", "0", "no"}, func(values []string) error {
			fo, err := strconv.Atoi(strings.TrimSpace(values[0]))
			if err != nil {
				return err
			}
			lo, err := strconv.Atoi(strings.TrimSpace(values[1]))
			if err != nil {
				return err
			}
			overwrite := strings.TrimSpace(values[2])
			if overwrite != "yes" && overwrite != "no" {
				return fmt.Errorf("art: replacement must be yes or no")
			}
			_, err = p.model.Execute(stdctx.Background(), art.Operation{Kind: "cel-range", Cells: slots, FrameOffset: fo, LayerOffset: lo, Copy: copyCells, Linked: linked, Overwrite: overwrite == "yes"})
			if err == nil {
				info := p.model.Session.Document.Info()
				p.view.timelineCells = nil
				for _, slot := range slots {
					l, f := timelineIndices(info, slot)
					p.view.timelineCells = append(p.view.timelineCells, art.CellSlot{Layer: info.Layers[l+lo].ID, Frame: info.Frames[f+fo].ID, Channel: slot.Channel})
				}
				if len(p.view.timelineCells) > 0 {
					first := p.view.timelineCells[0]
					p.view.timelineAnchor = first
					err = p.model.Session.Select(first.Layer, first.Frame, first.Channel)
				}
				p.view.refresh = true
			}
			return err
		})
}

func (p *artPanel) timelineGrid(ui *core.Context, area mathx.Rect, editable bool) {
	p.timelineGridWithWheel(ui, area, editable, true)
}

func (p *artPanel) timelineGridWithWheel(ui *core.Context, area mathx.Rect, editable, wheel bool) {
	if area.W < 60 || area.H < ui.Theme.RowHeight {
		return
	}
	s, v := p.model.Session, p.view
	info := s.Document.Info()
	rowH := ui.Theme.RowHeight + ui.Theme.Spacing
	labelW := min(float32(150), max(float32(86), area.W*.24))
	// Always leave a usable cel column, even at the dock's minimum width.
	labelW = min(labelW, max(float32(24), area.W-ui.Theme.ScrollbarW-32))
	columns := min(32, max(1, int((area.W-labelW-ui.Theme.ScrollbarW)/48)))
	visibleRows := max(1, min(len(info.Layers), int((area.H-rowH)/rowH)+2))
	columns = min(columns, max(1, 192/visibleRows))
	pages := (len(info.Frames) + columns - 1) / columns
	if index := artFrameIndex(info.Frames, s.Frame); index >= 0 {
		v.timelinePage = index / columns
	}
	v.timelineColumns = columns
	v.timelinePage = min(v.timelinePage, max(0, pages-1))
	frameStart := v.timelinePage * columns
	count := min(columns, len(info.Frames)-frameStart)
	cellW := (area.W - labelW - ui.Theme.ScrollbarW) / float32(columns)
	ui.Rect(area, ui.Theme.PanelAlt)
	header := mathx.Rect{X: area.X, Y: area.Y, W: labelW - 4, H: ui.Theme.RowHeight}
	p.actionMenu(ui, header, "cel-actions", i18n.Tr("editor.art.cel_actions", "Cels…"), func(item func(string, string, bool, func())) {
		item("move", i18n.Tr("editor.art.move_cels", "Move selected cels…"), editable, func() { p.askCelTransfer(false, false) })
		item("copy", i18n.Tr("editor.art.copy_cels", "Copy selected cels…"), editable, func() { p.askCelTransfer(true, false) })
		item("link", i18n.Tr("editor.art.link_cels", "Link selected cels…"), editable, func() { p.askCelTransfer(true, true) })
	})
	widgets.Tooltip(ui, ui.ID("cel-actions"), i18n.Tr("editor.art.cel_selection_hint", "Click a cel to select it. Shift selects a rectangle; Ctrl/Cmd toggles individual cels. Move, copy or link the selection from this menu."))
	for j := 0; j < count; j++ {
		f := info.Frames[frameStart+j]
		r := mathx.Rect{X: area.X + labelW + float32(j)*cellW, Y: area.Y, W: cellW - 2, H: ui.Theme.RowHeight}
		id := "frame-" + string(f.ID)
		if artTimelineSelectionButton(ui, id, r, fmt.Sprintf("%d · %d", frameStart+j+1, f.DurationMS), s.Frame == f.ID) {
			v.playing = false
			p.report(s.Select("", f.ID, ""))
			v.refresh = true
		}
		widgets.Tooltip(ui, ui.ID(id), fmt.Sprintf("%d · %d ms", frameStart+j+1, f.DurationMS))
		if s.Frame == f.ID {
			ui.Outline(r, ui.Theme.Accent)
		}
	}
	grid := mathx.Rect{X: area.X, Y: area.Y + rowH, W: area.W, H: max(0, area.H-rowH)}
	if wheel && ui.MouseOver(ui.ID("timeline-scroll"), grid) {
		v.timelineScroll -= ui.ScrollY(rowH * 3)
	}
	content := float32(len(info.Layers)) * rowH
	if v.timelineLayer != s.Layer {
		for i, layer := range info.Layers {
			if layer.ID == s.Layer {
				low, high := float32(i)*rowH, float32(i+1)*rowH
				if low < v.timelineScroll {
					v.timelineScroll = low
				}
				if high > v.timelineScroll+grid.H {
					v.timelineScroll = high - grid.H
				}
				break
			}
		}
		v.timelineLayer = s.Layer
	}
	v.timelineScroll = mathx.Clamp(v.timelineScroll, 0, max(0, content-grid.H))
	track := mathx.Rect{X: area.X + area.W - ui.Theme.ScrollbarW, Y: grid.Y, W: ui.Theme.ScrollbarW, H: grid.H}
	widgets.ScrollBar(ui, ui.ID("timeline-scrollbar"), track, &v.timelineScroll, content, grid.H, &v.timelineBar)
	ui.PushClip(grid)
	defer ui.PopClip()
	cels := make(map[art.CellSlot]art.Cel, len(info.Cels))
	links := make(map[art.ID]int)
	for _, cel := range info.Cels {
		if cel.Channel == s.Channel {
			cels[art.CellSlot{Layer: cel.Layer, Frame: cel.Frame, Channel: cel.Channel}] = cel
			links[cel.Image]++
		}
	}
	selected := make(map[art.CellSlot]bool)
	for _, slot := range p.timelineSelection() {
		selected[slot] = true
	}
	for i, layer := range info.Layers {
		y := grid.Y + float32(i)*rowH - v.timelineScroll
		if y+rowH < grid.Y || y >= grid.Y+grid.H {
			continue
		}
		r := mathx.Rect{X: area.X, Y: y, W: labelW - 4, H: ui.Theme.RowHeight}
		if artTimelineSelectionButton(ui, "timeline-layer-"+string(layer.ID), r, layer.Name, layer.ID == s.Layer) {
			p.report(s.Select(layer.ID, "", ""))
			v.refresh = true
		}
		widgets.Tooltip(ui, ui.ID("timeline-layer-"+string(layer.ID)), layer.Name)
		if layer.ID == s.Layer {
			ui.Outline(r, ui.Theme.Accent)
		}
		for j := 0; j < count; j++ {
			frame := info.Frames[frameStart+j]
			slot := art.CellSlot{Layer: layer.ID, Frame: frame.ID, Channel: s.Channel}
			r := mathx.Rect{X: area.X + labelW + float32(j)*cellW, Y: y, W: cellW - 2, H: ui.Theme.RowHeight}
			id := ui.ID("cel-" + string(layer.ID) + "-" + string(frame.ID))
			background := ui.Theme.Widget
			if selected[slot] {
				background = ui.Theme.SelectionDim
			}
			ui.Box(r, ui.Theme.Radius.Field, background, ui.Theme.Border)
			if layer.Kind == art.Group {
				continue
			}
			if cel, ok := cels[slot]; ok {
				size := max(0, min(r.H-4, r.W-20))
				thumb := mathx.Rect{X: r.X + 2, Y: r.Y + 2, W: size, H: size}
				if size > 0 {
					p.thumbnail(ui, thumb, layer.ID, frame.ID)
				}
				tip := i18n.Tr("editor.art.independent_cel", "Independent cel")
				mark := mathx.Rect{X: r.X + r.W - 11, Y: r.Y + r.H/2 - 2, W: 4, H: 4}
				if links[cel.Image] > 1 {
					tip = i18n.Tr("editor.art.linked_cel", "Linked cel: shared pixels")
					ui.Outline(mathx.Rect{X: mark.X - 3, Y: mark.Y - 2, W: 7, H: 5}, ui.Theme.TextDim)
					ui.Outline(mathx.Rect{X: mark.X + 2, Y: mark.Y + 1, W: 7, H: 5}, ui.Theme.TextDim)
				} else {
					ui.Rect(mark, ui.Theme.TextDim)
				}
				widgets.Tooltip(ui, id, tip)
			}
			if ui.Clicked(id, r) {
				p.selectTimelineCell(slot, ui.Mods().Has(input.ModShift), ui.Primary())
			}
			if selected[slot] {
				ui.Outline(r, ui.Theme.Accent)
			}
			if layer.ID == s.Layer && frame.ID == s.Frame {
				ui.Outline(mathx.Rect{X: r.X + 2, Y: r.Y + 2, W: r.W - 4, H: r.H - 4}, ui.Theme.Text)
			}
		}
	}
}

// Retain the same widget IDs and hit rectangles for the inline and docked
// timelines. A selected frame/layer remains visible in either editor theme.
func artTimelineSelectionButton(ui *core.Context, name string, r mathx.Rect, label string, selected bool) bool {
	id := ui.ID(name)
	clicked := ui.Clicked(id, r)
	background := ui.WidgetColor(id)
	if selected {
		background = ui.Theme.Selection
	}
	ui.Box(r, ui.Theme.Radius.Button, background, ui.Theme.Border)
	text := widgets.Elide(ui, label, max(0, r.W-8))
	ui.PushClip(r)
	ui.TextIn(r, text, ui.Theme.Text, core.AlignCenter)
	ui.PopClip()
	if text != label {
		widgets.Tooltip(ui, id, label)
	}
	return clicked
}

func artTimelineTransport(ui *core.Context, name string, r mathx.Rect, icon, label, tip string, active *bool) bool {
	id := ui.ID(name)
	clicked := ui.Clicked(id, r)
	if clicked && active != nil {
		*active = !*active
	}
	background := ui.WidgetColor(id)
	if active != nil && *active {
		background = ui.Theme.Selection
	}
	ui.Box(r, ui.Theme.Radius.Button, background, ui.Theme.Border)
	ui.PushClip(r)
	if label != "" && r.W >= 76 {
		iconRect := mathx.Rect{X: r.X + 4, Y: r.Y, W: r.H, H: r.H}
		widgets.IconIn(ui, iconRect, icon, ui.Theme.Text)
		textRect := mathx.Rect{X: iconRect.X + iconRect.W, Y: r.Y, W: max(0, r.W-iconRect.W-8), H: r.H}
		ui.TextIn(textRect, widgets.Elide(ui, label, max(0, textRect.W-4)), ui.Theme.Text, core.AlignCenter)
	} else {
		widgets.IconIn(ui, r, icon, ui.Theme.Text)
	}
	ui.PopClip()
	widgets.Tooltip(ui, id, tip)
	return clicked
}

func artTimelineDeleteFrame(ui *core.Context, r mathx.Rect, editable, multiple bool) bool {
	tip := i18n.Tr("editor.art.delete_frame", "Delete current frame")
	if editable && multiple {
		return artIconAction(ui, "frame-delete", r, "edit.delete", tip, true)
	}
	id := ui.ID("frame-delete")
	ui.MouseOver(id, r)
	ui.Box(r, ui.Theme.Radius.Button, ui.Theme.Widget, ui.Theme.Border)
	widgets.IconIn(ui, r, "edit.delete", ui.Theme.TextDisabled)
	if !editable {
		tip += " · " + i18n.Tr("editor.art.read_only", "Read-only")
	} else {
		tip += " · " + i18n.Tr("editor.art.last_frame_required", "A drawing must keep at least one frame")
	}
	widgets.Tooltip(ui, id, tip)
	return false
}

func (p *artPanel) timeline(ui *core.Context, r mathx.Rect, editable bool) {
	ui.Region(r)
	defer ui.PopCursor()
	ui.PushClip(r)
	defer ui.PopClip()
	cur, v, s := ui.Cursor(), p.view, p.model.Session
	info := s.Info()
	// Keep every existing control reachable when a dock is made shorter than
	// the toolbar, frame strip, details and a two-row cel grid combined.
	narrow := cur.W < 140
	minimum := 2*ui.Theme.Padding + 6*(ui.Theme.RowHeight+ui.Theme.Spacing) + 2*ui.Theme.Spacing
	if narrow {
		minimum += ui.Theme.RowHeight + ui.Theme.Spacing
	}
	contentHeight := max(r.H, minimum)
	overflow := contentHeight > r.H
	if overflow {
		if ui.MouseOver(ui.ID("timeline-panel-scroll"), r) {
			v.timelinePanelScroll -= ui.ScrollY(ui.Theme.RowHeight * 3)
		}
		v.timelinePanelScroll = mathx.Clamp(v.timelinePanelScroll, 0, contentHeight-r.H)
		cur.W = max(0, cur.W-ui.Theme.ScrollbarW-ui.Theme.Spacing)
		cur.Y -= v.timelinePanelScroll
	} else {
		v.timelinePanelScroll = 0
	}
	fullWidth := cur.W
	toolbar, frames, controls, details := cur.Next(0), cur.Next(0), cur.Next(0), cur.Next(0)
	var narrowActions mathx.Rect
	if narrow {
		narrowActions = cur.Next(0)
	}
	// The timeline can span the screen beside Assets. Keep frequent actions
	// compact instead of stretching four buttons over the entire dock.
	toolbar.W = min(toolbar.W, 400)
	controls.W = min(controls.W, 480)
	details.W = min(details.W, 480)
	// Commit a pending text edit before frame controls can change selection.
	// Otherwise clicking Add or another frame writes the old frame's value to
	// the newly selected frame on the release event.
	for _, frame := range info.Document.Frames {
		if frame.ID == s.Frame {
			if v.durationFrame != frame.ID || v.durationMS != frame.DurationMS {
				v.durationFrame, v.durationMS = frame.ID, frame.DurationMS
				v.duration = fmt.Sprint(frame.DurationMS)
			}
			break
		}
	}
	durationField, tagField, tagActions := artCell(details, 0, 3), artCell(details, 1, 3), artCell(details, 2, 3)
	var deleteFrame mathx.Rect
	if narrow {
		durationField, tagField = artCell(details, 0, 2), artCell(details, 1, 2)
		// Keep the existing four frame controls usable in narrow docks. Place
		// the delete icon beside the tag menu on the existing second row.
		deleteW := min(narrowActions.H, narrowActions.W)
		deleteFrame = mathx.Rect{X: narrowActions.X + narrowActions.W - deleteW, Y: narrowActions.Y, W: deleteW, H: narrowActions.H}
		tagActions = narrowActions
		tagActions.W = max(0, narrowActions.W-deleteW-ui.Theme.Spacing)
	}
	if _, commit := widgets.TextFieldRect(ui, ui.ID("duration"), durationField, &v.duration, false); commit && editable {
		var ms int
		if _, err := fmt.Sscan(v.duration, &ms); err != nil {
			p.report(err)
		} else {
			p.execute(art.Operation{Kind: "duration", Frame: s.Frame, DurationMS: ms})
		}
		info = s.Info()
	}
	widgets.Tooltip(ui, ui.ID("duration"), i18n.Tr("editor.art.frame_duration_hint", "Selected frame duration in milliseconds"))
	row := toolbar
	playIcon, playLabel := "play.start", i18n.Tr("editor.art.play", "Play")
	playTip := playLabel
	if v.playing {
		playIcon, playTip = "play.pause", i18n.Tr("editor.docsview.pause", "Pause")
	}
	if artTimelineTransport(ui, "play", artCell(row, 0, 4), playIcon, playLabel, playTip, &v.playing) {
		v.playStart = ui.Time
		v.refresh = true
	}
	onion := i18n.Tr("editor.art.onion", "Onion skin")
	if artToggle(ui, "onion", artCell(row, 1, 4), onion, &v.onion) {
		v.refresh = true
	}
	widgets.Tooltip(ui, ui.ID("onion"), onion)
	if artTimelineTransport(ui, "frame-prev", artCell(row, 2, 4), "play.step_back", "", i18n.Tr("editor.art.previous_frame", "Previous frame"), nil) {
		p.stepArtFrame(-1)
	}
	if artTimelineTransport(ui, "frame-next", artCell(row, 3, 4), "play.step", "", i18n.Tr("editor.art.next_frame", "Next frame"), nil) {
		p.stepArtFrame(1)
	}
	if fullWidth-toolbar.W >= 100 {
		status := mathx.Rect{X: toolbar.X + toolbar.W + 6, Y: toolbar.Y, W: max(0, fullWidth-toolbar.W-6), H: toolbar.H}
		index := artFrameIndex(info.Document.Frames, s.Frame)
		if index >= 0 {
			text := fmt.Sprintf("%d / %d · %d ms", index+1, len(info.Document.Frames), info.Document.Frames[index].DurationMS)
			ui.PushClip(status)
			ui.TextIn(status, widgets.Elide(ui, text, status.W), ui.Theme.TextDim, core.AlignLeft)
			ui.PopClip()
		}
	}
	row = frames
	perPage := max(1, int(row.W/68))
	pages := (len(info.Document.Frames) + perPage - 1) / perPage
	if index := artFrameIndex(info.Document.Frames, s.Frame); index >= 0 {
		v.framePage = index / perPage
	}
	v.framePage = min(v.framePage, max(0, pages-1))
	for j := 0; j < perPage; j++ {
		index := v.framePage*perPage + j
		if index >= len(info.Document.Frames) {
			break
		}
		frame := info.Document.Frames[index]
		cell := artCell(row, j, perPage)
		label := fmt.Sprintf("%d · %d", index+1, frame.DurationMS)
		if artTimelineSelectionButton(ui, string(frame.ID), cell, label, s.Frame == frame.ID) {
			v.playing = false
			p.report(s.Select("", frame.ID, ""))
			v.refresh = true
		}
		widgets.Tooltip(ui, ui.ID(string(frame.ID)), fmt.Sprintf("%d · %d ms", index+1, frame.DurationMS))
		if frame.ID == s.Frame {
			ui.Outline(cell, ui.Theme.Accent)
		}
	}
	row = controls
	controlCount := 5
	if narrow {
		controlCount = 4
	} else {
		deleteFrame = artCell(row, 4, controlCount)
	}
	if artButton(ui, "frame-add", artCell(row, 0, controlCount), i18n.Tr("editor.art.add_frame", "+ Frame")) && editable {
		p.execute(art.Operation{Kind: "add-frame", DurationMS: 100})
	}
	if artButton(ui, "frame-copy", artCell(row, 1, controlCount), i18n.Tr("editor.art.duplicate", "Duplicate")) && editable {
		p.execute(art.Operation{Kind: "duplicate-frame"})
	}
	if artButton(ui, "frame-link", artCell(row, 2, controlCount), i18n.Tr("editor.art.link", "Link")) && editable {
		p.execute(art.Operation{Kind: "duplicate-frame", Linked: true})
	}
	if artButton(ui, "cel-unlink", artCell(row, 3, controlCount), i18n.Tr("editor.art.unlink", "Unlink")) && editable {
		p.execute(art.Operation{Kind: "unlink"})
	}
	if artTimelineDeleteFrame(ui, deleteFrame, editable, len(info.Document.Frames) > 1) {
		v.playing = false
		p.execute(art.Operation{Kind: "delete-frame"})
	}
	widgets.TextFieldRect(ui, ui.ID("tag-name"), tagField, &v.tagName, false)
	p.timelineActions(ui, tagActions, editable)
	grid := mathx.Rect{X: cur.X, Y: cur.Y, W: cur.W, H: max(0, r.Y+contentHeight-v.timelinePanelScroll-cur.Y-ui.Theme.Padding)}
	p.timelineGridWithWheel(ui, grid, editable, !overflow)
	if overflow {
		track := mathx.Rect{X: r.X + r.W - ui.Theme.ScrollbarW, Y: r.Y, W: ui.Theme.ScrollbarW, H: r.H}
		widgets.ScrollBar(ui, ui.ID("timeline-panel-scrollbar"), track, &v.timelinePanelScroll, contentHeight, r.H, &v.timelinePanelBar)
	}
}
