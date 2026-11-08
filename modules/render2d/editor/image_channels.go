package editor

import (
	"fmt"
	"slices"
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/code"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/scene/mutate"
	"aego/ui/widgets"
)

var imageSettingLabels = map[string]i18n.Text{
	"role":            i18n.T("editor.image.role", "Source role"),
	"colorspace":      i18n.T("editor.image.colorspace", "RGB color space"),
	"filter":          i18n.T("editor.image.filter", "Texture filter"),
	"atlas":           i18n.T("editor.image.atlas_group", "Atlas group"),
	"padding":         i18n.T("editor.image.padding", "Atlas padding"),
	"extrude":         i18n.T("editor.image.extrude", "Extrude atlas edges"),
	"trim":            i18n.T("editor.image.trim", "Trim transparent bounds"),
	"mips":            i18n.T("editor.image.mips", "Mipmaps"),
	"normal_y":        i18n.T("editor.image.normal_y", "Normal green channel / Y"),
	"generate_normal": i18n.T("editor.image.generate_normal", "Generate normal at import"),
	"height_strength": i18n.T("editor.image.height_strength", "Height strength"),
	"height_channel":  i18n.T("editor.image.height_channel", "Height source channel"),
	"mask_channel":    i18n.T("editor.image.mask_channel", "Mask source channel"),
	"auto_normal":     i18n.T("editor.image.auto_normal", "Discover matching normal"),
	"auto_emission":   i18n.T("editor.image.auto_emission", "Discover matching emission"),
}

func (s *imagesEditor) metadataControls(d *document.AssetDocument, v *imageState, sections *imageSections) {
	ed, ui := s.ctx, s.ctx.UI
	if v.message != "" {
		widgets.LabelWrapped(ui, v.message, true)
	}
	if d.Metadata == nil || !d.Capabilities.Has(document.EditMetadata) || d.Metadata.ReadOnly {
		widgets.LabelWrapped(ui, i18n.Tr("editor.image.readonly", "This generated or built-in image is read-only. Edit its source asset to change import settings."), false)
		return
	}
	if d.Metadata.Conflict {
		widgets.LabelWrapped(ui, i18n.Tr("editor.image.conflict", "The metadata changed on disk while this document was dirty. Resolve the conflict before editing or saving."), true)
		if widgets.Button(ui, i18n.Tr("editor.image.keep_buffer", "Keep my buffer · acknowledge disk change")) {
			ed.Docs.KeepScript(d.Metadata)
		}
		if widgets.Button(ui, i18n.Tr("editor.image.reload_disk", "Reload disk · discard these edits")) {
			v.reload(ed, d)
		}
		return
	}
	s.metadataActions(d, v)
	if d.Metadata.Dirty() {
		widgets.LabelWrapped(ui, i18n.Tr("editor.image.dirty", "Unsaved metadata. Save settings writes the import settings; source image pixels are never edited."), false)
	}
	if d.WorkingPreview.Pending {
		widgets.LabelWrapped(ui, i18n.Tr("editor.image.working_pending", "Preparing the Undo/Redo generation for the running scene. The previous valid assets remain active."), false)
	} else if d.WorkingPreview.Error != "" {
		widgets.LabelWrapped(ui, i18n.Tr("editor.image.working_failed", "The working generation could not be applied: {error}", i18n.Str("error", d.WorkingPreview.Error)), true)
	} else if d.WorkingPreview.Applied != 0 && d.Metadata.Dirty() {
		widgets.LabelWrapped(ui, i18n.Tr("editor.image.working_active", "The scene shows this working metadata generation. Save settings to make it the exportable source version."), false)
	}
	if v.metaError != nil {
		widgets.LabelWrapped(ui, v.metaError.Error(), true)
		return
	}
	if v.meta == nil {
		return
	}
	if v.candidate.pending {
		widgets.LabelWrapped(ui, i18n.Tr("editor.image.preparing", "Preparing the latest normal candidate. The previous valid preview remains visible."), false)
	}
	if v.pendingSave {
		widgets.LabelWrapped(ui, i18n.Tr("editor.image.save_pending", "Save requested: waiting for the current normal candidate to validate."), false)
	}
	if v.candidate.err != nil {
		widgets.LabelWrapped(ui, i18n.Tr("editor.image.candidate_failed", "Candidate failed; the previous valid normal is retained: {error}", i18n.Str("error", v.candidate.err.Error())), true)
	}
	if v.candidate.good != nil && v.candidate.goodRevision != v.inputRevision {
		widgets.LabelWrapped(ui, i18n.Tr("editor.image.candidate_previous", "The displayed generated normal belongs to a previous metadata revision."), false)
	}
	if v.role == asset.RoleColor && imageSection(ui, "image.section.channels", i18n.Tr("editor.image.section_channels", "Surface channels"), &sections.channels) {
		if widgets.Button(ui, i18n.Tr("editor.image.albedo_open", "Albedo · open this source/frame")) {
			_, _ = ed.OpenAsset(d.Selected)
		}
		for _, channel := range []string{"normal", "height", "emission"} {
			s.channelField(d, v, channel)
		}
	}
	if imageSection(ui, "image.section.settings", i18n.Tr("editor.image.section_settings", "Import settings"), &sections.settings) {
		for _, spec := range asset.TextureSettings() {
			label, known := imageSettingLabels[spec.Name]
			if !known || (len(spec.Roles) > 0 && !slices.Contains(spec.Roles, v.role)) {
				continue
			}
			if spec.Name == "colorspace" && v.role != asset.RoleColor && v.role != asset.RoleEmissionRGB {
				continue
			}
			if (spec.Name == "padding" || spec.Name == "extrude" || spec.Name == "trim") && v.meta.Settings.String("atlas", "") == "" {
				continue
			}
			s.settingField(d, v, spec, label.String())
		}
		if v.role == asset.RoleHeight {
			widgets.LabelWrapped(ui, i18n.Tr("editor.image.height_shared", "These generation settings belong to this height source and affect all of its consumers after Save. Preview controls above are local only."), false)
			if v.meta.Settings.Bool("generate_normal", false) && widgets.Button(ui, i18n.Tr("editor.image.prepare", "Prepare normal candidate")) {
				if err := v.prepare(ed, d); err != nil {
					v.message = err.Error()
				}
			}
		}
		var unknown []string
		for key, value := range v.meta.Settings {
			_, registered := asset.TextureSetting(key)
			if !registered && key != "normal" && key != "height" && key != "emission" && !strings.HasSuffix(key, "_frames") {
				unknown = append(unknown, key+" = "+aetext.FormatValue(value))
			}
		}
		slices.Sort(unknown)
		for _, value := range unknown {
			widgets.LabelDim(ui, i18n.Tr("editor.image.unknown", "Preserved metadata: {value}", i18n.Str("value", value)))
		}
	}
	if env, err := ed.TextureAuthoringEnv(d.Asset); err == nil {
		for _, p := range asset.ValidateTextureSource(v.meta, d.Path, env.Width, env.Height, env.Resolve) {
			widgets.LabelWrapped(ui, p.Code+": "+p.Field+": "+p.Message, p.Error)
		}
	}
}

