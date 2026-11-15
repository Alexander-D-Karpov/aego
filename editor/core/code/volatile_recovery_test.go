package code

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/rand"
	"testing"

	"aego/editor/core/code/search"
)

func volatileRecoveryFixture(t *testing.T) (*Doc, []byte) {
	t.Helper()
	saved := []byte("gain=1\r\n")
	d := New(saved)
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	d.ReplaceSnapshot([]byte("gain=2\n"), false)
	if changed, err := d.ReplaceVolatile(d.Rev(), []byte("audio-edit-one")); err != nil || !changed {
		t.Fatal("cannot add first structured edit", err)
	}
	d.ReplaceSnapshot([]byte("gain=2\nloop=1\n"), false)
	if changed, err := d.ReplaceVolatile(d.Rev(), []byte("audio-edit-two")); err != nil || !changed {
		t.Fatal("cannot add second structured edit", err)
	}
	if !d.Undo() {
		t.Fatal("cannot prepare redo history")
	}
	if !d.Undo() {
		t.Fatal("cannot prepare redo history")
	}
	d.GLSL = true
	d.Select(1, 4)
	return d, saved
}

func assertVolatileDocsEqual(t *testing.T, a, b *Doc) {
	t.Helper()
	av, aa := a.Volatile()
	bv, ba := b.Volatile()
	if !bytes.Equal(a.Bytes(), b.Bytes()) || aa != ba || !bytes.Equal(av, bv) || a.Sel != b.Sel ||
		a.Dirty() != b.Dirty() || a.VolatileDirty() != b.VolatileDirty() ||
		a.CanUndo() != b.CanUndo() || a.CanRedo() != b.CanRedo() || a.GLSL != b.GLSL {
		t.Fatalf("recovered journal differs: text=%q/%q volatile=%q/%q selection=%+v/%+v undo=%v/%v redo=%v/%v",
			a.Bytes(), b.Bytes(), av, bv, a.Sel, b.Sel, a.CanUndo(), b.CanUndo(), a.CanRedo(), b.CanRedo())
	}
}

