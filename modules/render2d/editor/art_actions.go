package editor

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"path"
	"strconv"
	"strings"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/ui/core"
	"aego/ui/widgets"
)

// These commands are presentation adapters over the shared Art operations.
// A delayed dialog is tied to the same open document, selection and revision.
func (p *artPanel) askArtValues(title string, fields, values []string, apply func([]string) error) {
	model, doc, docs, pl := p.model, p.doc, p.ctx.Docs, p.ctx.Pipeline
	s := model.Session
	owner := s.Document
	revision, layer, frame, channel, selection := s.Document.Revision(), s.Layer, s.Frame, s.Channel, s.Selection
	p.ctx.Ask(context.Question{Title: title, Fields: fields, Values: values,
		Buttons: []string{i18n.Tr("editor.art.apply", "Apply"), i18n.Tr("editor.art.cancel", "Cancel")},
		Submit: func(v []string) {
			if len(v) != len(fields) {
				return
			}
			if p.ctx.Docs != docs || p.ctx.Pipeline != pl || docs.AssetDocument(doc.Asset) != doc || doc.Binding.Model != model || model.Session != s || s.Document != owner ||
				s.Document.Revision() != revision || s.Layer != layer || s.Frame != frame || s.Channel != channel || s.Selection != selection || s.Info().Gesture {
				p.report(fmt.Errorf("%s", i18n.Tr("editor.art.action_stale", "The document or selection changed. Open the action again.")))
				return
			}
			p.report(apply(v))
			p.view.refresh = true
		},
	})
}

func (p *artPanel) actionMenu(ui *core.Context, r mathx.Rect, id, title string, draw func(func(string, string, bool, func()))) {
	popup := ui.ID(id + "-popup")
	if artButton(ui, id, r, title) {
		ui.OpenPopup(popup, mathx.Vec2{X: r.X, Y: r.Y + r.H})
	}
	if !ui.BeginPopup(popup, 260) {
		return
	}
	draw(func(key, label string, enabled bool, run func()) {
		ui.PushID(key)
		tip := ""
		if key == "merge" {
			tip = i18n.Tr("editor.art.merge_hint", "Merge adjacent visible, unlocked layers of the same kind using Normal blend. Other combinations are rejected; pixels are never silently approximated.")
		}
		if widgets.Entry(ui, widgets.Item{Label: label, Tip: tip, Enabled: enabled, Checked: key == "isolate" && p.model.Session.Info().Isolation != ""}) {
			ui.ClosePopup(popup)
			run()
		}
		ui.PopID()
	})
	ui.EndPopup()
}

func (p *artPanel) authoringActions(ui *core.Context, editable bool) {
	p.authoringActionsFor(ui, editable, artActionsAll)
}

type artActionGroups uint8

const (
	artActionsLayer artActionGroups = 1 << iota
	artActionsPixels
	artActionsPalette
	artActionsDocument
	artActionsAll = artActionsLayer | artActionsPixels | artActionsPalette | artActionsDocument
)