func (s *imagesEditor) settingField(d *document.AssetDocument, v *imageState, spec asset.TextureSettingSpec, label string) {
	ui := s.ctx.UI
	value, explicit := v.meta.Settings[spec.Name]
	if !explicit {
		value = spec.Default
	}
	ui.PushID("setting." + spec.Name)
	defer ui.PopID()
	switch spec.Type {
	case "enum":
		current := 0
		word := value.Str
		if value.Kind == aetext.VIdent {
			word = value.Ident
		}
		for i, choice := range spec.Choices {
			if choice == word {
				current = i
			}
		}
		if widgets.Combo(ui, ui.ID("value"), widgets.Field(ui, label), &current, spec.Choices, false) {
			v.mutate(s.ctx, d, spec.Name, oneEdit(), mutate.TextureSetting(spec.Name, aetext.String(spec.Choices[current])))
		}
	case "bool":
		b := value.Bool
		if widgets.Checkbox(ui, label, &b) {
			v.mutate(s.ctx, d, spec.Name, oneEdit(), mutate.TextureSetting(spec.Name, aetext.Bool(b)))
		}
	case "float", "int":
		number := float32(value.AsFloat())
		opts := widgets.NumberOpts{Speed: .02, Min: 0, HasMin: true}
		if spec.Type == "int" {
			opts.Int, opts.Speed, opts.HasMax, opts.Max = true, .1, true, 64
		}
		edit := widgets.DragFloat32(ui, ui.ID("value"), widgets.Field(ui, label), &number, opts)
		encoded := aetext.Float32(number)
		if spec.Type == "int" {
			encoded = aetext.Int(int64(number))
		}
		v.mutate(s.ctx, d, spec.Name, edit, mutate.TextureSetting(spec.Name, encoded))
	case "string":
		text := value.Str
		if _, committed := widgets.TextFieldRect(ui, ui.ID("value"), widgets.Field(ui, label), &text, false); committed {
			v.mutate(s.ctx, d, spec.Name, oneEdit(), mutate.TextureSetting(spec.Name, aetext.String(text)))
		}
	}
}

