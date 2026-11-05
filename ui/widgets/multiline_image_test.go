package widgets

import (
	"testing"

	"aego/ui/core"
)

func TestMultilineMarkerAtCaretAndRenumberPreservesSelection(t *testing.T) {
	ui := core.New(nil)
	var m Multiline
	value := "Привет мир"
	m.Height(ui, value, 300, 3)
	m.b.SetCursor(7, false)
	m.Insert(&value, "[Image 1]")
	if value != "Привет [Image 1]мир" || m.b.Cursor() != 16 {
		t.Fatalf("inserted at the wrong rune: %q cursor=%d", value, m.b.Cursor())
	}
	m.b.SetCursor(m.b.Len(), false)
	if !m.ReplaceFirst(&value, "[Image 1]", "[Image 2]") || m.b.Cursor() != m.b.Len() {
		t.Fatalf("renumber moved the caret: %q cursor=%d", value, m.b.Cursor())
	}
	if !m.ReplaceFirst(&value, "[Image 2]", "") || value != "Привет мир" || m.b.Cursor() != m.b.Len() {
		t.Fatalf("removing marker damaged text or caret: %q cursor=%d", value, m.b.Cursor())
	}
}
