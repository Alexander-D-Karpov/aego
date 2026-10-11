package scripteditor

import "aego/editor/code"

func (p *Panel) restoreSel() {
	if !p.ownSet {
		return
	}
	c := p.doc.Code
	a, h := p.own.Anchor, p.own.Head
	if p.ownRev != c.Rev() {
		if x, ok := c.Map(p.ownRev, a); ok {
			a = x
		}
		if x, ok := c.Map(p.ownRev, h); ok {
			h = x
		}
	}
	n := c.Len()
	c.Sel = code.Selection{Anchor: min(max(a, 0), n), Head: min(max(h, 0), n)}
}

func (p *Panel) saveSel() {
	p.own, p.ownRev, p.ownSet = p.doc.Code.Sel, p.doc.Code.Rev(), true
}

func (p *Panel) cursor() int {
	if p.ownSet && p.ownRev == p.doc.Code.Rev() {
		return p.own.Head
	}
	return p.doc.Code.Cursor()
}

func (p *Panel) follow() {
	prim := p.host.panels[p.doc.Path]
	if prim == nil || prim == p {
		return
	}
	if prim.semVer == p.primVer && prim.snap == p.snap {
		return
	}
	p.primVer = prim.semVer
	p.snap, p.snapRev, p.semAt = prim.snap, prim.snapRev, prim.semAt
	p.diags = append(p.diags[:0], prim.diags...)
	p.sem = append(p.sem[:0], prim.sem...)
	p.semVer++
	p.marksAt = ^uint64(0)
	p.occKey = mark{}
}

func (p *Panel) RequestClose() {
	p.host.requestClose(p)
}
