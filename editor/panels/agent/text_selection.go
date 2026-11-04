package agent

import (
	"strings"

	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/richtext"
	"aego/ui/widgets"
)

type textPosition struct{ block, offset int }

func (a textPosition) before(b textPosition) bool {
	return a.block < b.block || a.block == b.block && a.offset < b.offset
}

type glyphSpan struct {
	x    float32
	at   int
	text string
	font *text.Font
	code bool
}

type selectionRow struct {
	y, h  float32
	spans []glyphSpan
}

type textGeometry struct {
	plain string
	rows  []selectionRow
}

type selectionCache struct {
	text     string
	width    float32
	font     *text.Font
	geometry *textGeometry
}

type transcriptPick struct {
	codeBlock            int
	a, b                 textPosition
	set, dragging, moved bool
	origin               mathx.Vec2
	body                 mathx.Rect
	focus, popup         core.ID
	menu                 int
	view, session        string
	cache                map[int]selectionCache
}

func wrapMessage(ui *core.Context, msg string, width float32) ([]string, []int) {
	if msg == "" {
		return nil, nil
	}
	var lines []string
	var starts []int
	base := 0
	for raw := range strings.SplitSeq(msg, "\n") {
		from := 0
		part := widgets.WrapLines(ui, raw, width)
		if len(part) == 0 {
			part = []string{""}
		}
		for _, line := range part {
			pos := from + strings.Index(raw[from:], line)
			lines = append(lines, line)
			starts = append(starts, base+pos)
			from = pos + len(line)
		}
		base += len(raw) + 1
	}
	return lines, starts
}

// Keep font runs rather than one heap entry per glyph across the transcript.
// Resolve byte offsets from actual advances only at the selection boundaries.
func (line *selectionRow) glyphs(s string, offset int, x float32, f *text.Font, code bool) {
	line.spans = append(line.spans, glyphSpan{x: x, at: offset, text: s, font: f, code: code})
}

func (run *glyphSpan) advance(prev, ch rune) float32 {
	if run.code && ch == '\t' {
		return 4 * run.font.Advance(' ')
	}
	return run.font.Kern(prev, ch) + run.font.Advance(ch)
}

func (run *glyphSpan) hit(x float32) int {
	pos := run.x
	var prev rune
	for i, ch := range run.text {
		next := pos + run.advance(prev, ch)
		if x < (pos+next)/2 {
			return run.at + i
		}
		pos, prev = next, ch
		if run.code && ch == '\t' {
			prev = ' '
		}
	}
	return run.at + len(run.text)
}

func (run *glyphSpan) edge(at int) float32 {
	pos := run.x
	var prev rune
	for i, ch := range run.text {
		if run.at+i >= at {
			break
		}
		pos += run.advance(prev, ch)
		prev = ch
		if run.code && ch == '\t' {
			prev = ' '
		}
	}
	return pos
}

func messageGeometry(ui *core.Context, b *block) *textGeometry {
	g := &textGeometry{}
	if b.kind == blockUser {
		g.plain = b.text
		for i, line := range b.lines {
			r := selectionRow{y: float32(i) * ui.LineHeight(), h: ui.LineHeight()}
			r.glyphs(line, b.lineBytes[i], 0, ui.Font(), false)
			g.rows = append(g.rows, r)
		}
		return g
	}
	var plain strings.Builder
	var previous *richtext.Line
	var codeRow int
	for i := range b.md.Doc().Lines {
		line := &b.md.Doc().Lines[i]
		r := selectionRow{y: line.Y, h: line.H}
		if previous != nil {
			if line.Block == previous.Block && (line.Kind == richtext.TextLine || line.Kind == richtext.QuoteLine || line.Kind == richtext.HeadingLine) {
				if len(line.Runs) > 0 && line.Runs[0].BreakSpace {
					plain.WriteByte(' ')
				}
			} else {
				plain.WriteByte('\n')
			}
		}
		if previous == nil || previous.Block != line.Block {
			codeRow = 0
		}
		if line.Kind == richtext.CodeLine {
			block := &b.md.Blocks()[line.Block]
			raw := ""
			if codeRow < len(block.Lines) {
				raw = block.Lines[codeRow]
			}
			r.glyphs(raw, plain.Len(), richtext.CodePad, b.md.RunFont(ui, richtext.Mono), true)
			plain.WriteString(raw)
			codeRow++
		} else {
			endX := float32(0)
			for j, run := range line.Runs {
				if j > 0 && run.X > endX+1 {
					if line.Kind == richtext.TableLine {
						plain.WriteByte('\t')
					} else {
						plain.WriteByte(' ')
					}
				}
				r.glyphs(run.Text, plain.Len(), run.X, b.md.RunFont(ui, run.Face), false)
				plain.WriteString(run.Text)
				endX = run.X + run.W
			}
		}
		if len(r.spans) == 0 {
			r.spans = []glyphSpan{{at: plain.Len()}}
		}
		g.rows = append(g.rows, r)
		previous = line
	}
	g.plain = plain.String()
	return g
}