// Independent dock panels expose the same commands against the same document;
// the combined legacy panel retains all four menus in their original order.
func (p *artPanel) authoringActionsFor(ui *core.Context, editable bool, groups artActionGroups) {
	cur, s := ui.Cursor(), p.model.Session
	info := s.Info()
	var layer art.Layer
	for _, v := range info.Document.Layers {
		if v.ID == s.Layer {
			layer = v
			break
		}
	}
	if groups&artActionsLayer != 0 {
		p.actionMenu(ui, cur.Next(0), "layer-actions", i18n.Tr("editor.art.layer_actions", "Layer…"), func(item func(string, string, bool, func())) {
			item("properties", i18n.Tr("editor.art.layer_settings", "Layer properties…"), editable, func() { p.askLayerProperties(layer) })
			item("group", i18n.Tr("editor.art.new_group", "New group"), editable, func() {
				p.execute(art.Operation{Kind: "add-layer", Name: i18n.Tr("editor.art.group", "Group"), LayerKind: art.Group})
			})
			item("reference", i18n.Tr("editor.art.new_reference", "New reference layer"), editable, func() {
				p.execute(art.Operation{Kind: "add-layer", Name: i18n.Tr("editor.art.reference", "Reference"), LayerKind: art.Reference})
			})
			item("duplicate", i18n.Tr("editor.art.duplicate", "Duplicate"), editable && layer.Kind != art.Group, func() { p.execute(art.Operation{Kind: "duplicate-layer"}) })
			item("alpha-lock", i18n.Tr("editor.art.toggle_alpha_lock", "Toggle alpha lock"), editable && layer.Kind != art.Group, func() {
				layer.AlphaLock = !layer.AlphaLock
				p.execute(art.Operation{Kind: "set-layer", LayerValue: &layer})
			})
			item("up", i18n.Tr("editor.art.layer_up", "Move layer up"), editable, func() { p.execute(art.Operation{Kind: "move-layer", Offset: -1}) })
			item("down", i18n.Tr("editor.art.layer_down", "Move layer down"), editable, func() { p.execute(art.Operation{Kind: "move-layer", Offset: 1}) })
			item("delete", i18n.Tr("editor.art.delete_layer", "Delete layer"), editable && len(info.Document.Layers) > 1, func() { p.execute(art.Operation{Kind: "delete-layer"}) })
			item("merge", i18n.Tr("editor.art.merge_down", "Merge down"), editable && layer.Kind != art.Group, func() { p.execute(art.Operation{Kind: "merge-down"}) })
			item("isolate", i18n.Tr("editor.art.isolate", "Isolate selected layer / Show all"), true, func() {
				id := s.Layer
				if s.Info().Isolation == id {
					id = ""
				}
				p.report(s.SetIsolation(id))
				p.view.refresh = true
			})
		})
	}
	if groups&artActionsPixels != 0 {
		p.actionMenu(ui, cur.Next(0), "pixel-actions", i18n.Tr("editor.art.pixel_actions", "Pixels…"), func(item func(string, string, bool, func())) {
			item("all", i18n.Tr("editor.art.select_all", "Select all"), true, func() { p.report(s.SetSelection("all", nil, 0)); p.view.refresh = true })
			item("none", i18n.Tr("editor.art.deselect", "Deselect"), true, func() { p.report(s.SetSelection("none", nil, 0)); p.view.refresh = true })
			item("copy", i18n.Tr("editor.art.copy", "Copy pixels"), true, func() { p.report(s.Copy()) })
			item("paste", i18n.Tr("editor.art.paste", "Paste pixels"), editable && s.Clipboard != nil, func() {
				at := image.Point{}
				if s.Selection != nil {
					at = s.Selection.Bounds().Min
				}
				p.report(s.Paste(at))
				p.view.refresh = true
			})
			for _, v := range []struct{ kind, label string }{
				{"flip-x", i18n.Tr("editor.art.flip_x", "Flip horizontally")}, {"flip-y", i18n.Tr("editor.art.flip_y", "Flip vertically")},
				{"rotate-cw", i18n.Tr("editor.art.rotate_cw", "Rotate 90° clockwise")}, {"rotate-ccw", i18n.Tr("editor.art.rotate_ccw", "Rotate 90° counterclockwise")},
			} {
				item(v.kind, v.label, editable && s.ActiveCel() != "", func() { p.execute(art.Operation{Kind: v.kind}) })
			}
			item("move", i18n.Tr("editor.art.move_pixels", "Move pixels…"), editable && s.ActiveCel() != "", func() { p.askTransform(false) })
			item("scale", i18n.Tr("editor.art.scale_pixels", "Scale pixels…"), editable && s.ActiveCel() != "", func() { p.askTransform(true) })
			item("clear", i18n.Tr("editor.art.clear_pixels", "Clear selected pixels"), editable && s.ActiveCel() != "", func() { p.execute(art.Operation{Kind: "clear"}) })
		})
	}
	if groups&artActionsPalette != 0 {
		p.actionMenu(ui, cur.Next(0), "palette-actions", i18n.Tr("editor.art.palette_actions", "Palette…"), func(item func(string, string, bool, func())) {
			item("add", i18n.Tr("editor.art.palette_add_foreground", "Add foreground to palette"), editable && len(info.Document.Palette) < 256, p.addForegroundSwatch)
			item("swatch", i18n.Tr("editor.art.palette_set_fg", "Set selected swatch to foreground"), editable, func() {
				index := min(max(p.view.palette, 0), len(info.Document.Palette)-1)
				swatch := info.Document.Palette[index]
				swatch.Color = artColorBytes(p.view.color)
				p.execute(art.Operation{Kind: "palette-color", Index: index, Swatch: &swatch})
			})
			item("left", i18n.Tr("editor.art.palette_left", "Move swatch left (preserve pixels)"), editable && p.view.palette > 1, func() { p.moveSwatch(-1) })
			item("right", i18n.Tr("editor.art.palette_right", "Move swatch right (preserve pixels)"), editable && p.view.palette > 0 && p.view.palette < len(info.Document.Palette)-1, func() { p.moveSwatch(1) })
			item("remap", i18n.Tr("editor.art.palette_remap", "Remap palette index…"), editable && info.Document.Canvas.Mode == art.Indexed, p.askPaletteRemap)
			item("extract", i18n.Tr("editor.art.palette_extract", "Extract palette from frame…"), editable, p.askPaletteExtract)
			item("ramp", i18n.Tr("editor.art.ramp_shade", "Shade using palette ramp…"), editable && s.ActiveCel() != "", p.askRampShade)
			item("import", i18n.Tr("editor.art.palette_import", "Import palette…"), editable, p.askPaletteImport)
			for _, v := range []struct {
				mode  art.Mode
				label string
			}{
				{art.RGBA, i18n.Tr("editor.art.mode_rgba", "Convert to RGBA")}, {art.Grayscale, i18n.Tr("editor.art.mode_grayscale", "Convert to grayscale")}, {art.Indexed, i18n.Tr("editor.art.mode_indexed", "Convert to indexed palette")},
			} {
				item(string(v.mode), v.label, editable && info.Document.Canvas.Mode != v.mode, func() { p.execute(art.Operation{Kind: "mode", Mode: v.mode}) })
			}
		})
	}
	if groups&artActionsDocument != 0 {
		p.actionMenu(ui, cur.Next(0), "document-actions", i18n.Tr("editor.art.export_actions", "Save / Export…"), func(item func(string, string, bool, func())) {
			item("copy", i18n.Tr("editor.art.save_copy", "Save editable copy…"), true, func() { p.askExport("aeart") })
			item("aseprite", i18n.Tr("editor.art.export_aseprite", "Export Aseprite interchange copy…"), true, p.askAsepriteExport)
			item("png", i18n.Tr("editor.art.export_png", "Export current frame PNG…"), true, func() { p.askExport("png") })
			item("sheet", i18n.Tr("editor.art.export_sheet", "Export sprite sheet and animation…"), true, func() { p.askExport("sheet") })
			item("palette", i18n.Tr("editor.art.export_palette", "Export palette…"), true, func() { p.askExport("palette") })
		})
	}

}

