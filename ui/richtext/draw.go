package richtext

import (
	"aego/core/mathx"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/theme"
	"aego/ui/widgets"
)

type fonts struct {
	face [H3 + 1]*text.Font
}

func (f *fonts) Width(face Face, s string) float32 {
	return f.face[face].Measure(s).X
}

func (f *fonts) Height(face Face) float32 {
	return f.face[face].LineHeight
}

func pick(ui *core.Context) *fonts {
	body := ui.Font()
	fs := &fonts{}
	fs.face[Body] = body
	fs.face[Mono] = ui.MonoFont()
	for i, k := range [...]float32{1.6, 1.3, 1.1} {
		f := body
		if set := ui.Fonts(); set != nil {
			if c := set.Closest(ui.Theme.FontSize*k, ui.Scale()); c != nil {
				f = c
			}
		}
		fs.face[H1+Face(i)] = f
	}
	return fs
}

type Action struct {
	Target string
	Run    int
}

type Text struct {
	Highlight  Highlighter
	RunButtons bool
	Scroll     float32
	blocks     []Block
	doc        *Doc
	width      float32
	key        *text.Font
}

func (t *Text) Set(blocks []Block) {
	t.blocks, t.doc, t.Scroll = blocks, nil, 0
}

func (t *Text) Replace(blocks []Block) {
	t.blocks, t.doc = blocks, nil
}

func (t *Text) Blocks() []Block {
	return t.blocks
}

func (t *Text) Doc() *Doc {
	return t.doc
}

func tokenColor(c *theme.CodeTheme, kind string) mathx.Color {
	switch kind {
	case "keyword":
		return c.Keyword
	case "type":
		return c.Type
	case "string":
		return c.String
	case "number":
		return c.Number
	case "comment":
		return c.Comment
	case "attr":
		return c.Attribute
	case "builtin":
		return c.Builtin
	case "func":
		return c.Function
	}
	return c.Plain
}

func runColor(th *theme.Theme, l *Line, r *Run) mathx.Color {
	switch {
	case l.Kind == CodeLine:
		return tokenColor(&th.Code, r.Token)
	case r.Target != "":
		return th.Accent
	case r.Style == Code:
		return th.Code.Plain
	case r.Style == Emphasis:
		return th.TextDim
	}
	return th.Text
}

func (t *Text) background(ui *core.Context, l *Line, x, y, w float32) {
	th := &ui.Theme
	switch l.Kind {
	case CodeLine:
		top, h := y, l.H
		if l.First {
			top -= CodePad
			h += CodePad
		}
		if l.Last {
			h += CodePad
		}
		ui.Rect(mathx.Rect{X: x, Y: top, W: w, H: h}, th.Code.Background)
	case TableLine:
		cols := l.Cols
		right := x + cols[len(cols)-1]
		top, h := y, l.H
		if l.RowTop {
			top -= 3
			h += 3
			ui.Rect(mathx.Rect{X: x, Y: top, W: cols[len(cols)-1], H: 1}, th.Border)
		}
		if l.Last {
			h += 3
			ui.Rect(mathx.Rect{X: x, Y: top + h, W: cols[len(cols)-1], H: 1}, th.Border)
		}
		if l.Header {
			ui.Rect(mathx.Rect{X: x, Y: top, W: cols[len(cols)-1], H: h}, th.PanelAlt)
		}
		for _, cx := range cols {
			ui.Rect(mathx.Rect{X: min(x+cx, right-1), Y: top, W: 1, H: h + 1}, th.Border)
		}
	case QuoteLine:
		ui.Rect(mathx.Rect{X: x + 4, Y: y, W: 2, H: l.H}, th.AccentDim)
	case HeadingLine:
		if l.Level <= 2 {
			ui.Rect(mathx.Rect{X: x, Y: y + l.H + 1, W: w, H: 1}, th.Separator)
		}
	case RuleLine:
		ui.Rect(mathx.Rect{X: x, Y: y + l.H/2, W: w, H: 1}, th.Border)
	}
}

