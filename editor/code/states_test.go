package code

import "testing"

func TestBracketAt(t *testing.T) {
	d := New([]byte("f(a[0]) {}\n)"))
	if a, b, st := d.BracketAt(1); st != BracketMatched || a != 1 || b != 6 {
		t.Fatalf("open paren: %d %d %v", a, b, st)
	}
	if a, b, st := d.BracketAt(9); st != BracketMatched || a != 9 || b != 8 {
		t.Fatalf("caret between braces: %d %d %v", a, b, st)
	}
	if a, _, st := d.BracketAt(12); st != BracketUnmatched || a != 11 {
		t.Fatalf("stray close: %d %v", a, st)
	}
	if _, _, st := New([]byte("abc")).BracketAt(1); st != BracketNone {
		t.Fatalf("no bracket: %v", st)
	}
}
