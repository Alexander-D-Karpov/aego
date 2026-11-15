package code

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"
)

func TestValidateVolatileStatesVisitsBothBranchesWithoutChangingJournal(t *testing.T) {
	original, saved := volatileRecoveryFixture(t)
	raw, err := original.EncodeVolatileJournal(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	d, err := DecodeVolatileJournal(context.Background(), raw, saved)
	if err != nil {
		t.Fatal(err)
	}
	before, err := d.EncodeVolatileJournal(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	var visited [][2]string
	err = d.ValidateVolatileStates(context.Background(), func(text, value []byte) error {
		visited = append(visited, [2]string{string(text), string(value)})
		// Even retained or modified callback inputs cannot alter the journal.
		for i := range text {
			text[i] ^= 1
		}
		for i := range value {
			value[i] ^= 1
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := [][2]string{
		{"gain=2\n", "audio-edit-one"},
		{"gain=2\n", ""},
		{"gain=1\n", ""},
		{"gain=2\nloop=1\n", "audio-edit-one"},
		{"gain=2\nloop=1\n", "audio-edit-two"},
	}
	if len(visited) != len(want) {
		t.Fatalf("visited %v, want %v", visited, want)
	}
	for i := range want {
		if visited[i] != want[i] {
			t.Fatalf("state %d = %q, want %q", i, visited[i], want[i])
		}
	}
	again, err := d.EncodeVolatileJournal(context.Background(), saved)
	if err != nil || !bytes.Equal(before, again) {
		t.Fatal("validation changed the journal", err)
	}
	assertVolatileDocsEqual(t, original, d)
}

func TestValidateVolatileStatesRejectsHiddenRedoPayload(t *testing.T) {
	d := New([]byte("metadata"))
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"valid", "invalid"} {
		if changed, err := d.ReplaceVolatile(d.Rev(), []byte(value)); err != nil || !changed {
			t.Fatal("cannot prepare history", err)
		}
	}
	if !d.Undo() {
		t.Fatal("cannot prepare hidden redo")
	}
	invalid := errors.New("unsupported payload")
	err := d.ValidateVolatileStates(context.Background(), func(_ []byte, value []byte) error {
		if string(value) == "invalid" {
			return invalid
		}
		return nil
	})
	if !errors.Is(err, invalid) {
		t.Fatalf("hidden invalid state was accepted: %v", err)
	}
	value, _ := d.Volatile()
	if string(value) != "valid" || !d.CanRedo() || !d.Redo() {
		t.Fatal("rejected validation modified the journal")
	}
	value, _ = d.Volatile()
	if string(value) != "invalid" {
		t.Fatal("rejected validation modified the redo branch")
	}
}

func TestValidateVolatileStatesChecksHistoricalTextAndValueTogether(t *testing.T) {
	d := New([]byte("valid metadata"))
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReplaceVolatile(d.Rev(), []byte("edit")); err != nil {
		t.Fatal(err)
	}
	d.ReplaceSnapshot([]byte("incompatible metadata"), false)
	d.ReplaceSnapshot([]byte("valid metadata"), false)
	errBadPair := errors.New("incompatible pair")
	err := d.ValidateVolatileStates(context.Background(), func(text, value []byte) error {
		if string(text) == "incompatible metadata" && string(value) == "edit" {
			return errBadPair
		}
		return nil
	})
	if !errors.Is(err, errBadPair) {
		t.Fatalf("historical text/value pair was accepted: %v", err)
	}
}

func TestValidateVolatileStatesRefusesCancellationAndMutation(t *testing.T) {
	newDoc := func() *Doc {
		d := New([]byte("text"))
		if err := d.AttachVolatile(); err != nil {
			t.Fatal(err)
		}
		if _, err := d.ReplaceVolatile(d.Rev(), []byte("edit")); err != nil {
			t.Fatal(err)
		}
		return d
	}
	t.Run("cancelled before", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		err := newDoc().ValidateVolatileStates(ctx, func(_, _ []byte) error { calls++; return nil })
		if !errors.Is(err, context.Canceled) || calls != 0 {
			t.Fatalf("cancelled traversal: calls=%d err=%v", calls, err)
		}
	})
	t.Run("cancelled by callback", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		err := newDoc().ValidateVolatileStates(ctx, func(_, _ []byte) error { calls++; cancel(); return nil })
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("cancelled traversal: calls=%d err=%v", calls, err)
		}
	})
	t.Run("callback changes document", func(t *testing.T) {
		d := newDoc()
		err := d.ValidateVolatileStates(context.Background(), func(_, _ []byte) error {
			_, editErr := d.ReplaceVolatile(d.Rev(), []byte("changed by callback"))
			return editErr
		})
		if err == nil {
			t.Fatal("callback mutation was accepted")
		}
		value, _ := d.Volatile()
		if string(value) != "changed by callback" {
			t.Fatal("validation incorrectly claimed to roll back the callback")
		}
	})
}

func TestValidateVolatileStatesCooperativeCancellation(t *testing.T) {
	d, saved := volatileRecoveryFixture(t)
	before, err := d.EncodeVolatileJournal(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512} {
		ctx := newVolatileCancelAtCheck(step)
		err := d.ValidateVolatileStates(ctx, func(_, _ []byte) error { return nil })
		ctx.cancel()
		if ctx.triggered && !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation at check %d returned %v", step, err)
		}
		if !ctx.triggered && err != nil {
			t.Fatalf("validation failed before cancellation at check %d: %v", step, err)
		}
	}
	after, err := d.EncodeVolatileJournal(context.Background(), saved)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("cancelled validation changed the journal", err)
	}
}

