package editor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"slices"
	"strings"

	"aego/art"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/artedit"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
	"aego/project"
)

// The draft owns no disk files. The original PNG identity continues to own
// its working model until an explicit, guarded first Save links a new source.
type pngArtDraft struct {
	ed       *context.Editor
	doc      *document.AssetDocument
	docs     *document.Manager
	pipe     *pipeline.Pipeline
	project  *project.Project
	path     string
	meta     *pipeline.Meta
	metadata *document.ScriptDocument
	metaRev  uint64
	guards   map[string][]byte
	frames   []artsource.TextureLinkFrame
	model    *artedit.Document
}

func openPNGArtDraft(ed *context.Editor, doc *document.AssetDocument) (*pngArtDraft, *artsource.TextureLink, error) {
	if ed == nil || ed.Pipeline == nil || ed.Docs == nil || doc == nil || !doc.Capabilities.Has(document.EditSource) {
		return nil, nil, fmt.Errorf("art: this PNG source is not writable")
	}
	if err := ed.Pipeline.CheckSourceRecovery(); err != nil {
		return nil, nil, err
	}
	if doc.Binding.Model != nil {
		return nil, nil, fmt.Errorf("art: this image already has another open structured editor")
	}
	if doc.Metadata == nil {
		return nil, nil, fmt.Errorf("%s", i18n.Tr("editor.art.metadata_required", "Repair the image metadata before creating an editable source."))
	}
	if doc.Metadata.ReadOnly || doc.Metadata.Dirty() || doc.Metadata.Conflict {
		return nil, nil, fmt.Errorf("%s", i18n.Tr("editor.art.save_image_settings", "Save or resolve the image import settings before creating a linked art source."))
	}
	d := &pngArtDraft{ed: ed, doc: doc, docs: ed.Docs, pipe: ed.Pipeline, project: ed.Project, path: doc.Path,
		metadata: doc.Metadata, metaRev: doc.Metadata.Code.Rev(), guards: make(map[string][]byte)}
	read := func(name string) ([]byte, error) {
		if script := d.docs.Script(name); script != nil && (script.Dirty() || script.Conflict) {
			return nil, fmt.Errorf("%s", i18n.Tr("editor.art.resolve_source_settings", "Save or resolve unsaved source settings before linking: {path}", i18n.Str("path", name)))
		}
		if data, ok := d.guards[name]; ok {
			return data, nil
		}
		data, err := d.pipe.SourceBytes(name)
		if err != nil {
			return nil, err
		}
		if data == nil {
			return nil, fmt.Errorf("art: source no longer exists: %s", name)
		}
		d.guards[name] = bytes.Clone(data)
		return d.guards[name], nil
	}
	metaBytes, err := read(d.path + pipeline.MetaSuffix)
	if err != nil {
		return nil, nil, err
	}
	normalized := bytes.ReplaceAll(bytes.ReplaceAll(metaBytes, []byte("\r\n"), []byte("\n")), []byte("\r"), []byte("\n"))
	if !bytes.Equal(normalized, d.metadata.Code.Bytes()) {
		return nil, nil, fmt.Errorf("%s", i18n.Tr("editor.art.external_source_conflict", "The image source or its metadata changed outside the editor. Resolve the conflict before linking."))
	}
	d.meta, err = pipeline.ParseMeta(metaBytes, d.path+pipeline.MetaSuffix)
	if err != nil {
		return nil, nil, err
	}
	link, err := artsource.TextureLinkOf(d.meta)
	if err != nil {
		return nil, nil, err
	}
	png, err := read(d.path)
	if err != nil {
		return nil, nil, err
	}
	if link != nil {
		hash := sha256.Sum256(png)
		if hex.EncodeToString(hash[:]) != link.SHA256 {
			return nil, nil, fmt.Errorf("%s", i18n.Tr("editor.art.external_source_conflict", "The image source or its metadata changed outside the editor. Resolve the conflict before linking."))
		}
		if err := d.verify(); err != nil {
			return nil, nil, err
		}
		return nil, link, nil
	}
	pixels, frames, err := artsource.ImportTextureDocument(&pipeline.ImportContext{
		ID: doc.Asset, Path: d.path, Source: png, Meta: d.meta,
		Read: read, SourcePath: d.pipe.PathOf, Lookup: d.pipe.Lookup,
	})
	if err != nil {
		return nil, nil, err
	}
	d.frames = frames
	hash := sha256.New()
	fmt.Fprintln(hash, "aego-png-art-recovery-v1")
	paths := make([]string, 0, len(d.guards))
	for name := range d.guards {
		paths = append(paths, name)
	}
	slices.Sort(paths)
	for _, name := range paths {
		data := d.guards[name]
		fmt.Fprintf(hash, "%d:%s:%d:", len(name), name, len(data))
		_, _ = hash.Write(data)
	}
	for _, frame := range frames {
		fmt.Fprintf(hash, "\n%s:%s", frame.Asset.String(), frame.Frame)
	}
	d.model, err = artedit.BindImported(d.docs, doc, pixels, png, hex.EncodeToString(hash.Sum(nil)), d.verify)
	if err != nil {
		return nil, nil, err
	}
	return d, nil, nil
}

