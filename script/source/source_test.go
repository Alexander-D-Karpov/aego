package source

import (
	"testing"
	"unicode/utf8"
)

func TestLineColOffsetRoundTrip(t *testing.T) {
	src := []byte("package p\nvar имя = \"é\"\r\n\tx\n")
	m := NewManager()
	id := m.Add("a.aes", src)
	for off := 0; off <= len(src); off++ {
		line, col := m.LineCol(id, uint32(off))
		want := off
		for want > 0 && want < len(src) && !utf8.RuneStart(src[want]) {
			want--
		}
		if got := m.Offset(id, line, col); int(got) != want {
			t.Fatalf("offset %d -> %d:%d -> %d, want %d", off, line, col, got, want)
		}
	}
}

func TestAddReplacesKeepsID(t *testing.T) {
	m := NewManager()
	a := m.Add("x.aes", []byte("one"))
	b := m.Add("x.aes", []byte("two\nlines"))
	if a != b {
		t.Fatalf("ids differ %d %d", a, b)
	}
	if m.LineCount(a) != 2 || string(m.Source(a)) != "two\nlines" {
		t.Fatal("source was not replaced")
	}
	if m.Path(NoFile) != "" || m.Source(99) != nil {
		t.Fatal("unknown ids must be empty")
	}
}

func TestSpanTo(t *testing.T) {
	a := Span{File: 1, Start: 4, End: 6}
	b := Span{File: 1, Start: 1, End: 5}
	if got := a.To(b); got.Start != 1 || got.End != 6 {
		t.Fatalf("join = %+v", got)
	}
	if got := a.To(Span{File: 2}); got != a {
		t.Fatal("different files must not join")
	}
}
