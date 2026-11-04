package shell

import (
	"path"
	"strconv"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/slice"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/scene/mutate"
	"aego/ui/core"
	"aego/ui/widgets"
)

func sliceModes() []string {
	return []string{i18n.Tr("editor.shell.slice.grid", "Grid"), i18n.Tr("editor.shell.slice.alpha", "Alpha")}
}

// sliceState is the slicing setup of one sheet while its inspector is open (D339).
type sliceState struct {
	sheet         ids.AssetID
	assetRevision uint64
	mode          int
	cell          [2]int32
	offset        [2]int32
	padding       [2]int32
	count         int32
	thresh        int32
	w, h          int
	px            []byte
	rects         []slice.Rect
	of            [9]int32
	info          string
	imp           importForm
	base          importForm
}

var filterModes = []string{"nearest", "linear"}

// importForm is the Import section of a texture before Apply (D339).
type importForm struct {
	filter  int
	srgb    bool
	mips    bool
	atlas   string
	padding int32
	extrude bool
}

func formOf(s pipeline.Settings) importForm {
	f := importForm{srgb: s.Bool("srgb", true), mips: s.Bool("mips", false), atlas: s.String("atlas", ""), padding: int32(s.Int("padding", 2)), extrude: s.Bool("extrude", true)}
	if s.String("filter", "nearest") == "linear" {
		f.filter = 1
	}
	return f
}

func (f importForm) settings(base pipeline.Settings) pipeline.Settings {
	s := base.Clone()
	if s == nil {
		s = pipeline.Settings{}
	}
	s["filter"] = aetext.Ident(filterModes[f.filter])
	s["srgb"] = aetext.Bool(f.srgb)
	s["mips"] = aetext.Bool(f.mips)
	s["atlas"] = aetext.String(f.atlas)
	s["padding"] = aetext.Int(int64(max(f.padding, 0)))
	s["extrude"] = aetext.Bool(f.extrude)
	return s
}

func (a *App) registerSlicer() {
	if a.ctx.AssetViews == nil {
		a.ctx.AssetViews = map[string]context.AssetView{}
	}
	// The image document provides Channels and the real pixel/lighting viewer.
	// Its source Inspector can also delegate to this original slicing body.
	a.ctx.DrawSpriteSlicer = a.drawSliceBody
	if _, dup := a.ctx.AssetViews[string(asset.ArtTexture)]; !dup {
		a.ctx.AssetViews[string(asset.ArtTexture)] = a.sliceView
	}
}

func (a *App) sliceSetup(id ids.AssetID, rel string, m *asset.SourceMeta) *sliceState {
	st := a.slicer
	if st != nil && st.sheet == id && st.assetRevision == a.ctx.AssetRev {
		// Undo/Redo or another view may change the shared import settings.
		if next := formOf(m.Settings); next != st.base {
			st.base, st.imp = next, next
		}
		return st
	}
	px, w, h, err := slice.Pixels(a.opt.Pipeline.Abs(rel))
	st = &sliceState{sheet: id, assetRevision: a.ctx.AssetRev, px: px, w: w, h: h, thresh: 8, of: [9]int32{-1}}
	if err != nil {
		st.info = err.Error()
	}
	side := int32(min(w, h))
	st.cell = [2]int32{side, side}
	st.imp = formOf(m.Settings)
	st.base = st.imp
	if len(m.Subs) > 0 {
		r := m.Subs[0].Rect
		st.cell = [2]int32{int32(r[2]), int32(r[3])}
		st.offset = [2]int32{int32(r[0]), int32(r[1])}
	}
	a.slicer = st
	return st
}

func (st *sliceState) plan() []slice.Rect {
	key := [9]int32{int32(st.mode), st.cell[0], st.cell[1], st.offset[0], st.offset[1], st.padding[0], st.padding[1], st.count, st.thresh}
	if key == st.of {
		return st.rects
	}
	st.of = key
	if st.mode == 1 {
		st.rects = slice.Alpha(st.px, st.w, st.h, uint8(mathx.Clamp(st.thresh, 0, 254)))
	} else {
		st.rects = slice.Grid(st.w, st.h, [2]int{int(st.cell[0]), int(st.cell[1])}, [2]int{int(st.offset[0]), int(st.offset[1])}, [2]int{int(st.padding[0]), int(st.padding[1])}, int(st.count))
	}
	return st.rects
}

