package editor

import (
	"bytes"
	stdctx "context"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"path"
	"strings"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/engine/i18n"
	"aego/gpu"
	"aego/platform/input"
	"aego/render/texture"
	"aego/ui/core"
	"aego/ui/widgets"
)

const artHandlerID = "render2d.art"

const (
	artRectSelect = 7 + iota
	artWand
	artEyedropper
	artEllipseSelect
	artLasso
)

// The panel owns only presentation. ArtBinding.Model is canonical across tabs,
// Inspector, recovery and confirmed agent operations; pixels never live in View.
type artPanel struct {
	ctx         *context.Editor
	reg         *ext.Registry
	doc         *document.AssetDocument
	model       *artedit.Document
	view        *artView
	err         error
	empty       bool
	blankBefore []byte
	docks       *artDocks
}

type artView struct {
	viewport                                                       imageViewport
	display                                                        imageDisplay
	pixels                                                         *imagePixels
	revision, device                                               uint64
	shownFrame                                                     art.ID
	shownChannel                                                   art.Channel
	shownOnion                                                     bool
	refresh                                                        bool
	tool, pane, surface, framePage, layerPage, palettePage         int
	size                                                           float64
	color                                                          mathx.Color
	palette                                                        int
	perfect, round, filled, mirrorX, mirrorY, grid, onion, playing bool
	playStart                                                      float64
	tag                                                            art.ID
	tagName                                                        string
	duration                                                       string
	durationFrame                                                  art.ID
	selecting                                                      bool
	selectionStart                                                 image.Point
	canvasStatus                                                   string
	selectionTool                                                  int
	selectionRevision                                              uint64
	selectionLayer, selectionFrame                                 art.ID
	selectionChannel                                               art.Channel
	selectionPoints                                                []image.Point
	outlineSelection                                               *art.Selection
	outline                                                        []artSelectionEdge
	outlineSimplified                                              bool
}

func artHooks(r *ext.Registry) error {
	ed := r.Editor()
	if ed == nil || ed.Docs == nil {
		return nil
	}
	if err := registerArtCreate(r); err != nil {
		return err
	}
	if err := registerAsepriteImport(r); err != nil {
		return err
	}
	docks, err := newArtDocks(r)
	if err != nil {
		return err
	}
	views := map[document.ID]*artView{}
	previewStamps := map[document.ID]artPreviewStamp{}
	open := r.DocumentPanelFactory(func(d *document.AssetDocument) workspace.Panel {
		model, err := artedit.Bind(ed.Docs, d)
		v := views[d.ID]
		if v == nil {
			v = &artView{size: 1, perfect: true, grid: true, color: mathx.White, refresh: true}
			views[d.ID] = v
		}
		panel := &artPanel{ctx: ed, reg: r, doc: d, model: model, view: v, err: err, docks: docks}
		if err != nil && ed.Pipeline != nil && d.Capabilities.Has(document.EditSource) {
			if data, readErr := ed.Pipeline.SourceBytes(d.Path); readErr == nil && isBlankArtSource(data) {
				panel.empty = true
				panel.blankBefore = append([]byte{}, data...)
			}
		}
		docks.active = panel
		current := r.CurrentWorkspace()
		if !current.User && (current.ID == workspace.Design || current.ID == workspace.Default) {
			r.SelectWorkspace(workspace.Art)
		}
		return panel
	})
	openSelected := func(d *document.AssetDocument) error {
		// Refuse a frame switch during a gesture before changing presentation.
		if model, ok := d.Binding.Model.(*artedit.Document); ok {
			if err := selectArtSubasset(model, d.Subasset); err != nil {
				return err
			}
		}
		if err := open(d); err != nil {
			return err
		}
		if model, ok := d.Binding.Model.(*artedit.Document); ok {
			return selectArtSubasset(model, d.Subasset)
		}
		return nil
	}
	if err := r.DocumentHandler(document.Handler{
		ID: artHandlerID, Name: i18n.T("editor.art.document", "Pixel Art / Frame Animation"), Icon: "file.art",
		Kinds: []asset.ArtifactKind{asset.ArtTexture, asset.ArtSpriteRegion, asset.ArtAnim}, Extensions: []string{".aeart"},
		SourceExtensionsOnly: true,
		Capabilities:         document.View | document.EditSource | document.EditMetadata | document.Preview | document.RevealSource,
		Open:                 openSelected,
	}); err != nil {
		return err
	}
	r.Tick(func() {
		pollArtSourcePreviews(ed, previewStamps)
		live := map[document.ID]bool{}
		for _, d := range ed.Docs.AssetDocuments() {
			live[d.ID] = true
		}
		for id, v := range views {
			if !live[id] {
				v.display.close(ed)
				delete(views, id)
			}
		}
	})
	return nil
}

