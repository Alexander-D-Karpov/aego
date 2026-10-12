package scripteditor

import (
	"aego/editor/code"
	"aego/editor/code/highlight"
)

type rainbowCache struct {
	rev    uint64
	ok     bool
	depths []int32
	spans  []highlight.Span
	line   []byte
}

func (r *rainbowCache) depth(c *code.Doc, l int) int {
	if !r.ok || r.rev != c.Rev() {
		r.build(c)
	}
	if l < 0 || l >= len(r.depths) {
		return 0
	}
	return int(r.depths[l])
}

func (r *rainbowCache) build(c *code.Doc) {
	n := c.Lines()
	r.depths = r.depths[:0]
	d := int32(0)
	for l := 0; l < n; l++ {
		r.depths = append(r.depths, d)
		r.line = c.LineBytes(l, r.line[:0])
		r.spans, _ = highlight.Line(r.line, c.LineState(l), r.spans[:0])
		for _, s := range r.spans {
			if s.Kind != highlight.Punctuation {
				continue
			}
			switch r.line[s.Start] {
			case '(', '[', '{':
				d++
			case ')', ']', '}':
				d = max(d-1, 0)
			}
		}
	}
	r.rev, r.ok = c.Rev(), true
}
