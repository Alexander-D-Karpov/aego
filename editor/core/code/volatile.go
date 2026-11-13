package code

import (
	"bytes"
	"errors"
)

// These payload limits are independent of the text editor's history limit.
// History accounting deliberately counts shared snapshots more than once; it
// does not include allocator rounding, text history or other process memory.
const (
	maxVolatileBytes        = 4 << 20
	maxVolatileHistoryBytes = 64 << 20
)

type volatileState struct {
	current []byte
	budget  int64
}

// AttachVolatile enables one initially empty, in-memory state alongside this
// text journal. It is not serialized by Bytes or MarkSaved. The document owner
// must guard its save, reload, close and recovery routes while it is dirty.
// This is owner-thread state, not a synchronization mechanism or a second log.
func (d *Doc) AttachVolatile() error {
	if d == nil || d.volatile != nil {
		return errors.New("volatile document state is already attached or unavailable")
	}
	d.volatile = &volatileState{budget: maxVolatileHistoryBytes}
	return nil
}

// Volatile returns a private copy. A nil value with attached=true is the clean
// initial state; nil and empty payloads are treated as equivalent.
func (d *Doc) Volatile() (value []byte, attached bool) {
	if d == nil || d.volatile == nil {
		return nil, false
	}
	return bytes.Clone(d.volatile.current), true
}

// VolatileDirty remains true until the in-memory state returns to its empty
// initial value. MarkSaved and SetSaved acknowledge text only.
func (d *Doc) VolatileDirty() bool {
	return d != nil && d.volatile != nil && len(d.volatile.current) != 0
}

// ReplaceVolatile creates one undo step in the text document's existing log.
// expectedRev guards both text and volatile changes since the caller captured
// its snapshot. It does not check external files or grant permission to save.
// Oversized input or retained history is rejected without changing either log.
func (d *Doc) ReplaceVolatile(expectedRev uint64, value []byte) (bool, error) {
	if d == nil || d.volatile == nil {
		return false, errors.New("volatile document state is not attached")
	}
	if d.rev != expectedRev {
		return false, errors.New("document changed before volatile edit")
	}
	if len(value) > maxVolatileBytes {
		return false, errors.New("volatile document state exceeds its size budget")
	}
	if bytes.Equal(d.volatile.current, value) {
		return false, nil
	}
	if d.rev == ^uint64(0) {
		return false, errors.New("document revision exhausted")
	}
	// Redo entries are about to be released. Counting each retained reference,
	// rather than deduplicating backing arrays, gives a conservative upper bound.
	used := int64(len(d.volatile.current))
	for _, g := range d.undo {
		if g.volatile {
			used += int64(len(g.volatileBefore)) + int64(len(g.volatileAfter))
			if used > d.volatile.budget {
				return false, errors.New("volatile document history exceeds its memory budget")
			}
		}
	}
	needed := int64(len(d.volatile.current)) + 2*int64(len(value))
	if used > d.volatile.budget-needed {
		return false, errors.New("volatile document history exceeds its memory budget")
	}
	next := bytes.Clone(value)
	d.clearRedo()
	d.seal()
	d.undo = append(d.undo, group{kind: KindOther, before: d.Sel, after: d.Sel,
		sealed: true, volatile: true, volatileBefore: d.volatile.current, volatileAfter: next})
	d.trimUndo()
	d.volatile.current = next
	d.advanceVolatile()
	return true, nil
}

func (d *Doc) advanceVolatile() {
	d.rev++
	// The text has not moved, but MapSpan's bounded revision journal must still
	// contain one entry for each revision so its coverage check stays sound.
	d.journal(0, 0, 0)
}
