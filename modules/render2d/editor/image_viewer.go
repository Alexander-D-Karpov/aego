package editor

import (
	"fmt"
	"image"
	"path"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/engine/i18n"
	"aego/ui/core"
	"aego/ui/widgets"
)

const imageHandlerID = "render2d.image"

type imagesEditor struct {
	ctx         *context.Editor
	reg         *ext.Registry
	states      map[document.ID]*imageState // GPU/view lifetime only; documents belong to Manager.
	choiceStamp uint64
	choiceList  []imageChoice
	choiceDocs  []imageChoiceRevision
}

type imagePanel struct {
	owner *imagesEditor
	doc   *document.AssetDocument
}

func imageHooks(r *ext.Registry) error {
	ed := r.Editor()
	if ed == nil || ed.Docs == nil {
		return nil
	}
	ed.ImageContent = func(id ids.AssetID, mode string) (image.Image, string, error) { return contentPixels(ed, id, mode) }
	s := &imagesEditor{ctx: ed, reg: r, states: map[document.ID]*imageState{}}
	open := r.DocumentPanelFactory(func(d *document.AssetDocument) workspace.Panel {
		s.state(d).closed = false
		return &imagePanel{owner: s, doc: d}
	})
	if err := r.DocumentHandler(document.Handler{ID: imageHandlerID,
		Name: i18n.T("editor.image.document", "Image / Sprite"), Icon: "file.texture",
		Kinds: []asset.ArtifactKind{asset.ArtTexture, asset.ArtSpriteRegion, asset.ArtAtlasPage}, Extensions: []string{".png"},
		Capabilities: document.View | document.EditMetadata | document.Preview | document.RevealSource, Open: open}); err != nil {
		return err
	}
	for _, kind := range []asset.ArtifactKind{asset.ArtTexture, asset.ArtSpriteRegion, asset.ArtAtlasPage} {
		if err := r.AssetInspector(string(kind), s.inspect); err != nil {
			return err
		}
	}
	r.Tick(s.tick)
	return artHooks(r)
}

func (s *imagesEditor) state(d *document.AssetDocument) *imageState {
	v, _ := d.View.(*imageState)
	if v == nil || v.closed {
		v = newImageState()
		d.View = v
	}
	s.states[d.ID] = v
	v.refresh(s.ctx, d)
	return v
}

func imagePanelID(d *document.AssetDocument) string     { return imageHandlerID + "/" + d.Asset.String() }
func (p *imagePanel) ID() string                        { return imagePanelID(p.doc) }
func (p *imagePanel) Title() string                     { return path.Base(p.doc.Path) }
func (p *imagePanel) Dirty() bool                       { return p.doc.Dirty() }
func (p *imagePanel) Icon() string                      { return "file.texture" }
func (p *imagePanel) Scope() string                     { return imageHandlerID }
func (p *imagePanel) Document() *document.AssetDocument { return p.doc }

func (s *imagesEditor) tick() {
	live := map[document.ID]*document.AssetDocument{}
	for _, d := range s.ctx.Docs.AssetDocuments() {
		if d.Handler == imageHandlerID {
			live[d.ID] = d
		}
	}
	for id, v := range s.states {
		d := live[id]
		if d == nil {
			v.close(s.ctx)
			delete(s.states, id)
			continue
		}
		v.poll(s.ctx, d)
		if v.closeAfterSave && !v.pendingSave && !d.Dirty() {
			(&imagePanel{owner: s, doc: d}).close(false)
			continue
		}
		if !v.viewSeen || !v.showPreview {
			v.preview.close(s.ctx)
		}
		if !v.viewSeen {
			v.display.close(s.ctx)
		}
		v.seen, v.viewSeen = false, false
	}
}

