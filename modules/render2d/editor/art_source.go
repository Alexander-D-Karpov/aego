package editor

import (
	"bytes"
	"fmt"
	"path"
	"slices"
	"strings"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/artedit"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
)

func canEditSourceArt(ed *context.Editor, id ids.AssetID) bool {
	if ed == nil || ed.Pipeline == nil || ed.Docs == nil || asset.IsBuiltin(id) {
		return false
	}
	source, ok := ed.SourcePathForAsset(id)
	if !ok {
		return false
	}
	switch strings.ToLower(path.Ext(source)) {
	case art.SourceExtension:
		return true
	case ".png":
		root, ok := ed.Pipeline.Lookup(source)
		if !ok {
			return false
		}
		meta, ok := ed.Pipeline.MetaOf(root)
		if !ok {
			return false
		}
		role, err := asset.TextureRoleOf(source, meta.Settings)
		return err == nil && role == asset.RoleColor
	}
	return false
}

// The ordinary source action uses the same canonical canvas route as Assets.
func editSourceArt(ed *context.Editor, id ids.AssetID) error {
	if !canEditSourceArt(ed, id) {
		return fmt.Errorf("%s", i18n.Tr("editor.art.source_unavailable", "This asset has no editable project image source."))
	}
	_, err := ed.OpenAsset(id)
	return err
}

func openLinkedArtFrame(ed *context.Editor, link *artsource.TextureLink, selected ids.AssetID) error {
	legacyNotice := func(err error) error {
		if err == nil && link.LayoutSHA256 == "" {
			ed.Notify(context.LevelWarn, i18n.Tr("editor.art.legacy_link_geometry", "This older PNG link cannot detect slicing changes. Keep its source rectangles unchanged and review them before importing."))
		}
		return err
	}
	var frame art.ID
	for _, f := range link.Frames {
		if f.Asset == selected {
			frame = f.Frame
			break
		}
	}
	if frame == "" && len(link.Frames) != 0 {
		frame = link.Frames[0].Frame
	}
	name := artsource.FrameName(frame, art.Albedo)
	if source, ok := ed.Pipeline.PathOf(link.Source); ok {
		if id, ok := ed.Pipeline.Lookup(source + pipeline.SubSep + name); ok {
			_, err := ed.OpenAsset(id)
			return legacyNotice(err)
		}
	}
	// Reuse the canonical dirty document rather than reopening a second copy.
	if existing := ed.Docs.AssetDocument(link.Source); existing != nil {
		if model, ok := existing.Binding.Model.(*artedit.Document); ok {
			if err := model.Session.Select("", frame, art.Albedo); err != nil {
				return err
			}
			_, err := ed.OpenAsset(link.Source)
			return legacyNotice(err)
		}
	}
	return fmt.Errorf("%s", i18n.Tr("editor.art.linked_frame_unavailable", "The linked art frame is unavailable. Resolve its import error before editing."))
}