func (t *Text) Draw(ui *core.Context, id core.ID, r mathx.Rect) Action {
	act := Action{Run: -1}
	if ui.Font() == nil {
		return act
	}
	th := &ui.Theme
	fs := pick(ui)
	pad := th.Padding * 2
	width := max(40, r.W-2*pad-th.ScrollbarW)
	if t.doc == nil || width != t.width || fs.face[Body] != t.key {
		t.doc = Layout(t.blocks, width, fs, t.Highlight)
		t.width, t.key = width, fs.face[Body]
	}
	d := t.doc
	content := d.Height + 2*pad
	maxOff := max(0, content-r.H)
	if ui.MouseOver(id, r) {
		if w := ui.ScrollY(th.RowHeight * 3); w != 0 {
			t.Scroll -= w
		}
	}
	t.Scroll = mathx.Clamp(t.Scroll, 0, maxOff)
	ox, oy := r.X+pad, r.Y+pad-t.Scroll
	ui.PushClip(r)
	link := 0
	for i := range d.Lines {
		l := &d.Lines[i]
		y := oy + l.Y
		if y-CodePad > r.Y+r.H {
			break
		}
		if y+l.H+CodePad < r.Y {
			for j := range l.Runs {
				if l.Runs[j].Target != "" {
					link++
				}
			}
			continue
		}
		t.background(ui, l, ox, y, width)
		for j := range l.Runs {
			run := &l.Runs[j]
			f := fs.face[run.Face]
			ui.PushFont(f)
			pos := mathx.Vec2{X: ox + run.X, Y: y + (l.H-f.LineHeight)/2}
			rect := mathx.Rect{X: pos.X, Y: y, W: run.W, H: l.H}
			if run.Style == Code && l.Kind != CodeLine {
				ui.Rect(mathx.Rect{X: rect.X - 2, Y: rect.Y + 1, W: rect.W + 4, H: rect.H - 2}, th.Code.Background)
			}
			col := runColor(th, l, run)
			if run.Target != "" {
				lid := core.IDSeq(id, 1000+link)
				link++
				if ui.Clicked(lid, rect) {
					act.Target = run.Target
				}
				if ui.Hot() == lid {
					ui.Rect(mathx.Rect{X: rect.X, Y: rect.Y + rect.H - 2, W: rect.W, H: 1}, th.Accent)
				}
			}
			ui.Text(run.Text, pos, col)
			if run.Style == Strong || l.Kind == HeadingLine || l.Header {
				ui.Text(run.Text, mathx.Vec2{X: pos.X + 0.6, Y: pos.Y}, col)
			}
			ui.PopFont()
		}
		if l.Run && t.RunButtons {
			bw := ui.TextSize("Run").X + 16
			br := mathx.Rect{X: ox + width - bw - 4, Y: y - CodePad + 4, W: bw, H: th.RowHeight + 2}
			if widgets.ButtonRect(ui, core.IDSeq(id, 50000+l.Block), br, "Run") {
				act.Run = l.Block
			}
		}
	}
	ui.PopClip()
	if content > r.H {
		track := mathx.Rect{X: r.X + r.W - th.ScrollbarW, Y: r.Y, W: th.ScrollbarW, H: r.H}
		ui.Rect(track, th.PanelAlt)
		thumbH := max(20, r.H*r.H/content)
		thumb := mathx.Rect{X: track.X + 1, Y: r.Y + (t.Scroll/maxOff)*(r.H-thumbH), W: track.W - 2, H: thumbH}
		tid := core.IDSeq(id, 1)
		if dlt, ok := ui.Drag(tid, thumb); ok && dlt.Y != 0 && r.H > thumbH {
			t.Scroll = mathx.Clamp(t.Scroll+dlt.Y*maxOff/(r.H-thumbH), 0, maxOff)
		}
		col := th.Widget
		if ui.Hot() == tid || ui.Active() == tid {
			col = th.WidgetHot
		}
		ui.Rect(thumb, col)
	}
	return act
}