func (p *imagePanel) RequestClose() {
	s, d := p.owner, p.doc
	if !d.Dirty() {
		p.close(false)
		return
	}
	s.ctx.Ask(context.Question{Title: i18n.Tr("editor.image.close_title", "Unsaved import settings"),
		Message: i18n.Tr("editor.image.close_message", "Save the metadata changes to {path} before closing?", i18n.Str("path", d.Path)),
		Buttons: []string{i18n.Tr("editor.image.save", "Save settings"), i18n.Tr("editor.image.discard", "Discard changes"), i18n.Tr("editor.image.cancel", "Cancel")},
		Then: func(choice int) {
			switch choice {
			case 0:
				v := s.state(d)
				v.closeAfterSave = true
				// The ordinary Save command validates/imports the source. Keep the
				// tab visible if validation is asynchronous or a conflict remains.
				v.save(s.ctx, d)
				if !d.Dirty() {
					p.close(false)
				}
			case 1:
				p.close(true)
			}
		}})
}

func (p *imagePanel) close(force bool) {
	s, d := p.owner, p.doc
	if err := s.ctx.Docs.CloseAsset(d.Asset, force); err != nil {
		s.ctx.Notify(context.LevelError, err.Error())
		return
	}
	if v, ok := d.View.(*imageState); ok {
		v.close(s.ctx)
	}
	delete(s.states, d.ID)
	s.reg.ClosePanel(p.ID())
}

func (p *imagePanel) Draw(f *services.Frame, area mathx.Rect) {
	s, d, ui := p.owner, p.doc, f.UI
	v := s.state(d)
	v.viewSeen = true
	ui.PushID(p.ID())
	defer ui.PopID()
	ui.PushClip(area)
	defer ui.PopClip()
	ui.Region(area)
	defer ui.PopCursor()
	cur := ui.Cursor()
	row := cur.Next(0)
	segment := row.W / 3
	modes := []string{i18n.Tr("editor.image.source", "Source pixels"), i18n.Tr("editor.image.imported", "Imported pixels")}
	if v.role == asset.RoleHeight {
		modes = append(modes, i18n.Tr("editor.image.generated", "Generated normal candidate"))
	}
	if v.mode >= len(modes) {
		v.mode = 0
	}
	if widgets.Combo(ui, ui.ID("image.mode"), mathx.Rect{X: row.X, Y: row.Y, W: segment - 4, H: row.H}, &v.mode, modes, false) {
		v.viewport.Ready = false
		v.display.close(s.ctx)
	}
	channel := int(v.channel)
	if widgets.Combo(ui, ui.ID("image.channel"), mathx.Rect{X: row.X + segment, Y: row.Y, W: segment - 4, H: row.H}, &channel, imageChannelNames, false) {
		v.channel = imageChannel(channel)
	}
	backgrounds := []string{i18n.Tr("editor.image.checker", "Checkerboard"), i18n.Tr("editor.image.light_background", "Light background"), i18n.Tr("editor.image.dark_background", "Dark background")}
	widgets.Combo(ui, ui.ID("image.background"), mathx.Rect{X: row.X + segment*2, Y: row.Y, W: segment - 4, H: row.H}, &v.background, backgrounds, false)
	row = cur.Next(0)
	fit := widgets.ButtonRect(ui, ui.ID("image.fit"), mathx.Rect{X: row.X, Y: row.Y, W: 52, H: row.H}, i18n.Tr("editor.image.fit", "Fit"))
	one := widgets.ButtonRect(ui, ui.ID("image.one"), mathx.Rect{X: row.X + 58, Y: row.Y, W: 52, H: row.H}, "1:1")
	widgets.ToggleButton(ui, ui.ID("image.grid"), mathx.Rect{X: row.X + 116, Y: row.Y, W: 80, H: row.H}, i18n.Tr("editor.image.grid", "Pixel grid"), &v.grid)
	widgets.ToggleButton(ui, ui.ID("image.preview"), mathx.Rect{X: row.X + 202, Y: row.Y, W: min(125, max(60, row.W-202)), H: row.H}, i18n.Tr("editor.image.surface", "Lit surface"), &v.showPreview)
	if len(v.content.Frames) > 0 {
		s.frameField(d, v)
	}
	widgets.LabelDim(ui, i18n.Tr("editor.image.navigation", "Wheel: zoom under cursor · middle/right drag: pan · pixel values are original bytes"))
	top, bottom := cur.Y, area.Y+area.H-28
	sideWidth := min(float32(300), max(210, area.W*.34))
	canvas := mathx.Rect{X: cur.X, Y: top, W: max(64, cur.W-sideWidth-10), H: max(32, bottom-top)}
	side := mathx.Rect{X: canvas.X + canvas.W + 10, Y: top, W: max(80, cur.W-canvas.W-10), H: canvas.H}
	if area.W < 500 {
		canvas.W = cur.W
		canvas.H *= .5
		side = mathx.Rect{X: cur.X, Y: canvas.Y + canvas.H + 8, W: cur.W, H: max(30, bottom-canvas.Y-canvas.H-8)}
	}
	pixels := v.pixels()
	if pixels != nil {
		if fit {
			v.viewport.fit(canvas, pixels.data.Width, pixels.data.Height)
		}
		if one {
			v.viewport.oneToOne(canvas, pixels.data.Width, pixels.data.Height, ui.Scale())
		}
	}
	var pivot *[2]float32
	if v.meta != nil {
		value := v.meta.EffectivePivot()
		if frame, ok := v.meta.Frame(d.Selected); ok {
			value = frame.EffectivePivot()
		}
		pivot = &value
	}
	status := drawImageCanvas(s.ctx, canvas, pixels, &v.display, &v.viewport, v.channel, v.background, v.grid, v.highlight, pivot)
	if pixels == nil {
		reason := v.content.SourceError
		if v.mode == 1 {
			reason = v.content.ImportedError
		} else if v.mode == 2 {
			reason = i18n.Tr("editor.image.no_candidate", "Prepare a generated normal candidate from the height settings.")
		}
		ui.TextIn(canvas, reason, ui.Theme.Warn, core.AlignCenter)
	}
	ui.Text(status, mathx.Vec2{X: area.X + ui.Theme.Padding, Y: bottom + 5}, ui.Theme.TextDim)
	ui.PushClip(side)
	ui.Region(side)
	defer ui.PopCursor()
	defer ui.PopClip()
	defer v.beginScroll(s.ctx, 0)()
	if v.showPreview {
		previewArea := ui.Cursor().Next(min(240, max(100, side.W-16)))
		var candidate *imagePixels
		if v.role == asset.RoleHeight {
			candidate = v.candidate.good
		}
		v.preview.draw(s.ctx, previewArea, v.surface(s.ctx, d), candidate, v.content.SourceRect)
		v.preview.fields(s.ctx)
	}
	s.metadataControls(d, v)
	s.info(d, v)
}

