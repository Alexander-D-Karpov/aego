package code

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash/fnv"
)

const (
	volatileJournalMagic          = "AEVJNL1\x00"
	maxVolatileJournalBytes       = 64 << 20
	maxVolatileJournalTextBytes   = 16 << 20
	maxVolatileJournalLines       = 1 << 16
	maxVolatileJournalRecords     = 1 << 13
	maxVolatileJournalReplayBytes = 256 << 20
	volatileJournalChunk          = 64 << 10
	maxVolatileJournalRevision    = ^uint64(0) / 2
)

var errVolatileJournal = errors.New("invalid or oversized volatile recovery journal")

// EncodeVolatileJournal returns a bounded, self-contained in-memory recovery
// candidate for the existing combined text/volatile Undo and Redo journal.
// saved is the exact saved text preimage, including its original line endings.
// The caller must verify that preimage and any associated source on disk. The
// checkpoint comparison uses Doc's existing non-cryptographic text hash;
// the SHA-256 in the candidate only binds the supplied bytes, not their origin
// or existence. This method reads and writes no files, grants no permission to
// save an EDL, and is not an atomic filesystem check. Access to d belongs to
// its owner thread. The returned bytes can contain private user data and need
// private storage; they must not be logged or sent as telemetry.
// The caller and context must remain valid until the method returns.
func (d *Doc) EncodeVolatileJournal(ctx context.Context, saved []byte) ([]byte, error) {
	if ctx == nil || d == nil || d.buf == nil || d.idx == nil || d.volatile == nil || d.batch != nil ||
		len(saved) > maxVolatileJournalTextBytes || d.Len() > maxVolatileJournalTextBytes || d.rev >= maxVolatileJournalRevision {
		return nil, errVolatileJournal
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h := fnv.New64a()
	for rest := normalize(saved); len(rest) > 0; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n := min(len(rest), volatileJournalChunk)
		_, _ = h.Write(rest[:n])
		rest = rest[n:]
	}
	if h.Sum64() != d.saved {
		return nil, errors.New("saved text does not match the journal checkpoint")
	}
	text := d.Bytes()
	if err := validateVolatileJournal(ctx, text, d.volatile.current, d.Sel, d.undo, d.redo); err != nil {
		return nil, err
	}
	w := volatileJournalWriter{ctx: ctx}
	w.raw([]byte(volatileJournalMagic))
	if d.GLSL {
		w.byte(1)
	} else {
		w.byte(0)
	}
	w.uint64(d.rev)
	digest, err := volatileJournalDigest(ctx, saved)
	if err != nil {
		return nil, err
	}
	w.raw(digest[:])
	w.blob(text)
	w.blob(d.volatile.current)
	w.selection(d.Sel)
	w.uint32(uint32(len(d.undo)))
	w.uint32(uint32(len(d.redo)))
	for _, groups := range [][]group{d.undo, d.redo} {
		for _, g := range groups {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			w.group(g)
			if w.err != nil {
				return nil, w.err
			}
		}
	}
	if w.err != nil {
		return nil, w.err
	}
	checksum, err := volatileJournalDigest(ctx, w.data)
	if err != nil {
		return nil, err
	}
	w.data = append(w.data, checksum[:]...)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return w.data, nil
}

