package widgets

import (
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/textedit"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMultilineDraftWrapAndHeight(t *testing.T) {
	ui := core.New(text.NewFont(14, 1, 16, 12))
	var m Multiline
	wide := m.Height(ui, "Привет", 300, 5)
	narrow := m.Height(ui, "Привет\nдлинный текст", 30, 5)
	if len(m.lines) < 4 || narrow <= wide {
		t.Fatalf("layout %v vs %v, %v rows", narrow, wide, len(m.lines))
	}
	for _, l := range m.lines {
		if !utf8.ValidString(l.text) {
			t.Fatal("partial rune")
		}
	}
	if tall := m.Height(ui, strings.Repeat("one\n", 100), 180, 5); tall != narrow {
		t.Fatal("uncapped draft height", tall, narrow)
	}
	m.layout(ui, "one two three", ui.Font().Advance('a')*8.2)
	if m.lines[0].text != "one two " || m.lines[1].text != "three" {
		t.Fatal(m.lines)
	}
	m.layout(ui, strings.Repeat("long draft\n", 30), 500)
	m.layout(ui, "short", 500)
	for _, line := range m.lines[:cap(m.lines)][len(m.lines):] {
		if line.text != "" {
			t.Fatal("cache kept a discarded draft")
		}
	}
	b := textedit.New("")
	b.Multiline = true
	b.Insert("один\r\nдва")
	if b.String() != "один\nдва" {
		t.Fatal(b.String())
	}
	b = textedit.New("")
	b.Insert("a\nb")
	if b.String() != "a b" {
		t.Fatal("single-line input changed")
	}
}
