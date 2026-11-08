package editor

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"aego/art"
	"aego/asset/pipeline"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
)

// A PNG is not an Art Document. Conversion creates a separate source and does
// not replace the original or reassign any SpriteRenderer or sliced frames.
func canCopyPNGToArt(ed *context.Editor, doc *document.AssetDocument) bool {
	if ed == nil || ed.Pipeline == nil || ed.Docs == nil || doc == nil || !strings.EqualFold(path.Ext(doc.Path), ".png") {
		return false
	}
	rel, ok := ed.Pipeline.PathOf(doc.Asset)
	return ok && rel == doc.Path
}

func askPNGArtCopy(ed *context.Editor, doc *document.AssetDocument) error {
	if !canCopyPNGToArt(ed, doc) {
		return fmt.Errorf("image: a project PNG source is required")
	}
	pl, docs, project := ed.Pipeline, ed.Docs, ed.Project
	before, err := pl.SourceBytes(doc.Path)
	if err != nil {
		return err
	}
	if before == nil {
		return fmt.Errorf("image: PNG source no longer exists")
	}
	drawing, err := art.ImportPNG(before, art.DefaultLimits())
	if err != nil {
		return err
	}
	defer drawing.Close()
	encoded, err := drawing.Encode()
	if err != nil {
		return err
	}
	stem := strings.TrimSuffix(path.Base(doc.Path), path.Ext(doc.Path))
	defaultPath := assetops.UniqueFile(ed, path.Dir(doc.Path), stem, art.SourceExtension)
	ed.Ask(context.Question{
		Title:   i18n.Tr("editor.art.png_edit_copy", "Edit as .aeart copy…"),
		Message: i18n.Tr("editor.art.png_copy_notice", "Whole PNG only; the original and scene stay unchanged."),
		Fields:  []string{i18n.Tr("editor.art.source_path", "Source path")},
		Values:  []string{defaultPath},
		Buttons: []string{i18n.Tr("editor.art.create", "Create"), i18n.Tr("editor.art.cancel", "Cancel")},
		Submit: func(values []string) {
			if len(values) != 1 {
				return
			}
			fail := func(err error) { ed.Notify(context.LevelError, err.Error()) }
			if ed.Pipeline != pl || ed.Docs != docs || ed.Project != project || docs.AssetDocument(doc.Asset) != doc {
				fail(fmt.Errorf("image: project or document changed before creating the copy"))
				return
			}
			target := strings.TrimSpace(values[0])
			if !strings.EqualFold(path.Ext(target), art.SourceExtension) {
				fail(fmt.Errorf("image: editable copy must use .aeart"))
				return
			}
			current, err := pl.SourceBytes(doc.Path)
			if err != nil {
				fail(err)
				return
			}
			if current == nil || !bytes.Equal(before, current) {
				fail(fmt.Errorf("image: PNG source changed before creating the copy"))
				return
			}
			edits, err := newArtCopyEdits(pl, target, encoded)
			if err != nil {
				fail(err)
				return
			}
			// Keep the original image unchanged throughout import, not just while
			// the confirmation dialog is open.
			edits = append(edits, pipeline.SourceEdit{Path: doc.Path, Before: before, After: before})
			batch, err := pl.ApplySources(edits, false)
			if err != nil {
				fail(err)
				return
			}
			ed.ApplyBatch(batch)
			id, ok := pl.Lookup(target)
			if !ok {
				fail(fmt.Errorf("image: editable copy is not in the asset index"))
				return
			}
			if _, err := ed.OpenAsset(id); err != nil {
				fail(err)
			}
		},
	})
	return nil
}
