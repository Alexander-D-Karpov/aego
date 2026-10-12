package richtext

import (
	"sort"
	"strconv"
	"strings"
)

type Face uint8

const (
	Body Face = iota
	Mono
	H1
	H2
	H3
)

type Measurer interface {
	Width(f Face, s string) float32
	Height(f Face) float32
}

type Token struct {
	Start int
	End   int
	Kind  string
}

type Highlighter func(lang, line string) []Token

type Run struct {
	Text   string
	X      float32
	W      float32
	Face   Face
	Style  Style
	Target string
	Token  string
}

type LineKind uint8

const (
	TextLine LineKind = iota
	HeadingLine
	CodeLine
	TableLine
	RuleLine
	QuoteLine
)

type Line struct {
	Kind   LineKind
	Y      float32
	H      float32
	Runs   []Run
	Block  int
	First  bool
	Last   bool
	Header bool
	RowTop bool
	Cols   []float32
	Level  int
	Run    bool
}

type Doc struct {
	Lines  []Line
	Height float32
	Width  float32
}

const (
	BlockGap   = 8
	ListIndent = 18
	CodePad    = 8
	CellPad    = 6
	QuoteInset = 14
	MinColumn  = 48
)

type piece struct {
	text   string
	style  Style
	target string
	face   Face
	space  bool
}

func pieces(spans []Span, face Face) []piece {
	var out []piece
	for _, sp := range spans {
		f := face
		if sp.Style == Code {
			f = Mono
		}
		s := sp.Text
		for s != "" {
			if s[0] == ' ' {
				j := 0
				for j < len(s) && s[j] == ' ' {
					j++
				}
				out = append(out, piece{text: " ", style: sp.Style, target: sp.Target, face: f, space: true})
				s = s[j:]
				continue
			}
			j := strings.IndexByte(s, ' ')
			if j < 0 {
				j = len(s)
			}
			out = append(out, piece{text: s[:j], style: sp.Style, target: sp.Target, face: f})
			s = s[j:]
		}
	}
	return out
}

func wrap(spans []Span, face Face, x0, width float32, m Measurer) [][]Run {
	var lines [][]Run
	var cur []Run
	x := x0
	limit := x0 + width
	for _, p := range pieces(spans, face) {
		if p.space && len(cur) == 0 {
			continue
		}
		w := m.Width(p.face, p.text)
		if !p.space && len(cur) > 0 && x+w > limit {
			if last := &cur[len(cur)-1]; strings.HasSuffix(last.Text, " ") {
				last.Text = strings.TrimRight(last.Text, " ")
				last.W = m.Width(last.Face, last.Text)
			}
			lines = append(lines, cur)
			cur, x = nil, x0
		}
		if n := len(cur); n > 0 {
			last := &cur[n-1]
			if last.Style == p.style && last.Target == p.target && last.Face == p.face {
				last.Text += p.text
				last.W = m.Width(last.Face, last.Text)
				x = last.X + last.W
				continue
			}
		}
		cur = append(cur, Run{Text: p.text, X: x, W: w, Face: p.face, Style: p.style, Target: p.target})
		x += w
	}
	if len(cur) > 0 {
		lines = append(lines, cur)
	}
	return lines
}

func headingFace(level int) Face {
	switch level {
	case 1:
		return H1
	case 2:
		return H2
	}
	return H3
}

type builder struct {
	m     Measurer
	hl    Highlighter
	width float32
	doc   Doc
	y     float32
}

func (b *builder) add(l Line) {
	l.Y = b.y
	b.doc.Lines = append(b.doc.Lines, l)
	b.y += l.H
}

func (b *builder) text(block int, kind LineKind, spans []Span, face Face, x float32, level int) {
	h := b.m.Height(face)
	for _, runs := range wrap(spans, face, x, b.width-x, b.m) {
		b.add(Line{Kind: kind, H: h, Runs: runs, Block: block, Level: level})
	}
}