func TestVolatileRecoveryRoundTripUnifiedUndoRedo(t *testing.T) {
	d, saved := volatileRecoveryFixture(t)
	originalRevision := d.Rev()
	raw, err := d.EncodeVolatileJournal(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := DecodeVolatileJournal(context.Background(), raw, saved)
	if err != nil || restored == d {
		t.Fatal("cannot decode detached recovery journal", err)
	}
	if restored.Rev() != originalRevision+1 {
		t.Fatal("restored journal reused the previous revision")
	}
	if _, _, ok := restored.MapSpan(originalRevision, 0, 1); ok {
		t.Fatal("restored journal claimed the old text span mapping")
	}
	assertVolatileDocsEqual(t, d, restored)
	// Neither the encoded input nor the caller's saved bytes are retained.
	raw[0] ^= 1
	saved[0] ^= 1
	assertVolatileDocsEqual(t, d, restored)
	for _, action := range []string{"redo", "redo", "undo", "undo", "undo", "undo", "redo", "redo", "redo", "redo"} {
		var a, b bool
		if action == "undo" {
			a, b = d.Undo(), restored.Undo()
		} else {
			a, b = d.Redo(), restored.Redo()
		}
		if a != b {
			t.Fatalf("%s availability differs", action)
		}
		assertVolatileDocsEqual(t, d, restored)
	}
}

func TestVolatileRecoveryKeepsCleanStateAndRedo(t *testing.T) {
	d := New([]byte("saved\n"))
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReplaceVolatile(d.Rev(), []byte("private")); err != nil || !d.Undo() {
		t.Fatal("cannot prepare a clean document with structured redo", err)
	}
	raw, err := d.EncodeVolatileJournal(context.Background(), []byte("saved\n"))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := DecodeVolatileJournal(context.Background(), raw, []byte("saved\n"))
	if err != nil || restored.Dirty() || !restored.CanRedo() || !restored.Redo() || !restored.VolatileDirty() {
		t.Fatal("recovery lost clean state or structured redo", err)
	}
}

func TestVolatileRecoveryOwnsDecodedStateAndHistory(t *testing.T) {
	saved := []byte("base")
	d := New(saved)
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	const structured = "private-audio-payload"
	const inserted = "unique-text-payload"
	if changed, err := d.ReplaceVolatile(d.Rev(), []byte(structured)); err != nil || !changed {
		t.Fatal("cannot prepare structured state", err)
	}
	d.Replace(1, 0, inserted)
	raw, err := d.EncodeVolatileJournal(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := DecodeVolatileJournal(context.Background(), raw, saved)
	if err != nil {
		t.Fatal(err)
	}
	body := raw[:len(raw)-sha256.Size]
	for _, needle := range []string{structured, inserted} {
		first, last := bytes.Index(body, []byte(needle)), bytes.LastIndex(body, []byte(needle))
		if first < 0 || last <= first {
			t.Fatalf("recovery did not contain both live and historical %q", needle)
		}
		body[first] ^= 1
		body[last] ^= 1
	}
	value, attached := restored.Volatile()
	if !attached || string(value) != structured || string(restored.Bytes()) != "b"+inserted+"ase" {
		t.Fatal("decoded live state retained caller-owned input")
	}
	if !restored.Undo() {
		t.Fatal("cannot undo decoded text history")
	}
	if !restored.Undo() {
		t.Fatal("cannot undo decoded structured history")
	}
	if !restored.Redo() {
		t.Fatal("cannot redo decoded structured history")
	}
	if !restored.Redo() {
		t.Fatal("cannot replay decoded history")
	}
	value, _ = restored.Volatile()
	if string(value) != structured || string(restored.Bytes()) != "b"+inserted+"ase" {
		t.Fatal("decoded history retained caller-owned input")
	}
}

func resignVolatileJournal(data []byte) {
	checksum := sha256.Sum256(data[:len(data)-sha256.Size])
	copy(data[len(data)-sha256.Size:], checksum[:])
}

func TestVolatileRecoveryRejectsCorruptOrWrongPreimage(t *testing.T) {
	d, saved := volatileRecoveryFixture(t)
	raw, err := d.EncodeVolatileJournal(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	for n := range len(raw) {
		if recovered, err := DecodeVolatileJournal(context.Background(), raw[:n], saved); err == nil || recovered != nil {
			t.Fatalf("accepted truncation at %d of %d bytes", n, len(raw))
		}
	}
	if recovered, err := DecodeVolatileJournal(context.Background(), raw, []byte("gain=1\n")); err == nil || recovered != nil {
		t.Fatal("accepted a byte-different saved preimage with the same normalized text")
	}
	offset := len(volatileJournalMagic) + 1 + 8 + sha256.Size
	offset += 4 + int(binary.BigEndian.Uint32(raw[offset:offset+4]))
	offset += 4 + int(binary.BigEndian.Uint32(raw[offset:offset+4]))
	selectionOffset, groupOffset := offset, offset+8+8
	if groupOffset+26 > len(raw)-sha256.Size || raw[groupOffset+1] != 0 {
		t.Fatal("unexpected recovery fixture group layout")
	}
	for _, reason := range []string{"checksum", "magic", "flags", "revision", "text-length", "selection", "group-count", "group-kind", "variant", "record-count", "record-offset", "trailing"} {
		t.Run(reason, func(t *testing.T) {
			bad := bytes.Clone(raw)
			switch reason {
			case "checksum":
				bad[len(bad)-1] ^= 1
			case "magic":
				bad[0] ^= 1
				resignVolatileJournal(bad)
			case "flags":
				bad[len(volatileJournalMagic)] = 2
				resignVolatileJournal(bad)
			case "revision":
				offset := len(volatileJournalMagic) + 1
				binary.BigEndian.PutUint64(bad[offset:offset+8], maxVolatileJournalRevision)
				resignVolatileJournal(bad)
			case "text-length":
				offset := len(volatileJournalMagic) + 1 + 8 + sha256.Size
				binary.BigEndian.PutUint32(bad[offset:offset+4], ^uint32(0))
				resignVolatileJournal(bad)
			case "selection":
				binary.BigEndian.PutUint32(bad[selectionOffset:selectionOffset+4], ^uint32(0))
				resignVolatileJournal(bad)
			case "group-count":
				offset := len(volatileJournalMagic) + 1 + 8 + sha256.Size
				textLen := int(binary.BigEndian.Uint32(bad[offset : offset+4]))
				offset += 4 + textLen
				valueLen := int(binary.BigEndian.Uint32(bad[offset : offset+4]))
				offset += 4 + valueLen + 8 // current selection
				binary.BigEndian.PutUint32(bad[offset:offset+4], ^uint32(0))
				resignVolatileJournal(bad)
			case "group-kind":
				bad[groupOffset] = 255
				resignVolatileJournal(bad)
			case "variant":
				bad[groupOffset+1] = 2
				resignVolatileJournal(bad)
			case "record-count":
				offset := groupOffset + 2 + 16
				binary.BigEndian.PutUint32(bad[offset:offset+4], ^uint32(0))
				resignVolatileJournal(bad)
			case "record-offset":
				offset := groupOffset + 2 + 16 + 4
				binary.BigEndian.PutUint32(bad[offset:offset+4], ^uint32(0))
				resignVolatileJournal(bad)
			case "trailing":
				bad = append(bad[:len(bad)-sha256.Size], append([]byte{1}, bad[len(bad)-sha256.Size:]...)...)
				resignVolatileJournal(bad)
			}
			if recovered, err := DecodeVolatileJournal(context.Background(), bad, saved); err == nil || recovered != nil {
				t.Fatalf("accepted %s recovery", reason)
			}
		})
	}
}

func TestVolatileRecoveryRejectsRechecksummedHistoryMismatch(t *testing.T) {
	for _, structured := range []bool{false, true} {
		name := "text"
		if structured {
			name = "structured"
		}
		t.Run(name, func(t *testing.T) {
			saved := []byte("abc")
			d := New(saved)
			if err := d.AttachVolatile(); err != nil {
				t.Fatal(err)
			}
			needle := []byte("X")
			if structured {
				needle = []byte("private-audio-state")
				if _, err := d.ReplaceVolatile(d.Rev(), needle); err != nil {
					t.Fatal(err)
				}
			} else {
				d.Replace(1, 0, string(needle))
			}
			raw, err := d.EncodeVolatileJournal(context.Background(), saved)
			if err != nil {
				t.Fatal(err)
			}
			i := bytes.LastIndex(raw[:len(raw)-sha256.Size], needle)
			if i < 0 {
				t.Fatal("could not locate the recorded history preimage")
			}
			raw[i] ^= 1
			resignVolatileJournal(raw)
			if recovered, err := DecodeVolatileJournal(context.Background(), raw, saved); err == nil || recovered != nil {
				t.Fatal("accepted an internally inconsistent history with a valid checksum")
			}
		})
	}
}

func TestVolatileRecoveryMutationsNeverPanic(t *testing.T) {
	d, saved := volatileRecoveryFixture(t)
	raw, err := d.EncodeVolatileJournal(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(1942))
	for iteration := 0; iteration < 512; iteration++ {
		bad := bytes.Clone(raw)
		for changes := 1 + rng.Intn(4); changes > 0; changes-- {
			i := rng.Intn(len(bad) - sha256.Size)
			bad[i] ^= byte(1 + rng.Intn(255))
		}
		resignVolatileJournal(bad)
		recovered, err := DecodeVolatileJournal(context.Background(), bad, saved)
		if err != nil {
			if recovered != nil {
				t.Fatal("a rejected candidate returned a document")
			}
			continue
		}
		if recovered == nil {
			t.Fatal("a successful decode returned no document")
		}
		// A valid alternate history can be constructed with a new checksum;
		// it must still be safe to replay. The checksum is not authentication.
		for step := 0; step < 10 && recovered.Undo(); step++ {
		}
		for step := 0; step < 10 && recovered.Redo(); step++ {
		}
	}
}

func TestVolatileRecoveryBoundsAndCancellation(t *testing.T) {
	d, saved := volatileRecoveryFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if raw, err := d.EncodeVolatileJournal(ctx, saved); !errors.Is(err, context.Canceled) || raw != nil {
		t.Fatal("cancelled capture returned a recovery", err)
	}
	raw, err := d.EncodeVolatileJournal(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	if recovered, err := DecodeVolatileJournal(ctx, raw, saved); !errors.Is(err, context.Canceled) || recovered != nil {
		t.Fatal("cancelled decode returned a document", err)
	}
	if out, err := d.EncodeVolatileJournal(context.Background(), []byte("not the saved preimage")); err == nil || out != nil {
		t.Fatal("captured against the wrong saved checkpoint")
	}
	if out, err := d.EncodeVolatileJournal(context.Background(), make([]byte, maxVolatileJournalTextBytes+1)); err == nil || out != nil {
		t.Fatal("accepted an oversized saved text preimage")
	}
	text := []byte("abcdefgh")
	remaining := int64(4)
	if out, err := replayVolatileRecord(context.Background(), text, record{off: 0, removed: []byte("a"), inserted: []byte("b")}, false, &remaining); err == nil || out != nil || string(text) != "abcdefgh" {
		t.Fatal("replay exceeded its work budget or mutated its input")
	}
	for _, revision := range []uint64{maxVolatileJournalRevision, ^uint64(0)} {
		d.rev = revision
		if out, err := d.EncodeVolatileJournal(context.Background(), saved); err == nil || out != nil {
			t.Fatal("captured an exhausted or unsafe revision")
		}
	}
}

func TestVolatileRecoveryRejectsUninitializedDocument(t *testing.T) {
	defer func() {
		if recover() != nil {
			t.Fatal("uninitialized document panicked during recovery capture")
		}
	}()
	var d Doc
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	if raw, err := d.EncodeVolatileJournal(context.Background(), nil); err == nil || raw != nil {
		t.Fatal("uninitialized document produced a recovery candidate")
	}
}

func TestVolatileRecoveryBoundsLineIndexAndIntermediateStates(t *testing.T) {
	tooMany := bytes.Repeat([]byte{'\n'}, maxVolatileJournalLines)
	if err := validateVolatileJournal(context.Background(), tooMany, nil, Selection{}, nil, nil); err == nil {
		t.Fatal("accepted a current text with too many lines")
	}
	nearLimit := tooMany[:len(tooMany)-1]
	remaining := int64(maxVolatileJournalReplayBytes)
	insert := record{off: 0, inserted: []byte{'\n'}}
	if out, err := replayVolatileRecord(context.Background(), nearLimit, insert, false, &remaining); err == nil || out != nil {
		t.Fatal("accepted an intermediate text with too many lines")
	}
}

type volatileCancelAtCheck struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
	triggered bool
}

func newVolatileCancelAtCheck(n int) *volatileCancelAtCheck {
	ctx, cancel := context.WithCancel(context.Background())
	return &volatileCancelAtCheck{Context: ctx, cancel: cancel, remaining: n}
}

func (c *volatileCancelAtCheck) Err() error {
	if !c.triggered {
		c.remaining--
		if c.remaining == 0 {
			c.triggered = true
			c.cancel()
		}
	}
	return c.Context.Err()
}

func TestVolatileRecoveryCooperativeCancellation(t *testing.T) {
	d, saved := volatileRecoveryFixture(t)
	raw, err := d.EncodeVolatileJournal(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512} {
		capture := newVolatileCancelAtCheck(step)
		out, err := d.EncodeVolatileJournal(capture, saved)
		capture.cancel()
		if capture.triggered && (!errors.Is(err, context.Canceled) || out != nil) {
			t.Fatalf("capture returned a candidate after cancellation at check %d: %v", step, err)
		}
		if !capture.triggered && (err != nil || len(out) == 0) {
			t.Fatalf("capture failed before cancellation at check %d: %v", step, err)
		}
		decode := newVolatileCancelAtCheck(step)
		recovered, err := DecodeVolatileJournal(decode, raw, saved)
		decode.cancel()
		if decode.triggered && (!errors.Is(err, context.Canceled) || recovered != nil) {
			t.Fatalf("decode returned a document after cancellation at check %d: %v", step, err)
		}
		if !decode.triggered && (err != nil || recovered == nil) {
			t.Fatalf("decode failed before cancellation at check %d: %v", step, err)
		}
	}
}

func TestVolatileRecoveryRandomizedHistory(t *testing.T) {
	for seed := int64(0); seed < 40; seed++ {
		rng := rand.New(rand.NewSource(seed))
		saved := []byte("base\n")
		d := New(saved)
		if err := d.AttachVolatile(); err != nil {
			t.Fatal(err)
		}
		for range 120 {
			switch rng.Intn(7) {
			case 0, 1:
				off := rng.Intn(d.Len() + 1)
				n := rng.Intn(d.Len() - off + 1)
				d.Replace(off, n, string([]byte{byte('a' + rng.Intn(20))}))
			case 2:
				if _, err := d.ReplaceVolatile(d.Rev(), []byte{byte(1 + rng.Intn(8))}); err != nil {
					t.Fatalf("seed %d: %v", seed, err)
				}
			case 3:
				d.Undo()
			case 4:
				d.Redo()
			case 5:
				if d.Len() >= 2 {
					d.ReplaceRanges([][2]int{{0, 1}, {d.Len() - 1, d.Len()}}, [][]byte{[]byte("X"), []byte("Y")})
				}
			case 6:
				d.Select(rng.Intn(d.Len()+1), rng.Intn(d.Len()+1))
			}
		}
		raw, err := d.EncodeVolatileJournal(context.Background(), saved)
		if err != nil {
			t.Fatalf("seed %d capture: %v", seed, err)
		}
		restored, err := DecodeVolatileJournal(context.Background(), raw, saved)
		if err != nil {
			t.Fatalf("seed %d decode: %v", seed, err)
		}
		assertVolatileDocsEqual(t, d, restored)
		for {
			a, b := d.Undo(), restored.Undo()
			if a != b {
				t.Fatalf("seed %d undo availability differs", seed)
			}
			assertVolatileDocsEqual(t, d, restored)
			if !a {
				break
			}
		}
		for {
			a, b := d.Redo(), restored.Redo()
			if a != b {
				t.Fatalf("seed %d redo availability differs", seed)
			}
			assertVolatileDocsEqual(t, d, restored)
			if !a {
				break
			}
		}
	}
}

func TestVolatileRecoveryEditorOperationGroups(t *testing.T) {
	cases := []struct {
		name string
		run  func(*Doc)
	}{
		{"type", func(d *Doc) { d.Type("a"); d.Type("b"); d.Paste("\r\nc") }},
		{"indent", func(d *Doc) { d.Select(0, d.Len()); d.Indent(); d.Unindent() }},
		{"comment", func(d *Doc) { d.Select(0, d.Len()); d.ToggleComment(); d.ToggleComment() }},
		{"duplicate", func(d *Doc) { d.SetCursor(0, false); d.DuplicateLines() }},
		{"delete-lines", func(d *Doc) { d.SetCursor(2, false); d.DeleteLines() }},
		{"shift-down", func(d *Doc) { d.SetCursor(0, false); d.ShiftLines(1) }},
		{"shift-up", func(d *Doc) { d.SetCursor(d.LineStart(1), false); d.ShiftLines(-1) }},
		{"newline", func(d *Doc) { d.SetCursor(d.LineEnd(0), false); d.Newline() }},
		{"delete-utf8", func(d *Doc) { d.SetCursor(len("\tα"), false); d.Delete(Left, false) }},
		{"replace-all", func(d *Doc) { d.ReplaceAll("line", "row", search.Options{}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			saved := []byte("\tαbc\n\tline two\nlast\n")
			d := New(saved)
			if err := d.AttachVolatile(); err != nil {
				t.Fatal(err)
			}
			if _, err := d.ReplaceVolatile(d.Rev(), []byte("structured-before")); err != nil {
				t.Fatal(err)
			}
			tc.run(d)
			if _, err := d.ReplaceVolatile(d.Rev(), []byte("structured-after")); err != nil {
				t.Fatal(err)
			}
			if !d.Undo() {
				t.Fatal("cannot prepare redo")
			}
			raw, err := d.EncodeVolatileJournal(context.Background(), saved)
			if err != nil {
				t.Fatal("cannot capture grouped editor operation", err)
			}
			restored, err := DecodeVolatileJournal(context.Background(), raw, saved)
			if err != nil {
				t.Fatal("cannot decode grouped editor operation", err)
			}
			assertVolatileDocsEqual(t, d, restored)
			for {
				a, b := d.Undo(), restored.Undo()
				if a != b {
					t.Fatal("undo availability differs")
				}
				assertVolatileDocsEqual(t, d, restored)
				if !a {
					break
				}
			}
			for {
				a, b := d.Redo(), restored.Redo()
				if a != b {
					t.Fatal("redo availability differs")
				}
				assertVolatileDocsEqual(t, d, restored)
				if !a {
					break
				}
			}
		})
	}
}