// sliceView is the inspector of a texture: the sheet with its cells, and the slicing setup.
func (a *App) sliceView(ctx *context.Editor, id ids.AssetID) {
	d, err := a.sliceDocument(id)
	if err != nil {
		widgets.LabelWrapped(ctx.UI, err.Error(), true)
		return
	}
	meta, err := asset.ParseSourceMeta(d.Metadata.Code.Bytes(), d.Metadata.Path)
	if err != nil {
		widgets.LabelWrapped(ctx.UI, err.Error(), true)
		return
	}
	st := a.sliceSetup(id, d.Path, meta)
	widgets.Label(ctx.UI, path.Base(d.Path)+"  "+strconv.Itoa(st.w)+"×"+strconv.Itoa(st.h))
	a.drawSliceBody(id, meta)
}

func (a *App) drawSliceBody(id ids.AssetID, meta *asset.SourceMeta) {
	ui := a.ctx.UI
	rel, ok := a.opt.Pipeline.PathOf(id)
	if !ok {
		return
	}
	st := a.sliceSetup(id, rel, meta)
	if st.info != "" {
		widgets.LabelWrapped(ui, st.info, true)
		return
	}
	a.importSection(ui, id, st, meta.Settings)
	widgets.HeaderIcon(ui, "anim.slice", i18n.Tr("editor.shell.slicing", "Slicing"), false)
	rects := st.plan()
	box := ui.Cursor().Next(min(ui.Cursor().W, 260))
	a.drawSheet(ui, id, box, st, meta.Subs, rects)
	widgets.Combo(ui, ui.ID("slicemode"), widgets.Field(ui, i18n.Tr("editor.shell.slice_by", "Slice by")), &st.mode, sliceModes(), false)
	if st.mode == 0 {
		widgets.DragInt(ui, ui.ID("cw"), i18n.Tr("editor.shell.cell_width", "Cell width"), &st.cell[0])
		widgets.DragInt(ui, ui.ID("ch"), i18n.Tr("editor.shell.cell_height", "Cell height"), &st.cell[1])
		widgets.DragInt(ui, ui.ID("ox"), i18n.Tr("editor.shell.offset_x", "Offset X"), &st.offset[0])
		widgets.DragInt(ui, ui.ID("oy"), i18n.Tr("editor.shell.offset_y", "Offset Y"), &st.offset[1])
		widgets.DragInt(ui, ui.ID("px"), i18n.Tr("editor.shell.padding_x", "Padding X"), &st.padding[0])
		widgets.DragInt(ui, ui.ID("py"), i18n.Tr("editor.shell.padding_y", "Padding Y"), &st.padding[1])
		widgets.DragInt(ui, ui.ID("count"), i18n.Tr("editor.shell.count_0_all", "Count (0 = all)"), &st.count)
	} else {
		widgets.DragInt(ui, ui.ID("alpha"), i18n.Tr("editor.shell.alpha_above", "Alpha above"), &st.thresh)
	}
	widgets.LabelDim(ui, i18n.Tr("editor.shell.rects_cell_s_subs_in", "{rects} cell(s); {subs} in the meta", i18n.Str("rects", strconv.Itoa(len(rects))), i18n.Str("subs", strconv.Itoa(len(meta.Subs)))))
	if widgets.Button(ui, i18n.Tr("editor.shell.slice", "Slice")) {
		a.applySlice(id, rel, meta.Subs, rects)
	}
	if len(meta.Subs) > 0 && widgets.Button(ui, i18n.Tr("editor.shell.remove_cells", "Remove Cells")) {
		a.applySlice(id, rel, meta.Subs, nil)
	}
}

func (a *App) drawSheet(ui *core.Context, id ids.AssetID, box mathx.Rect, st *sliceState, subs []pipeline.SubAsset, rects []slice.Rect) {
	th := &ui.Theme
	ui.Rect(box, th.Background)
	scale := min(box.W/float32(st.w), box.H/float32(st.h))
	img := mathx.Rect{X: box.X + (box.W-float32(st.w)*scale)/2, Y: box.Y + (box.H-float32(st.h)*scale)/2, W: float32(st.w) * scale, H: float32(st.h) * scale}
	if reg, ok := a.thumbs.ByID(id); ok {
		ui.List().Texture(reg, img, mathx.White)
	}
	ui.PushClip(box)
	at := func(r slice.Rect) mathx.Rect {
		return mathx.Rect{X: img.X + float32(r[0])*scale, Y: img.Y + float32(r[1])*scale, W: float32(r[2]) * scale, H: float32(r[3]) * scale}
	}
	for _, r := range rects {
		ui.Outline(at(r), th.Accent)
	}
	for _, s := range subs {
		cr := at(s.Rect)
		ui.Outline(cr, th.TextDim)
		label := s.Name[strings.LastIndexByte(s.Name, '_')+1:]
		if ui.TextSize(label).X+4 < cr.W && ui.LineHeight() < cr.H {
			ui.Text(label, mathx.Vec2{X: cr.X + 2, Y: cr.Y + 1}, th.Text)
		}
	}
	ui.PopClip()
}

