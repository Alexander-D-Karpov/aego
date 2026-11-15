package editor

import (
	"bytes"
	stdctx "context"
	"errors"
	"fmt"
	"image"
	"io"
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
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/engine/i18n"
	"aego/ui/widgets"
)

const artHandlerID = "render2d.art"
const artPNGHandlerID = "render2d.art.png"

const (
	artRectSelect = 7 + iota
	artWand
	artEyedropper
	artEllipseSelect
	artLasso
	artCurve
	artPolygon
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
	viewport                                                            imageViewport
	display                                                             imageDisplay
	pixels                                                              *imagePixels
	revision, device                                                    uint64
	shownFrame                                                          art.ID
	shownIsolation                                                      art.ID
	shownChannel                                                        art.Channel
	shownOnion                                                          bool
	refresh                                                             bool
	tool, pane, surface, framePage, layerPage, palettePage, brushSource int
	size, dither, eraserStrength                                        float64
	eraserMode                                                          int
	color, background                                                   mathx.Color
	recentColors                                                        []art.Color
	pickerHue, pickerSaturation                                         float32
	pickerLast                                                          art.Color
	pickerReady                                                         bool
	pickOnce                                                            bool
	pickReturnTool                                                      int
	palette                                                             int
	backgroundPalette                                                   int
	perfect, round, filled, mirrorX, mirrorY, grid, onion, playing      bool
	playStart                                                           float64
	tag                                                                 art.ID
	tagPage                                                             int
	tagName                                                             string
	duration                                                            string
	durationFrame                                                       art.ID
	durationMS                                                          int
	selecting                                                           bool
	panDragging                                                         bool
	selectionStart                                                      image.Point
	canvasStatus                                                        string
	selectionTool                                                       int
	selectionRevision                                                   uint64
	selectionLayer, selectionFrame                                      art.ID
	selectionChannel                                                    art.Channel
	selectionPoints                                                     []image.Point
	outlineSelection                                                    *art.Selection
	outline                                                             []artSelectionEdge
	outlineSimplified                                                   bool
	propertyScroll, propertyContent                                     float32
	propertyBar                                                         widgets.ScrollState
	partScroll                                                          map[string]*artPartScroll
	toolSample                                                          artToolSample
	toolScroll, toolContent                                             float32
	toolBar                                                             widgets.ScrollState
	timelineScroll                                                      float32
	timelinePanelScroll                                                 float32
	timelineLayer                                                       art.ID
	timelineColumns, timelinePage                                       int
	timelineBar                                                         widgets.ScrollState
	timelinePanelBar                                                    widgets.ScrollState
	timelineCells                                                       []art.CellSlot
	timelineAnchor                                                      art.CellSlot
	thumbnailClock                                                      uint64
	thumbnails                                                          map[artThumbnailKey]*artThumbnail
	construction                                                        []image.Point
	constructionTool                                                    int
	canvasBackground                                                    int
	tiled                                                               bool
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
	drafts := map[document.ID]*pngArtDraft{}
	var blank *blankArtDraft
	var open func(*document.AssetDocument) error
	open = r.DocumentPanelFactory(func(d *document.AssetDocument) workspace.Panel {
		model, _ := d.Binding.Model.(*artedit.Document)
		var err error
		if model == nil {
			model, err = artedit.Bind(ed.Docs, d)
		}
		v := views[d.ID]
		if v == nil {
			v = &artView{size: 1, eraserStrength: 100, perfect: true, grid: true, color: mathx.White, refresh: true}
			views[d.ID] = v
		}
		panel := &artPanel{ctx: ed, reg: r, doc: d, model: model, view: v, err: err, docks: docks}
		if model != nil {
			d.Binding.Actions[document.ActionSaveAs] = func() error { panel.askSaveAs(); return nil }
		}
		if draft := drafts[d.ID]; draft != nil && model != nil && model.Draft {
			d.Binding.BeforeSave = draft.verify
			d.Binding.SaveUnchanged = true
			d.Binding.Save = func() error {
				return draft.askSave(panel, func(next *document.AssetDocument) error {
					views[next.ID] = v
					delete(views, d.ID)
					delete(drafts, d.ID)
					r.ClosePanel(panel.ID())
					return open(next)
				})
			}
		}
		if draft := blank; draft != nil && draft.doc == d && model == draft.model && model.Draft {
			d.Binding.BeforeSave = draft.verify
			d.Binding.SaveUnchanged = true
			d.Binding.Save = func() error {
				return draft.askSave(panel, func(next *document.AssetDocument) error {
					views[next.ID] = v
					delete(views, d.ID)
					blank = nil
					r.ClosePanel(panel.ID())
					if err := ed.Docs.CloseAsset(d.Asset, false); err != nil {
						ed.Notify(context.LevelWarn, err.Error())
					}
					return open(next)
				})
			}
			// A first Save As must adopt the new asset, not leave an unrelated
			// copy while the user's original drawing remains untitled.
			d.Binding.Actions[document.ActionSaveAs] = func() error {
				err := d.Binding.Save()
				if errors.Is(err, document.ErrSavePending) {
					return nil
				}
				return err
			}
		}
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
	if err := r.DocumentHandler(document.Handler{
		ID: artPNGHandlerID, Name: i18n.T("editor.art.document", "Pixel Art / Frame Animation"), Icon: "file.art",
		Kinds: []asset.ArtifactKind{asset.ArtTexture, asset.ArtSpriteRegion}, Extensions: []string{".png"}, Roles: []string{string(asset.RoleColor)},
		SourceExtensionsOnly: true,
		Capabilities:         document.View | document.EditSource | document.EditMetadata | document.Preview | document.RevealSource,
		Open: func(d *document.AssetDocument) error {
			draft := drafts[d.ID]
			if draft == nil {
				var link *artsource.TextureLink
				var err error
				draft, link, err = openPNGArtDraft(ed, d)
				if err != nil {
					return err
				}
				if link != nil {
					return openLinkedArtFrame(ed, link, d.Selected)
				}
				drafts[d.ID] = draft
			}
			if err := draft.selectFrame(d.Selected); err != nil {
				return err
			}
			// Remove only the old presentation, never its shared metadata document.
			r.ClosePanel(imagePanelID(d))
			return open(d)
		},
	}); err != nil {
		return err
	}
	wasArt := false
	r.Tick(func() {
		pollArtSourcePreviews(ed, previewStamps)
		live := map[document.ID]bool{}
		for _, d := range ed.Docs.AssetDocuments() {
			live[d.ID] = true
		}
		for id, v := range views {
			if !live[id] {
				v.close(ed)
				delete(views, id)
			}
		}
		for id := range drafts {
			if !live[id] {
				delete(drafts, id)
			}
		}
		if blank != nil && ed.Docs.AssetDocument(blank.doc.Asset) != blank.doc {
			blank = nil
		}
		preset := r.CurrentWorkspace()
		inArt := preset.ID == workspace.Art || preset.User && preset.Base == workspace.Art
		entered := inArt && !wasArt
		wasArt = inArt
		if !entered || ed.Project == nil || ed.Pipeline == nil {
			return
		}
		// Reuse an existing drawing, especially a dirty one, rather than hiding
		// it behind a newly created blank on workspace switches.
		if active := docks.active; active != nil && active.model != nil && ed.Docs.AssetDocument(active.doc.Asset) == active.doc {
			if err := open(active.doc); err != nil {
				ed.Notify(context.LevelError, err.Error())
			}
			return
		}
		for _, doc := range ed.Docs.AssetDocuments() {
			if _, ok := doc.Binding.Model.(*artedit.Document); ok {
				if err := open(doc); err != nil {
					ed.Notify(context.LevelError, err.Error())
				}
				return
			}
		}
		var err error
		blank, err = newWorkspaceBlank(ed)
		if err == nil {
			err = open(blank.doc)
		}
		if err != nil {
			ed.Notify(context.LevelError, err.Error())
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
	if errors.Is(err, document.ErrSavePending) {
		return
	}
	p.err = err
	if err != nil {
		p.ctx.Notify(context.LevelError, err.Error())
	}
}
func (p *artPanel) close(force bool) {
	if err := p.ctx.Docs.CloseAsset(p.doc.Asset, force); err != nil {
		p.report(err)
		return
	}
	p.view.close(p.ctx)
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
