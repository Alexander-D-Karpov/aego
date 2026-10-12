package richtext

import (
	"reflect"
	"testing"
)

func TestInline(t *testing.T) {
	got := Inline("Pause with [`yield`](ref:keyword/yield), **not** *here* or snake_case a * b \\*x\\*")
	want := []Span{
		{Text: "Pause with "},
		{Text: "yield", Style: Code, Target: "ref:keyword/yield"},
		{Text: ", "},
		{Text: "not", Style: Strong},
		{Text: " "},
		{Text: "here", Style: Emphasis},
		{Text: " or snake_case a * b *x*"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
}

func TestParse(t *testing.T) {
	md := "# Title\n\nFirst line\ncontinues.\n\n- one\n  more\n- two\n1. first\n\n| Form | Resumes |\n|---|---|\n| `yield` | next \\| frame |\n\n```aes run\npackage demo\n```\n\n    indented code\n\n> quote\n> more\n\n---\n"
	bs := Parse(md)
	kinds := []Kind{Heading, Paragraph, ListItem, ListItem, ListItem, Table, CodeBlock, CodeBlock, Quote, Rule}
	if len(bs) != len(kinds) {
		t.Fatalf("got %d blocks: %+v", len(bs), bs)
	}
	for i, k := range kinds {
		if bs[i].Kind != k {
			t.Errorf("block %d is %d, want %d", i, bs[i].Kind, k)
		}
	}
	if PlainText(bs[1].Spans) != "First line continues." || PlainText(bs[2].Spans) != "one more" || bs[4].Ordinal != 1 {
		t.Errorf("paragraph/list text: %q %q %d", PlainText(bs[1].Spans), PlainText(bs[2].Spans), bs[4].Ordinal)
	}
	tb := bs[5]
	if !tb.Header || len(tb.Rows) != 2 || PlainText(tb.Rows[1][1]) != "next | frame" || tb.Rows[1][0][0].Style != Code {
		t.Errorf("table: %+v", tb)
	}
	if !bs[6].Runnable() || bs[6].Lang != "aes" || bs[6].Lines[0] != "package demo" {
		t.Errorf("fence: %+v", bs[6])
	}
	if bs[7].Lines[0] != "indented code" || bs[7].Runnable() {
		t.Errorf("indented: %+v", bs[7])
	}
	if PlainText(bs[8].Spans) != "quote more" {
		t.Errorf("quote: %+v", bs[8])
	}
}
