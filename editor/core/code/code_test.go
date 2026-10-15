package code

import (
	"bytes"
	"math/rand"
	"testing"
	"time"

	"aego/editor/core/code/buffer"
	"aego/editor/core/code/lines"
	"aego/editor/core/code/search"
)

func TestTable(t *testing.T) {
	b := buffer.New([]byte("hello world"))
	b.Insert(5, []byte(","))
	b.Delete(0, 1)
	b.Insert(b.Len(), []byte("!"))
	b.Insert(b.Len(), []byte("?"))
	if got := string(b.Bytes()); got != "ello, world!?" {
		t.Fatalf("got %q", got)
	}
	if got := string(b.Slice(2, 3, nil)); got != "lo," {
		t.Fatalf("slice %q", got)
	}
}

func TestRandomEditsKeepIndex(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	ref := []byte("a\nbb\n\nccc\n")
	b := buffer.New(ref)
	idx := lines.New(ref)
	alphabet := []byte("xy\n\t")
	for i := 0; i < 3000; i++ {
		off := rnd.Intn(len(ref) + 1)
		n := 0
		if off < len(ref) && rnd.Intn(2) == 0 {
			n = rnd.Intn(min(4, len(ref)-off) + 1)
		}
		ins := make([]byte, rnd.Intn(4))
		for k := range ins {
			ins[k] = alphabet[rnd.Intn(len(alphabet))]
		}
		ref = append(append(append([]byte(nil), ref[:off]...), ins...), ref[off+n:]...)
		b.Delete(off, n)
		b.Insert(off, ins)
		idx.Edit(off, n, ins)
	}
	if !bytes.Equal(b.Bytes(), ref) {
		t.Fatal("piece table diverged")
	}
	want := lines.New(ref)
	if idx.Count() != want.Count() {
		t.Fatalf("lines %d, want %d", idx.Count(), want.Count())
	}
	for l := 0; l < want.Count(); l++ {
		if idx.Offset(l) != want.Offset(l) || idx.End(l) != want.End(l) {
			t.Fatalf("line %d differs", l)
		}
	}
}

func clocked(src string) (*Doc, *time.Time) {
	d := New([]byte(src))
	now := time.Unix(0, 0)
	d.Now = func() time.Time { return now }
	return d, &now
}

func TestUndoGroups(t *testing.T) {
	d, now := clocked("")
	for _, s := range []string{"a", "b", "c"} {
		d.Type(s)
		*now = now.Add(100 * time.Millisecond)
	}
	*now = now.Add(time.Second)
	d.Type("d")
	d.Undo()
	if got := string(d.Bytes()); got != "abc" {
		t.Fatalf("after first undo %q", got)
	}
	d.Undo()
	if got := string(d.Bytes()); got != "" || d.Dirty() {
		t.Fatalf("after second undo %q dirty=%v", got, d.Dirty())
	}
	d.Redo()
	if got := string(d.Bytes()); got != "abc" {
		t.Fatalf("redo %q", got)
	}
}

func TestUTF8Cursor(t *testing.T) {
	d := New([]byte("привет мир\nxy"))
	d.SetCursor(0, false)
	d.Move(Right, true, false)
	if l, c := d.LineCol(d.Cursor()); l != 0 || c != 6 {
		t.Fatalf("word right lands at %d:%d", l, c)
	}
	d.Move(Down, false, false)
	if l, c := d.LineCol(d.Cursor()); l != 1 || c != 2 {
		t.Fatalf("down lands at %d:%d", l, c)
	}
	d.Move(Up, false, false)
	if _, c := d.LineCol(d.Cursor()); c != 6 {
		t.Fatalf("goal column lost: %d", c)
	}
	d.Delete(Left, false)
	if got := string(d.Bytes()); got != "приве мир\nxy" {
		t.Fatalf("delete %q", got)
	}
}

func TestIndentCommentNewline(t *testing.T) {
	d := New([]byte("a\nb\nc"))
	d.Select(0, d.Len())
	d.Indent()
	if got := string(d.Bytes()); got != "\ta\n\tb\n\tc" {
		t.Fatalf("indent %q", got)
	}
	d.Unindent()
	d.ToggleComment()
	if got := string(d.Bytes()); got != "// a\n// b\n// c" {
		t.Fatalf("comment %q", got)
	}
	d.ToggleComment()
	if got := string(d.Bytes()); got != "a\nb\nc" {
		t.Fatalf("uncomment %q", got)
	}
	e := New([]byte("\tfunc A() {}"))
	e.SetCursor(len("\tfunc A() {"), false)
	e.Newline()
	if got := string(e.Bytes()); got != "\tfunc A() {\n\t\t\n\t}" {
		t.Fatalf("newline %q", got)
	}
}

func TestSearchReplace(t *testing.T) {
	d := New([]byte("Foo foo food"))
	if n := d.ReplaceAll("foo", "bar", search.Options{Word: true}); n != 2 {
		t.Fatalf("replaced %d", n)
	}
	if got := string(d.Bytes()); got != "bar bar food" {
		t.Fatalf("got %q", got)
	}
	d.Undo()
	if got := string(d.Bytes()); got != "Foo foo food" {
		t.Fatalf("undo %q", got)
	}
}
