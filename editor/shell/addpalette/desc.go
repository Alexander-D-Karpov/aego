package addpalette

import (
	"strings"

	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/scene/mutate"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

type suggestion struct {
	td    *schema.TypeDescriptor
	label string
	on    bool
}

type desc struct {
	item    int
	sel     int
	title   string
	cat     string
	icon    string
	doc     string
	also    string
	skip    string
	why     string
	choices []mutate.Choice
	names   [][]string
	picks   []int
	sugg    []suggestion
	focus   int
}

func (d *desc) extras() []*schema.TypeDescriptor {
	var out []*schema.TypeDescriptor
	for _, sg := range d.sugg {
		if sg.on {
			out = append(out, sg.td)
		}
	}
	return out
}

// keys cycles the alternative of the first RequiresOneOf group with Tab.
func (d *desc) keys(ui *core.Context) {
	if len(d.choices) == 0 || !ui.KeyPressed(input.KeyTab) {
		return
	}
	c := d.focus
	n := len(d.choices[c].Alts)
	if ui.Mods().Has(input.ModShift) {
		d.picks[c] = (d.picks[c] + n - 1) % n
	} else {
		d.picks[c] = (d.picks[c] + 1) % n
	}
	d.dirty()
}

func (d *desc) dirty() { d.sel = -2 }

func (s *Service) prepare(i int) {
	d := &s.desc
	d.item, d.sel = i, i
	d.title, d.cat, d.icon, d.doc, d.also, d.skip, d.why = "", "", "", "", "", "", ""
	d.choices, d.names, d.picks, d.sugg, d.focus = nil, nil, nil, nil, 0
	if i < 0 || i >= len(s.items) {
		return
	}
	it := &s.items[i]
	e := &s.entries[i]
	d.title, d.cat, d.icon, d.why = it.Label, it.Category, it.Icon, it.Disabled
	switch e.kind {
	case kindAction:
		d.doc = e.act.Doc
		return
	case kindTemplate:
		d.doc = i18n.Tr("editor.addpalette.creates", "Creates {name}", i18n.Str("name", e.tpl.Name))
		if t := s.tplComponent(e.tpl.ID); t != nil && t.Doc != "" {
			d.doc = t.Doc
		}
		return
	case kindScript:
		d.doc = e.td.Doc
		if d.doc == "" {
			d.doc = i18n.Tr("editor.addpalette.script", "Script {name}", i18n.Str("name", e.td.FullName))
		}
	default:
		d.doc = e.td.Doc
	}
	if len(e.skip) > 0 && len(e.nodes) > 0 {
		d.skip = i18n.Tr("editor.addpalette.not_added_to", "Not added to {nodes}", i18n.Str("nodes", strings.Join(e.skip, ", ")))
	}
	if e.kind != kindComp || len(e.nodes) == 0 {
		return
	}
	sc := s.doc.Scene()
	h := sc.Handle(e.nodes[0])
	d.choices = mutate.Choices(sc, e.td, h)
	d.picks = make([]int, len(d.choices))
	for _, c := range d.choices {
		var names []string
		for _, a := range c.Alts {
			names = append(names, displayName(a, s.d.Meta))
		}
		d.names = append(d.names, names)
	}
	s.alsoAdds()
	for _, name := range e.td.Suggests {
		td, ok := sc.Registry().ByName(name)
		if !ok || !sc.Accepts(h, td.ID) {
			continue
		}
		missing := false
		for _, n := range e.nodes {
			missing = missing || !has(sc, sc.Handle(n), td.ID)
		}
		if missing {
			d.sugg = append(d.sugg, suggestion{td: td, label: i18n.Tr("editor.addpalette.also_add", "Also add {name}", i18n.Str("name", displayName(td, s.d.Meta)))})
		}
	}
}

func (s *Service) alsoAdds() {
	d := &s.desc
	e := &s.entries[d.item]
	sc := s.doc.Scene()
	req, err := mutate.RequiredPick(sc, e.td, sc.Handle(e.nodes[0]), d.picks)
	d.also = ""
	if err != nil || len(req) == 0 {
		return
	}
	names := make([]string, len(req))
	for i, t := range req {
		names[i] = displayName(t, s.d.Meta)
	}
	d.also = i18n.Tr("editor.addpalette.also_adds", "Also adds: {names}", i18n.Str("names", strings.Join(names, ", ")))
}

func (s *Service) tplComponent(id string) *schema.TypeDescriptor {
	sc := s.doc.Scene()
	for _, td := range sc.Registry().Components() {
		if td.Template != nil && td.Template.ID == id {
			return td
		}
	}
	return nil
}

func (s *Service) describe(ui *core.Context, r mathx.Rect, item int) {
	d := &s.desc
	switch {
	case item != d.item:
		s.prepare(item)
	case d.sel == -2:
		d.sel = item
		s.alsoAdds()
	}
	if item < 0 {
		ui.TextIn(mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: ui.Theme.RowHeight}, i18n.Tr("editor.addpalette.nothing_selected", "Nothing selected"), ui.Theme.TextDisabled, core.AlignLeft)
		return
	}
	th := &ui.Theme
	ui.PushCursor(core.Cursor{X: r.X + 4, Y: r.Y, W: r.W - 8, RowH: th.RowHeight, Pad: th.Padding, Space: 2})
	defer ui.PopCursor()
	head := ui.Cursor().Next(0)
	widgets.IconAt(ui, mathx.Rect{X: head.X, Y: head.Y + (head.H-16)/2, W: 16, H: 16}, d.icon, th.Text)
	ui.Text(d.title, mathx.Vec2{X: head.X + 22, Y: ui.TextY(head)}, th.Text)
	ui.TextIn(head, CategoryTitle(d.cat), th.TextDisabled, core.AlignRight)
	if d.why != "" {
		widgets.LabelWrapped(ui, d.why, false)
	}
	if d.doc != "" {
		widgets.LabelWrapped(ui, d.doc, true)
	}
	if d.also != "" {
		ui.Cursor().Next(4)
		widgets.LabelWrapped(ui, d.also, false)
	}
	for c := range d.choices {
		ui.Cursor().Next(4)
		widgets.LabelDim(ui, i18n.Tr("editor.addpalette.needs_one_of_tab_switches", "Needs one of (Tab switches):"))
		for j, name := range d.names[c] {
			if widgets.CheckRow(ui, core.IDSeq(core.IDOf("addpalette.alt"), c*64+j), name, d.picks[c] == j) {
				d.picks[c], d.focus = j, c
				d.dirty()
			}
		}
	}
	if len(d.sugg) > 0 {
		ui.Cursor().Next(4)
	}
	for j := range d.sugg {
		sg := &d.sugg[j]
		if widgets.CheckRow(ui, core.IDSeq(core.IDOf("addpalette.sugg"), j), sg.label, sg.on) {
			sg.on = !sg.on
		}
	}
	if d.skip != "" {
		ui.Cursor().Next(4)
		widgets.LabelWrapped(ui, d.skip, true)
	}
}