func (g *textGeometry) offset(mouse mathx.Vec2) int {
	if len(g.rows) == 0 {
		return 0
	}
	row := &g.rows[len(g.rows)-1]
	for i := range g.rows {
		if mouse.Y < g.rows[i].y+g.rows[i].h {
			row = &g.rows[i]
			break
		}
	}
	for i := range row.spans {
		run := &row.spans[i]
		if i+1 == len(row.spans) || mouse.X < row.spans[i+1].x {
			return run.hit(mouse.X)
		}
	}
	return 0
}

func (g *textGeometry) highlight(ui *core.Context, r mathx.Rect, from, to int) {
	c := ui.Theme.Accent
	c.A = .30
	clip, clipped := ui.ClipRect()
	for _, row := range g.rows {
		if clipped && (r.Y+row.y+row.h < clip.Y || r.Y+row.y > clip.Y+clip.H) {
			continue
		}
		start, end := float32(0), float32(0)
		hit := false
		for i := range row.spans {
			run := &row.spans[i]
			if run.at < to && run.at+len(run.text) > from {
				if !hit {
					start = run.edge(from)
					hit = true
				}
				end = run.edge(to)
			}
		}
		if hit && end > start {
			ui.Rect(mathx.Rect{X: r.X + start, Y: r.Y + row.y, W: end - start, H: row.h}, c)
		}
	}
}

func (s *transcriptPick) geometry(ui *core.Context, i int, b *block) *textGeometry {
	if x, ok := s.cache[i]; ok && x.text == b.text && x.width == b.hw && x.font == ui.Font() {
		return x.geometry
	}
	if s.cache == nil {
		s.cache = make(map[int]selectionCache)
	} else if len(s.cache) >= 8 {
		clear(s.cache)
	}
	g := messageGeometry(ui, b)
	s.cache[i] = selectionCache{b.text, b.hw, ui.Font(), g}
	return g
}

func (s *transcriptPick) bounds() (textPosition, textPosition) {
	if s.b.before(s.a) {
		return s.b, s.a
	}
	return s.a, s.b
}

func (p *Panel) beginTextPick(ui *core.Context, body mathx.Rect, maxOff float32) {
	s := &p.textPick
	if s.view != p.viewing || s.session != p.session {
		*s = transcriptPick{view: p.viewing, session: p.session}
	}
	s.body, s.focus, s.popup = body, ui.ID("transcript.pick"), ui.ID("transcript.copy-menu")
	if s.dragging {
		y := ui.Mouse().Y
		if y < body.Y+8 || y > body.Y+body.H-8 {
			d := ui.LineHeight()
			if y < body.Y+8 {
				d = -d
			}
			p.scroll = mathx.Clamp(p.scroll+d, 0, maxOff)
			p.stick = false
		}
	}
}

