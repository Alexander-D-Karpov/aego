package textedit

import "testing"

func TestCursorMovesByRunes(t *testing.T) {
	b := New("Привет")
	if b.Cursor() != 6 {
		t.Fatalf("cursor = %d", b.Cursor())
	}
	b.Move(Left, false, false)
	b.Insert("ж")
	if b.String() != "Привежт" {
		t.Fatalf("got %q", b.String())
	}
}

func TestSelectionReplace(t *testing.T) {
	b := New("hello world")
	b.SetCursor(0, false)
	b.Move(Right, true, true)
	if b.Copy() != "hello" {
		t.Fatalf("selection = %q", b.Copy())
	}
	b.Insert("bye")
	if b.String() != "bye world" {
		t.Fatalf("got %q", b.String())
	}
}

func TestWordMovementAndDelete(t *testing.T) {
	b := New("foo bar_baz  qux")
	b.Move(Left, true, false)
	if b.Cursor() != 13 {
		t.Fatalf("cursor = %d", b.Cursor())
	}
	b.Move(Left, true, false)
	if b.Cursor() != 4 {
		t.Fatalf("cursor = %d", b.Cursor())
	}
	b.Delete(Right, true)
	if b.String() != "foo   qux" {
		t.Fatalf("got %q", b.String())
	}
}

func TestComposition(t *testing.T) {
	b := New("ab")
	b.Composition("にほ", 2)
	if string(b.CompositionRunes()) != "にほ" || b.String() != "ab" {
		t.Fatal("composition must not touch the text")
	}
	b.Insert("日本")
	if b.String() != "ab日本" || len(b.CompositionRunes()) != 0 {
		t.Fatalf("got %q", b.String())
	}
}

func TestUndoCoalescesTyping(t *testing.T) {
	b := New("")
	for _, c := range "abc" {
		b.Insert(string(c))
	}
	b.Insert(" ")
	b.Insert("d")
	if !b.Undo() || b.String() != "abc " {
		t.Fatalf("after first undo %q", b.String())
	}
	b.Undo()
	if b.String() != "abc" {
		t.Fatalf("after second undo %q", b.String())
	}
	b.Undo()
	if b.String() != "" {
		t.Fatalf("after third undo %q", b.String())
	}
	if b.Undo() {
		t.Fatal("undo on empty history")
	}
}

func TestCutAndMaxLen(t *testing.T) {
	b := New("abcdef")
	b.SetCursor(1, false)
	b.SetCursor(3, true)
	if b.Cut() != "bc" || b.String() != "adef" {
		t.Fatalf("got %q", b.String())
	}
	b.MaxLen = 5
	b.Insert("xyz")
	if b.String() != "axdef" {
		t.Fatalf("max len ignored: %q", b.String())
	}
}

func TestInsertDropsControlChars(t *testing.T) {
	b := New("")
	b.Insert("a\tb\nc")
	if b.String() != "ab c" {
		t.Fatalf("got %q", b.String())
	}
}