func (d *pngArtDraft) verify() error {
	if err := d.pipe.CheckSourceRecovery(); err != nil {
		return err
	}
	if d.ed.Docs != d.docs || d.ed.Pipeline != d.pipe || d.ed.Project != d.project || d.docs.AssetDocument(d.doc.Asset) != d.doc || d.doc.Path != d.path || d.doc.Metadata != d.metadata || d.metadata.Code.Rev() != d.metaRev || d.metadata.ReadOnly || d.metadata.Dirty() || d.metadata.Conflict {
		return fmt.Errorf("%s", i18n.Tr("editor.art.link_target_changed", "The project, image or import settings changed before linking. Open Edit Source Art again."))
	}
	if name, ok := d.pipe.PathOf(d.doc.Asset); !ok || name != d.path {
		return fmt.Errorf("art: image identity or path changed")
	}
	for name, before := range d.guards {
		if source := d.docs.Script(name); source != nil && (source.Dirty() || source.Conflict) {
			return fmt.Errorf("%s", i18n.Tr("editor.art.resolve_source_settings", "Save or resolve unsaved source settings before linking: {path}", i18n.Str("path", name)))
		}
		current, err := d.pipe.SourceBytes(name)
		if err != nil || current == nil || !bytes.Equal(before, current) {
			return fmt.Errorf("%s", i18n.Tr("editor.art.external_source_conflict", "The image source or its metadata changed outside the editor. Resolve the conflict before linking."))
		}
	}
	return d.pipe.CheckSourceRecovery()
}

