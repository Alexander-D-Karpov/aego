package widgets

import (
	"strings"
	"testing"
	"unicode/utf8"

	"aego/core/mathx"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/theme"
)

func confirmTestContext(size mathx.Vec2) *core.Context {
	font := text.NewFont(14, 1, 16, 12)
	for r := rune(32); r < 127; r++ {
		font.SetGlyph(r, text.FontGlyph{Advance: 8})
	}
	ui := core.New(font)
	ui.Theme = theme.Editor()
	ui.Begin(&input.State{}, size, 1)
	return ui
}

func TestWrapLinesHonoursNewlinesAndLongPaths(t *testing.T) {
	ui := confirmTestContext(mathx.Vec2{X: 800, Y: 600})
	defer ui.End()
	path := "/home/пользователь/" + strings.Repeat("long-directory/", 9) + "project"
	lines := WrapLines(ui, "Question?\n"+path+"\n\nEnd", 160)
	if len(lines) < 4 || lines[0] != "Question?" || lines[len(lines)-2] != "" || lines[len(lines)-1] != "End" {
		t.Fatalf("explicit lines lost: %#v", lines)
	}
	for _, line := range lines {
		if !utf8.ValidString(line) || strings.ContainsAny(line, "\r\n") || ui.TextSize(line).X > 160 {
			t.Fatalf("invalid wrapped line %q", line)
		}
	}
	if got := strings.Join(lines[1:len(lines)-2], ""); got != path {
		t.Fatalf("path lost text: %q", got)
	}
}

func TestConfirmFitsWrappedMessageAndButtons(t *testing.T) {
	for _, size := range []mathx.Vec2{{X: 800, Y: 600}, {X: 310, Y: 180}} {
		ui := confirmTestContext(size)
		message := "Allow native playtests?\n/home/user/" + strings.Repeat("a-long-path/", 12)
		l := layoutConfirm(ui, message, []string{"Allow once", "Allow always", "Deny"})
		if l.window.X < 0 || l.window.Y < 0 || l.window.X+l.window.W > size.X || l.window.Y+l.window.H > size.Y {
			t.Fatalf("modal outside screen %v: %+v", size, l.window)
		}
		for i, r := range l.buttons {
			if r.X < l.window.X || r.X+r.W > l.window.X+l.window.W || r.Y < l.message.Y+l.message.H || r.Y+r.H > l.window.Y+l.window.H {
				t.Fatalf("button %d overlaps or escapes modal: %+v in %+v; message %+v", i, r, l.window, l.message)
			}
			for j := 0; j < i; j++ {
				q := l.buttons[j]
				if r.X < q.X+q.W && r.X+r.W > q.X && r.Y < q.Y+q.H && r.Y+r.H > q.Y {
					t.Fatalf("buttons overlap: %+v %+v", r, q)
				}
			}
		}
		ui.End()
	}
}
