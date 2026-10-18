package projectsettings

import (
	"aego/editor/ext/inspect"
	es "aego/engine/settings"
	"aego/schema"
)

type Ref struct {
	Label  string
	Detail string
	Page   string
	Field  string
	Tab    int
}

var projectFields = []string{"Name", "Module", "Startup Scene", "Default Space", "Width", "Height", "Title"}

func (p *Panel) Index() []Ref {
	pages := p.pages()
	title := map[string]string{}
	for _, pg := range pages {
		title[pg.id] = pg.title
	}
	var out []Ref
	add := func(label, page string, tab int, note string) {
		d := "Project Settings › " + title[page]
		if note != "" {
			d += " · " + note
		}
		out = append(out, Ref{Label: label, Detail: d, Page: page, Field: label, Tab: tab})
	}
	for _, f := range projectFields {
		add(f, "project", 0, "")
	}
	var custom map[string][]string
	if p.d.Pages != nil {
		custom = map[string][]string{}
		for _, sp := range p.d.Pages() {
			custom[sp.Section] = sp.Keywords
		}
	}
	sectionOf := map[string]string{}
	for _, sec := range p.d.Sections.List() {
		if _, ok := title[sec.ID]; !ok {
			continue
		}
		sectionOf[sec.Owner] = sec.ID
		for _, k := range custom[sec.ID] {
			add(k, sec.ID, 0, "")
		}
		if sec.Type == nil {
			continue
		}
		for i := range sec.Type.Fields {
			fd := &sec.Type.Fields[i]
			if !fd.Flags.Has(schema.FlagEditor) || fd.Hints.Hidden {
				continue
			}
			label := fd.Hints.DisplayName
			if label == "" {
				label = inspect.Humanize(fd.Name)
			}
			add(label, sec.ID, 0, "")
		}
	}
	if st := p.d.Settings; st != nil {
		st.Each(func(_ es.Key, d *es.Desc) {
			switch {
			case !d.Flags.Has(es.Project):
				add(d.Label(), "registry", 0, "player setting "+d.ID)
			case sectionOf[d.Owner] != "":
				add(d.Label(), sectionOf[d.Owner], 1, "default for players")
			default:
				add(d.Label(), "defaults/"+d.Owner, 0, "default for players")
			}
		})
	}
	return out
}

func (p *Panel) FocusRef(r Ref) {
	p.Focus(r.Page, r.Field)
	if r.Tab > 0 {
		p.tab[r.Page] = r.Tab
	}
}