func (s *imagesEditor) frameField(d *document.AssetDocument, v *imageState) {
	ui := s.ctx.UI
	labels := []string{i18n.Tr("editor.image.whole_sheet", "Whole image")}
	selected := 0
	for i, frame := range v.content.Frames {
		labels = append(labels, frame.Name)
		if frame.ID == d.Selected {
			selected = i + 1
		}
	}
	if widgets.Combo(ui, ui.ID("image.frame"), widgets.Field(ui, i18n.Tr("editor.image.frame", "Source frame")), &selected, labels, false) {
		d.Selected, d.Subasset = d.Asset, ""
		if selected > 0 {
			frame := v.content.Frames[selected-1]
			d.Selected, d.Subasset = frame.ID, frame.Name
		}
		v.refresh(s.ctx, d)
	}
}

type imageInspectorMode uint8

const (
	imageInspectAuto imageInspectorMode = iota
	imageInspectChannels
	imageInspectSlicing
)

func (s *imagesEditor) inspect(ed *context.Editor, id ids.AssetID) {
	d, err := ed.DocumentForAsset(id)
	if err != nil {
		widgets.LabelWrapped(ed.UI, err.Error(), true)
		return
	}
	v := s.state(d)
	ui := ed.UI
	ui.PushID("image.inspector:" + d.Asset.String())
	defer ui.PopID()
	// Only the editable source sheet may be sliced. Atlas pages and individual
	// frame selections retain their own channel/region inspector.
	canSlice := ed.DrawSpriteSlicer != nil && d.Asset == id && d.Capabilities.Has(document.EditMetadata) && d.Metadata != nil && !d.Metadata.Conflict && !d.Metadata.ReadOnly && v.meta != nil && v.metaError == nil && v.content.Source != nil
	slicing := canSlice && (v.inspectorMode == imageInspectSlicing || v.inspectorMode == imageInspectAuto && len(v.meta.Subs) != 0)
	if canSlice {
		row := ui.Cursor().Next(0)
		label := i18n.Tr("editor.shell.slicing", "Slicing")
		heading := path.Base(d.Path)
		if slicing {
			label = i18n.Tr("editor.image.channels", "Surface channels and import settings")
			if ui.TextSize(label).X+24 > row.W*.55 {
				label = i18n.Tr("editor.shell.import", "Import")
			}
			heading = fmt.Sprintf("%s  %d×%d", heading, v.content.Source.data.Width, v.content.Source.data.Height)
		}
		w := min(row.W*.55, ui.TextSize(label).X+24)
		left := mathx.Rect{X: row.X, Y: row.Y, W: max(0, row.W-w-4), H: row.H}
		ui.PushClip(left)
		ui.Text(heading, mathx.Vec2{X: left.X, Y: ui.TextY(left)}, ui.Theme.Text)
		ui.PopClip()
		if widgets.ButtonRect(ui, ui.ID("image.inspect.mode"), mathx.Rect{X: row.X + row.W - w, Y: row.Y, W: w, H: row.H}, label) {
			if slicing {
				v.inspectorMode = imageInspectChannels
			} else {
				v.inspectorMode = imageInspectSlicing
			}
			slicing = !slicing
		}
	} else {
		widgets.Label(ui, path.Base(d.Path))
	}
	if slicing {
		defer v.beginScroll(ed, 2)()
		ed.DrawSpriteSlicer(id, v.meta)
		s.metadataActions(d, v)
		if v.message != "" {
			widgets.LabelWrapped(ui, v.message, true)
		}
		return
	}
	defer v.beginScroll(ed, 1)()
	if widgets.Button(ui, i18n.Tr("editor.image.open", "Open Image / Sprite")) {
		_, err = ed.OpenAsset(id)
		if err != nil {
			v.message = err.Error()
		}
	}
	if len(v.content.Frames) > 0 {
		s.frameField(d, v)
	}
	s.metadataControls(d, v)
	s.info(d, v)
}