type imageChoice struct {
	info asset.TextureSourceInfo
	name string
}

type imageChoiceRevision struct {
	id       document.ID
	code     *code.Doc
	revision uint64
}

func (s *imagesEditor) choices() []imageChoice {
	if s.ctx.Pipeline == nil {
		return nil
	}
	stamp := s.ctx.AssetRev + 1
	scripts := s.ctx.Docs.Scripts()
	changed := len(s.choiceDocs) != len(scripts)
	for i, d := range scripts {
		stamp = stamp*1099511628211 ^ uint64(d.ID) ^ d.Code.Rev()
		current := imageChoiceRevision{d.ID, d.Code, d.Code.Rev()}
		if i >= len(s.choiceDocs) || s.choiceDocs[i] != current {
			changed = true
		}
	}
	if s.choiceStamp == stamp && !changed {
		return s.choiceList
	}
	s.choiceStamp = stamp
	s.choiceDocs = s.choiceDocs[:0]
	for _, d := range scripts {
		s.choiceDocs = append(s.choiceDocs, imageChoiceRevision{d.ID, d.Code, d.Code.Rev()})
	}
	s.choiceList = s.choiceList[:0]
	overlay := func(name string) ([]byte, bool) {
		if d := s.ctx.Docs.Script(name); d != nil && d.Dirty() {
			return d.Code.Bytes(), true
		}
		return nil, false
	}
	for _, a := range s.ctx.Pipeline.Assets() {
		if a.Importer != "texture" {
			continue
		}
		info, ok := s.ctx.Pipeline.TextureSourceInfo(a.ID, overlay)
		if !ok {
			continue
		}
		s.choiceList = append(s.choiceList, imageChoice{info: info, name: a.Path})
		stored, ok := s.ctx.Pipeline.MetaOf(a.ID)
		meta := &stored
		if bytes, present := overlay(a.Path + ".meta"); present {
			meta, _ = asset.ParseSourceMeta(bytes, a.Path+".meta")
			ok = meta != nil
		}
		if !ok {
			continue
		}
		for _, frame := range meta.Subs {
			fi := info
			fi.ID, fi.Rect, fi.Width, fi.Height, fi.Pivot = frame.ID, frame.Rect, frame.Rect[2], frame.Rect[3], frame.EffectivePivot()
			s.choiceList = append(s.choiceList, imageChoice{info: fi, name: a.Path + "#" + frame.Name})
		}
	}
	return s.choiceList
}

func imageChannelCompatible(channel string, source, target asset.TextureSourceInfo) bool {
	if source.ID == target.ID || source.SourceID == target.ID || target.Width != source.Width || target.Height != source.Height || target.Pivot != source.Pivot {
		return false
	}
	switch channel {
	case "normal":
		return target.Role == asset.RoleNormal || (target.Role == asset.RoleHeight && target.GenerateNormal)
	case "height":
		return target.Role == asset.RoleHeight
	case "emission":
		return target.Role == asset.RoleEmissionRGB || target.Role == asset.RoleEmissionMask
	}
	return false
}

