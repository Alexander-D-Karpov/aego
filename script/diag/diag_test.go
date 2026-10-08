package diag

import (
	"bytes"
	"strings"
	"testing"

	"aego/script/source"
)

func TestPrintCaret(t *testing.T) {
	sm := source.NewManager()
	id := sm.Add("main.aes", []byte("package p\n\tx := @@\n"))
	d := Errorf(UnexpectedChar, source.Span{File: id, Start: 16, End: 18}, "bad %s", "token").
		WithNote(source.Span{File: id, Start: 0, End: 7}, "file starts here")
	var b bytes.Buffer
	Print(&b, sm, []Diagnostic{d})
	want := "main.aes:2:7: error AES1001: bad token\n" +
		"    \tx := @@\n" +
		"    \t     ^~\n" +
		"  main.aes:1:1: note: file starts here\n" +
		"    package p\n" +
		"    ^~~~~~~\n"
	if b.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", b.String(), want)
	}
}

func TestSortAndCount(t *testing.T) {
	ds := []Diagnostic{
		Errorf("A", source.Span{File: 1, Start: 9}, "a"),
		Warnf("B", source.Span{File: 1, Start: 2}, "b"),
		Errorf("C", source.Span{File: 0, Start: 5}, "c"),
	}
	Sort(ds)
	got := ds[0].Code + ds[1].Code + ds[2].Code
	if got != "CBA" {
		t.Fatalf("order %s", got)
	}
	if e, w := Count(ds); e != 2 || w != 1 || !HasErrors(ds) {
		t.Fatalf("count %d %d", e, w)
	}
	if !strings.Contains(Format(nil, ds[0]), "error C: c") {
		t.Fatal("format without manager")
	}
}