func (a *App) applySlice(id ids.AssetID, rel string, old []pipeline.SubAsset, rects []slice.Rect) {
	base := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	err := a.textureMutation(id, mutate.Tx{Label: "Slice image", Ops: []mutate.Op{mutate.TextureFramesReplace(slice.Plan(base, old, rects))}})
	if err != nil {
		a.ctx.Notify(context.LevelError, err.Error())
		return
	}
	a.ctx.Notify(context.LevelInfo, i18n.Note("editor.shell.sliced_rel_into_rects_cell", "Sliced {rel} into {rects} cell(s)", i18n.Str("rel", path.Base(rel)), i18n.Str("rects", strconv.Itoa(len(rects)))))
}

// importSection stages the legacy form in the same source metadata journal.
func (a *App) importSection(ui *core.Context, id ids.AssetID, st *sliceState, cur pipeline.Settings) {
	widgets.HeaderIcon(ui, "file.download", i18n.Tr("editor.shell.import", "Import"), false)
	f := &st.imp
	widgets.Combo(ui, ui.ID("filter"), widgets.Field(ui, i18n.Tr("editor.shell.filter", "Filter")), &f.filter, filterModes, false)
	widgets.Checkbox(ui, i18n.Tr("editor.shell.srgb", "sRGB"), &f.srgb)
	widgets.Checkbox(ui, i18n.Tr("editor.shell.mipmaps", "Mipmaps"), &f.mips)
	widgets.TextFieldRect(ui, ui.ID("atlas"), widgets.Field(ui, i18n.Tr("editor.shell.atlas_group", "Atlas group")), &f.atlas, true)
	widgets.DragInt(ui, ui.ID("pad"), i18n.Tr("editor.shell.atlas_padding", "Atlas padding"), &f.padding)
	widgets.Checkbox(ui, i18n.Tr("editor.shell.extrude_edges", "Extrude edges"), &f.extrude)
	if *f == formOf(cur) {
		return
	}
	row := ui.Cursor().Next(0)
	half := (row.W - 4) / 2
	if widgets.ButtonRect(ui, ui.ID("impapply"), mathx.Rect{X: row.X, Y: row.Y, W: half, H: row.H}, i18n.Tr("editor.shell.apply", "Apply")) {
		a.applyImport(id, cur, f.settings(cur))
	}
	if widgets.ButtonRect(ui, ui.ID("imprevert"), mathx.Rect{X: row.X + half + 4, Y: row.Y, W: half, H: row.H}, i18n.Tr("editor.shell.revert", "Revert")) {
		*f = formOf(cur)
	}
}

func (a *App) applyImport(id ids.AssetID, before, after pipeline.Settings) {
	var ops []mutate.Op
	// Only the known fields of this legacy form change; hidden channel and
	// future metadata remain in the shared document. The typed mutation model
	// applies the same validation as the Channels Inspector and CLI.
	for _, name := range []string{"filter", "srgb", "mips", "atlas", "padding", "extrude"} {
		if v, ok := after[name]; ok {
			ops = append(ops, mutate.TextureSetting(name, v))
		}
	}
	err := a.textureMutation(id, mutate.Tx{Label: "Texture import settings", Ops: ops})
	if err != nil {
		a.ctx.Notify(context.LevelError, err.Error())
		return
	}
	if a.slicer != nil && a.slicer.sheet == id {
		a.slicer.imp = formOf(after)
	}
}

func (a *App) sliceDocument(id ids.AssetID) (*document.AssetDocument, error) {
	d, err := a.ctx.DocumentForAsset(id)
	if err != nil {
		return nil, err
	}
	// The standalone slicer uses the same source journal when the image module
	// is absent; both entry points share the document if it is installed.
	if d.Metadata == nil {
		d.Metadata, err = a.ctx.Docs.OpenScript(d.Path + ".meta")
		if err != nil {
			return nil, err
		}
		d.Metadata.ExplicitSave = true
	}
	return d, nil
}

func (a *App) textureMutation(id ids.AssetID, tx mutate.Tx) error {
	d, err := a.sliceDocument(id)
	if err != nil {
		return err
	}
	env, err := a.ctx.TextureAuthoringEnv(d.Asset)
	if err != nil {
		return err
	}
	if _, err := document.ApplyTextureMutation(d.Metadata, env, tx, false); err != nil {
		return err
	}
	if d.Handler == "asset.readonly" {
		// Without an image document handler the original standalone Slice/Apply
		// command includes import; there is no image tab with a Save command.
		return a.ctx.SaveAsset(d)
	}
	return nil
}