func (d *pngArtDraft) selectFrame(id ids.AssetID) error {
	if d.model == nil || len(d.frames) == 0 {
		return fmt.Errorf("art: image has no editable frame")
	}
	frame := d.frames[0].Frame
	found := id == d.doc.Asset
	for _, candidate := range d.frames {
		if candidate.Asset == id {
			frame = candidate.Frame
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s", i18n.Tr("editor.art.frame_unavailable", "The selected art frame is no longer available."))
	}
	if d.model.Session.Frame == frame && d.model.Session.Channel == art.Albedo {
		return nil
	}
	return d.model.Session.Select("", frame, art.Albedo)
}

func (d *pngArtDraft) askSave(panel *artPanel, transferred func(*document.AssetDocument) error) error {
	if err := d.verify(); err != nil {
		return err
	}
	if d.model.PendingRecovery || d.model.Session.Info().Gesture {
		return fmt.Errorf("art: resolve recovery and finish the current gesture before saving")
	}
	session := d.model.Session
	snapshot, err := session.Document.Snapshot()
	if err != nil {
		return err
	}
	encoded, err := snapshot.Encode()
	if err != nil {
		return err
	}
	revision := d.model.PreviewRevision()
	current := func() bool {
		return d.doc.Binding.Model == d.model && d.model.Draft && !d.model.PendingRecovery && d.model.Session == session && !session.Info().Gesture && d.model.PreviewCurrent(revision)
	}
	stem := strings.TrimSuffix(path.Base(d.path), path.Ext(d.path))
	d.ed.Ask(context.Question{
		Title:            i18n.Tr("editor.art.save_png_draft", "Save editable image source"),
		Message:          i18n.Tr("editor.art.save_png_draft_notice", "Save the drawing as an .aeart source and link it in the PNG import metadata. Replacing an existing drawing requires confirmation. The PNG itself is not overwritten; changes affect all users of this shared image."),
		Fields:           []string{i18n.Tr("editor.art.source_path", "Source path")},
		Values:           []string{assetops.UniqueFile(d.ed, path.Dir(d.path), stem, art.SourceExtension)},
		Buttons:          []string{i18n.Tr("editor.art.save", "Save"), i18n.Tr("editor.art.cancel", "Cancel")},
		DestructiveLabel: i18n.Tr("editor.art.replace", "Replace"),
		DestructiveWhen:  func(values []string) bool { return artSaveReplaces(d.ed, values) },
		Submit: func(values []string) {
			fail := func(err error) { panel.report(err) }
			if len(values) != 1 || !current() {
				fail(art.ErrConflict)
				return
			}
			if err := d.verify(); err != nil {
				fail(err)
				return
			}
			target, err := prepareArtSaveTarget(d.ed, d.doc, strings.TrimSpace(values[0]), snapshot)
			if err != nil {
				fail(err)
				return
			}
			if !target.replacement {
				for _, key := range []string{"filter", "colorspace", "srgb"} {
					if value, ok := d.meta.Settings[key]; ok {
						target.meta.Settings[key] = value
					}
				}
			}
			target.confirm(func() {
				if !current() {
					fail(art.ErrConflict)
					return
				}
				if err := d.verify(); err != nil {
					fail(err)
					return
				}
				if err := target.verify(); err != nil {
					fail(err)
					return
				}
				metaBefore := d.guards[d.path+pipeline.MetaSuffix]
				linked, err := pipeline.ParseMeta(metaBefore, d.path+pipeline.MetaSuffix)
				if err == nil {
					err = artsource.SetTextureLink(linked, target.meta.ID, d.guards[d.path], d.frames)
				}
				if err != nil {
					fail(err)
					return
				}
				edits := target.edits(encoded)
				edits = append(edits, pipeline.SourceEdit{Path: d.path + pipeline.MetaSuffix, Before: metaBefore, After: pipeline.WriteMeta(linked)})
				paths := make([]string, 0, len(d.guards))
				for name := range d.guards {
					if name != d.path+pipeline.MetaSuffix {
						paths = append(paths, name)
					}
				}
				slices.Sort(paths)
				for _, name := range paths {
					edits = append(edits, pipeline.SourceEdit{Path: name, Before: d.guards[name], After: d.guards[name], ReadOnlyGuard: true})
				}
				var batch *pipeline.Batch
				err = session.Document.Publish(snapshot, func() error {
					var err error
					batch, err = d.pipe.ApplySources(edits, false)
					return err
				})
				if err != nil {
					fail(err)
					return
				}
				d.ed.ApplyBatch(batch)
				if err := d.docs.AcceptSavedScript(d.metadata, metaBefore, edits[2].After); err != nil {
					fail(fmt.Errorf("art: source was published, but the open image settings could not be synchronized; the drawing is still open: %w", err))
					return
				}
				next, err := d.ed.DocumentForAsset(target.meta.ID)
				if err == nil {
					err = d.model.RebindSaved(next, snapshot)
				}
				if err != nil {
					fail(fmt.Errorf("art: source was published, but the drawing remains open as an unsaved draft: %w", err))
					return
				}
				if err := d.docs.ClearOwnedAssetRecovery(d.doc); err != nil {
					d.ed.Notify(context.LevelWarn, err.Error())
				}
				if err := transferred(next); err != nil {
					fail(err)
					return
				}
				d.ed.Notify(context.LevelInfo, i18n.Tr("editor.art.png_draft_saved", "Saved linked drawing: {path}", i18n.Str("path", target.path)))
			})
		},
	})
	return document.ErrSavePending
}