// DecodeVolatileJournal validates a candidate and returns a new detached Doc;
// it never changes an existing document, reads files, or restores recovery.
// saved must be the same exact saved text preimage supplied at capture. A hash
// mismatch, malformed input, inconsistent history or exhausted budget refuses
// the entire candidate. The checksum is corruption detection, not authority.
// The caller must separately check the open document's identity and revision,
// source identity, disk preimages and an explicit user decision before adopting
// the returned Doc. Revisions belong to a particular Doc, not a global epoch;
// the owner must invalidate old requests and validate the domain-specific
// volatile payloads before use. Previously captured span mappings are invalid
// for the new Doc. Restored groups are sealed rather than merged with later
// typing or gestures.
// The caller must not mutate data or saved until this method returns.
func DecodeVolatileJournal(ctx context.Context, data, saved []byte) (*Doc, error) {
	if ctx == nil || len(saved) > maxVolatileJournalTextBytes || len(data) > maxVolatileJournalBytes ||
		len(data) < len(volatileJournalMagic)+1+8+sha256.Size+sha256.Size {
		return nil, errVolatileJournal
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body, checksum := data[:len(data)-sha256.Size], data[len(data)-sha256.Size:]
	sum, err := volatileJournalDigest(ctx, body)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(checksum, sum[:]) {
		return nil, errVolatileJournal
	}
	r := volatileJournalReader{ctx: ctx, data: body}
	magic := r.take(len(volatileJournalMagic))
	if r.err != nil {
		return nil, r.err
	}
	if string(magic) != volatileJournalMagic {
		return nil, errVolatileJournal
	}
	flags := r.byte()
	revision := r.uint64()
	savedHash := r.take(sha256.Size)
	if r.err != nil {
		return nil, r.err
	}
	actualHash, err := volatileJournalDigest(ctx, saved)
	if err != nil {
		return nil, err
	}
	if flags > 1 || revision >= maxVolatileJournalRevision || !bytes.Equal(savedHash, actualHash[:]) {
		return nil, errVolatileJournal
	}
	text := r.blob(maxVolatileJournalTextBytes)
	value := r.blob(maxVolatileBytes)
	selection := r.selection()
	undoCount, redoCount := r.uint32(), r.uint32()
	if r.err != nil {
		return nil, r.err
	}
	if uint64(undoCount)+uint64(redoCount) > historyLimit {
		return nil, errVolatileJournal
	}
	undo, redo := make([]group, int(undoCount)), make([]group, int(redoCount))
	records := 0
	for _, groups := range [][]group{undo, redo} {
		for i := range groups {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			groups[i] = r.group(&records)
			if r.err != nil {
				return nil, r.err
			}
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.offset != len(r.data) {
		return nil, errVolatileJournal
	}
	if err := validateVolatileJournal(ctx, text, value, selection, undo, redo); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d := New(text)
	d.GLSL, d.Sel, d.rev = flags == 1, selection, revision+1
	d.SetSaved(saved)
	d.volatile = &volatileState{current: bytes.Clone(value), budget: maxVolatileHistoryBytes}
	clonedUndo, err := cloneVolatileGroups(ctx, undo)
	if err != nil {
		return nil, err
	}
	clonedRedo, err := cloneVolatileGroups(ctx, redo)
	if err != nil {
		return nil, err
	}
	d.undo, d.redo = clonedUndo, clonedRedo
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return d, nil
}

func volatileJournalDigest(ctx context.Context, data []byte) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	h := sha256.New()
	for len(data) > 0 {
		if err := ctx.Err(); err != nil {
			return digest, err
		}
		n := min(len(data), volatileJournalChunk)
		_, _ = h.Write(data[:n])
		data = data[n:]
	}
	if err := ctx.Err(); err != nil {
		return digest, err
	}
	copy(digest[:], h.Sum(nil))
	return digest, nil
}

// validateVolatileJournal replays both branches from the current state. It
// checks every record's exact preimage rather than trusting offsets or merely
// accepting a checksum. Work and intermediate text sizes are bounded.
func validateVolatileJournal(ctx context.Context, text, value []byte, selection Selection, undo, redo []group) error {
	if len(text) > maxVolatileJournalTextBytes || len(value) > maxVolatileBytes ||
		bytes.IndexByte(text, '\r') >= 0 || bytes.Count(text, newline) >= maxVolatileJournalLines ||
		!volatileSelectionValid(selection, len(text)) ||
		len(undo)+len(redo) > historyLimit {
		return errVolatileJournal
	}
	volatileBytes, records := int64(len(value)), 0
	// Count a conservative wire budget before scanning or replaying the record
	// payloads. A live text journal can retain more data than its current text.
	wireBytes := int64(len(text)) + int64(len(value)) + 256
	for _, groups := range [][]group{undo, redo} {
		for _, g := range groups {
			if err := ctx.Err(); err != nil {
				return err
			}
			if g.kind > KindDelete {
				return errVolatileJournal
			}
			wireBytes += 64
			if g.volatile {
				if g.kind != KindOther || len(g.recs) != 0 || len(g.volatileBefore) > maxVolatileBytes ||
					len(g.volatileAfter) > maxVolatileBytes || bytes.Equal(g.volatileBefore, g.volatileAfter) {
					return errVolatileJournal
				}
				payload := int64(len(g.volatileBefore)) + int64(len(g.volatileAfter))
				volatileBytes += payload
				wireBytes += payload
				if volatileBytes > maxVolatileHistoryBytes || wireBytes > maxVolatileJournalBytes {
					return errVolatileJournal
				}
				continue
			}
			if len(g.recs) == 0 || len(g.recs) > maxVolatileJournalRecords-records ||
				g.volatileBefore != nil || g.volatileAfter != nil {
				return errVolatileJournal
			}
			records += len(g.recs)
			for _, rec := range g.recs {
				if err := ctx.Err(); err != nil {
					return err
				}
				wireBytes += 12 + int64(len(rec.removed)) + int64(len(rec.inserted))
				if wireBytes > maxVolatileJournalBytes {
					return errVolatileJournal
				}
				if rec.off < 0 || rec.off > maxVolatileJournalTextBytes || len(rec.removed) > maxVolatileJournalTextBytes ||
					len(rec.inserted) > maxVolatileJournalTextBytes || bytes.IndexByte(rec.removed, '\r') >= 0 ||
					bytes.IndexByte(rec.inserted, '\r') >= 0 {
					return errVolatileJournal
				}
			}
		}
	}
	remaining := int64(maxVolatileJournalReplayBytes)
	for _, branch := range []struct {
		groups []group
		undo   bool
	}{{undo, true}, {redo, false}} {
		currentText, currentValue := text, value
		for i := len(branch.groups) - 1; i >= 0; i-- {
			if err := ctx.Err(); err != nil {
				return err
			}
			g := branch.groups[i]
			before, after := g.before, g.after
			if branch.undo {
				before, after = after, before
			}
			if !volatileSelectionValid(before, len(currentText)) {
				return errVolatileJournal
			}
			if g.volatile {
				want, next := g.volatileBefore, g.volatileAfter
				if branch.undo {
					want, next = next, want
				}
				if int64(len(currentValue))+int64(len(want)) > remaining || !bytes.Equal(currentValue, want) {
					return errVolatileJournal
				}
				remaining -= int64(len(currentValue)) + int64(len(want))
				currentValue = next
			} else if branch.undo {
				for j := len(g.recs) - 1; j >= 0; j-- {
					var err error
					currentText, err = replayVolatileRecord(ctx, currentText, g.recs[j], true, &remaining)
					if err != nil {
						return err
					}
				}
			} else {
				for _, rec := range g.recs {
					var err error
					currentText, err = replayVolatileRecord(ctx, currentText, rec, false, &remaining)
					if err != nil {
						return err
					}
				}
			}
			if !volatileSelectionValid(after, len(currentText)) {
				return errVolatileJournal
			}
		}
	}
	return nil
}

func volatileSelectionValid(s Selection, length int) bool {
	return s.Anchor >= 0 && s.Anchor <= length && s.Head >= 0 && s.Head <= length
}

func replayVolatileRecord(ctx context.Context, text []byte, rec record, undo bool, remaining *int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	want, next := rec.removed, rec.inserted
	if undo {
		want, next = next, want
	}
	if rec.off < 0 || rec.off > len(text) || len(want) > len(text)-rec.off {
		return nil, errVolatileJournal
	}
	nextLength := len(text) - len(want) + len(next)
	// Count line-table work as well as text copies: a small text made almost
	// entirely of newlines must not allocate an unbounded index during restore.
	cost := 3*int64(len(text)) + 3*int64(nextLength)
	if nextLength > maxVolatileJournalTextBytes || cost > *remaining {
		return nil, errVolatileJournal
	}
	if !bytes.Equal(text[rec.off:rec.off+len(want)], want) {
		return nil, errVolatileJournal
	}
	newlines := bytes.Count(text, newline) - bytes.Count(want, newline) + bytes.Count(next, newline)
	if newlines >= maxVolatileJournalLines {
		return nil, errVolatileJournal
	}
	*remaining -= cost
	out := make([]byte, nextLength)
	copy(out, text[:rec.off])
	copy(out[rec.off:], next)
	copy(out[rec.off+len(next):], text[rec.off+len(want):])
	return out, nil
}

func cloneVolatileGroups(ctx context.Context, groups []group) ([]group, error) {
	out := make([]group, len(groups))
	for i, g := range groups {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out[i] = group{kind: g.kind, before: g.before, after: g.after, sealed: true, volatile: g.volatile,
			volatileBefore: bytes.Clone(g.volatileBefore), volatileAfter: bytes.Clone(g.volatileAfter)}
		if len(g.recs) > 0 {
			out[i].recs = make([]record, len(g.recs))
			for j, rec := range g.recs {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				out[i].recs[j] = record{off: rec.off, removed: bytes.Clone(rec.removed), inserted: bytes.Clone(rec.inserted)}
			}
		}
	}
	return out, nil
}

type volatileJournalWriter struct {
	ctx  context.Context
	data []byte
	err  error
}

func (w *volatileJournalWriter) raw(data []byte) {
	if w.err != nil {
		return
	}
	if len(data) > maxVolatileJournalBytes-sha256.Size-len(w.data) {
		w.err = errVolatileJournal
		return
	}
	for len(data) > 0 {
		if err := w.ctx.Err(); err != nil {
			w.err = err
			return
		}
		n := min(len(data), volatileJournalChunk)
		w.data = append(w.data, data[:n]...)
		data = data[n:]
	}
}

func (w *volatileJournalWriter) byte(value byte) { w.raw([]byte{value}) }

func (w *volatileJournalWriter) uint32(value uint32) {
	var data [4]byte
	binary.BigEndian.PutUint32(data[:], value)
	w.raw(data[:])
}

func (w *volatileJournalWriter) uint64(value uint64) {
	var data [8]byte
	binary.BigEndian.PutUint64(data[:], value)
	w.raw(data[:])
}

func (w *volatileJournalWriter) blob(data []byte) {
	w.uint32(uint32(len(data)))
	w.raw(data)
}

func (w *volatileJournalWriter) selection(s Selection) {
	w.uint32(uint32(s.Anchor))
	w.uint32(uint32(s.Head))
}

func (w *volatileJournalWriter) group(g group) {
	w.byte(byte(g.kind))
	if g.volatile {
		w.byte(1)
	} else {
		w.byte(0)
	}
	w.selection(g.before)
	w.selection(g.after)
	if g.volatile {
		w.blob(g.volatileBefore)
		w.blob(g.volatileAfter)
		return
	}
	w.uint32(uint32(len(g.recs)))
	for _, rec := range g.recs {
		if w.err != nil {
			return
		}
		w.uint32(uint32(rec.off))
		w.blob(rec.removed)
		w.blob(rec.inserted)
	}
}

type volatileJournalReader struct {
	ctx    context.Context
	data   []byte
	offset int
	err    error
}

func (r *volatileJournalReader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if err := r.ctx.Err(); err != nil {
		r.err = err
		return nil
	}
	if n < 0 || n > len(r.data)-r.offset {
		r.err = errVolatileJournal
		return nil
	}
	data := r.data[r.offset : r.offset+n]
	r.offset += n
	return data
}

func (r *volatileJournalReader) byte() byte {
	data := r.take(1)
	if data == nil {
		return 0
	}
	return data[0]
}

func (r *volatileJournalReader) uint32() uint32 {
	data := r.take(4)
	if data == nil {
		return 0
	}
	return binary.BigEndian.Uint32(data)
}

func (r *volatileJournalReader) uint64() uint64 {
	data := r.take(8)
	if data == nil {
		return 0
	}
	return binary.BigEndian.Uint64(data)
}

func (r *volatileJournalReader) blob(limit int) []byte {
	n := r.uint32()
	if r.err != nil {
		return nil
	}
	if uint64(n) > uint64(limit) || uint64(n) > uint64(len(r.data)-r.offset) {
		r.err = errVolatileJournal
		return nil
	}
	return r.take(int(n))
}

func (r *volatileJournalReader) selection() Selection {
	anchor, head := r.uint32(), r.uint32()
	if anchor > maxVolatileJournalTextBytes || head > maxVolatileJournalTextBytes {
		r.err = errVolatileJournal
		return Selection{}
	}
	return Selection{Anchor: int(anchor), Head: int(head)}
}

func (r *volatileJournalReader) group(records *int) group {
	g := group{kind: Kind(r.byte())}
	variant := r.byte()
	g.before, g.after = r.selection(), r.selection()
	if r.err != nil {
		return group{}
	}
	if variant > 1 {
		r.err = errVolatileJournal
		return group{}
	}
	if variant == 1 {
		g.volatile = true
		g.volatileBefore = r.blob(maxVolatileBytes)
		g.volatileAfter = r.blob(maxVolatileBytes)
		return g
	}
	count := r.uint32()
	if r.err != nil {
		return group{}
	}
	if uint64(count) > uint64(maxVolatileJournalRecords-*records) ||
		uint64(count)*12 > uint64(len(r.data)-r.offset) {
		r.err = errVolatileJournal
		return group{}
	}
	*records += int(count)
	g.recs = make([]record, int(count))
	for i := range g.recs {
		off := r.uint32()
		if off > maxVolatileJournalTextBytes {
			r.err = errVolatileJournal
			return group{}
		}
		g.recs[i] = record{off: int(off), removed: r.blob(maxVolatileJournalTextBytes), inserted: r.blob(maxVolatileJournalTextBytes)}
		if r.err != nil {
			return group{}
		}
	}
	return g
}
