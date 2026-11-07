package history

// ByteCommand is optional: pixel commands report retained COW data while scene
// commands keep their established step-count limit. It is not another journal.
type ByteCommand interface{ HistoryBytes() int64 }

func entryBytes(e entry) int64 {
	var n int64
	for _, c := range e.cmds {
		if b, ok := c.(ByteCommand); ok {
			n += max(0, b.HistoryBytes())
		}
	}
	return n
}

// TrimBytes releases oldest undo entries to enforce an authoring memory budget.
// Call after a completed Exec, never inside a transaction. The closest undo and
// redo steps are retained. A fresh backing slice releases discarded COW images.
func (h *History) TrimBytes(limit int64) {
	if h.tx != nil || limit < 1 {
		return
	}
	var used int64
	for _, e := range h.undo {
		used += entryBytes(e)
	}
	for _, e := range h.redo {
		used += entryBytes(e)
	}
	drop := 0
	for used > limit && drop < len(h.undo) {
		used -= entryBytes(h.undo[drop])
		h.base = h.undo[drop].seq
		drop++
	}
	if drop > 0 {
		h.undo = append([]entry(nil), h.undo[drop:]...)
	}
	// The front of redo is the most distant future step.
	dropRedo := 0
	for used > limit && dropRedo < len(h.redo) {
		used -= entryBytes(h.redo[dropRedo])
		if h.redo[dropRedo].seq == h.saved {
			h.savedLost = true
		}
		dropRedo++
	}
	if dropRedo > 0 {
		h.redo = append([]entry(nil), h.redo[dropRedo:]...)
	}
	if len(h.redo) == 0 {
		h.redo = nil
	}
	if drop > 0 || dropRedo > 0 {
		h.notify()
	}
}
