package inspector

import (
	"slices"
	"strconv"
	"strings"

	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/schema"
)

const confirmOver = 10

func (p *Panel) mixedTip(d *document.SceneDocument, td *schema.TypeDescriptor, fd *schema.FieldDescriptor, base string) string {
	counts := map[string]int{}
	var order []string
	for i, n := range p.nodes {
		if i == 64 {
			break
		}
		v, err := d.Props.Get(n, td.ID, fd.ID)
		if err != nil {
			continue
		}
		s := p.summary(d, fd, v, false)
		if _, seen := counts[s]; !seen {
			order = append(order, s)
		}
		counts[s]++
	}
	var b strings.Builder
	b.WriteString("Values differ:")
	for i, s := range order {
		if i == 6 {
			b.WriteString("\n" + strconv.Itoa(len(order)-i) + " more")
			break
		}
		b.WriteString("\n" + s + "   ×" + strconv.Itoa(counts[s]))
	}
	if base != "" {
		return base + "\n\n" + b.String()
	}
	return b.String()
}

func (p *Panel) confirm(d *document.SceneDocument, label string, run func() error) {
	nodes := slices.Clone(p.nodes)
	if len(nodes) <= confirmOver {
		p.report(run())
		return
	}
	s := d.Scene()
	var names []string
	for i, n := range nodes {
		if i == 8 {
			break
		}
		if h := s.Handle(n); !h.IsZero() {
			names = append(names, s.Name(h))
		}
	}
	msg := label + " changes " + strconv.Itoa(len(nodes)) + " nodes: " + strings.Join(names, ", ")
	if len(nodes) > len(names) {
		msg += " and " + strconv.Itoa(len(nodes)-len(names)) + " more"
	}
	doc := d.ID
	p.d.Notices.Ask(context.Question{
		Title:   label,
		Message: msg + ".",
		Buttons: []string{"Apply", "Cancel"},
		Then: func(choice int) {
			if cur := p.cur(); choice != 0 || cur == nil || cur.ID != doc {
				return
			}
			saved := p.nodes
			p.nodes = nodes
			p.report(run())
			p.nodes = saved
		},
	})
}