func (s *imagesEditor) info(d *document.AssetDocument, v *imageState) {
	ui := s.ctx.UI
	widgets.LabelDim(ui, d.Path)
	widgets.LabelDim(ui, "AssetID: "+d.Selected.String())
	if v.content.Source != nil {
		widgets.LabelDim(ui, fmt.Sprintf("%d × %d · %s", v.content.Source.data.Width, v.content.Source.data.Height, v.role))
	}
	if v.meta != nil {
		srgb, err := asset.TextureSRGB(v.role, v.meta.Settings)
		space := "linear"
		if srgb {
			space = "sRGB"
		}
		if err == nil {
			widgets.LabelDim(ui, i18n.Tr("editor.image.colorspace_info", "Imported RGB: {space} · alpha remains linear", i18n.Str("space", space)))
		}
		pivot := v.meta.EffectivePivot()
		if f, found := v.meta.Frame(d.Selected); found {
			pivot = f.EffectivePivot()
		}
		widgets.LabelWrapped(ui, i18n.Tr("editor.image.pivot_info", "Source/frame pivot: {x}, {y}. The surface preview uses a local center origin; node origins are unchanged.", i18n.Str("x", fmt.Sprintf("%.3g", pivot[0])), i18n.Str("y", fmt.Sprintf("%.3g", pivot[1]))), false)
	}
	if !v.content.Page.IsZero() {
		r := v.content.Region
		logicalW, logicalH := r.SourceW, r.SourceH
		if logicalW == 0 || logicalH == 0 {
			logicalW, logicalH = r.W, r.H
		}
		widgets.LabelWrapped(ui, i18n.Tr("editor.image.region_info", "Atlas pixels: {rect} · logical {size} · trim {trim}", i18n.Str("rect", fmt.Sprintf("%d,%d %d×%d", r.X, r.Y, r.W, r.H)), i18n.Str("size", fmt.Sprintf("%d×%d", logicalW, logicalH)), i18n.Str("trim", fmt.Sprintf("%d,%d", r.TrimX, r.TrimY))), false)
		if v.content.Page != d.Asset && widgets.Button(ui, i18n.Tr("editor.image.open_atlas", "Open Atlas · highlight region")) {
			page, err := s.ctx.OpenAsset(v.content.Page)
			if err == nil {
				pv := s.state(page)
				pv.mode, pv.highlight, pv.viewport.Ready = 1, imageRegionRect(v.content.Region), false
			} else {
				v.message = err.Error()
			}
		}
	}
	if d.Note != "" {
		widgets.LabelWrapped(ui, d.Note, true)
	}
	if v.content.ImportedError != "" {
		widgets.LabelWrapped(ui, v.content.ImportedError, true)
	} else if v.content.Imported != nil {
		widgets.LabelDim(ui, i18n.Tr("editor.image.import_committed", "Imported view: committed runtime artifact"))
	}
	if s.ctx.Pipeline != nil {
		for _, info := range s.ctx.Pipeline.Assets() {
			if info.ID == d.Asset && info.Error != "" {
				widgets.LabelWrapped(ui, i18n.Tr("editor.image.import_failed", "Current import failed; any imported image shown is last-good: {error}", i18n.Str("error", info.Error)), true)
				break
			}
		}
	}
	for _, group := range []struct {
		label string
		list  []ids.AssetID
	}{{i18n.Tr("editor.image.dependencies", "Dependencies"), v.content.Dependencies}, {i18n.Tr("editor.image.consumers", "Consumers and materials"), v.content.Consumers}} {
		if len(group.list) == 0 {
			continue
		}
		widgets.Label(ui, group.label)
		ui.PushID(group.label)
		for _, id := range group.list {
			label := id.Short()
			if s.ctx.Loader != nil && s.ctx.Loader.Index() != nil {
				if entry, found := s.ctx.Loader.Index().Lookup(id); found {
					label = entry.Name
				}
			}
			if widgets.Button(ui, label) {
				if _, err := s.ctx.OpenAsset(id); err != nil {
					v.message = err.Error()
				}
			}
		}
		ui.PopID()
	}
	if s.ctx.RevealPath != nil && d.Capabilities.Has(document.RevealSource) && widgets.Button(ui, i18n.Tr("editor.document.reveal_source", "Reveal Source")) {
		s.ctx.RevealPath(d.Path)
	}
}

