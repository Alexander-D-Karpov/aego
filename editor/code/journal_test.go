package code

import (
	"strings"
	"testing"
	"time"

	"aego/editor/code/highlight"
)

func TestBlockCommentState(t *testing.T) {
	src := "a\n/*\n" + strings.Repeat("x\n", 50) + "*/\nb\n"
	d := New([]byte(src))
	if d.LineState(10) != highlight.InComment {
		t.Fatalf("line 10 should start inside a comment")
	}
	if d.LineState(53) != highlight.Normal {
		t.Fatalf("line 53 should start outside a comment")
	}
	d.Replace(d.LineStart(1), 2, "//")
	if d.LineState(10) != highlight.Normal {
		t.Fatalf("comment state was not recomputed after the edit")
	}
	d.Replace(d.LineStart(1), 2, "/*")
	if d.LineState(10) != highlight.InComment {
		t.Fatalf("comment state was not restored")
	}
}

func TestMapSpan(t *testing.T) {
	d := New([]byte("hello world"))
	r0 := d.Rev()
	d.Replace(0, 0, "xx")
	if s, e, ok := d.MapSpan(r0, 6, 11); !ok || s != 8 || e != 13 {
		t.Fatalf("span after insert: %d %d %v", s, e, ok)
	}
	if s, e, ok := d.MapSpan(r0, 0, 5); !ok || s != 2 || e != 7 {
		t.Fatalf("span with insert at start: %d %d %v", s, e, ok)
	}
	d.Replace(4, 1, "Q")
	if _, _, ok := d.MapSpan(r0, 0, 5); ok {
		t.Fatalf("span with an edit inside must be dropped")
	}
	if off, ok := d.Unmap(r0, 8); !ok || off != 6 {
		t.Fatalf("unmap: %d %v", off, ok)
	}
	if _, ok := d.Unmap(r0, 1); ok {
		t.Fatalf("unmap into inserted text must fail")
	}
}

func TestManyEditsFast(t *testing.T) {
	d := New([]byte(strings.Repeat("\tvar x int = 1 // comment\n", 10000)))
	start := time.Now()
	for i := 0; i < 1000; i++ {
		d.SetCursor(d.LineStart(5000), false)
		d.Type("/")
	}
	if el := time.Since(start); el > 500*time.Millisecond {
		t.Fatalf("1000 edits took %v", el)
	}
}

func BenchmarkTyping(b *testing.B) {
	d := New([]byte(strings.Repeat("\tvar x int = 1 /* c */\n", 10000)))
	for b.Loop() {
		d.SetCursor(d.LineStart(5000), false)
		d.Type("a")
	}
}
