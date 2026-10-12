package draw

import (
	"testing"

	"aego/core/mathx"
	"aego/render/text"
)

func TestTextKeepsItsFont(t *testing.T) {
	l := NewList(8)
	mono := text.NewFont(14, 1, 16, 12)
	l.Text("a", mathx.Vec2{}, mathx.Color{})
	l.SetFont(mono)
	l.Text("b", mathx.Vec2{}, mathx.Color{})
	l.SetFont(nil)
	l.Text("c", mathx.Vec2{}, mathx.Color{})
	s := l.streams[LayerBase]
	if s[0].font != nil || s[1].font != mono || s[2].font != nil {
		t.Fatalf("fonts = %p %p %p", s[0].font, s[1].font, s[2].font)
	}
	l.SetFont(mono)
	l.Reset()
	if l.Font() != nil {
		t.Fatal("reset must drop the font override")
	}
}
