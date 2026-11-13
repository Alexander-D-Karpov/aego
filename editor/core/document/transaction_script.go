package document

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"unicode/utf8"

	"aego/core/errs"
	"aego/editor/core/code"
)

// The existing source transaction bounds metadata at 16 MiB. This limit is
// deliberately local to this optional adoption path, not a new limit on the
// ordinary text editor or permission to raise an importer's source limit.
const maxTransactionScriptBytes = 16 << 20

// DirtyScriptTransaction captures one existing shared text journal, its exact
// disk preimage and its current dirty postimage. It does not write files or
// guard any associated source. The caller must arrange a guarded transaction
// for every related file and handle a failure after publication explicitly.
// Like other filesystem content checks, this is not an atomic CAS.
// Document-manager operations belong on the manager's owner thread.
type DirtyScriptTransaction struct {
	owner      *Manager
	doc        *ScriptDocument
	journal    *code.Doc
	id         ID
	path       string
	revision   uint64
	savedEdits uint64
	before     []byte
	after      []byte
	accepted   bool
}

// Images rechecks the captured journal and disk preimage immediately before a
// caller's transaction and returns independent copies. It does not lock files
// or other documents: the caller must run its guarded transaction on the same
// owner thread. An importer-canonicalized postimage will not be adopted unless
// its bytes are still exactly this prepared postimage.
func (t *DirtyScriptTransaction) Images() (before, after []byte, err error) {
	if err := t.validate(); err != nil {
		return nil, nil, err
	}
	if err := t.owner.checkScriptTransactionImage(t.path, t.before); err != nil {
		return nil, nil, err
	}
	if !t.current() {
		return nil, nil, transactionScriptError("source document changed before publishing the transaction")
	}
	return bytes.Clone(t.before), bytes.Clone(t.after), nil
}

func transactionScriptError(message string) error {
	return errs.New(errs.EditorDocument, message)
}

// TrackedScriptSnapshot returns the exact saved bytes backing a clean, open
// journal, including original line endings. It neither saves nor changes the
// journal or recovery. Disk identity/content checks are best-effort, not CAS.
func (m *Manager) TrackedScriptSnapshot(d *ScriptDocument) ([]byte, error) {
	if m == nil || d == nil || d.Code == nil || m.Script(d.Path) != d ||
		!d.OnDisk || !d.diskTracked || d.Conflict || d.Dirty() || m.relatedVolatile(d) || d.Code.Len() > maxTransactionScriptBytes {
		return nil, transactionScriptError("source document has no current clean disk snapshot")
	}
	journal, revision, hash := d.Code, d.Code.Rev(), d.diskHash
	f, err := m.OpenSource(d.Path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, maxTransactionScriptBytes+1))
	if err := errors.Join(readErr, f.Close()); err != nil {
		return nil, err
	}
	if len(data) > maxTransactionScriptBytes || sha256.Sum256(data) != hash ||
		m.Script(d.Path) != d || d.Code != journal || d.Code.Rev() != revision ||
		!d.OnDisk || !d.diskTracked || d.diskHash != hash || d.Conflict || d.Dirty() || m.relatedVolatile(d) ||
		!bytes.Equal(normalizeScriptText(data), d.Code.Bytes()) {
		return nil, transactionScriptError("source document changed while reading its disk snapshot")
	}
	return data, nil
}

// PrepareDirtyScriptTransaction captures a dirty document only when before is
// both its tracked disk checkpoint and the current exact disk content. before
// must be non-nil even for an empty file: source transactions use nil to mean
// an absent path. No source is published, recovery removed, or journal changed.
func (m *Manager) PrepareDirtyScriptTransaction(d *ScriptDocument, before []byte) (*DirtyScriptTransaction, error) {
	if m == nil || d == nil || d.Code == nil || d.Code.VolatileDirty() || m.relatedVolatile(d) || d.ReadOnly || d.Conflict || !d.OnDisk || !d.diskTracked ||
		m.Script(d.Path) != d || before == nil || len(before) > maxTransactionScriptBytes || d.Code.Len() > maxTransactionScriptBytes || !d.Dirty() ||
		sha256.Sum256(before) != d.diskHash {
		return nil, transactionScriptError("source document cannot prepare a dirty transaction")
	}
	t := &DirtyScriptTransaction{owner: m, doc: d, journal: d.Code, id: d.ID, path: d.Path,
		revision: d.Code.Rev(), savedEdits: d.SavedEdits, before: bytes.Clone(before), after: d.Code.Bytes()}
	if !utf8.Valid(t.after) || bytes.IndexByte(t.after, 0) >= 0 || bytes.Equal(t.after, normalizeScriptText(t.before)) {
		return nil, transactionScriptError("dirty transaction needs a changed UTF-8 text postimage")
	}
	if err := t.validate(); err != nil {
		return nil, err
	}
	if err := m.checkScriptTransactionImage(t.path, t.before); err != nil {
		return nil, err
	}
	if !t.current() {
		return nil, transactionScriptError("source document changed while preparing the transaction")
	}
	return t, nil
}

func (t *DirtyScriptTransaction) current() bool {
	if t == nil || t.owner == nil || t.doc == nil || t.journal == nil || t.accepted {
		return false
	}
	d := t.doc
	return d.ID == t.id && d.Path == t.path && t.owner.Script(t.path) == d && d.Code == t.journal &&
		!d.ReadOnly && !d.Conflict && !t.owner.relatedVolatile(d) && d.OnDisk && d.diskTracked && d.diskHash == sha256.Sum256(t.before) &&
		d.SavedEdits == t.savedEdits && d.Code.Rev() == t.revision && d.Dirty() && bytes.Equal(d.Code.Bytes(), t.after)
}

func (t *DirtyScriptTransaction) validate() error {
	if !t.current() {
		return transactionScriptError("source document changed during the transaction")
	}
	if validate := t.doc.ValidateSave; validate != nil {
		if err := validate(bytes.Clone(t.after)); err != nil {
			return err
		}
	}
	if !t.current() {
		return transactionScriptError("source document changed during transaction validation")
	}
	return nil
}

func (m *Manager) checkScriptTransactionImage(path string, expected []byte) error {
	f, err := m.OpenSource(path)
	if err != nil {
		return err
	}
	actual, readErr := io.ReadAll(io.LimitReader(f, int64(len(expected))+1))
	if err := errors.Join(readErr, f.Close()); err != nil {
		return err
	}
	if !bytes.Equal(actual, expected) {
		return transactionScriptError("source file no longer matches the transaction image")
	}
	return nil
}

// AcceptDirtyScriptTransaction adopts only the exact prepared postimage after
// the caller's transaction has published it. It marks the existing journal's
// checkpoint clean without adding an edit or deleting recovery, so Undo after
// Save becomes dirty again. It emits no save/import event: the transaction
// caller owns publication and must not reimport the same edit a second time.
// A failed acceptance never rewrites the disk or clears the working journal.
func (m *Manager) AcceptDirtyScriptTransaction(t *DirtyScriptTransaction) error {
	if m == nil || t == nil || t.owner != m {
		return transactionScriptError("dirty source transaction belongs to another document manager")
	}
	if err := t.validate(); err != nil {
		return err
	}
	if err := m.checkScriptTransactionImage(t.path, t.after); err != nil {
		return err
	}
	if !t.current() {
		return transactionScriptError("source document changed before accepting the transaction")
	}
	d := t.doc
	d.Code.MarkSaved()
	d.SavedEdits++
	d.diskHash, d.diskTracked = sha256.Sum256(t.after), true
	d.removedAck = false
	m.statScript(d)
	t.accepted = true
	return nil
}