func (v *imageState) beginScroll(ed *context.Editor, slot int) func() {
	ui := ed.UI
	clip, ok := ui.ClipRect()
	if !ok {
		return func() {}
	}
	cur := ui.Cursor()
	top, width := cur.Y, cur.W
	r := mathx.Rect{X: cur.X, Y: top, W: width, H: max(0, clip.Y+clip.H-top)}
	ui.PushClip(r)
	if ui.MouseOver(ui.ID("image.details.scroll"), r) {
		v.scroll[slot] -= ui.ScrollY(ui.Theme.RowHeight * 3)
	}
	v.scroll[slot] = mathx.Clamp(v.scroll[slot], 0, max(0, v.scrollHeight[slot]-r.H))
	cur.Y -= v.scroll[slot]
	if v.scrollHeight[slot] > r.H {
		cur.W -= 8
	}
	return func() {
		v.scrollHeight[slot] = cur.Y + v.scroll[slot] - top
		cur.W = width
		widgets.ScrollBar(ui, ui.ID("image.details.scrollbar"), mathx.Rect{X: r.X + r.W - 6, Y: r.Y, W: 6, H: r.H}, &v.scroll[slot], v.scrollHeight[slot], r.H, &v.scrollBar[slot])
		ui.PopClip()
	}
}
