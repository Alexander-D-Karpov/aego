package assets

import (
	"testing"

	"aego/editor/core/code/highlight"
)

func TestDeclName(t *testing.T) {
	for _, c := range []struct {
		line string
		name string
		kind highlight.Kind
		ok   bool
	}{
		{"const FloorNormal float = -0.5", "FloorNormal", highlight.Constant, true},
		{"func TestIsFloor() {", "TestIsFloor", highlight.Function, true},
		{"func (p *Player) Update(dt float) {", "Update", highlight.Method, true},
		{"var speed = 3", "speed", highlight.Variable, true},
		{"type Enemy struct {", "Enemy", highlight.Type, true},
		{"// comment", "", 0, false},
	} {
		name, kind, ok := declName([]string{c.line})
		if name != c.name || kind != c.kind || ok != c.ok {
			t.Errorf("%q: got %q %d %v", c.line, name, kind, ok)
		}
	}
}