func (p *artPanel) askLayerProperties(layer art.Layer) {
	groups := p.model.Session.Document.Info().Layers
	parent := ""
	for _, group := range groups {
		if group.ID == layer.Parent {
			parent = group.Name
		}
	}
	p.askArtValues(i18n.Tr("editor.art.layer_settings", "Layer properties…"),
		[]string{i18n.Tr("editor.art.layer_name", "Name"), i18n.Tr("editor.art.opacity", "Opacity (0–255)"), i18n.Tr("editor.art.blend_mode", "Blend: normal / multiply / screen / add"), i18n.Tr("editor.art.parent_group", "Parent group name (empty = root)")},
		[]string{layer.Name, strconv.Itoa(int(layer.Opacity)), string(layer.Blend), parent}, func(values []string) error {
			opacity, err := strconv.ParseUint(strings.TrimSpace(values[1]), 10, 8)
			if err != nil {
				return err
			}
			layer.Name, layer.Opacity, layer.Blend, layer.Parent = values[0], uint8(opacity), art.Blend(strings.TrimSpace(values[2])), ""
			name := strings.TrimSpace(values[3])
			if name != "" {
				for _, group := range groups {
					if group.Kind == art.Group && group.Name == name {
						if layer.Parent != "" {
							return fmt.Errorf("art: more than one group is named %q", name)
						}
						layer.Parent = group.ID
					}
				}
				if layer.Parent == "" {
					return fmt.Errorf("art: group %q does not exist", name)
				}
			}
			_, err = p.model.Execute(stdctx.Background(), art.Operation{Kind: "set-layer", LayerValue: &layer})
			return err
		})
}

func (p *artPanel) askTransform(scale bool) {
	s := p.model.Session
	r := image.Rect(0, 0, s.Document.Info().Canvas.Width, s.Document.Info().Canvas.Height)
	if s.Selection != nil {
		r = s.Selection.Bounds()
	}
	title, fields, values := i18n.Tr("editor.art.move_pixels", "Move pixels…"), []string{"X", "Y"}, []string{"0", "0"}
	if scale {
		title, fields, values = i18n.Tr("editor.art.scale_pixels", "Scale pixels…"), []string{i18n.Tr("editor.art.width", "Width"), i18n.Tr("editor.art.height_pixels", "Height")}, []string{strconv.Itoa(r.Dx()), strconv.Itoa(r.Dy())}
	}
	p.askArtValues(title, fields, values, func(v []string) error {
		x, err := strconv.Atoi(strings.TrimSpace(v[0]))
		if err != nil {
			return err
		}
		y, err := strconv.Atoi(strings.TrimSpace(v[1]))
		if err != nil {
			return err
		}
		op := art.Operation{Kind: "move", Points: []image.Point{image.Pt(x, y)}}
		if scale {
			op = art.Operation{Kind: "scale", Width: x, Height: y}
		}
		_, err = p.model.Execute(stdctx.Background(), op)
		return err
	})
}