func (p *Panel) pickText(ui *core.Context, i int, b *block, r mathx.Rect) {
	s := &p.textPick
	if ui.Font() == nil || ui.Input() == nil || r.H <= 0 {
		return
	}
	oldA, oldB := s.bounds()
	selected := s.set && i >= oldA.block && i <= oldB.block
	mouse := ui.Mouse()
	if s.dragging {
		mouse.Y = mathx.Clamp(mouse.Y, s.body.Y+1, s.body.Y+s.body.H-1)
	}
	onLine := mouse.Y >= r.Y && mouse.Y <= r.Y+r.H
	pressed, right := ui.Input().ButtonPressed(input.MouseLeft), ui.Input().ButtonPressed(input.MouseRight)
	hover := (pressed || right) && ui.MouseOver(ui.ID("selectable-text"), r)
	if !selected && !hover && !(s.dragging && onLine) {
		return
	}
	if pressed && hover {
		clear(s.cache)
		s.origin, s.dragging, s.moved, s.set = mouse, true, false, true
		ui.SetFocus(s.focus)
	}
	g := s.geometry(ui, i, b)
	point := textPosition{i, g.offset(mathx.Vec2{X: mouse.X - r.X, Y: mouse.Y - r.Y})}
	if pressed && hover {
		s.a, s.b = point, point
	}
	if s.dragging && onLine && (mouse.X != s.origin.X || mouse.Y != s.origin.Y) {
		s.b = point
		if mathx.Abs(mouse.X-s.origin.X) > 3 || mathx.Abs(mouse.Y-s.origin.Y) > 3 {
			s.moved = true
		}
	}
	if right && hover {
		a, z := s.bounds()
		if !s.set || point.before(a) || z.before(point) {
			s.a, s.b, s.set = point, point, true
			clear(s.cache)
		}
		s.menu, s.codeBlock = i, -1
		if b.kind == blockText {
			for _, line := range b.md.Doc().Lines {
				if line.Kind == richtext.CodeLine && mouse.Y-r.Y >= line.Y && mouse.Y-r.Y < line.Y+line.H {
					s.codeBlock = line.Block
					break
				}
			}
		}
		ui.SetFocus(s.focus)
		ui.OpenPopup(s.popup, ui.Mouse())
	}
	a, z := s.bounds()
	if !s.set || a == z || i < a.block || i > z.block {
		return
	}
	from, to := 0, len(g.plain)
	if i == a.block {
		from = a.offset
	}
	if i == z.block {
		to = z.offset
	}
	g.highlight(ui, r, from, to)
}

func (p *Panel) copyPicked(ui *core.Context) {
	s := &p.textPick
	a, z := s.bounds()
	if !s.set || a == z || ui.Clipboard == nil {
		return
	}
	var out strings.Builder
	list := p.list()
	for i := max(0, a.block); i <= min(z.block, len(list)-1); i++ {
		b := &list[i]
		if b.kind != blockUser && b.kind != blockText {
			continue
		}
		g := s.geometry(ui, i, b)
		from, to := 0, len(g.plain)
		if i == a.block {
			from = min(to, a.offset)
		}
		if i == z.block {
			to = min(to, z.offset)
		}
		if from >= to {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(g.plain[from:to])
	}
	if out.Len() > 0 {
		ui.Clipboard.SetText(out.String())
	}
}

func (p *Panel) finishTextPick(ui *core.Context) {
	s := &p.textPick
	if ui.Input() != nil && !ui.Input().ButtonDown(input.MouseLeft) {
		if s.dragging && ui.Input().ButtonReleased(input.MouseLeft) {
			// A Markdown link owns its initial press. Focus the transcript after
			// release so an ordinary click still opens the link.
			ui.SetFocus(s.focus)
		}
		s.dragging = false
	}
	if ui.Focus() == s.focus && ui.Primary() && ui.KeyPressed(input.KeyC) {
		p.copyPicked(ui)
	}
	if widgets.ContextMenu(ui, s.popup) {
		available := ui.Clipboard != nil
		if widgets.MenuItem(ui, i18n.Tr("editor.agent.copy_selection", "Copy selection"), "", available && s.set && s.a != s.b, false) {
			p.copyPicked(ui)
		}
		list := p.list()
		if s.menu >= 0 && s.menu < len(list) && list[s.menu].kind == blockText && s.codeBlock >= 0 {
			code := list[s.menu].md.Blocks()
			if s.codeBlock < len(code) && code[s.codeBlock].Kind == richtext.CodeBlock && widgets.MenuItem(ui, i18n.Tr("editor.agent.copy_code", "Copy code block"), "", available, false) {
				ui.Clipboard.SetText(strings.Join(code[s.codeBlock].Lines, "\n"))
			}
		}
		if widgets.MenuItem(ui, i18n.Tr("editor.agent.copy_message", "Copy original message"), "", available && s.menu >= 0 && s.menu < len(list), false) {
			ui.Clipboard.SetText(list[s.menu].text)
		}
		widgets.EndContextMenu(ui)
	}
}
