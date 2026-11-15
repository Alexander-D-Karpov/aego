package artedit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"

	"aego/art"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/engine/i18n"
)

// Document is owned by AssetBinding.Model. View panels, Inspector and agents
// resolve this same object, including unsaved pixels, instead of reopening disk.
// Session and History calls are serialized on Aego's UI/mutation thread.
type Document struct {
	Asset                               *document.AssetDocument
	Session                             *art.Session
	History                             *history.History
	manager                             *document.Manager
	source                              *document.BinarySource
	limits                              art.Limits
	closed                              bool
	Draft                               bool
	recoveryHash                        string
	recoveryCheck                       func() error
	PendingRecovery                     bool
	previewDocument                     *art.Document
	previewArtRevision, previewRevision uint64
}

func Bind(m *document.Manager, d *document.AssetDocument) (*Document, error) {
	if m == nil || d == nil {
		return nil, fmt.Errorf("art: document manager and asset are required")
	}
	if b, ok := d.Binding.Model.(*Document); ok && !b.closed {
		return b, nil
	}
	lim := art.DefaultLimits()
	source, data, err := m.OpenBinarySource(d, lim.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	pixels, err := art.Decode(data, lim)
	if err != nil {
		return nil, err
	}
	return bindPixels(m, d, source, pixels, "", nil)
}

// BindImported takes ownership of pixels, including on error. The original
// source is only a baseline for recovery/conflict checks, never a save target.
// inputHash covers all importer inputs; verify guards them before recovery.
func BindImported(m *document.Manager, d *document.AssetDocument, pixels *art.Document, expected []byte, inputHash string, verify func() error) (*Document, error) {
	if pixels == nil {
		return nil, fmt.Errorf("art: imported pixels are required")
	}
	if m == nil || d == nil || d.Binding.Model != nil || inputHash == "" || verify == nil {
		pixels.Close()
		return nil, fmt.Errorf("art: invalid imported document binding")
	}
	source, data, err := m.OpenBinarySource(d, art.DefaultLimits().MaxFileBytes)
	if err != nil || !bytes.Equal(data, expected) {
		pixels.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("art: source changed during import")
	}
	if err := verify(); err != nil {
		pixels.Close()
		return nil, err
	}
	return bindPixels(m, d, source, pixels, inputHash, verify)
}

// BindNew takes ownership of an in-memory document. Its stable recovery key
// and read-only validation belong to the host; there is no disk source to read
// or overwrite before an explicit first Save.
func BindNew(m *document.Manager, d *document.AssetDocument, pixels *art.Document, recoveryKey string, verify func() error) (*Document, error) {
	if pixels == nil {
		return nil, fmt.Errorf("art: new pixels are required")
	}
	if m == nil || d == nil || !d.Transient || m.AssetDocument(d.Asset) != d || d.Binding.Model != nil || recoveryKey == "" || verify == nil {
		pixels.Close()
		return nil, fmt.Errorf("art: invalid new document binding")
	}
	if err := verify(); err != nil {
		pixels.Close()
		return nil, err
	}
	return bindPixels(m, d, nil, pixels, recoveryKey, verify)
}

func bindPixels(m *document.Manager, d *document.AssetDocument, source *document.BinarySource, pixels *art.Document, inputHash string, verify func() error) (*Document, error) {
	lim := art.DefaultLimits()
	b := &Document{Asset: d, manager: m, source: source, limits: lim, Draft: inputHash != "", recoveryHash: inputHash, recoveryCheck: verify}
	var err error
	if err = b.install(pixels, false); err != nil {
		pixels.Close()
		return nil, err
	}
	if _, recoveryErr := m.ReadAssetRecovery(d, lim.MaxFileBytes*2); recoveryErr == nil {
		b.PendingRecovery = true
	} else if !errors.Is(recoveryErr, os.ErrNotExist) {
		b.Close()
		return nil, recoveryErr
	}
	b.attachBinding()
	return b, nil
}

func (b *Document) attachBinding() {
	d := b.Asset
	d.Binding = document.AssetBinding{
		Model:   b,
		Actions: b.editActions(),
		Dirty:   func() bool { return b.Session.Info().Dirty },
		Save:    b.Save, Undo: b.SessionUndo, Redo: b.SessionRedo,
		CanUndo:         func() bool { return b.History.CanUndo() || b.Session.Info().Gesture },
		CanRedo:         func() bool { return b.History.CanRedo() },
		Close:           b.Close,
		Recovery:        b.Recovery,
		PendingRecovery: func() bool { return b.PendingRecovery },
	}
	if !b.Draft {
		d.Binding.PreviewRevision = b.PreviewRevision
		d.Binding.PreviewSnapshot = b.PreviewSnapshot
		d.Binding.PreviewCurrent = b.PreviewCurrent
		d.Binding.PreviewSource = func() []byte { data, _, _ := b.PreviewSnapshot(); return data }
	}
}

// RebindSaved moves the same session and history to a newly published source.
// It does not write or remove any file. Callers retain the old binding if any
// guard fails, even if the preceding transaction already published a copy.
func (b *Document) RebindSaved(dst *document.AssetDocument, snapshot *art.Snapshot) error {
	if b.closed || !b.Draft || b.PendingRecovery || b.Session.Info().Gesture || dst == nil || b.manager.AssetDocument(dst.Asset) != dst || dst.Binding.Model != nil || !dst.Capabilities.Has(document.EditSource) {
		return fmt.Errorf("art: cannot transfer the imported drawing to this source")
	}
	if _, err := b.manager.ReadAssetRecovery(dst, b.limits.MaxFileBytes*2); err == nil {
		return fmt.Errorf("art: the destination has a pending recovery")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	want, err := snapshot.Encode()
	if err != nil {
		return err
	}
	source, data, err := b.manager.OpenBinarySource(dst, b.limits.MaxFileBytes)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, want) {
		return fmt.Errorf("art: the new source changed before the drawing was transferred")
	}
	if err := b.Session.Document.Publish(snapshot, source.Check); err != nil {
		return err
	}
	if err := b.Session.MarkSnapshotSaved(snapshot); err != nil {
		return err
	}
	old := b.Asset
	old.Binding = document.AssetBinding{}
	b.Asset, b.source, b.Draft, b.recoveryHash, b.recoveryCheck = dst, source, false, "", nil
	b.attachBinding()
	b.History.MarkSaved()
	return nil
}

func (b *Document) editActions() map[document.EditAction]func() error {
	actions := make(map[document.EditAction]func() error)
	for _, action := range []document.EditAction{document.ActionCopy, document.ActionCut, document.ActionPaste, document.ActionDelete, document.ActionSelectAll} {
		actions[action] = func() error {
			if b.closed {
				return art.ErrClosed
			}
			if action == document.ActionSelectAll {
				return b.Session.SetSelection("all", nil, 0)
			}
			if action == document.ActionCopy {
				return b.Session.Copy()
			}
			if err := b.checkEdit(); err != nil {
				return err
			}
			if action == document.ActionPaste {
				at := image.Point{}
				if b.Session.Selection != nil {
					at = b.Session.Selection.Bounds().Min
				}
				return b.Session.Paste(at)
			}
			if action == document.ActionCut {
				if err := b.Session.Copy(); err != nil {
					return err
				}
			}
			_, err := b.Execute(context.Background(), art.Operation{Kind: "clear"})
			return err
		}
	}
	return actions
}

func (b *Document) install(d *art.Document, dirty bool) error {
	h := history.New(nil)
	h.SetLimit(b.limits.MaxHistorySteps)
	s, err := art.NewSession(d, &historyHost{history: h, maxBytes: b.limits.MaxHistoryBytes})
	if err != nil {
		return err
	}
	if b.Session != nil {
		// Recovery replaces the pixel document, not the user's navigation.
		// A layer or frame may legitimately have been deleted in the recovered
		// copy; retain each still-valid selection independently and otherwise
		// keep the new session's safe default.
		_ = s.Select(b.Session.Layer, "", "")
		_ = s.Select("", b.Session.Frame, "")
		_ = s.Select("", "", b.Session.Channel)
		b.Session.CancelGesture()
		b.Session.Document.Close()
		b.History.Clear()
	}
	b.Session, b.History = s, h
	if dirty {
		s.MarkUnsaved()
	} else {
		h.MarkSaved()
	}
	return nil
}
func (b *Document) checkWritable() error {
	if b.closed {
		return art.ErrClosed
	}
	if !b.Asset.Capabilities.Has(document.EditSource) {
		return fmt.Errorf("art: source is read-only")
	}
	return nil
}
func (b *Document) checkEdit() error {
	if err := b.checkWritable(); err != nil {
		return err
	}
	if b.PendingRecovery {
		return fmt.Errorf("%s", i18n.Tr("editor.art.recovery_pending_edit", "Recover or discard the previous recovery before editing this drawing."))
	}
	return nil
}

// CheckDraftInputs is read-only. It allows other authoring workflows to reject
// a draft whose original image, slicing or surface inputs have changed.
func (b *Document) CheckDraftInputs() error {
	if b.closed {
		return art.ErrClosed
	}
	if b.Draft && b.recoveryCheck != nil {
		return b.recoveryCheck()
	}
	return nil
}
func (b *Document) Save() error {
	if err := b.checkEdit(); err != nil {
		return err
	}
	if b.Draft {
		return fmt.Errorf("art: an imported image needs an explicit destination before saving")
	}
	if b.Session.Info().Gesture {
		return fmt.Errorf("art: finish or cancel the current gesture before saving")
	}
	snapshot, err := b.Session.Document.Snapshot()
	if err != nil {
		return err
	}
	data, err := snapshot.Encode()
	if err != nil {
		return err
	}
	if err = b.Session.Document.Publish(snapshot, func() error { return b.source.Commit(data) }); err != nil {
		return err
	}
	if err = b.Session.MarkSnapshotSaved(snapshot); err != nil {
		return err
	}
	b.History.MarkSaved()
	// Import publication must not be suppressed by recovery-file cleanup failure.
	b.source.NotifySaved()
	b.PendingRecovery = false
	return b.manager.ClearOwnedAssetRecovery(b.Asset)
}
func (b *Document) SessionUndo() error {
	if err := b.checkEdit(); err != nil {
		return err
	}
	return b.Session.Undo()
}
func (b *Document) SessionRedo() error {
	if err := b.checkEdit(); err != nil {
		return err
	}
	return b.Session.Redo()
}
func (b *Document) Close() {
	if b.closed {
		return
	}
	b.closed = true
	if b.Session != nil {
		b.Session.ClearPreview()
	}
	b.Session.CancelGesture()
	b.Session.Document.Close()
	b.History.Clear()
}
func (b *Document) Execute(ctx context.Context, op art.Operation) (art.ID, error) {
	if err := b.checkEdit(); err != nil {
		return "", err
	}
	return b.Session.Execute(ctx, op)
}

// Proposal is prepared without mutation. Accept must be routed through the
// existing confirmation/Mutation Service; it is not an implicit auto-approval.
type Proposal struct {
	owner    *Document
	change   *art.Change
	Revision uint64
	Results  []art.ID
}

func (b *Document) Propose(ctx context.Context, revision uint64, label string, ops []art.Operation) (*Proposal, error) {
	if err := b.checkEdit(); err != nil {
		return nil, err
	}
	if b.Session.Info().Gesture {
		return nil, fmt.Errorf("art: a user gesture is in progress")
	}
	change, results, err := b.Session.Document.Propose(ctx, revision, label, ops)
	if err != nil {
		return nil, err
	}
	return &Proposal{owner: b, change: change, Revision: revision, Results: results}, nil
}
func (b *Document) Accept(p *Proposal) error {
	if err := b.checkEdit(); err != nil {
		return err
	}
	if p == nil || p.owner != b || p.Revision != b.Session.Document.Revision() {
		return art.ErrConflict
	}
	if b.Session.Info().Gesture {
		return fmt.Errorf("art: a user gesture is in progress")
	}
	return b.Session.History.Exec(p.change)
}
func (b *Document) Capture(frame art.ID, channel art.Channel) (*image.NRGBA, uint64, error) {
	if b.closed {
		return nil, 0, art.ErrClosed
	}
	s, err := b.Session.Document.Snapshot()
	if err != nil {
		return nil, 0, err
	}
	pixels, err := s.Render(frame, art.RenderOptions{Channel: channel})
	return pixels, s.Revision(), err
}

type recovery struct {
	Version   int    `json:"version"`
	SourceSHA string `json:"sourceSHA256"`
	Data      []byte `json:"data"`
}

func (b *Document) Recovery() ([]byte, error) {
	if b.closed {
		return nil, art.ErrClosed
	}
	if b.PendingRecovery {
		return nil, fmt.Errorf("%s", i18n.Tr("editor.art.recovery_pending_edit", "Recover or discard the previous recovery before editing this drawing."))
	}
	s, err := b.Session.Document.Snapshot()
	if err != nil {
		return nil, err
	}
	data, err := s.Encode()
	if err != nil {
		return nil, err
	}
	hash := b.recoveryHash
	if hash == "" {
		if b.source == nil {
			return nil, fmt.Errorf("art: recovery has no source identity")
		}
		hash = b.source.SHA256()
	}
	return json.Marshal(recovery{1, hash, data})
}

// Recover rejects disk conflicts; the UI must explicitly ask before calling it.
func (b *Document) Recover() error {
	if err := b.checkWritable(); err != nil {
		return err
	}
	data, err := b.manager.ReadAssetRecovery(b.Asset, b.limits.MaxFileBytes*2)
	if err != nil {
		return err
	}
	var r recovery
	if err = json.Unmarshal(data, &r); err != nil {
		return err
	}
	hash := b.recoveryHash
	if hash == "" {
		if b.source == nil {
			return fmt.Errorf("art: recovery has no source identity")
		}
		hash = b.source.SHA256()
	}
	if r.Version != 1 || r.SourceSHA != hash {
		return fmt.Errorf("art: recovery conflicts with source on disk")
	}
	if b.source != nil {
		if err = b.source.Check(); err != nil {
			return err
		}
	} else if !b.Draft || !b.Asset.Transient {
		return fmt.Errorf("art: recovery source is unavailable")
	}
	if b.recoveryCheck != nil {
		if err := b.recoveryCheck(); err != nil {
			return err
		}
	}
	d, err := art.Decode(r.Data, b.limits)
	if err != nil {
		return err
	}
	if err = b.install(d, true); err != nil {
		d.Close()
		return err
	}
	b.manager.TrackRestoredAssetRecovery(b.Asset, data)
	b.PendingRecovery = false
	return nil
}

// Bytes is the retained COW estimate used by the existing agent proposal queue.
func (p *Proposal) Bytes() int64 {
	if p == nil || p.change == nil {
		return 0
	}
	return p.change.Bytes()
}

func (b *Document) DiscardRecovery() error {
	if b.closed {
		return art.ErrClosed
	}
	if err := b.manager.ClearAssetRecovery(b.Asset); err != nil {
		return err
	}
	b.PendingRecovery = false
	return nil
}