func selectArtSubasset(model *artedit.Document, subasset string) error {
	if !strings.HasPrefix(subasset, "frame_") {
		return nil
	}
	for _, frame := range model.Session.Document.Info().Frames {
		for _, channel := range []art.Channel{art.Albedo, art.Normal, art.Height, art.Emission} {
			if artsource.FrameName(frame.ID, channel) == subasset {
				if model.Session.Frame == frame.ID && model.Session.Channel == channel {
					return nil
				}
				return model.Session.Select("", frame.ID, channel)
			}
		}
	}
	return fmt.Errorf("%s", i18n.Tr("editor.art.frame_unavailable", "The selected art frame is no longer available."))
}

func (p *artPanel) ID() string                        { return artHandlerID + "/" + p.doc.Asset.String() }
func (p *artPanel) Title() string                     { return path.Base(p.doc.Path) }
func (p *artPanel) Dirty() bool                       { return p.doc.Dirty() }
func (p *artPanel) Icon() string                      { return "file.art" }
func (p *artPanel) Scope() string                     { return artHandlerID }
func (p *artPanel) Document() *document.AssetDocument { return p.doc }
func (p *artPanel) report(err error) {
	if err != nil {
		p.err = err
		p.ctx.Notify(context.LevelError, err.Error())
	}
}
func (p *artPanel) close(force bool) {
	if err := p.ctx.Docs.CloseAsset(p.doc.Asset, force); err != nil {
		p.report(err)
		return
	}
	p.view.display.close(p.ctx)
	p.reg.ClosePanel(p.ID())
}
func (p *artPanel) RequestClose() {
	if p.model != nil {
		p.model.Session.CancelGesture()
	}
	if !p.doc.Dirty() {
		p.close(false)
		return
	}
	p.ctx.Ask(context.Question{
		Title:   i18n.Tr("editor.art.unsaved", "Unsaved drawing"),
		Message: i18n.Tr("editor.art.close_message", "Save changes to {path}?", i18n.Str("path", p.doc.Path)),
		Buttons: []string{i18n.Tr("editor.art.save", "Save"), i18n.Tr("editor.art.discard", "Discard"), i18n.Tr("editor.art.cancel", "Cancel")},
		Then: func(choice int) {
			switch choice {
			case 0:
				if err := p.ctx.SaveAsset(p.doc); err != nil {
					p.report(err)
					return
				}
				if !p.doc.Dirty() {
					p.close(false)
				}
			case 1:
				p.close(true)
			}
		},
	})
}