func (p *artPanel) moveSwatch(offset int) {
	n, from := len(p.model.Session.Document.Info().Palette), p.view.palette
	to := from + offset
	if from < 0 || from >= n || to < 0 || to >= n {
		return
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	order[from], order[to] = order[to], order[from]
	_, err := p.model.Execute(stdctx.Background(), art.Operation{Kind: "palette-reorder", PaletteOrder: order})
	p.report(err)
	if err == nil {
		p.view.palette, p.view.refresh = to, true
		if p.view.backgroundPalette == from {
			p.view.backgroundPalette = to
		} else if p.view.backgroundPalette == to {
			p.view.backgroundPalette = from
		}
	}
}

func (p *artPanel) askPaletteRemap() {
	p.askArtValues(i18n.Tr("editor.art.palette_remap", "Remap palette index…"), []string{i18n.Tr("editor.art.palette_from", "Source index"), i18n.Tr("editor.art.palette_to", "Destination index")}, []string{strconv.Itoa(p.view.palette), strconv.Itoa(p.view.palette)}, func(v []string) error {
		from, err := strconv.Atoi(strings.TrimSpace(v[0]))
		if err != nil {
			return err
		}
		to, err := strconv.Atoi(strings.TrimSpace(v[1]))
		if err != nil {
			return err
		}
		n := len(p.model.Session.Document.Info().Palette)
		if from < 0 || from >= n || to < 0 || to >= n {
			return fmt.Errorf("art: invalid palette index")
		}
		order := make([]int, n)
		for i := range order {
			order[i] = i
		}
		order[from] = to
		_, err = p.model.Execute(stdctx.Background(), art.Operation{Kind: "palette-remap", PaletteOrder: order})
		return err
	})
}
func (p *artPanel) askPaletteExtract() {
	p.askArtValues(i18n.Tr("editor.art.palette_extract", "Extract palette from frame…"), []string{i18n.Tr("editor.art.color_count", "Maximum colors (2–256)")}, []string{"32"}, func(v []string) error {
		n, err := strconv.Atoi(strings.TrimSpace(v[0]))
		if err != nil {
			return err
		}
		_, err = p.model.Execute(stdctx.Background(), art.Operation{Kind: "palette-extract", ColorLimit: n})
		return err
	})
}
func (p *artPanel) askPaletteImport() {
	p.askArtValues(i18n.Tr("editor.art.palette_import", "Import palette…"), []string{i18n.Tr("editor.art.palette_file", "Palette file (.json or .gpl)")}, []string{""}, func(v []string) error {
		data, err := pipeline.ReadExternalFile(strings.TrimSpace(v[0]), art.MaxPaletteBytes)
		if err != nil {
			return err
		}
		palette, err := art.DecodePalette(data, strings.ToLower(strings.TrimPrefix(path.Ext(v[0]), ".")))
		if err != nil {
			return err
		}
		_, err = p.model.Execute(stdctx.Background(), art.Operation{Kind: "palette-load", Palette: palette})
		return err
	})
}

func (p *artPanel) askRampShade() {
	n := len(p.model.Session.Document.Info().Palette)
	p.askArtValues(i18n.Tr("editor.art.ramp_shade", "Shade using palette ramp…"),
		[]string{i18n.Tr("editor.art.ramp_first", "First palette index"), i18n.Tr("editor.art.ramp_last", "Last palette index"), i18n.Tr("editor.art.ramp_step", "Step (negative = darker)")},
		[]string{"1", strconv.Itoa(n - 1), "1"}, func(v []string) error {
			values := [3]int{}
			for i := range values {
				n, err := strconv.Atoi(strings.TrimSpace(v[i]))
				if err != nil {
					return err
				}
				values[i] = n
			}
			_, err := p.model.Execute(stdctx.Background(), art.Operation{Kind: "ramp-shade", RampFirst: values[0], RampLast: values[1], Offset: values[2]})
			return err
		})
}

func (p *artPanel) askExport(format string) {
	ext := ".png"
	if format == "aeart" {
		ext = ".aeart"
	}
	if format == "palette" {
		ext = ".palette.json"
	}
	dir, stem := artExportLocation(p.doc, "-export")
	target := assetops.UniqueFile(p.ctx, dir, stem, ext)
	p.askArtValues(i18n.Tr("editor.art.export_actions", "Save / Export…"), []string{i18n.Tr("editor.art.new_export_path", "New project path (existing files are not overwritten)")}, []string{target}, func(v []string) error {
		return p.exportTo(strings.TrimSpace(v[0]), format)
	})
}

func artExportLocation(doc *document.AssetDocument, suffix string) (string, string) {
	if doc.Transient {
		return "assets", "drawing" + suffix
	}
	return path.Dir(doc.Path), strings.TrimSuffix(path.Base(doc.Path), path.Ext(doc.Path)) + suffix
}

// Interchange deliberately does not replace the current Aego source. Plain
// Aseprite files do not yet carry its stable identities or all surface data.
func (p *artPanel) askAsepriteExport() {
	model, doc, docs, pl, project := p.model, p.doc, p.ctx.Docs, p.ctx.Pipeline, p.ctx.Project
	if model == nil || docs == nil || pl == nil || model.PendingRecovery {
		p.report(fmt.Errorf("%s", i18n.Tr("editor.art.aseprite_export_unavailable", "Resolve pending recovery and open a project drawing before exporting.")))
		return
	}
	if model.Session.Info().Gesture {
		p.report(fmt.Errorf("art: finish or cancel the gesture before export"))
		return
	}
	revision := model.PreviewRevision()
	snapshot, err := model.Session.Document.Snapshot()
	if err != nil {
		p.report(err)
		return
	}
	data, notes, err := snapshot.EncodeAseprite()
	if err != nil {
		p.report(fmt.Errorf("%s: %w", i18n.Tr("editor.art.aseprite_export_unsupported", "Cannot export this drawing without losing data"), err))
		return
	}
	current := func() bool {
		return p.ctx.Docs == docs && p.ctx.Pipeline == pl && p.ctx.Project == project && docs.AssetDocument(doc.Asset) == doc &&
			doc.Binding.Model == model && !model.PendingRecovery && !model.Session.Info().Gesture && model.PreviewCurrent(revision)
	}
	if !current() {
		p.report(art.ErrConflict)
		return
	}
	message := i18n.Tr("editor.art.aseprite_export_warning", "This is an unindexed interchange copy, not a replacement for your source. Aego identities and asset links are not carried into the copy. Your original and its links remain unchanged. Verify the copy in Aseprite before using it.")
	for _, note := range notes {
		// The identity warning above is localized and deliberately prominent.
		if note.Code != "aego-identities" {
			message += "\n\n" + note.Message
		}
	}
	p.ctx.Ask(context.Question{
		Title: i18n.Tr("editor.art.export_aseprite", "Export Aseprite interchange copy…"), Message: message,
		Buttons: []string{i18n.Tr("editor.art.export_copy", "Export copy"), i18n.Tr("editor.art.cancel", "Cancel")},
		Then: func(choice int) {
			if choice != 0 {
				return
			}
			if !current() {
				p.report(art.ErrConflict)
				return
			}
			dir, stem := artExportLocation(doc, "-export")
			target := assetops.UniqueFile(p.ctx, dir, stem, ".aseprite")
			p.askArtValues(i18n.Tr("editor.art.export_aseprite", "Export Aseprite interchange copy…"),
				[]string{i18n.Tr("editor.art.new_export_path", "New project path (existing files are not overwritten)")}, []string{target}, func(values []string) error {
					name := strings.TrimSpace(values[0])
					if !strings.EqualFold(path.Ext(name), ".aseprite") {
						return fmt.Errorf("%s", i18n.Tr("editor.art.aseprite_export_extension", "The interchange copy must use .aseprite."))
					}
					if !current() {
						return art.ErrConflict
					}
					for _, openPath := range []string{name, name + pipeline.MetaSuffix} {
						if docs.Script(openPath) != nil {
							return fmt.Errorf("%s", i18n.Tr("editor.art.aseprite_export_open_target", "Close the open destination or its metadata before exporting."))
						}
					}
					for _, open := range docs.AssetDocuments() {
						if open.Path == name {
							return fmt.Errorf("%s", i18n.Tr("editor.art.aseprite_export_open_target", "Close the open destination or its metadata before exporting."))
						}
					}
					// No importer or sidecar is installed by this interchange export.
					// Both a new file and absence of an orphan sidecar are guarded.
					edits := []pipeline.SourceEdit{
						{Path: name, After: data, Auxiliary: true},
						{Path: name + pipeline.MetaSuffix, ReadOnlyGuard: true},
					}
					batch, err := pl.ApplySources(edits, false)
					if err != nil {
						return err
					}
					p.ctx.ApplyBatch(batch)
					p.ctx.Notify(context.LevelInfo, i18n.Tr("editor.art.aseprite_exported", "Created interchange copy: {path}", i18n.Str("path", name)))
					return nil
				})
		},
	})
}

func (p *artPanel) askSaveAs() {
	ed, doc, model := p.ctx, p.doc, p.model
	if ed == nil || ed.Docs == nil || ed.Pipeline == nil || ed.Project == nil || doc == nil || model == nil || model.PendingRecovery || model.Session.Info().Gesture {
		p.report(fmt.Errorf("art: resolve recovery and finish the current gesture before saving"))
		return
	}
	docs, pl, project, session := ed.Docs, ed.Pipeline, ed.Project, model.Session
	owner := session.Document
	layer, frame, channel, selection := session.Layer, session.Frame, session.Channel, session.Selection
	revision := model.PreviewRevision()
	snapshot, err := owner.Snapshot()
	if err != nil {
		p.report(err)
		return
	}
	encoded, err := snapshot.Encode()
	if err != nil {
		p.report(err)
		return
	}
	current := func() error {
		if ed.Docs != docs || ed.Pipeline != pl || ed.Project != project || docs.AssetDocument(doc.Asset) != doc || doc.Binding.Model != model || model.Session != session || session.Document != owner || model.PendingRecovery || session.Info().Gesture || session.Layer != layer || session.Frame != frame || session.Channel != channel || session.Selection != selection || !model.PreviewCurrent(revision) {
			return art.ErrConflict
		}
		return model.CheckDraftInputs()
	}
	if err := current(); err != nil {
		p.report(err)
		return
	}
	dir, stem := artExportLocation(doc, "-copy")
	ed.Ask(context.Question{
		Title:            i18n.Tr("editor.shell.save_as", "Save As…"),
		Message:          i18n.Tr("editor.art.save_blank_notice", "Choose a project source. Replacing an existing drawing requires confirmation."),
		Fields:           []string{i18n.Tr("editor.art.source_path", "Source path")},
		Values:           []string{assetops.UniqueFile(ed, dir, stem, art.SourceExtension)},
		Buttons:          []string{i18n.Tr("editor.art.save", "Save"), i18n.Tr("editor.art.cancel", "Cancel")},
		DestructiveLabel: i18n.Tr("editor.art.replace", "Replace"),
		DestructiveWhen:  func(values []string) bool { return artSaveReplaces(ed, values) },
		Submit: func(values []string) {
			if len(values) != 1 {
				p.report(art.ErrConflict)
				return
			}
			if err := current(); err != nil {
				p.report(err)
				return
			}
			name, err := artSavePath(values[0])
			if err != nil {
				p.report(err)
				return
			}
			if !model.Draft && name == doc.Path {
				before, err := pl.SourceBytes(name)
				if err != nil {
					p.report(err)
					return
				}
				metaBefore, err := pl.SourceBytes(name + pipeline.MetaSuffix)
				if err == nil {
					err = artCurrentSaveGuard(ed, doc, before, metaBefore)
				}
				if err != nil {
					p.report(err)
					return
				}
				ed.Ask(context.Question{
					Title:   i18n.Tr("editor.art.replace_title", "Replace existing drawing?"),
					Message: i18n.Tr("editor.art.replace_notice", "Replace {path}? Its current drawing will be overwritten. The asset identity is kept, so all users of this shared asset can change.", i18n.Str("path", name)),
					Buttons: []string{i18n.Tr("editor.art.replace", "Replace"), i18n.Tr("editor.art.cancel", "Cancel")},
					Then: func(choice int) {
						if choice != 0 {
							return
						}
						if err := current(); err != nil {
							p.report(err)
							return
						}
						if err := artCurrentSaveGuard(ed, doc, before, metaBefore); err != nil {
							p.report(err)
							return
						}
						if doc.Binding.BeforeSave != nil {
							if err := doc.Binding.BeforeSave(); err != nil {
								p.report(err)
								return
							}
							if err := current(); err != nil {
								p.report(err)
								return
							}
							if err := artCurrentSaveGuard(ed, doc, before, metaBefore); err != nil {
								p.report(err)
								return
							}
						}
						// The normal save checks the original BinarySource generation,
						// marks this same history saved and emits its usual import event.
						p.report(model.Save())
						p.view.refresh = true
					},
				})
				return
			}
			target, err := prepareArtSaveTarget(ed, doc, name, snapshot)
			if err != nil {
				p.report(err)
				return
			}
			target.confirm(func() {
				if err := current(); err != nil {
					p.report(err)
					return
				}
				if err := target.verify(); err != nil {
					p.report(err)
					return
				}
				var batch *pipeline.Batch
				err := owner.Publish(snapshot, func() error {
					var err error
					batch, err = pl.ApplySources(target.edits(encoded), false)
					return err
				})
				if err == nil {
					ed.ApplyBatch(batch)
					// The copy or confirmed target is a separate identity; the
					// original dirty document and its consumers remain open.
					_, err = ed.OpenAssetPath(target.path)
				}
				p.report(err)
				p.view.refresh = true
			})
		},
	})
}

// Export is a new, independently editable bundle. The same pipeline transaction
// publishes PNG, frame metadata and the engine's existing .aeanim atomically.
func (p *artPanel) exportTo(target, format string) error {
	if p.model.Session.Info().Gesture {
		return fmt.Errorf("art: finish or cancel the gesture before export")
	}
	revision := p.model.PreviewRevision()
	snapshot, err := p.model.Session.Document.Snapshot()
	if err != nil {
		return err
	}
	if !p.model.PreviewCurrent(revision) {
		return art.ErrConflict
	}
	var edits []pipeline.SourceEdit
	pl := p.ctx.Pipeline
	switch format {
	case "aeart":
		if !strings.EqualFold(path.Ext(target), art.SourceExtension) {
			return fmt.Errorf("art: editable copies need .aeart")
		}
		data, err := snapshot.Encode()
		if err != nil {
			return err
		}
		edits, err = newArtCopyEdits(pl, target, data)
		if err != nil {
			return err
		}
	case "palette":
		if !strings.EqualFold(path.Ext(target), ".json") {
			return fmt.Errorf("art: palettes need .json")
		}
		data, err := art.EncodePalette(snapshot.Info().Palette)
		if err != nil {
			return err
		}
		edits = []pipeline.SourceEdit{{Path: target, After: data, Auxiliary: true}}
	case "png", "sheet":
		if !strings.EqualFold(path.Ext(target), ".png") {
			return fmt.Errorf("art: pixel exports need .png")
		}
		meta, err := pl.NewSourceMeta(target)
		if err != nil {
			return err
		}
		meta.Settings["auto_normal"], meta.Settings["auto_emission"] = aetext.Bool(false), aetext.Bool(false)
		switch p.model.Session.Channel {
		case art.Normal:
			meta.Settings["role"], meta.Settings["colorspace"] = aetext.String("normal"), aetext.String("linear")
		case art.Height:
			meta.Settings["role"], meta.Settings["colorspace"] = aetext.String("height"), aetext.String("linear")
		case art.Emission:
			meta.Settings["role"] = aetext.String("emission-rgb")
		}
		var data []byte
		if format == "png" {
			pixels, err := snapshot.Render(p.model.Session.Frame, art.RenderOptions{Channel: p.model.Session.Channel})
			if err != nil {
				return err
			}
			var b bytes.Buffer
			if err := png.Encode(&b, pixels); err != nil {
				return err
			}
			data = b.Bytes()
		} else {
			sheet, err := snapshot.ExportSheet(0, p.model.Session.Channel)
			if err != nil {
				return err
			}
			data = sheet.PNG
			frames := make(map[art.ID]ids.AssetID, len(sheet.Frames))
			info := snapshot.Info()
			for _, frame := range sheet.Frames {
				name := artsource.FrameName(frame.ID, p.model.Session.Channel)
				id := artsource.StableID(meta.ID, name)
				frames[frame.ID] = id
				sub := asset.SourceFrame{Name: name, ID: id, Rect: [4]int{frame.Rect.Min.X, frame.Rect.Min.Y, frame.Rect.Dx(), frame.Rect.Dy()}}
				if pivot, ok := info.FramePivot(frame.ID); ok {
					sub.Pivot, sub.HasPivot = [2]float32{float32(pivot.X) / float32(info.Canvas.Width), float32(pivot.Y) / float32(info.Canvas.Height)}, true
				}
				meta.Subs = append(meta.Subs, sub)
			}
			clip, err := artsource.ClipText(snapshot, "", frames)
			if err != nil {
				return err
			}
			base := strings.TrimSuffix(target, path.Ext(target))
			clipMeta, err := pl.NewSourceMeta(base + ".aeanim")
			if err != nil {
				return err
			}
			jsonData, err := json.MarshalIndent(sheet, "", "  ")
			if err != nil {
				return err
			}
			edits = append(edits, pipeline.SourceEdit{Path: base + ".aeanim", After: clip}, pipeline.SourceEdit{Path: base + ".aeanim.meta", After: pipeline.WriteMeta(clipMeta)}, pipeline.SourceEdit{Path: base + ".frames.json", After: jsonData, Auxiliary: true})
		}
		edits = append(edits, pipeline.SourceEdit{Path: target, After: data}, pipeline.SourceEdit{Path: target + ".meta", After: pipeline.WriteMeta(meta)})
	default:
		return fmt.Errorf("art: unknown export format")
	}
	if !p.model.PreviewCurrent(revision) {
		return art.ErrConflict
	}
	batch, err := pl.ApplySources(edits, false)
	if err != nil {
		return err
	}
	p.ctx.ApplyBatch(batch)
	p.ctx.Notify(context.LevelInfo, i18n.Tr("editor.art.exported", "Exported: {path}", i18n.Str("path", target)))
	return nil
}

func (p *artPanel) timelineActions(ui *core.Context, r mathx.Rect, editable bool) {
	s, v := p.model.Session, p.view
	info := s.Document.Info()
	p.actionMenu(ui, r, "timeline-actions", i18n.Tr("editor.art.frames_tags", "Frames / Tags…"), func(item func(string, string, bool, func())) {
		item("new-tag", i18n.Tr("editor.art.new_tag", "New animation tag…"), editable, func() { p.askTagProperties(art.Tag{Name: v.tagName, Direction: art.Forward}) })
		var selected art.Tag
		for _, tag := range info.Tags {
			if tag.ID == v.tag {
				selected = tag
			}
		}
		item("edit-tag", i18n.Tr("editor.art.edit_tag", "Edit selected tag…"), editable && selected.ID != "", func() { p.askTagProperties(selected) })
		item("delete-tag", i18n.Tr("editor.art.delete_tag", "Delete selected tag"), editable && selected.ID != "", func() { p.execute(art.Operation{Kind: "delete-tag", Tag: &selected}); v.tag = "" })
		item("all-frames", i18n.Tr("editor.art.all_frames", "Play all frames"), true, func() { v.tag = ""; v.playing = false; v.refresh = true })
		pages := (len(info.Tags) + 7) / 8
		v.tagPage = min(v.tagPage, max(0, pages-1))
		for i := v.tagPage * 8; i < min(len(info.Tags), (v.tagPage+1)*8); i++ {
			tag := info.Tags[i]
			item(string(tag.ID), tag.Name, true, func() { v.tag, v.tagName, v.playing, v.refresh = tag.ID, tag.Name, false, true })
		}
		if pages > 1 {
			item("tags-previous", i18n.Tr("editor.art.previous_tags", "Previous tags"), v.tagPage > 0, func() { v.tagPage-- })
			item("tags-next", i18n.Tr("editor.art.next_tags", "Next tags"), v.tagPage < pages-1, func() { v.tagPage++ })
		}
		item("frame-back", i18n.Tr("editor.art.frame_back", "Move frame earlier"), editable, func() { p.moveFrame(-1) })
		item("frame-forward", i18n.Tr("editor.art.frame_forward", "Move frame later"), editable, func() { p.moveFrame(1) })
		item("delete-frame", i18n.Tr("editor.art.delete_frame", "Delete current frame"), editable && len(info.Frames) > 1, func() { p.execute(art.Operation{Kind: "delete-frame"}) })
		item("pivot", i18n.Tr("editor.art.edit_pivot", "Frame pivot…"), editable, p.askPivot)
		item("copy-animation", i18n.Tr("editor.art.copy_animation", "Copy animation range…"), p.docks != nil, p.askCopyAnimation)
		item("paste-animation", i18n.Tr("editor.art.paste_animation", "Paste animation"), editable && p.docks != nil && p.docks.animationClipboard != nil, func() {
			p.report(p.model.PasteAnimation(stdctx.Background(), p.docks.animationClipboard))
			p.view.refresh = true
		})
	})
	widgets.Tooltip(ui, ui.ID("timeline-actions"), i18n.Tr("editor.art.tag", "Tag frames"))
}

func (p *artPanel) moveFrame(offset int) {
	s := p.model.Session
	info := s.Document.Info()
	order := make([]art.ID, len(info.Frames))
	at := -1
	for i, f := range info.Frames {
		order[i] = f.ID
		if f.ID == s.Frame {
			at = i
		}
	}
	to := at + offset
	if at < 0 || to < 0 || to >= len(order) {
		return
	}
	order[at], order[to] = order[to], order[at]
	p.execute(art.Operation{Kind: "frame-order", FrameOrder: order})
}

func (p *artPanel) askTagProperties(tag art.Tag) {
	frames := p.model.Session.Document.Info().Frames
	from, to := 1, len(frames)
	for i, f := range frames {
		if f.ID == tag.From {
			from = i + 1
		}
		if f.ID == tag.To {
			to = i + 1
		}
	}
	p.askArtValues(i18n.Tr("editor.art.tag_settings", "Animation tag"),
		[]string{i18n.Tr("editor.art.layer_name", "Name"), i18n.Tr("editor.art.tag_from", "First frame (1-based)"), i18n.Tr("editor.art.tag_to", "Last frame (1-based)"), i18n.Tr("editor.art.tag_direction", "Direction: forward / reverse / ping-pong")},
		[]string{tag.Name, strconv.Itoa(from), strconv.Itoa(to), string(tag.Direction)}, func(values []string) error {
			a, err := strconv.Atoi(strings.TrimSpace(values[1]))
			if err != nil {
				return err
			}
			b, err := strconv.Atoi(strings.TrimSpace(values[2]))
			if err != nil {
				return err
			}
			if a < 1 || b < a || b > len(frames) {
				return fmt.Errorf("art: invalid tag frame range")
			}
			tag.Name, tag.From, tag.To, tag.Direction = values[0], frames[a-1].ID, frames[b-1].ID, art.Direction(strings.TrimSpace(values[3]))
			id, err := p.model.Execute(stdctx.Background(), art.Operation{Kind: "tag", Tag: &tag})
			if err == nil {
				p.view.tag, p.view.tagName, p.view.playing = id, tag.Name, false
			}
			return err
		})
}

func (p *artPanel) askCopyAnimation() {
	frames := p.model.Session.Document.Info().Frames
	p.askArtValues(i18n.Tr("editor.art.copy_animation", "Copy animation range…"),
		[]string{i18n.Tr("editor.art.tag_from", "First frame (1-based)"), i18n.Tr("editor.art.tag_to", "Last frame (1-based)")},
		[]string{"1", strconv.Itoa(len(frames))}, func(v []string) error {
			first, err := strconv.Atoi(strings.TrimSpace(v[0]))
			if err != nil {
				return err
			}
			last, err := strconv.Atoi(strings.TrimSpace(v[1]))
			if err != nil {
				return err
			}
			if first < 1 || last < first || last > len(frames) {
				return fmt.Errorf("art: invalid animation frame range")
			}
			ids := make([]art.ID, last-first+1)
			for i := range ids {
				ids[i] = frames[first-1+i].ID
			}
			clip, err := p.model.Session.Document.CopyAnimation(ids)
			if err == nil {
				p.docks.animationClipboard = clip
			}
			return err
		})
}

func (p *artPanel) askPivot() {
	info := p.model.Session.Document.Info()
	frame := p.model.Session.Frame
	slice := art.Slice{Name: "pivot", Frame: frame, Rect: image.Rect(0, 0, info.Canvas.Width, info.Canvas.Height), Pivot: image.Pt(info.Canvas.Width/2, info.Canvas.Height/2)}
	if pivot, ok := info.FramePivot(frame); ok {
		slice.Pivot = pivot
	}
	for _, v := range info.Slices {
		if v.Name == "pivot" && v.Frame == frame {
			slice = v
		}
	}
	p.askArtValues(i18n.Tr("editor.art.edit_pivot", "Frame pivot…"), []string{"X", "Y"}, []string{strconv.Itoa(slice.Pivot.X), strconv.Itoa(slice.Pivot.Y)}, func(v []string) error {
		x, err := strconv.Atoi(strings.TrimSpace(v[0]))
		if err != nil {
			return err
		}
		y, err := strconv.Atoi(strings.TrimSpace(v[1]))
		if err != nil {
			return err
		}
		slice.Pivot = image.Pt(x, y)
		_, err = p.model.Execute(stdctx.Background(), art.Operation{Kind: "slice", Slice: &slice})
		return err
	})
}
