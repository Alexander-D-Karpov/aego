package editor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"aego/art"
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/artedit"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
	"aego/project"
)

// A stable, project-local recovery identity lets a later editor session offer
// the previous blank drawing explicitly. It is never published as an asset ID.
// An actual indexed asset using this value blocks the blank rather than being
// confused with it. Recovery files remain in the project's existing directory.
var artWorkspaceBlankID = ids.AssetID{Hi: 0xe49fba64808b3ec1, Lo: 0x8b53129d8c996c02}

type blankArtDraft struct {
	ed      *context.Editor
	docs    *document.Manager
	pipe    *pipeline.Pipeline
	project *project.Project
	doc     *document.AssetDocument
	model   *artedit.Document
}

func newWorkspaceBlank(ed *context.Editor) (*blankArtDraft, error) {
	if ed == nil || ed.Docs == nil || ed.Pipeline == nil || ed.Project == nil {
		return nil, fmt.Errorf("art: an open project is required for the blank canvas")
	}
	b := &blankArtDraft{ed: ed, docs: ed.Docs, pipe: ed.Pipeline, project: ed.Project}
	if asset.IsBuiltin(artWorkspaceBlankID) {
		return nil, fmt.Errorf("art: the blank canvas identity conflicts with a built-in asset")
	}
	if _, ok := b.pipe.Entry(artWorkspaceBlankID); ok {
		return nil, fmt.Errorf("art: the blank canvas identity is already used by a project asset")
	}
	if _, ok := b.pipe.PathOf(artWorkspaceBlankID); ok {
		return nil, fmt.Errorf("art: the blank canvas identity is already used by a project source")
	}
	name := i18n.Tr("editor.art.blank_title", "Untitled drawing")
	doc, err := b.docs.NewTransientAsset(artWorkspaceBlankID, name, artHandlerID, asset.ArtTexture, document.View|document.EditSource|document.Preview)
	if err != nil {
		return nil, err
	}
	b.doc = doc
	pixels, err := art.New(32, 32, art.RGBA, art.DefaultLimits())
	if err != nil {
		_ = b.docs.CloseAsset(doc.Asset, false)
		return nil, err
	}
	key := sha256.Sum256([]byte("aego-art-workspace-blank-recovery-v1"))
	b.model, err = artedit.BindNew(b.docs, doc, pixels, hex.EncodeToString(key[:]), b.verify)
	if err != nil {
		_ = b.docs.CloseAsset(doc.Asset, false)
		return nil, err
	}
	return b, nil
}

func (b *blankArtDraft) verify() error {
	if b.ed.Docs != b.docs || b.ed.Pipeline != b.pipe || b.ed.Project != b.project || b.doc == nil || !b.doc.Transient || b.docs.AssetDocument(b.doc.Asset) != b.doc || b.doc.Asset != artWorkspaceBlankID {
		return fmt.Errorf("art: the project or blank drawing changed")
	}
	if err := b.pipe.CheckSourceRecovery(); err != nil {
		return err
	}
	if _, ok := b.pipe.Entry(b.doc.Asset); ok {
		return fmt.Errorf("art: the blank canvas identity is now used by a project asset")
	}
	if _, ok := b.pipe.PathOf(b.doc.Asset); ok {
		return fmt.Errorf("art: the blank canvas identity is now used by a project source")
	}
	return nil
}

func (b *blankArtDraft) askSave(panel *artPanel, transferred func(*document.AssetDocument) error) error {
	if err := b.verify(); err != nil {
		return err
	}
	if b.model.PendingRecovery || b.model.Session.Info().Gesture {
		return fmt.Errorf("art: resolve recovery and finish the current gesture before saving")
	}
	session := b.model.Session
	snapshot, err := session.Document.Snapshot()
	if err != nil {
		return err
	}
	encoded, err := snapshot.Encode()
	if err != nil {
		return err
	}
	revision := b.model.PreviewRevision()
	current := func() bool {
		return b.doc.Binding.Model == b.model && b.model.Draft && !b.model.PendingRecovery && b.model.Session == session && !session.Info().Gesture && b.model.PreviewCurrent(revision)
	}
	b.ed.Ask(context.Question{
		Title:            i18n.Tr("editor.art.save_blank", "Save drawing as an asset"),
		Message:          i18n.Tr("editor.art.save_blank_notice", "Choose a project source. Replacing an existing drawing requires confirmation."),
		Fields:           []string{i18n.Tr("editor.art.source_path", "Source path")},
		Values:           []string{assetops.UniqueFile(b.ed, "assets", "drawing", art.SourceExtension)},
		Buttons:          []string{i18n.Tr("editor.art.save", "Save"), i18n.Tr("editor.art.cancel", "Cancel")},
		DestructiveLabel: i18n.Tr("editor.art.replace", "Replace"),
		DestructiveWhen:  func(values []string) bool { return artSaveReplaces(b.ed, values) },
		Submit: func(values []string) {
			fail := func(err error) { panel.report(err) }
			if len(values) != 1 || !current() {
				fail(art.ErrConflict)
				return
			}
			if err := b.verify(); err != nil {
				fail(err)
				return
			}
			target, err := prepareArtSaveTarget(b.ed, b.doc, strings.TrimSpace(values[0]), snapshot)
			if err != nil {
				fail(err)
				return
			}
			target.confirm(func() {
				if !current() {
					fail(art.ErrConflict)
					return
				}
				if err := b.verify(); err != nil {
					fail(err)
					return
				}
				if err := target.verify(); err != nil {
					fail(err)
					return
				}
				edits := target.edits(encoded)
				var batch *pipeline.Batch
				err = session.Document.Publish(snapshot, func() error {
					var err error
					batch, err = b.pipe.ApplySources(edits, false)
					return err
				})
				if err != nil {
					fail(err)
					return
				}
				b.ed.ApplyBatch(batch)
				next, err := b.ed.DocumentForAsset(target.meta.ID)
				if err == nil {
					err = b.model.RebindSaved(next, snapshot)
				}
				if err != nil {
					fail(fmt.Errorf("art: the source was published, but the drawing remains open as an unsaved draft: %w", err))
					return
				}
				if err := b.docs.ClearOwnedAssetRecovery(b.doc); err != nil {
					b.ed.Notify(context.LevelWarn, err.Error())
				}
				if err := transferred(next); err != nil {
					fail(err)
					return
				}
				b.ed.Notify(context.LevelInfo, i18n.Tr("editor.art.blank_saved", "Saved drawing: {path}", i18n.Str("path", target.path)))
			})
		},
	})
	return document.ErrSavePending
}