func (p *artPanel) execute(op art.Operation) {
	if p.model == nil {
		return
	}
	_, err := p.model.Execute(stdctx.Background(), op)
	p.report(err)
	p.view.refresh = true
}
func artCell(row mathx.Rect, column, count int) mathx.Rect {
	w := (row.W - float32(count-1)*4) / float32(count)
	return mathx.Rect{X: row.X + float32(column)*(w+4), Y: row.Y, W: w, H: row.H}
}
func artButton(ui *core.Context, id string, r mathx.Rect, text string) bool {
	return widgets.ButtonRect(ui, ui.ID(id), r, text)
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
	row := cur.Next(0)
	if artButton(ui, "save", artCell(row, 0, 4), i18n.Tr("editor.art.save", "Save")) && editable {
		p.report(p.ctx.SaveAsset(p.doc))
	}
	if artButton(ui, "undo", artCell(row, 1, 4), i18n.Tr("editor.art.undo", "Undo")) && editable {
		p.report(p.model.SessionUndo())
		v.refresh = true
	}
	if artButton(ui, "redo", artCell(row, 2, 4), i18n.Tr("editor.art.redo", "Redo")) && editable {
		p.report(p.model.SessionRedo())
		v.refresh = true
	}
	if artButton(ui, "fit", artCell(row, 3, 4), i18n.Tr("editor.art.fit", "Fit")) {
		v.viewport.Ready = false
	}
	row = cur.Next(0)
	tools := []widgets.IconOption{
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
	}
	widgets.IconCombo(ui, ui.ID("tool"), artCell(row, 0, 3), &v.tool, tools)
	widgets.DragNumber(ui, ui.ID("brush-size"), artCell(row, 1, 3), &v.size, widgets.NumberOpts{Int: true, HasMin: true, Min: 1, HasMax: true, Max: 64, Speed: .3})
	if info.Document.Canvas.Mode != art.Indexed || s.Channel != art.Albedo {
		widgets.ColorField(ui, ui.ID("paint-color"), artCell(row, 2, 3), &v.color, false)
	} else {
		ui.TextIn(artCell(row, 2, 3), fmt.Sprintf("#%d", v.palette), ui.Theme.Text, core.AlignCenter)
	}
	docked := p.reg.PanelVisible("art.properties") && p.reg.PanelVisible("art.timeline")
	narrow := area.W < 740 && !docked
	if narrow {
		row = cur.Next(0)
		widgets.Combo(ui, ui.ID("pane"), row, &v.pane, []string{i18n.Tr("editor.art.canvas", "Canvas"), i18n.Tr("editor.art.properties", "Tools / Layers"), i18n.Tr("editor.art.timeline", "Timeline")}, false)
	}
	// Navigation and actions never steal keyboard input from a text field.
	if f.Focused && !ui.WantsKeyboard() {
		if ui.KeyPressed(input.KeyEscape) && !ui.AnyPopup() {
			pending := info.Gesture || v.selecting
			s.CancelGesture()
			v.selecting = false
			v.selectionPoints = nil
			if !pending {
				p.report(s.SetSelection("none", nil, 0))
			}
			v.refresh = true
			ui.ReleaseMouse()
		}
		if ui.Primary() && editable {
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
			}
		}
	}
	// Finish a stroke even when the pointer leaves the panel or focus changes.
	if !f.Focused && (info.Gesture || v.selecting) {
		s.CancelGesture()
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
	text := i18n.Tr("editor.art.navigation", "Wheel: zoom · middle/right drag: pan · Esc: cancel gesture or clear selection")
	if v.canvasStatus != "" && (docked || !narrow || v.pane == 0) {
		text = v.canvasStatus + " · " + text
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

func (p *artPanel) askInitializeEmpty() {
	ctx, pl, docs, doc := p.ctx, p.ctx.Pipeline, p.ctx.Docs, p.doc
	before := append([]byte{}, p.blankBefore...)
	message := i18n.Tr("editor.art.initialize_empty_confirm", "Create a drawing in this blank file?")
	if len(before) > 0 {
		message = i18n.Tr("editor.art.initialize_blank_backup", "The original text will be backed up in .aego.")
	}
	ctx.Ask(context.Question{
		Title:   i18n.Tr("editor.art.initialize_empty", "Initialize empty drawing…"),
		Message: message,
		Fields:  []string{i18n.Tr("editor.art.width", "Width"), i18n.Tr("editor.art.height_pixels", "Height"), i18n.Tr("editor.art.color_mode", "Color mode: rgba / grayscale / indexed")},
		Values:  []string{"32", "32", "rgba"},
		Buttons: []string{i18n.Tr("editor.art.create", "Create"), i18n.Tr("editor.art.cancel", "Cancel")},
		Submit: func(values []string) {
			if len(values) != 3 {
				return
			}
			if ctx.Pipeline != pl || ctx.Docs != docs || docs.AssetDocument(doc.Asset) != doc || p.model != nil || !p.empty {
				p.report(fmt.Errorf("art: project or document changed before initialization"))
				return
			}
			if rel, ok := pl.PathOf(doc.Asset); !ok || rel != doc.Path {
				p.report(fmt.Errorf("art: source changed before initialization"))
				return
			}
			current, err := pl.SourceBytes(doc.Path)
			if err != nil {
				p.report(err)
				return
			}
			if current == nil || !bytes.Equal(current, before) {
				p.report(fmt.Errorf("art: source changed before initialization"))
				return
			}
			data, err := blankArtSource(values[0], values[1], values[2])
			if err != nil {
				p.report(err)
				return
			}
			backup, err := backupBlankArt(ctx, doc, before)
			if err != nil {
				p.report(err)
				return
			}
			// Before is non-nil even for a zero-byte file. The transaction
			// rejects a file changed while the dialog or backup was prepared.
			batch, err := pl.ApplySources([]pipeline.SourceEdit{{Path: doc.Path, Before: before, After: data}}, false)
			if err != nil {
				p.report(err)
				return
			}
			ctx.ApplyBatch(batch)
			p.empty = false
			p.model, p.err = artedit.Bind(docs, doc)
			p.view.refresh = true
			if p.err != nil {
				p.report(p.err)
			} else if backup != "" {
				ctx.Notify(context.LevelInfo, i18n.Tr("editor.art.blank_backup_saved", "Original text saved at {path}", i18n.Str("path", backup)))
			}
		},
	})
}

func isBlankArtSource(data []byte) bool {
	return data != nil && len(data) <= 4096 && len(bytes.Trim(data, " \t\r\n")) == 0
}

func backupBlankArt(ctx *context.Editor, doc *document.AssetDocument, data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	if ctx.Project == nil {
		return "", fmt.Errorf("art: cannot back up the original without a project")
	}
	root, err := os.OpenRoot(ctx.Project.Root)
	if err != nil {
		return "", err
	}
	defer root.Close()
	info, err := root.Lstat(".aego")
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("art: cannot safely access the project's .aego directory")
	}
	base := ".aego/art-blank-" + doc.Asset.String()
	for i := 0; i < 1000; i++ {
		name := base + ".original"
		if i > 0 {
			name = fmt.Sprintf("%s-%d.original", base, i+1)
		}
		f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		n, writeErr := f.Write(data)
		if writeErr == nil && n != len(data) {
			writeErr = io.ErrShortWrite
		}
		if writeErr == nil {
			writeErr = f.Sync()
		}
		writeErr = errors.Join(writeErr, f.Close())
		if writeErr != nil {
			return "", writeErr
		}
		return name, nil
	}
	return "", fmt.Errorf("art: no available backup name")
}

func (p *artPanel) canvas(f *services.Frame, r mathx.Rect) {
	if r.W < 1 || r.H < 1 {
		return
	}
	ui, v, s := f.UI, p.view, p.model.Session
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
	id := ui.ID("art.paint")
	pt := v.viewport.imagePoint(r, ui.Mouse())
	point := image.Pt(int(math.Floor(float64(pt.X))), int(math.Floor(float64(pt.Y))))
	brush := art.Brush{Size: int(v.size), Round: v.round, PixelPerfect: v.perfect, MirrorX: v.mirrorX, MirrorY: v.mirrorY,
		Paint:      art.Paint{Color: artColorBytes(v.color)},
		Background: art.Paint{Color: art.Color{A: 0}},
	}
	if info.Document.Canvas.Mode == art.Indexed && s.Channel == art.Albedo {
		brush.Paint.UseIndex = true
		brush.Paint.Index = v.palette
	}
	editable := p.doc.Capabilities.Has(document.EditSource) && !p.model.PendingRecovery
	if !v.playing && (v.tool == artEyedropper || editable) && ui.MouseOver(id, r) && ui.MousePressed(input.MouseLeft) {
		switch v.tool {
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
					if paint.UseIndex {
						v.palette, v.palettePage = paint.Index, paint.Index/8
					}
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
	if s.Info().Gesture {
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
	if v.refresh || v.revision != info.Document.Revision || v.shownFrame != frame || v.shownChannel != s.Channel || v.shownOnion != v.onion {
		pixels, err := s.Preview(frame, v.onion && !v.playing, true)
		if err != nil {
			p.report(err)
			return
		}
		v.pixels = &imagePixels{data: texture.Data{Width: pixels.Rect.Dx(), Height: pixels.Rect.Dy(), Format: gpu.TexSRGBA8, Pixels: pixels.Pix}, semantic: asset.TextureColor}
		v.revision = info.Document.Revision
		v.shownFrame = frame
		v.shownChannel = s.Channel
		v.shownOnion = v.onion
		v.refresh = false
	}
	// Separate paint capture keeps Image Viewer's middle/right navigation from
	// interpreting a left stroke as a pan. It still owns texture presentation.
	v.canvasStatus = drawImageCanvas(p.ctx, r, v.pixels, &v.display, &v.viewport, imageRGBA, 0, v.grid, [4]int{}, nil)
	p.drawSelection(ui, r, point)
}

func (p *artPanel) properties(ui *core.Context, r mathx.Rect, editable bool) {
	ui.Region(r)
	defer ui.PopCursor()
	ui.PushClip(r)
	defer ui.PopClip()
	cur, v, s := ui.Cursor(), p.view, p.model.Session
	info := s.Info()
	row := cur.Next(0)
	widgets.ToggleButton(ui, ui.ID("perfect"), artCell(row, 0, 2), i18n.Tr("editor.art.pixel_perfect", "Pixel perfect"), &v.perfect)
	widgets.ToggleButton(ui, ui.ID("round"), artCell(row, 1, 2), i18n.Tr("editor.art.round", "Round brush"), &v.round)
	row = cur.Next(0)
	widgets.ToggleButton(ui, ui.ID("filled"), artCell(row, 0, 2), i18n.Tr("editor.art.filled", "Filled shapes"), &v.filled)
	widgets.ToggleButton(ui, ui.ID("grid"), artCell(row, 1, 2), i18n.Tr("editor.art.grid", "Pixel grid"), &v.grid)
	row = cur.Next(0)
	widgets.ToggleButton(ui, ui.ID("mirror-x"), artCell(row, 0, 2), i18n.Tr("editor.art.mirror_x", "Mirror X"), &v.mirrorX)
	widgets.ToggleButton(ui, ui.ID("mirror-y"), artCell(row, 1, 2), i18n.Tr("editor.art.mirror_y", "Mirror Y"), &v.mirrorY)
	row = cur.Next(0)
	if widgets.Combo(ui, ui.ID("surface"), row, &v.surface, []string{i18n.Tr("editor.art.albedo", "Albedo"), i18n.Tr("editor.art.normal", "Normals"), i18n.Tr("editor.art.height", "Height"), i18n.Tr("editor.art.emission", "Emission")}, false) {
		p.report(s.Select("", "", []art.Channel{art.Albedo, art.Normal, art.Height, art.Emission}[v.surface]))
		v.refresh = true
	}
	row = cur.Next(0)
	if len(info.Document.Palette) > 0 {
		const count = 8
		pages := (len(info.Document.Palette) + count - 1) / count
		v.palettePage = min(v.palettePage, max(0, pages-1))
		if artButton(ui, "palette-prev", artCell(row, 0, 10), "‹") {
			v.palettePage = max(0, v.palettePage-1)
		}
		for j := 0; j < count; j++ {
			index := v.palettePage*count + j
			if index >= len(info.Document.Palette) {
				break
			}
			sw := info.Document.Palette[index].Color
			cell := artCell(row, j+1, 10)
			widgets.Swatch(ui, cell, mathx.ColorRGBA8(sw.R, sw.G, sw.B, sw.A), false)
			if ui.Pressed(ui.ID("swatch-"+fmt.Sprint(index)), cell) {
				v.palette = index
				v.color = mathx.ColorRGBA8(sw.R, sw.G, sw.B, sw.A)
			}
			if v.palette == index {
				ui.Outline(cell, ui.Theme.Accent)
			}
		}
		if artButton(ui, "palette-next", artCell(row, 9, 10), "›") {
			v.palettePage = min(pages-1, v.palettePage+1)
		}
	}
	row = cur.Next(0)
	if artButton(ui, "add-layer", artCell(row, 0, 3), i18n.Tr("editor.art.add_layer", "+ Layer")) && editable {
		p.execute(art.Operation{Kind: "add-layer", Name: i18n.Tr("editor.art.layer", "Layer"), LayerKind: art.Raster})
	}
	if artButton(ui, "layer-prev", artCell(row, 1, 3), "‹") {
		v.layerPage = max(0, v.layerPage-1)
	}
	if artButton(ui, "layer-next", artCell(row, 2, 3), "›") {
		v.layerPage++
	}
	rows := max(1, int((r.Y+r.H-cur.Y)/(ui.Theme.RowHeight+4)))
	pages := (len(info.Document.Layers) + rows - 1) / rows
	v.layerPage = min(v.layerPage, max(0, pages-1))
	for i := v.layerPage * rows; i < min(len(info.Document.Layers), (v.layerPage+1)*rows); i++ {
		layer := info.Document.Layers[i]
		row = cur.Next(0)
		ui.PushID(string(layer.ID))
		if artButton(ui, "select", mathx.Rect{X: row.X, Y: row.Y, W: row.W - 60, H: row.H}, layer.Name) {
			p.report(s.Select(layer.ID, "", ""))
			v.refresh = true
		}
		if layer.ID == s.Layer {
			ui.Outline(row, ui.Theme.Accent)
		}
		visibleIcon, visibleTip := "ui.hidden", i18n.Tr("editor.art.show_layer", "Show layer")
		if layer.Visible {
			visibleIcon, visibleTip = "anim.preview", i18n.Tr("editor.art.hide_layer", "Hide layer")
		}
		if artIconAction(ui, "visible", mathx.Rect{X: row.X + row.W - 56, Y: row.Y, W: 26, H: row.H}, visibleIcon, visibleTip, editable) {
			layer.Visible = !layer.Visible
			p.execute(art.Operation{Kind: "set-layer", LayerValue: &layer})
		}
		lockIcon, lockTip := "ui.unlock", i18n.Tr("editor.art.lock_layer", "Lock layer")
		if layer.Locked {
			lockIcon, lockTip = "ui.lock", i18n.Tr("editor.art.unlock_layer", "Unlock layer")
		}
		if artIconAction(ui, "locked", mathx.Rect{X: row.X + row.W - 26, Y: row.Y, W: 26, H: row.H}, lockIcon, lockTip, editable) {
			layer.Locked = !layer.Locked
			p.execute(art.Operation{Kind: "set-layer", LayerValue: &layer})
		}
		ui.PopID()
	}
}

func (p *artPanel) timeline(ui *core.Context, r mathx.Rect, editable bool) {
	ui.Region(r)
	defer ui.PopCursor()
	ui.PushClip(r)
	defer ui.PopClip()
	cur, v, s := ui.Cursor(), p.view, p.model.Session
	info := s.Info()
	row := cur.Next(0)
	if widgets.ToggleButton(ui, ui.ID("play"), artCell(row, 0, 4), i18n.Tr("editor.art.play", "Play"), &v.playing) {
		v.playStart = ui.Time
		v.refresh = true
	}
	if widgets.ToggleButton(ui, ui.ID("onion"), artCell(row, 1, 4), i18n.Tr("editor.art.onion", "Onion skin"), &v.onion) {
		v.refresh = true
	}
	if artButton(ui, "frame-prev", artCell(row, 2, 4), "‹") {
		v.framePage = max(0, v.framePage-1)
	}
	if artButton(ui, "frame-next", artCell(row, 3, 4), "›") {
		v.framePage++
	}
	row = cur.Next(0)
	perPage := max(1, int(row.W/68))
	pages := (len(info.Document.Frames) + perPage - 1) / perPage
	v.framePage = min(v.framePage, max(0, pages-1))
	for j := 0; j < perPage; j++ {
		index := v.framePage*perPage + j
		if index >= len(info.Document.Frames) {
			break
		}
		frame := info.Document.Frames[index]
		cell := artCell(row, j, perPage)
		label := fmt.Sprintf("%d · %d", index+1, frame.DurationMS)
		if artButton(ui, string(frame.ID), cell, label) {
			v.playing = false
			p.report(s.Select("", frame.ID, ""))
			v.refresh = true
		}
		if frame.ID == s.Frame {
			ui.Outline(cell, ui.Theme.Accent)
		}
	}
	row = cur.Next(0)
	if artButton(ui, "frame-add", artCell(row, 0, 4), i18n.Tr("editor.art.add_frame", "+ Frame")) && editable {
		p.execute(art.Operation{Kind: "add-frame", DurationMS: 100})
	}
	if artButton(ui, "frame-copy", artCell(row, 1, 4), i18n.Tr("editor.art.duplicate", "Duplicate")) && editable {
		p.execute(art.Operation{Kind: "duplicate-frame"})
	}
	if artButton(ui, "frame-link", artCell(row, 2, 4), i18n.Tr("editor.art.link", "Link")) && editable {
		p.execute(art.Operation{Kind: "duplicate-frame", Linked: true})
	}
	if artButton(ui, "cel-unlink", artCell(row, 3, 4), i18n.Tr("editor.art.unlink", "Unlink")) && editable {
		p.execute(art.Operation{Kind: "unlink"})
	}
	row = cur.Next(0)
	if v.durationFrame != s.Frame {
		v.durationFrame = s.Frame
		for _, f := range info.Document.Frames {
			if f.ID == s.Frame {
				v.duration = fmt.Sprint(f.DurationMS)
			}
		}
	}
	if _, commit := widgets.TextFieldRect(ui, ui.ID("duration"), artCell(row, 0, 3), &v.duration, false); commit && editable {
		var ms int
		if _, err := fmt.Sscan(v.duration, &ms); err != nil {
			p.report(err)
		} else {
			p.execute(art.Operation{Kind: "duration", DurationMS: ms})
		}
	}
	widgets.TextFieldRect(ui, ui.ID("tag-name"), artCell(row, 1, 3), &v.tagName, false)
	if artButton(ui, "tag-add", artCell(row, 2, 3), i18n.Tr("editor.art.tag", "Tag frames")) && editable && len(info.Document.Frames) > 0 {
		p.execute(art.Operation{Kind: "tag", Tag: &art.Tag{Name: v.tagName, From: info.Document.Frames[0].ID, To: info.Document.Frames[len(info.Document.Frames)-1].ID, Direction: art.Forward}})
	}
}
