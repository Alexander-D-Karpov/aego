package code

import (
	"strings"
	"testing"
)

func TestDepthCarriesAcrossLines(t *testing.T) {
	d := New([]byte("a(\nb[\nc\n]\n)\n"))
	want := []int{0, 1, 2, 2, 1, 0}
	for l, w := range want {
		if got := d.LineState(l).Depth(); got != w {
			t.Fatalf("line %d depth = %d, want %d", l, got, w)
		}
	}
}

func TestEditRestatesIncrementally(t *testing.T) {
	d := New([]byte(strings.Repeat("x\n", 2000)))
	d.restated = 0
	d.Replace(d.LineStart(1000), 0, "y")
	if d.restated > 2 {
		t.Fatalf("plain edit rescanned %d lines", d.restated)
	}
	d.restated = 0
	d.Replace(d.LineStart(1000), 0, "{")
	if d.LineState(999).Depth() != 0 || d.LineState(1001).Depth() != 1 || d.LineState(2000).Depth() != 1 {
		t.Fatal("brace depth did not reach the lines below")
	}
	if d.restated > 1001 {
		t.Fatalf("rescanned %d lines, the lines above the edit must stay", d.restated)
	}
	d.restated = 0
	d.Replace(d.LineStart(1500), 0, "z")
	if d.restated > 2 {
		t.Fatalf("edit below the brace rescanned %d lines", d.restated)
	}
}

func TestUnmatched(t *testing.T) {
	src := "h{\n    if(x[y]) {\n        k(]\n    }\n}\n"
	d := New([]byte(src))
	k := strings.Index(src, "k(") + 1
	if !d.Unmatched(k) || !d.Unmatched(k+1) {
		t.Fatal("k( and ] must be unmatched")
	}
	if d.Unmatched(strings.Index(src, "{")) || d.Unmatched(strings.Index(src, "(x")) {
		t.Fatal("balanced brackets reported as unmatched")
	}
	open := strings.Index(src, "(x")
	if a, b, st := d.BracketAt(open); st != BracketMatched || a != open || src[b] != ')' {
		t.Fatalf("bracket at = %d %d %d", a, b, st)
	}
}

func TestTypeArgsAndStrings(t *testing.T) {
	src := `n.Get<Body>(a < b, ")")`
	d := New([]byte(src))
	lt := strings.Index(src, "<")
	if _, b, st := d.BracketAt(lt); st != BracketMatched || src[b] != '>' {
		t.Fatal("type argument brackets did not match")
	}
	if _, _, st := d.BracketAt(strings.Index(src, " < ") + 2); st != BracketNone {
		t.Fatal("comparison treated as a bracket")
	}
	if d.Unmatched(strings.Index(src, `")"`) + 1) {
		t.Fatal("bracket inside a string reported")
	}
}

func TestBlockAt(t *testing.T) {
	src := "func f() {\n    if x {\n        y()\n    }\n}\n"
	d := New([]byte(src))
	b, ok := d.BlockAt(strings.Index(src, "y()"))
	if !ok || b.Line0 != 1 || b.Line1 != 3 || b.Level != 1 || b.Depth != 1 {
		t.Fatalf("block = %+v %v", b, ok)
	}
	if _, ok := d.BlockAt(len(src)); ok {
		t.Fatal("block outside all braces")
	}
}
