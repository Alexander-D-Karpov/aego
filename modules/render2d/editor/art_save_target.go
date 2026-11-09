package editor

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
	"aego/project"
)

// Paths are project-relative. Accept a conventional leading ./ without
// accepting parent traversal, an absolute path, or platform-specific aliases.
func artSavePath(value string) (string, error) {
	rel := strings.TrimSpace(value)
	for strings.HasPrefix(rel, "./") {
		rel = strings.TrimPrefix(rel, "./")
	}
	if !fs.ValidPath(rel) || !filepath.IsLocal(rel) || strings.ContainsAny(rel, "\\:#") {
		return "", fmt.Errorf("art: enter a project-relative source path")
	}
	return rel, nil
}

// The red label is a hint, never permission to overwrite. SourceBytes and the
// journalled transaction perform the actual validation and preimage checks.
func artSaveReplaces(ed *context.Editor, values []string) bool {
	if ed == nil || ed.Project == nil || len(values) != 1 {
		return false
	}
	rel, err := artSavePath(values[0])
	if err != nil {
		return false
	}
	root, err := os.OpenRoot(ed.Project.Root)
	if err != nil {
		return false
	}
	defer root.Close()
	_, err = root.Lstat(rel)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

type artSaveTarget struct {
	ed          *context.Editor
	docs        *document.Manager
	pipe        *pipeline.Pipeline
	project     *project.Project
	owner       *document.AssetDocument
	path        string
	before      []byte
	metaBefore  []byte
	meta        *pipeline.Meta
	replacement bool
}

func prepareArtSaveTarget(ed *context.Editor, owner *document.AssetDocument, target string, snapshot *art.Snapshot) (*artSaveTarget, error) {
	if ed == nil || ed.Docs == nil || ed.Pipeline == nil || ed.Project == nil || owner == nil || snapshot == nil {
		return nil, fmt.Errorf("art: project and drawing are required")
	}
	var err error
	target, err = artSavePath(target)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(path.Ext(target), art.SourceExtension) {
		return nil, fmt.Errorf("%s", i18n.Tr("editor.art.link_extension", "The editable source must use .aeart."))
	}
	if err := ed.Pipeline.CheckSourceRecovery(); err != nil {
		return nil, err
	}
	d := &artSaveTarget{ed: ed, docs: ed.Docs, pipe: ed.Pipeline, project: ed.Project, owner: owner, path: target}
	if d.before, err = d.pipe.SourceBytes(target); err != nil {
		return nil, err
	}
	if d.metaBefore, err = d.pipe.SourceBytes(target + pipeline.MetaSuffix); err != nil {
		return nil, err
	}
	d.replacement = d.before != nil
	if d.replacement {
		if d.metaBefore == nil {
			return nil, fmt.Errorf("%s", i18n.Tr("editor.art.replace_missing_meta", "The existing drawing has no metadata. Resolve its identity before replacing it."))
		}
		d.meta, err = pipeline.ParseMeta(d.metaBefore, target+pipeline.MetaSuffix)
		if err != nil || d.meta.ID.IsZero() || asset.IsBuiltin(d.meta.ID) || d.meta.Importer != "artsource" {
			return nil, fmt.Errorf("%s", i18n.Tr("editor.art.replace_invalid_meta", "The existing drawing has invalid or incompatible metadata. It was not replaced."))
		}
		old, decodeErr := art.Decode(d.before, art.DefaultLimits())
		if decodeErr != nil {
			return nil, fmt.Errorf("%s: %w", i18n.Tr("editor.art.replace_invalid_source", "The existing drawing could not be read; it was not replaced"), decodeErr)
		}
		oldInfo := old.Info()
		old.Close()
		if err := artReplacementKeepsReferences(oldInfo, snapshot.Info()); err != nil {
			return nil, err
		}
	} else {
		if d.metaBefore != nil {
			return nil, fmt.Errorf("%s", i18n.Tr("editor.art.replace_orphan_meta", "The destination already has metadata but no source. Resolve it or choose another name."))
		}
		d.meta, err = d.pipe.NewSourceMeta(target)
		if err != nil {
			return nil, err
		}
	}
	if err := d.verify(); err != nil {
		return nil, err
	}
	return d, nil
}

// A replacement may change pixels, but must not silently remove existing
// frame, channel, or tag identities used by scenes, clips, and linked PNGs.
func artReplacementKeepsReferences(old, next art.Info) error {
	names := func(info art.Info) map[string]bool {
		channels := map[art.Channel]bool{art.Albedo: true}
		for _, cel := range info.Cels {
			channels[cel.Channel] = true
		}
		out := map[string]bool{artsource.ClipName(""): true}
		for _, frame := range info.Frames {
			for channel := range channels {
				out[artsource.FrameName(frame.ID, channel)] = true
			}
		}
		for _, tag := range info.Tags {
			out[artsource.ClipName(tag.ID)] = true
		}
		return out
	}
	current := names(next)
	for name := range names(old) {
		if !current[name] {
			return fmt.Errorf("%s", i18n.Tr("editor.art.replace_incompatible", "Replacing this drawing would remove existing frame, animation, or surface references. Choose a new name instead."))
		}
	}
	return nil
}

func sameArtSource(a, b []byte) bool {
	return (a == nil) == (b == nil) && bytes.Equal(a, b)
}

// Saving back to the currently open source uses its normal BinarySource guard
// and history. Check its sidecar and both path generations before offering or
// accepting the explicit replacement; do not save unrelated dirty metadata.
func artCurrentSaveGuard(ed *context.Editor, doc *document.AssetDocument, before, metaBefore []byte) error {
	if ed == nil || ed.Pipeline == nil || ed.Docs == nil || doc == nil || ed.Docs.AssetDocument(doc.Asset) != doc || before == nil || metaBefore == nil {
		return fmt.Errorf("art: the current source or its metadata is unavailable")
	}
	if err := ed.Pipeline.CheckSourceRecovery(); err != nil {
		return err
	}
	if name, ok := ed.Pipeline.PathOf(doc.Asset); !ok || name != doc.Path {
		return fmt.Errorf("art: the current source identity changed")
	}
	meta, err := pipeline.ParseMeta(metaBefore, doc.Path+pipeline.MetaSuffix)
	if err != nil || meta.ID != doc.Asset || meta.Importer != "artsource" {
		return fmt.Errorf("art: the current source metadata identity changed")
	}
	for _, entry := range []struct {
		name string
		data []byte
	}{{doc.Path, before}, {doc.Path + pipeline.MetaSuffix, metaBefore}} {
		if script := ed.Docs.Script(entry.name); script != nil {
			if script.Dirty() || script.Conflict || entry.name == doc.Path {
				return fmt.Errorf("art: close or resolve the open source settings before saving: %s", entry.name)
			}
			normalized := bytes.ReplaceAll(bytes.ReplaceAll(entry.data, []byte("\r\n"), []byte("\n")), []byte("\r"), []byte("\n"))
			if !bytes.Equal(script.Code.Bytes(), normalized) {
				return fmt.Errorf("art: the open source metadata differs from disk: %s", entry.name)
			}
		}
		if pending, err := ed.Docs.HasScriptRecovery(entry.name); err != nil {
			return err
		} else if pending {
			return fmt.Errorf("art: the source has a pending recovery: %s", entry.name)
		}
		got, err := ed.Pipeline.SourceBytes(entry.name)
		if err != nil || !sameArtSource(got, entry.data) {
			return fmt.Errorf("%s", i18n.Tr("editor.art.replace_changed", "The destination changed while saving. Review it and try again."))
		}
	}
	return nil
}

func (d *artSaveTarget) verify() error {
	if d.ed.Docs != d.docs || d.ed.Pipeline != d.pipe || d.ed.Project != d.project || d.docs.AssetDocument(d.owner.Asset) != d.owner {
		return fmt.Errorf("art: the project or drawing changed before saving")
	}
	if err := d.pipe.CheckSourceRecovery(); err != nil {
		return err
	}
	for _, name := range []string{d.path, d.path + pipeline.MetaSuffix} {
		if script := d.docs.Script(name); script != nil {
			// A clean metadata document may have been acquired by Inspector,
			// without the drawing itself being opened. Replacement keeps its
			// exact metadata bytes, so reuse it only while its baseline agrees.
			if !d.replacement || name == d.path || script.Dirty() || script.Conflict {
				return fmt.Errorf("art: close or resolve the open destination before saving: %s", name)
			}
			normalized := bytes.ReplaceAll(bytes.ReplaceAll(d.metaBefore, []byte("\r\n"), []byte("\n")), []byte("\r"), []byte("\n"))
			if !bytes.Equal(script.Code.Bytes(), normalized) {
				return fmt.Errorf("art: open destination metadata differs from the saved file: %s", name)
			}
		}
		pending, err := d.docs.HasScriptRecovery(name)
		if err != nil {
			return err
		}
		if pending {
			return fmt.Errorf("art: destination has a pending recovery: %s", name)
		}
	}
	for _, open := range d.docs.AssetDocuments() {
		if open != d.owner && (open.Path == d.path || open.Asset == d.meta.ID) {
			pending := open.Binding.PendingRecovery != nil && open.Binding.PendingRecovery()
			if !d.replacement || open.Dirty() || pending || open.Binding.Model != nil || open.Binding.Close != nil || open.Binding.Save != nil || open.Source != nil || open.Scene != nil || !open.Capabilities.Has(document.EditSource) {
				return fmt.Errorf("art: close or resolve the open destination before saving: %s", d.path)
			}
		}
	}
	if d.meta.ID == d.owner.Asset || d.meta.ID.IsZero() || asset.IsBuiltin(d.meta.ID) {
		return fmt.Errorf("art: destination identity is already in use")
	}
	pending, err := d.docs.HasAssetRecovery(d.meta.ID)
	if err != nil {
		return err
	}
	if pending {
		return fmt.Errorf("art: destination has a pending recovery: %s", d.path)
	}
	for _, entry := range []struct {
		path string
		data []byte
	}{{d.path, d.before}, {d.path + pipeline.MetaSuffix, d.metaBefore}} {
		got, err := d.pipe.SourceBytes(entry.path)
		if err != nil || !sameArtSource(got, entry.data) {
			return fmt.Errorf("%s", i18n.Tr("editor.art.replace_changed", "The destination changed while saving. Review it and try again."))
		}
	}
	if d.replacement {
		id, ok := d.pipe.Lookup(d.path)
		if !ok || id != d.meta.ID {
			return fmt.Errorf("art: existing destination identity is not current; reimport it before replacing")
		}
		if err := d.pipe.VerifyCurrent(id); err != nil {
			return fmt.Errorf("art: resolve the existing destination before replacing: %w", err)
		}
		for _, id := range d.pipe.Dependents(id) {
			if open := d.docs.AssetDocument(id); open != nil && (open.Dirty() || open.Binding.PendingRecovery != nil && open.Binding.PendingRecovery()) {
				return fmt.Errorf("art: a dependent asset has unsaved changes or pending recovery: %s", open.Path)
			}
			if pending, err := d.docs.HasAssetRecovery(id); err != nil {
				return err
			} else if pending {
				return fmt.Errorf("art: a dependent asset has a pending recovery: %s", id.String())
			}
		}
	} else {
		if _, ok := d.pipe.Entry(d.meta.ID); ok {
			return fmt.Errorf("art: destination identity is already indexed")
		}
		if _, ok := d.pipe.PathOf(d.meta.ID); ok {
			return fmt.Errorf("art: destination identity is already indexed")
		}
	}
	return nil
}

func (d *artSaveTarget) edits(encoded []byte) []pipeline.SourceEdit {
	metaAfter := d.metaBefore
	if !d.replacement {
		metaAfter = pipeline.WriteMeta(d.meta)
	}
	return []pipeline.SourceEdit{
		{Path: d.path, Before: d.before, After: encoded},
		{Path: d.path + pipeline.MetaSuffix, Before: d.metaBefore, After: metaAfter, ReadOnlyGuard: d.replacement},
	}
}

func (d *artSaveTarget) confirm(publish func()) {
	if !d.replacement {
		publish()
		return
	}
	d.ed.Ask(context.Question{
		Title:   i18n.Tr("editor.art.replace_title", "Replace existing drawing?"),
		Message: i18n.Tr("editor.art.replace_notice", "Replace {path}? Its current drawing will be overwritten. The asset identity is kept, so all users of this shared asset can change.", i18n.Str("path", d.path)),
		Buttons: []string{i18n.Tr("editor.art.replace", "Replace"), i18n.Tr("editor.art.cancel", "Cancel")},
		Then: func(choice int) {
			if choice == 0 {
				publish()
			}
		},
	})
}
