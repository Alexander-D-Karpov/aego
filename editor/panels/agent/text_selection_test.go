package agent

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

type testTranscriptClipboard struct{ value string }

func (c *testTranscriptClipboard) Text() string     { return c.value }
func (c *testTranscriptClipboard) SetText(s string) { c.value = s }

func transcriptTestUI() *core.Context {
	f := text.NewFont(14, 1, 18, 14)
	for _, r := range " ?αβвводабстркаКопия😀return值123" {
		f.SetGlyph(r, text.FontGlyph{Advance: 8})
	}
	ui := core.New(f)
	ui.Theme = theme.Editor()
	return ui
}

func TestDragAndCopyUnicodeTranscript(t *testing.T) {
	ui := transcriptTestUI()
	clip := &testTranscriptClipboard{}
	ui.Clipboard = clip
	p := &Panel{blocks: []block{{kind: blockUser, text: "α😀β"}}}
	body := mathx.Rect{W: 280, H: 100}
	b := &p.blocks[0]
	b.h = p.measure(ui, b, 260)
	b.hw = 260
	var state input.State
	draw := func() {
		ui.Begin(&state, mathx.Vec2{X: 300, Y: 120}, 1)
		ui.Region(body)
		p.beginTextPick(ui, body, 0)
		p.pickText(ui, 0, b, mathx.Rect{X: 20, Y: 15, W: 180, H: 20})
		p.finishTextPick(ui)
		ui.PopCursor()
		ui.End()
	}
	at := mathx.Vec2{X: 28, Y: 22}
	state.BeginFrame()
	state.ApplyMouseMove(at, mathx.Vec2{})
	state.ApplyMouseDown(at, input.MouseLeft)
	draw()
	at.X = 36
	state.BeginFrame()
	state.ApplyMouseMove(at, mathx.Vec2{X: 8})
	draw()
	state.BeginFrame()
	state.ApplyMouseUp(at, input.MouseLeft)
	draw()
	state.BeginFrame()
	state.ApplyKeyDown(input.KeyC, 0, input.PrimaryMod(), false)
	draw()
	if clip.Text() != "😀" {
		t.Fatalf("copied %q; selection %+v..%+v", clip.Text(), p.textPick.a, p.textPick.b)
	}
}

func TestCopyRenderedMarkdownAcrossMessages(t *testing.T) {
	ui := transcriptTestUI()
	clip := &testTranscriptClipboard{}
	ui.Clipboard = clip
	p := &Panel{blocks: []block{
		{kind: blockUser, text: "αβ\n  ввод"},
		{kind: blockNotice, text: "internal status"},
		{kind: blockText, text: "**строка** [link](https://example.org)\n\n```go\n\treturn 123\n```"},
	}}
	for i := range p.blocks {
		b := &p.blocks[i]
		b.hw = 180
		b.h = p.measure(ui, b, b.hw)
	}
	g := p.textPick.geometry(ui, 2, &p.blocks[2])
	if g.plain != "строка link\n\treturn 123" {
		t.Fatalf("rendered copy %q", g.plain)
	}
	p.textPick.set = true
	// A reverse drag across blocks must preserve the original hard break,
	// leading whitespace and code tab while omitting Markdown delimiters.
	p.textPick.a = textPosition{2, len(g.plain)}
	p.textPick.b = textPosition{0, len("α")}
	p.copyPicked(ui)
	if want := "β\n  ввод\n\nстрока link\n\treturn 123"; clip.Text() != want {
		t.Fatalf("got %q, want %q", clip.Text(), want)
	}
	if !strings.Contains(p.blocks[2].text, "https://example.org") {
		t.Fatal("rendering mutated the original message")
	}
}

func TestClickMarkdownLinkStillOpensButDragDoesNot(t *testing.T) {
	ui := transcriptTestUI()
	var opened []string
	p := &Panel{blocks: []block{{kind: blockText, text: "[link](https://example.org)"}}, d: Deps{Open: func(target string) { opened = append(opened, target) }}}
	b := &p.blocks[0]
	b.hw = 240
	b.h = p.measure(ui, b, b.hw)
	body := mathx.Rect{W: 240, H: 100}
	var state input.State
	draw := func() {
		ui.Begin(&state, mathx.Vec2{X: 300, Y: 130}, 1)
		ui.Region(body)
		p.beginTextPick(ui, body, 0)
		p.draw(ui, 0, b, body)
		p.finishTextPick(ui)
		ui.PopCursor()
		ui.End()
	}
	click := func(x float32, move bool) {
		at := mathx.Vec2{X: 2*ui.Theme.Padding + x, Y: 2*ui.Theme.Padding + 8}
		state.BeginFrame()
		state.ApplyMouseMove(at, mathx.Vec2{})
		state.ApplyMouseDown(at, input.MouseLeft)
		draw()
		if move {
			at.X += 12
			state.BeginFrame()
			state.ApplyMouseMove(at, mathx.Vec2{X: 12})
			draw()
		}
		state.BeginFrame()
		state.ApplyMouseUp(at, input.MouseLeft)
		draw()
	}
	click(1, false)
	if len(opened) != 1 || opened[0] != "https://example.org" {
		t.Fatalf("click: %v", opened)
	}
	click(1, true)
	if len(opened) != 1 || p.textPick.a == p.textPick.b {
		t.Fatalf("drag opened a link or made no selection: %v", opened)
	}
	at := mathx.Vec2{X: 2*ui.Theme.Padding + 8, Y: 2*ui.Theme.Padding + 8}
	state.BeginFrame()
	state.ApplyMouseDown(at, input.MouseRight)
	draw()
	if !ui.PopupOpen(p.textPick.popup) || p.textPick.menu != 0 {
		t.Fatal("missing message context menu")
	}
}

func TestWrappedMessageSelectionKeepsUnicodeCharacters(t *testing.T) {
	ui := transcriptTestUI()
	clip := &testTranscriptClipboard{}
	ui.Clipboard = clip
	const msg = "абв😀😀😀абв😀😀"
	for _, width := range []float32{1, 24, 40} {
		lines, starts := wrapMessage(ui, msg, width)
		p := &Panel{blocks: []block{{kind: blockUser, text: msg, lines: lines, lineBytes: starts, hw: width}}}
		g := p.textPick.geometry(ui, 0, &p.blocks[0])
		for i, line := range lines {
			if !utf8.ValidString(line) {
				t.Errorf("width %g, wrapped at byte %d inside a character: %q", width, starts[i], line)
				break
			}
			a := g.offset(mathx.Vec2{X: 0, Y: float32(i) * ui.LineHeight()})
			b := g.offset(mathx.Vec2{X: width + 8, Y: float32(i) * ui.LineHeight()})
			p.textPick.a, p.textPick.b, p.textPick.set = textPosition{0, a}, textPosition{0, b}, true
			p.copyPicked(ui)
			if clip.Text() != line {
				t.Fatalf("copied %q, want complete line %q", clip.Text(), line)
			}
		}
	}
}
