package agent

import (
	"strings"
	"testing"
	"unicode/utf8"

	"aego/core/mathx"
	"aego/render/text"
)

func TestAssistantWrappedIdentifiersAreSelectable(t *testing.T) {
	id := strings.Repeat("Wя😀ii", 8)
	for _, tc := range []struct{ name, markdown, want string }{
		{"long inline asset", "`" + id + "`", id},
		{"adjacent styles", "WWWW**яяя**", "WWWWяяя"},
		{"real separator", "WWWW **яяя**", "WWWW яяя"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ui := transcriptTestUI()
			for ch, advance := range map[rune]float32{'W': 10, 'я': 9, '😀': 14, 'i': 3, ' ': 5} {
				ui.Font().SetGlyph(ch, text.FontGlyph{Advance: advance})
			}
			ui.Font().SetKern('W', 'я', -2)
			clip := &testTranscriptClipboard{}
			ui.Clipboard = clip
			p := &Panel{blocks: []block{{kind: blockText, text: tc.markdown}}}
			b := &p.blocks[0]
			b.hw = 48 + 4*ui.Theme.Padding + ui.Theme.ScrollbarW
			b.h = p.measure(ui, b, b.hw)
			g := p.textPick.geometry(ui, 0, b)
			if len(g.rows) < 2 {
				t.Fatalf("identifier did not wrap inside the %g-pixel text column: %+v", b.md.Doc().Width, b.md.Doc().Lines)
			}
			for _, row := range g.rows {
				for _, span := range row.spans {
					if !utf8.ValidString(span.text) || span.edge(span.at+len(span.text)) > b.md.Doc().Width+.01 {
						t.Fatalf("text runs beyond the selectable area: %q", span.text)
					}
				}
			}
			first, last := g.rows[0], g.rows[len(g.rows)-1]
			p.textPick.a = textPosition{0, g.offset(mathx.Vec2{X: 0, Y: first.y + first.h/2})}
			p.textPick.b = textPosition{0, g.offset(mathx.Vec2{X: b.md.Doc().Width, Y: last.y + last.h/2})}
			p.textPick.set = true
			p.copyPicked(ui)
			if got := clip.Text(); got != tc.want {
				t.Fatalf("mouse selection copied %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAssistantSelectionKeepsMarkdownSeparators(t *testing.T) {
	ui := transcriptTestUI()
	for _, tc := range []struct {
		name, markdown, want string
		width                float32
	}{
		{"ordinary words", "word1 word2", "word1 word2", 48},
		{"compact table", "| Left | Right |\n|---|---|\n| one | two |", "Left\tRight\none\ttwo", 200},
		{"fenced code", "```go\n\treturn 123\n```", "\treturn 123", 48},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &Panel{blocks: []block{{kind: blockText, text: tc.markdown}}}
			b := &p.blocks[0]
			b.hw = tc.width + 4*ui.Theme.Padding + ui.Theme.ScrollbarW
			b.h = p.measure(ui, b, b.hw)
			g := p.textPick.geometry(ui, 0, b)
			if g.plain != tc.want {
				t.Fatalf("rendered text %q, want %q", g.plain, tc.want)
			}
			switch tc.name {
			case "ordinary words", "compact table":
				if len(g.rows) != 2 {
					t.Fatalf("expected two rows, got %d", len(g.rows))
				}
			case "fenced code":
				if len(g.rows) != 1 {
					t.Fatalf("code line fragmented: %d rows", len(g.rows))
				}
			}
		})
	}
}