func TestValidateVolatileStatesRejectsInvalidArguments(t *testing.T) {
	validate := func(_, _ []byte) error { return nil }
	var nilDoc *Doc
	if nilDoc.ValidateVolatileStates(context.Background(), validate) == nil {
		t.Fatal("nil document was accepted")
	}
	d := New([]byte("text"))
	if d.ValidateVolatileStates(context.Background(), validate) == nil {
		t.Fatal("document without structured state was accepted")
	}
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	contexts := []context.Context{nil}
	if d.ValidateVolatileStates(contexts[0], validate) == nil || d.ValidateVolatileStates(context.Background(), nil) == nil {
		t.Fatal("nil context or validator was accepted")
	}
}

func TestValidateVolatileStatesRejectsOversizedBeforeCallback(t *testing.T) {
	d := New(bytes.Repeat([]byte("x"), maxVolatileJournalTextBytes+1))
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	calls := 0
	err := d.ValidateVolatileStates(context.Background(), func(_, _ []byte) error { calls++; return nil })
	if err == nil || calls != 0 {
		t.Fatalf("oversized text accepted: calls=%d err=%v", calls, err)
	}
}

func TestValidateVolatileStatesMatchesUndoRedoTraversal(t *testing.T) {
	for seed := int64(0); seed < 24; seed++ {
		rng := rand.New(rand.NewSource(seed))
		saved := []byte("base\n")
		d := New(saved)
		if err := d.AttachVolatile(); err != nil {
			t.Fatal(err)
		}
		for range 60 {
			switch rng.Intn(6) {
			case 0, 1:
				offset := rng.Intn(d.Len() + 1)
				length := rng.Intn(d.Len() - offset + 1)
				d.Replace(offset, length, string([]byte{byte('a' + rng.Intn(20))}))
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
			}
		}
		raw, err := d.EncodeVolatileJournal(context.Background(), saved)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		decode := func() *Doc {
			t.Helper()
			doc, err := DecodeVolatileJournal(context.Background(), raw, saved)
			if err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}
			return doc
		}
		pair := func(doc *Doc) [2]string {
			value, _ := doc.Volatile()
			return [2]string{string(doc.Bytes()), string(value)}
		}
		undo, redo := decode(), decode()
		want := [][2]string{pair(undo)}
		for undo.Undo() {
			want = append(want, pair(undo))
		}
		for redo.Redo() {
			want = append(want, pair(redo))
		}
		var got [][2]string
		err = decode().ValidateVolatileStates(context.Background(), func(text, value []byte) error {
			got = append(got, [2]string{string(text), string(value)})
			return nil
		})
		if err != nil || len(got) != len(want) {
			t.Fatalf("seed %d: visited %d states, want %d: %v", seed, len(got), len(want), err)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("seed %d state %d: got %q, want %q", seed, i, got[i], want[i])
			}
		}
	}
}

func TestValidateVolatileStatesBoundsCallbackCopies(t *testing.T) {
	d := New(make([]byte, 1<<20))
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	for i := range 70 {
		if changed, err := d.ReplaceVolatile(d.Rev(), []byte(fmt.Sprintf("value-%d", i))); err != nil || !changed {
			t.Fatal("cannot prepare bounded history", err)
		}
	}
	calls := 0
	err := d.ValidateVolatileStates(context.Background(), func(_, _ []byte) error { calls++; return nil })
	if err == nil || calls == 0 || calls >= 71 {
		t.Fatalf("unbounded validation: calls=%d err=%v", calls, err)
	}
	value, _ := d.Volatile()
	if string(value) != "value-69" || !d.CanUndo() {
		t.Fatal("budget failure modified the journal")
	}
}