func (b *builder) code(block int, blk *Block) {
	h := b.m.Height(Mono)
	b.y += CodePad
	start := len(b.doc.Lines)
	lines := blk.Lines
	if len(lines) == 0 {
		lines = []string{""}
	}
	for _, src := range lines {
		src = strings.ReplaceAll(src, "\t", "    ")
		l := Line{Kind: CodeLine, H: h, Block: block}
		x := float32(CodePad)
		var toks []Token
		if b.hl != nil {
			toks = b.hl(blk.Lang, src)
			sort.Slice(toks, func(i, j int) bool { return toks[i].Start < toks[j].Start })
		}
		at := 0
		emit := func(s, kind string) {
			if s == "" {
				return
			}
			w := b.m.Width(Mono, s)
			l.Runs = append(l.Runs, Run{Text: s, X: x, W: w, Face: Mono, Style: Code, Token: kind})
			x += w
		}
		for _, t := range toks {
			if t.Start < at || t.End > len(src) || t.End <= t.Start {
				continue
			}
			emit(src[at:t.Start], "")
			emit(src[t.Start:t.End], t.Kind)
			at = t.End
		}
		emit(src[at:], "")
		b.add(l)
	}
	b.doc.Lines[start].First = true
	b.doc.Lines[start].Run = blk.Runnable()
	b.doc.Lines[len(b.doc.Lines)-1].Last = true
	b.y += CodePad
}

func (b *builder) table(block int, blk *Block) {
	cols := 0
	for _, r := range blk.Rows {
		cols = max(cols, len(r))
	}
	if cols == 0 {
		return
	}
	natural := make([]float32, cols)
	for _, r := range blk.Rows {
		for c, cell := range r {
			w := float32(0)
			for _, p := range pieces(cell, Body) {
				w += b.m.Width(p.face, p.text)
			}
			natural[c] = max(natural[c], w+2*CellPad)
		}
	}
	total := float32(0)
	for _, w := range natural {
		total += w
	}
	widths := natural
	if total > b.width {
		widths = make([]float32, cols)
		for c, w := range natural {
			widths[c] = max(MinColumn, w*b.width/total)
		}
	}
	xs := make([]float32, cols+1)
	for c := range widths {
		xs[c+1] = xs[c] + widths[c]
	}
	h := b.m.Height(Body)
	for ri, r := range blk.Rows {
		var cellLines [][][]Run
		rows := 1
		for c := 0; c < cols; c++ {
			var cl [][]Run
			if c < len(r) {
				cl = wrap(r[c], Body, xs[c]+CellPad, widths[c]-2*CellPad, b.m)
			}
			cellLines = append(cellLines, cl)
			rows = max(rows, len(cl))
		}
		b.y += 3
		for li := 0; li < rows; li++ {
			l := Line{Kind: TableLine, H: h, Block: block, Header: blk.Header && ri == 0, Cols: xs}
			if li == 0 {
				l.RowTop = true
			}
			for _, cl := range cellLines {
				if li < len(cl) {
					l.Runs = append(l.Runs, cl[li]...)
				}
			}
			b.add(l)
		}
		b.doc.Lines[len(b.doc.Lines)-1].Last = true
		b.y += 3
	}
}

func Layout(blocks []Block, width float32, m Measurer, hl Highlighter) *Doc {
	b := &builder{m: m, hl: hl, width: max(width, 40)}
	for i := range blocks {
		blk := &blocks[i]
		if i > 0 {
			b.y += BlockGap
			if blk.Kind == Heading {
				b.y += BlockGap
			}
		}
		switch blk.Kind {
		case Heading:
			b.text(i, HeadingLine, blk.Spans, headingFace(blk.Level), 0, blk.Level)
		case Paragraph:
			b.text(i, TextLine, blk.Spans, Body, 0, 0)
		case Quote:
			b.text(i, QuoteLine, blk.Spans, Body, QuoteInset, 0)
		case ListItem:
			if i > 0 && blocks[i-1].Kind == ListItem {
				b.y -= BlockGap * 3 / 4
			}
			x := float32(blk.Level+1) * ListIndent
			mark := "•"
			if blk.Ordinal > 0 {
				mark = strconv.Itoa(blk.Ordinal) + "."
			}
			start := len(b.doc.Lines)
			b.text(i, TextLine, blk.Spans, Body, x, blk.Level)
			if start < len(b.doc.Lines) {
				l := &b.doc.Lines[start]
				mw := m.Width(Body, mark)
				l.Runs = append([]Run{{Text: mark, X: x - 6 - mw, W: mw, Face: Body}}, l.Runs...)
			}
		case CodeBlock:
			b.code(i, blk)
		case Table:
			b.table(i, blk)
		case Rule:
			b.add(Line{Kind: RuleLine, H: BlockGap * 2, Block: i})
		}
	}
	b.doc.Height = b.y
	b.doc.Width = b.width
	return &b.doc
}
