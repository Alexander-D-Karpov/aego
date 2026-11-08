package artedit

import (
	"aego/art"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/engine/i18n"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
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
	b := &Document{Asset: d, manager: m, source: source, limits: lim}
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
	d.Binding = document.AssetBinding{
		Model:           b,
		PreviewRevision: b.PreviewRevision,
		PreviewSnapshot: b.PreviewSnapshot,
		PreviewCurrent:  b.PreviewCurrent,
		PreviewSource:   func() []byte { data, _, _ := b.PreviewSnapshot(); return data },
		Dirty:           func() bool { return b.Session.Info().Dirty },
		Save:            b.Save, Undo: b.SessionUndo, Redo: b.SessionRedo,
		CanUndo:         func() bool { return b.History.CanUndo() || b.Session.Info().Gesture },
		CanRedo:         func() bool { return b.History.CanRedo() },
		Close:           b.Close,
		Recovery:        b.Recovery,
		PendingRecovery: func() bool { return b.PendingRecovery },
	}
	return b, nil
}
func (b *Document) install(d *art.Document, dirty bool) error {
	h := history.New(nil)
	h.SetLimit(b.limits.MaxHistorySteps)
	s, err := art.NewSession(d, &historyHost{history: h, maxBytes: b.limits.MaxHistoryBytes})
	if err != nil {
		return err
	}
	if b.Session != nil {
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
func (b *Document) Save() error {
	if err := b.checkEdit(); err != nil {
		return err
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
	return json.Marshal(recovery{1, b.source.SHA256(), data})
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
	if r.Version != 1 || r.SourceSHA != b.source.SHA256() {
		return fmt.Errorf("art: recovery conflicts with source on disk")
	}
	if err = b.source.Check(); err != nil {
		return err
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
