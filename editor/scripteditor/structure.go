package scripteditor

import (
	"aego/core/mathx"
	"aego/ui/core"
)

const guideLevels = 32

func (p *Panel) updateBlock() {
	c := p.doc.Code
	k := mark{rev: c.Rev(), off: c.Cursor()}
	if k == p.blockKey {
		return
	}
	p.blockKey = k
	p.block, p.blockOK = c.BlockAt(c.Cursor())
}

func (p *Panel) inBlock(l int) bool {
	return p.host.Block && p.blockOK && l >= p.block.Line0 && l <= p.block.Line1
}

func (p *Panel) buildGuides(first, last int) {
	p.guideAt = first
	p.guides = p.guides[:0]
	if !p.host.Rainbow || last <= first {
		return
	}
	for i := (last - first) * guideLevels; i > 0; i-- {
		p.guides = append(p.guides, -1)
	}
	for _, g := range p.doc.Code.Guides() {
		if g.Level >= guideLevels || g.Line1 <= first || g.Line0+1 >= last {
			continue
		}
		for l := max(g.Line0+1, first); l < min(g.Line1, last); l++ {
			p.guides[(l-first)*guideLevels+g.Level] = g.Depth
		}
	}
}

func (p *Panel) guideDepth(l, k int) int {
	if k < guideLevels {
		if i := (l-p.guideAt)*guideLevels + k; i >= 0 && i < len(p.guides) && p.guides[i] >= 0 {
			return p.guides[i]
		}
	}
	return k
}

func (p *Panel) drawGuides(ui *core.Context, v *lineView, x0, y, spaceW float32, l int) {
	ct := &ui.Theme.Code
	for k := 0; k*tabWidth < v.indent; k++ {
		active := p.blockOK && p.block.Level == k && l > p.block.Line0 && l < p.block.Line1
		col := ct.IndentGuide
		if active {
			col = ct.IndentGuideActive
		}
		if p.host.Rainbow {
			d := p.guideDepth(l, k) % len(ct.GuideRainbow)
			col = ct.GuideRainbow[d]
			if active {
				col = ct.GuideRainbowActive[d]
			}
		}
		x := x0 + float32(k*tabWidth)*spaceW
		ui.Rect(mathx.Rect{X: x, Y: y, W: 1, H: p.lineH}, col)
	}
}
