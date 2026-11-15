package editor

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"path"
	"strconv"
	"strings"

	"aego/art"
	"aego/asset"
	"aego/asset/pipeline"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
)

// This is a separate, explicitly confirmed conversion; the existing whole-PNG
// copy remains available. It cannot transfer the original scene's AssetRefs.
func askPNGFramesArtCopy(ed *context.Editor, doc *document.AssetDocument) error {
	if !canCopyPNGToArt(ed, doc) || doc.Metadata == nil {
		return fmt.Errorf("%s", i18n.Tr("editor.art.png_frames_no_metadata", "A project PNG with saved slice metadata is required."))
	}
	pl, docs, project := ed.Pipeline, ed.Docs, ed.Project
	sourcePath, sourceID, metadata := doc.Path, doc.Asset, doc.Metadata
	beforePNG, err := pl.SourceBytes(sourcePath)
	if err != nil {
		return err
	}
	beforeMeta, err := pl.SourceBytes(sourcePath + pipeline.MetaSuffix)
	if err != nil {
		return err
	}
	if beforePNG == nil || beforeMeta == nil {
		return fmt.Errorf("%s", i18n.Tr("editor.art.png_frames_no_metadata", "A project PNG with saved slice metadata is required."))
	}
	// Code documents normalize line endings; compare their actual content to the
	// saved sidecar without changing either file.
	normalized := bytes.ReplaceAll(bytes.ReplaceAll(beforeMeta, []byte("\r\n"), []byte("\n")), []byte("\r"), []byte("\n"))
	if metadata.Dirty() || metadata.Conflict || !bytes.Equal(normalized, metadata.Code.Bytes()) {
		return fmt.Errorf("%s", i18n.Tr("editor.art.png_frames_save_metadata", "Save or resolve the image metadata before copying its slices."))
	}
	meta, err := asset.ParseSourceMeta(beforeMeta, sourcePath+pipeline.MetaSuffix)
	if err != nil {
		return err
	}
	if meta.ID != sourceID || len(meta.Subs) < 2 {
		return fmt.Errorf("%s", i18n.Tr("editor.art.png_frames_no_metadata", "A project PNG with saved slice metadata is required."))
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(beforePNG))
	if err != nil {
		return err
	}
	rects := make([]image.Rectangle, 0, len(meta.Subs))
	for _, frame := range meta.Subs {
		r := frame.Rect // x, y, width, height
		if r[0] < 0 || r[1] < 0 || r[0] >= cfg.Width || r[1] >= cfg.Height || r[2] <= 0 || r[3] <= 0 || r[2] > cfg.Width-r[0] || r[3] > cfg.Height-r[1] {
			return fmt.Errorf("%s", i18n.Tr("editor.art.png_frames_invalid_rect", "A saved slice is empty or outside the PNG."))
		}
		rects = append(rects, image.Rect(r[0], r[1], r[0]+r[2], r[1]+r[3]))
	}
	revision := metadata.Code.Rev()
	current := func() bool {
		return ed.Pipeline == pl && ed.Docs == docs && ed.Project == project &&
			docs.AssetDocument(sourceID) == doc && doc.Path == sourcePath && doc.Metadata == metadata &&
			!metadata.Dirty() && !metadata.Conflict && metadata.Code.Rev() == revision && bytes.Equal(metadata.Code.Bytes(), normalized)
	}
	stem := strings.TrimSuffix(path.Base(sourcePath), path.Ext(sourcePath)) + "-animation"
	fail := func(err error) { ed.Notify(context.LevelError, err.Error()) }
	submit := func(values []string) {
		if len(values) != 2 {
			return
		}
		if !current() {
			fail(fmt.Errorf("%s", i18n.Tr("editor.art.png_frames_changed", "The project, image document or its metadata changed; start the copy again.")))
			return
		}
		target := strings.TrimSpace(values[0])
		if !strings.EqualFold(path.Ext(target), art.SourceExtension) {
			fail(fmt.Errorf("image: editable copy must use .aeart"))
			return
		}
		duration, err := strconv.Atoi(strings.TrimSpace(values[1]))
		if err != nil || duration < 1 || duration > 600000 {
			fail(fmt.Errorf("%s", i18n.Tr("editor.art.png_frames_invalid_duration", "Frame duration must be an integer from 1 to 600000 ms.")))
			return
		}
		drawing, err := art.ImportPNGFrames(beforePNG, rects, duration, art.DefaultLimits())
		if err != nil {
			fail(err)
			return
		}
		defer drawing.Close()
		encoded, err := drawing.Encode()
		if err != nil {
			fail(err)
			return
		}
		if !current() {
			fail(fmt.Errorf("%s", i18n.Tr("editor.art.png_frames_changed", "The project, image document or its metadata changed; start the copy again.")))
			return
		}
		edits, err := newArtCopyEdits(pl, target, encoded)
		if err != nil {
			fail(err)
			return
		}
		// Strict guards keep both original files unchanged even if another
		// importer tries to normalize the PNG sidecar during this operation.
		edits = append(edits,
			pipeline.SourceEdit{Path: sourcePath, Before: beforePNG, After: beforePNG, ReadOnlyGuard: true},
			pipeline.SourceEdit{Path: sourcePath + pipeline.MetaSuffix, Before: beforeMeta, After: beforeMeta, ReadOnlyGuard: true})
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
	}
	// A full confirmation is separate from the path prompt: Confirm wraps and
	// scrolls long translated messages, whereas prompt labels are one line.
	ed.Ask(context.Question{
		Title:   i18n.Tr("editor.art.png_frames_copy", "Create animation from PNG slices…"),
		Message: i18n.Tr("editor.art.png_frames_notice", "Creates a separate .aeart from saved slices in metadata order. Choose one duration for all frames. Only color pixels are copied: frame names, IDs, pivots, normal/height/emission channels, materials and existing animations are not transferred. Original PNG, metadata and scene references stay unchanged; assign the new resources explicitly."),
		Buttons: []string{i18n.Tr("editor.shell.continue", "Continue"), i18n.Tr("editor.art.cancel", "Cancel")},
		Then: func(choice int) {
			if choice != 0 {
				return
			}
			if !current() {
				fail(fmt.Errorf("%s", i18n.Tr("editor.art.png_frames_changed", "The project, image document or its metadata changed; start the copy again.")))
				return
			}
			ed.Ask(context.Question{
				Title: i18n.Tr("editor.art.png_frames_copy", "Create animation from PNG slices…"),
				Fields: []string{
					i18n.Tr("editor.art.source_path", "Source path"),
					i18n.Tr("editor.art.png_frames_duration", "Frame duration (ms)"),
				},
				Values:  []string{assetops.UniqueFile(ed, path.Dir(sourcePath), stem, art.SourceExtension), "100"},
				Buttons: []string{i18n.Tr("editor.art.create", "Create"), i18n.Tr("editor.art.cancel", "Cancel")},
				Submit:  submit,
			})
		},
	})
	return nil
}