func (s *imagesEditor) channelField(d *document.AssetDocument, v *imageState, channel string) {
	ui, ed := s.ctx.UI, s.ctx
	ui.PushID("channel." + channel)
	defer ui.PopID()
	label := i18n.Tr("editor.image.normal_channel", "Normal")
	if channel == "height" {
		label = i18n.Tr("editor.image.height_channel_asset", "Height source")
	} else if channel == "emission" {
		label = i18n.Tr("editor.image.emission_channel", "Emission · RGB or mask")
	}
	frame := d.Selected
	if frame.IsZero() {
		frame = d.Asset
	}
	ref, explicit, err := asset.TextureChannel(v.meta, frame, channel)
	if err != nil {
		widgets.LabelWrapped(ui, err.Error(), true)
		return
	}
	env, err := ed.TextureAuthoringEnv(d.Asset)
	if err != nil {
		widgets.LabelWrapped(ui, err.Error(), true)
		return
	}
	source, validSource := env.Resolve(frame)
	current := 0
	labels := []string{i18n.Tr("editor.image.inherit", "Inherit / discover"), i18n.Tr("editor.image.disabled_channel", "Disabled explicitly")}
	refs := []ids.AssetID{{}, {}}
	if explicit {
		current = 1
	}
	for _, choice := range s.choices() {
		if !validSource || !imageChannelCompatible(channel, source, choice.info) {
			continue
		}
		if explicit && choice.info.ID == ref {
			current = len(labels)
		}
		labels, refs = append(labels, choice.name+" · "+string(choice.info.Role)), append(refs, choice.info.ID)
	}
	if explicit && !ref.IsZero() && current == 1 {
		current = len(labels)
		labels, refs = append(labels, i18n.Tr("editor.image.retained_reference", "Unresolved or incompatible: {asset}", i18n.Str("asset", ref.String()))), append(refs, ref)
	}
	r := widgets.Field(ui, label)
	if ref, inherit, picked := v.chooseChannel(ui, r, label, labels, refs, current, frame); picked {
		op := mutate.TextureChannel(frame, channel, ref)
		if inherit {
			op = mutate.TextureChannelReset(frame, channel)
		}
		v.mutate(ed, d, channel, oneEdit(), op)
	}
	if ed.DnD != nil {
		if payload, active := ed.DnD.Payload(); active && ed.DnD.Hovering(ui, r) {
			item, isAsset := payload.Data.(context.AssetPayload)
			target, found := env.Resolve(item.ID)
			if payload.Kind == context.KindAsset && isAsset && found && validSource && imageChannelCompatible(channel, source, target) {
				if _, dropped := ed.DnD.Target(ui, r, context.KindAsset); dropped {
					v.mutate(ed, d, channel, oneEdit(), mutate.TextureChannel(frame, channel, item.ID))
				}
			} else {
				ed.DnD.Target(ui, r, "incompatible-surface-channel")
			}
		}
	}
	// Absence and explicit zero stay distinct. Display the effective inherited
	// association without serializing it into the source metadata.
	effective := ref
	if effective.IsZero() && !explicit && ed.Loader != nil {
		if channel == "normal" {
			effective, _, _ = ed.Loader.NormalFor(frame)
		} else if channel == "emission" {
			effective, _, _ = ed.Loader.EmissionFor(frame)
		}
	}
	if !effective.IsZero() {
		widgets.LabelDim(ui, effective.String())
		if widgets.Button(ui, i18n.Tr("editor.image.open_channel", "Open {channel} source", i18n.Str("channel", label))) {
			if _, err := ed.OpenAsset(effective); err != nil {
				v.message = err.Error()
			}
		}
		if channel == "height" {
			if info, found := env.Resolve(effective); found {
				widgets.LabelWrapped(ui, i18n.Tr("editor.image.height_settings_owner", "Channel, strength and Y are shared settings of {source}. Open that height source to preview and edit them.", i18n.Str("source", info.Path)), false)
				if !info.GenerateNormal {
					widgets.LabelWrapped(ui, i18n.Tr("editor.image.height_not_generated", "This height reference is retained, but Generate normal is disabled on its source."), true)
				}
			}
		}
	}
	if frame != d.Asset {
		widgets.LabelDim(ui, fmt.Sprintf("%s → %s", frame.Short(), channel))
	}
}

// Both source-inspector modes use the same document Save/Undo/Redo owner.
func (s *imagesEditor) metadataActions(d *document.AssetDocument, v *imageState) {
	ed, ui := s.ctx, s.ctx.UI
	row := ui.Cursor().Next(0)
	w := (row.W - 8) / 3
	if widgets.ButtonRect(ui, ui.ID("image.save"), mathx.Rect{X: row.X, Y: row.Y, W: w, H: row.H}, i18n.Tr("editor.image.save_button", "Save")) {
		v.save(ed, d)
	}
	if widgets.ButtonRect(ui, ui.ID("image.undo"), mathx.Rect{X: row.X + w + 4, Y: row.Y, W: w, H: row.H}, i18n.Tr("editor.image.undo", "Undo")) {
		if err := ed.UndoAsset(d); err != nil {
			v.message = err.Error()
		}
		v.gesture = ""
	}
	if widgets.ButtonRect(ui, ui.ID("image.redo"), mathx.Rect{X: row.X + 2*w + 8, Y: row.Y, W: w, H: row.H}, i18n.Tr("editor.image.redo", "Redo")) {
		if err := ed.RedoAsset(d); err != nil {
			v.message = err.Error()
		}
		v.gesture = ""
	}
}