func askLinkedArtSource(ed *context.Editor, doc *document.AssetDocument, meta *pipeline.Meta) error {
	pl, docs, project := ed.Pipeline, ed.Docs, ed.Project
	selected, sourcePath := doc.Selected, doc.Path
	metadata := doc.Metadata
	metadataRevision := metadata.Code.Rev()
	guards := map[string][]byte{}
	read := func(name string) ([]byte, error) {
		if source := docs.Script(name); source != nil && (source.Dirty() || source.Conflict) {
			return nil, fmt.Errorf("%s", i18n.Tr("editor.art.resolve_source_settings", "Save or resolve unsaved source settings before linking: {path}", i18n.Str("path", name)))
		}
		if data, ok := guards[name]; ok {
			return data, nil
		}
		data, err := pl.SourceBytes(name)
		if err != nil {
			return nil, err
		}
		if data == nil {
			return nil, fmt.Errorf("art: source no longer exists: %s", name)
		}
		guards[name] = bytes.Clone(data)
		return data, nil
	}
	before, err := read(sourcePath)
	if err != nil {
		return err
	}
	metaBefore, err := read(sourcePath + pipeline.MetaSuffix)
	if err != nil {
		return err
	}
	normalized := bytes.ReplaceAll(bytes.ReplaceAll(metaBefore, []byte("\r\n"), []byte("\n")), []byte("\r"), []byte("\n"))
	if !bytes.Equal(normalized, metadata.Code.Bytes()) {
		return fmt.Errorf("%s", i18n.Tr("editor.art.external_source_conflict", "The image source or its metadata changed outside the editor. Resolve the conflict before linking."))
	}
	drawing, frames, err := artsource.ImportTextureDocument(&pipeline.ImportContext{
		ID: doc.Asset, Path: sourcePath, Source: before, Meta: meta,
		Read: read, SourcePath: pl.PathOf, Lookup: pl.Lookup,
	})
	if err != nil {
		return err
	}
	defer drawing.Close()
	encoded, err := drawing.Encode()
	if err != nil {
		return err
	}
	stem := strings.TrimSuffix(path.Base(sourcePath), path.Ext(sourcePath))
	current := func() bool {
		return ed.Pipeline == pl && ed.Docs == docs && ed.Project == project && docs.AssetDocument(doc.Asset) == doc &&
			doc.Path == sourcePath && doc.Metadata == metadata && metadata.Code.Rev() == metadataRevision &&
			!metadata.ReadOnly && !metadata.Dirty() && !metadata.Conflict
	}
	submit := func(values []string) {
		if len(values) != 1 {
			return
		}
		fail := func(err error) { ed.Notify(context.LevelError, err.Error()) }
		if !current() {
			fail(fmt.Errorf("%s", i18n.Tr("editor.art.link_target_changed", "The project, image or import settings changed before linking. Open Edit Source Art again.")))
			return
		}
		for name := range guards {
			if source := docs.Script(name); source != nil && (source.Dirty() || source.Conflict) {
				fail(fmt.Errorf("%s", i18n.Tr("editor.art.resolve_source_settings", "Save or resolve unsaved source settings before linking: {path}", i18n.Str("path", name))))
				return
			}
		}
		target := strings.TrimSpace(values[0])
		if !strings.EqualFold(path.Ext(target), art.SourceExtension) {
			fail(fmt.Errorf("%s", i18n.Tr("editor.art.link_extension", "The editable source must use .aeart.")))
			return
		}
		newMeta, err := pl.NewSourceMeta(target)
		if err != nil {
			fail(err)
			return
		}
		for _, key := range []string{"filter", "colorspace", "srgb"} {
			if value, ok := meta.Settings[key]; ok {
				newMeta.Settings[key] = value
			}
		}
		linkedMeta, err := pipeline.ParseMeta(metaBefore, sourcePath+pipeline.MetaSuffix)
		if err == nil {
			err = artsource.SetTextureLink(linkedMeta, newMeta.ID, before, frames)
		}
		if err != nil {
			fail(err)
			return
		}
		edits := []pipeline.SourceEdit{
			{Path: target, After: encoded},
			{Path: target + pipeline.MetaSuffix, After: pipeline.WriteMeta(newMeta)},
			{Path: sourcePath + pipeline.MetaSuffix, Before: metaBefore, After: pipeline.WriteMeta(linkedMeta)},
		}
		paths := make([]string, 0, len(guards))
		for name := range guards {
			if name != sourcePath+pipeline.MetaSuffix {
				paths = append(paths, name)
			}
		}
		slices.Sort(paths)
		for _, name := range paths {
			// Normalization of inputs must not silently change source PNGs,
			// linked surface files, or their metadata during the transaction.
			edits = append(edits, pipeline.SourceEdit{Path: name, Before: guards[name], After: guards[name], ReadOnlyGuard: true})
		}
		batch, err := pl.ApplySources(edits, false)
		if err != nil {
			fail(err)
			return
		}
		ed.ApplyBatch(batch)
		// ApplySources records the actual canonicalized metadata generation.
		if err := docs.AcceptSavedScript(metadata, metaBefore, edits[2].After); err != nil {
			fail(err)
			return
		}
		link, err := artsource.TextureLinkOf(linkedMeta)
		if err == nil {
			err = openLinkedArtFrame(ed, link, selected)
		}
		if err != nil {
			fail(err)
		}
	}
	// A path prompt renders its message on one line. Use the scrollable
	// confirmation first so the shared-resource and metadata warning is visible.
	ed.Ask(context.Question{
		Title:   i18n.Tr("editor.art.create_linked_source", "Create linked art source"),
		Message: i18n.Tr("editor.art.linked_source_notice", "Create an editable source for {path} and store a link in its image import metadata. Sprite and animation IDs stay the same; edits affect every consumer of this shared asset. The original PNG is preserved.", i18n.Str("path", sourcePath)),
		Buttons: []string{i18n.Tr("editor.shell.continue", "Continue"), i18n.Tr("editor.art.cancel", "Cancel")},
		Then: func(choice int) {
			if choice != 0 {
				return
			}
			if !current() {
				ed.Notify(context.LevelError, i18n.Tr("editor.art.link_target_changed", "The project, image or import settings changed before linking. Open Edit Source Art again."))
				return
			}
			ed.Ask(context.Question{
				Title:   i18n.Tr("editor.art.create_linked_source", "Create linked art source"),
				Fields:  []string{i18n.Tr("editor.art.source_path", "Source path")},
				Values:  []string{assetops.UniqueFile(ed, path.Dir(sourcePath), stem, art.SourceExtension)},
				Buttons: []string{i18n.Tr("editor.art.create_linked", "Create linked source"), i18n.Tr("editor.art.cancel", "Cancel")},
				Submit:  submit,
			})
		},
	})
	return nil
}
