package inspector

import (
	"strconv"

	"aego/core/mathx"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

func hintLabel(n string, i int) string {
	if n == "" {
		return strconv.Itoa(i)
	}
	return n
}

func (p *Panel) choices(id core.ID, r mathx.Rect, fd *schema.FieldDescriptor, v *schema.Value, mixed bool) widgets.Edit {
	src := fd.Hints.Choices
	names := p.d.Sections.Names(src.Section, src.List)
	var opts []string
	var idx []int64
	cur := -1
	for i, n := range names {
		if n == "" && int64(i) != v.I {
			continue
		}
		if int64(i) == v.I {
			cur = len(opts)
		}
		opts = append(opts, hintLabel(n, i))
		idx = append(idx, int64(i))
	}
	if widgets.Combo(p.f.UI, id, r, &cur, opts, mixed) && cur >= 0 {
		*v = intValue(fd.Kind, idx[cur])
		return all
	}
	return widgets.Edit{}
}

func (p *Panel) bitset(id core.ID, r mathx.Rect, fd *schema.FieldDescriptor, v *schema.Value, mixed bool) widgets.Edit {
	src := fd.Hints.Bitset
	names := p.d.Sections.Names(src.Section, src.List)
	if len(names) == 0 || len(names) > 64 {
		return widgets.Edit{}
	}
	labels := make([]string, len(names))
	for i, n := range names {
		labels[i] = hintLabel(n, i)
	}
	mask := uint64(v.I)
	if !widgets.MaskField(p.f.UI, id, r, &mask, labels, mixed) {
		return widgets.Edit{}
	}
	full := ^uint64(0)
	if len(names) < 64 {
		full = 1<<len(names) - 1
	}
	n := int64(mask & full)
	if mask&full == full {
		n = -1
	}
	*v = intValue(fd.Kind, n)
	return all
}
