//go:build aego_golden

package golden

import (
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/editor/code"
	"aego/editor/code/highlight"
	"aego/editor/code/search"
	"aego/engine/res"
	"aego/gpu"
	"aego/render"
	"aego/render/text"
	"aego/render/view"
	"aego/ui/draw"
	"aego/ui/theme"
	"aego/vfs"
)

type codeView struct {
	src   string
	caret string
	sel   string
	occ   string
	write []bool
	cur   int
}

func monoFont(t *testing.T, r *render.Renderer) *text.Font {
	t.Helper()
	files, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	set, err := text.LoadSet(files, r.Textures(), "engine://fonts", "ui-mono", []int{14}, []int{1})
	if err != nil {
		t.Fatalf("mono atlas missing, run make genfont: %v", err)
	}
	return set.Fonts()[0]
}

func codeCases(t *testing.T, r *render.Renderer) []testCase {
	t.Helper()
	font := monoFont(t, r)
	return []testCase{
		{"code_rainbow", 4, drawCode(font, codeView{src: "f(a[b{c(d[e])}])\ng(((((((x)))))))\nn.Get<Body>(v)"})},
		{"code_mismatch", 4, drawCode(font, codeView{src: "h{\n    if(x[y]) {\n        k(]\n    }\n}"})},
		{"code_guides", 4, drawCode(font, codeView{src: "f() {\n    if x {\n        y()\n        z()\n    }\n    w()\n}", caret: "y()"})},
		{"code_occurrences", 4, drawCode(font, codeView{
			src:   "x := 1\nx = x + 2\nprint(x)",
			sel:   "print",
			occ:   "x",
			write: []bool{true, true, false, false},
			cur:   2,
		})},
	}
}

func codeColor(ct *theme.CodeTheme, k highlight.Kind) mathx.Color {
	switch k {
	case highlight.Keyword:
		return ct.Keyword
	case highlight.Number:
		return ct.Number
	case highlight.String:
		return ct.String
	case highlight.Comment:
		return ct.Comment
	case highlight.Operator:
		return ct.Operator
	case highlight.Punctuation, highlight.Bracket:
		return ct.Punctuation
	case highlight.Function, highlight.Method:
		return ct.Function
	}
	return ct.Plain
}

func waveList(l *draw.List, xa, xb, base float32, col mathx.Color) {
	for x := xa; x < xb; x += 4 {
		l.Rect(mathx.Rect{X: x, Y: base, W: 2, H: 1.5}, col)
		l.Rect(mathx.Rect{X: x + 2, Y: base + 1, W: 2, H: 1.5}, col)
	}
}

func drawCode(font *text.Font, cv codeView) sceneFunc {
	return func(r *render.Renderer, tgt gpu.TextureHandle) {
		th := theme.Editor()
		ct := &th.Code
		doc := code.New([]byte(cv.src))
		l := draw.NewList(256)
		l.SetFont(font)
		adv := font.Advance('0')
		lh := font.LineHeight + 3
		at := func(off int) mathx.Vec2 {
			ln := doc.Line(off)
			return mathx.Vec2{X: 12 + float32(off-doc.LineStart(ln))*adv, Y: 12 + float32(ln)*lh}
		}
		l.Rect(mathx.Rect{W: size, H: size}, ct.Background)
		block, inBlock := code.Block{}, false
		if cv.caret != "" {
			block, inBlock = doc.BlockAt(strings.Index(cv.src, cv.caret))
		}
		if inBlock {
			l.Rect(mathx.Rect{Y: 12 + float32(block.Line0)*lh, W: size, H: float32(block.Line1-block.Line0+1) * lh}, ct.CurrentBlock)
		}
		for _, g := range doc.Guides() {
			d := g.Depth % len(ct.GuideRainbow)
			col := ct.GuideRainbow[d]
			if inBlock && g.Open == block.Open {
				col = ct.GuideRainbowActive[d]
			}
			for ln := g.Line0 + 1; ln < g.Line1; ln++ {
				p := at(doc.LineStart(ln))
				l.Rect(mathx.Rect{X: p.X + float32(g.Level*code.TabWidth)*adv, Y: p.Y, W: 1, H: lh}, col)
			}
		}
		if cv.occ != "" {
			w := float32(len(cv.occ)) * adv
			for i, off := range search.All([]byte(cv.src), cv.occ, search.Options{Case: true, Word: true}) {
				p := at(off)
				fill := ct.OccurrenceDim
				if i == cv.cur {
					fill = ct.Occurrence
				}
				l.Rect(mathx.Rect{X: p.X, Y: p.Y + 1, W: w, H: lh - 2}, fill)
				if i < len(cv.write) && cv.write[i] {
					l.Rect(mathx.Rect{X: p.X, Y: p.Y + lh - 2, W: w, H: 1}, ct.OccurrenceWrite)
				}
			}
		}
		if cv.sel != "" {
			p := at(strings.Index(cv.src, cv.sel))
			l.Rect(mathx.Rect{X: p.X, Y: p.Y, W: float32(len(cv.sel)) * adv, H: lh}, th.Selection)
		}
		for ln := 0; ln < doc.Lines(); ln++ {
			raw := doc.LineBytes(ln, nil)
			st := doc.LineState(ln)
			spans, _ := highlight.Line(raw, st, nil)
			levels := highlight.BracketLevels(spans, raw, st.Depth(), nil)
			ls := doc.LineStart(ln)
			for i, s := range spans {
				p := at(ls + s.Start)
				col := codeColor(ct, s.Kind)
				if s.Kind == highlight.Bracket {
					switch {
					case doc.Unmatched(ls + s.Start):
						col = ct.BracketMismatch
						waveList(l, p.X, p.X+adv, p.Y+lh-3, col)
					case levels[i] >= 0:
						col = ct.Rainbow[levels[i]%len(ct.Rainbow)]
					}
				}
				l.Text(string(raw[s.Start:s.End]), mathx.Vec2{X: p.X, Y: p.Y + 1.5}, col)
			}
		}
		v := view.ScreenView(size, size, 1)
		v.Target, v.Clear, v.DoClear = tgt, mathx.Color{A: 1}, true
		f := r.View(v)
		l.Flush(f, font, r.White(), 0)
		f.End()
	}
}
