package shell

import (
	"fmt"
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/theme"
	"aego/ui/widgets"
)

func TestQuestionPromptFitsWarningFieldsAndActions(t *testing.T) {
	bundle := i18n.Default()
	t.Cleanup(func() { bundle.SetLanguage(i18n.English) })
	font := text.NewFont(14, 1, 16, 12)
	for r := rune(32); r <= 0x04ff; r++ {
		font.SetGlyph(r, text.FontGlyph{Advance: 8})
	}
	for _, language := range []string{i18n.English, i18n.Russian} {
		bundle.SetLanguage(language)
		q := context.Question{
			Title:   i18n.Tr("editor.art.create_linked_source", "Create linked art source"),
			Message: i18n.Tr("editor.art.linked_source_notice", "Create an editable source for {path}. Existing sprite and animation references keep their IDs. Edits will update every consumer of this shared asset; the original PNG is preserved.", i18n.Str("path", "assets/surfaces/runner.png")),
			Fields:  []string{i18n.Tr("editor.art.source_path", "Source path")},
		}
		labels := [2]string{i18n.Tr("editor.art.create_linked", "Create linked source"), i18n.Tr("editor.art.cancel", "Cancel")}
		for _, size := range []mathx.Vec2{{X: 1280, Y: 900}, {X: 720, Y: 500}, {X: 360, Y: 540}, {X: 320, Y: 180}, {X: 220, Y: 220}} {
			t.Run(fmt.Sprintf("%s-%.0fx%.0f", language, size.X, size.Y), func(t *testing.T) {
				ui := core.New(font)
				ui.Theme = theme.Editor()
				ui.Begin(&input.State{}, size, 1)
				defer ui.End()
				l := layoutPrompt(ui, q, labels)
				inside := func(inner, outer mathx.Rect) bool {
					return inner.X >= outer.X && inner.Y >= outer.Y && inner.X+inner.W <= outer.X+outer.W && inner.Y+inner.H <= outer.Y+outer.H
				}
				if !inside(l.window, mathx.Rect{W: size.X, H: size.Y}) || !inside(l.body, l.window) || l.body.H <= 0 {
					t.Fatalf("prompt leaves the viewport or hides its content: %+v", l)
				}
				for i, button := range l.buttons {
					if !inside(button, l.window) || button.Y < l.body.Y+l.body.H {
						t.Fatalf("action %q is outside the modal or overlaps the body: %+v", labels[i], l)
					}
					lines := widgets.WrapLines(ui, labels[i], button.W-28)
					if strings.Join(strings.Fields(strings.Join(lines, " ")), " ") != strings.Join(strings.Fields(labels[i]), " ") || float32(len(lines))*ui.Theme.RowHeight > button.H {
						t.Fatalf("action text was shortened or does not fit: %q %+v", labels[i], button)
					}
					for _, line := range lines {
						if ui.TextSize(line).X > button.W-28 {
							t.Fatalf("action line %q overflows %+v", line, button)
						}
					}
				}
				a, b := l.buttons[0], l.buttons[1]
				if a.X < b.X+b.W && a.X+a.W > b.X && a.Y < b.Y+b.H && a.Y+a.H > b.Y {
					t.Fatalf("confirmation and cancellation overlap: %+v", l.buttons)
				}
				warning := widgets.WrapLines(ui, q.Message, l.body.W)
				// A path can wrap mid-word at the narrowest size. Only visual
				// whitespace changes; every warning/path character must remain.
				if len(warning) < 2 || strings.Join(strings.Fields(strings.Join(warning, "")), "") != strings.Join(strings.Fields(q.Message), "") {
					t.Fatal("shared-asset warning is not fully preserved by wrapping")
				}
				if len(l.fields) != 1 || l.fields[0]+ui.Theme.RowHeight > l.content+0.01 {
					t.Fatal("the path field is missing from the scrollable body")
				}
				if size.Y <= 220 && l.content <= l.body.H {
					t.Fatal("short window did not retain a scrollable warning and field")
				}
			})
		}
	}
}
