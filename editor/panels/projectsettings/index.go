package projectsettings

import (
	"aego/editor/ext/inspect"
	"aego/engine/i18n"
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

// projectFields use the keys of the labels drawProject draws, so a search result flashes the field.
func projectFields() []string {
	return []string{i18n.Tr("editor.projectsettings.name", "Name"), i18n.Tr("editor.projectsettings.module", "Module"), i18n.Tr("editor.projectsettings.startup_scene", "Startup Scene"), i18n.Tr("editor.projectsettings.default_space", "Default Space"), i18n.Tr("editor.projectsettings.width", "Width"), i18n.Tr("editor.projectsettings.height", "Height"), i18n.Tr("editor.projectsettings.game_title", "Title")}
}

func (p *Panel) Index() []Ref {
	pages := p.pages()
	title := map[string]string{}
	for _, pg := range pages {
		title[pg.id] = pg.title
	}
	var out []Ref
	add := func(label, page string, tab int, note string) {
		d := i18n.Tr("editor.projectsettings.title", "Project Settings") + " › " + title[page]
		if note != "" {
			d += " · " + note
		}
		out = append(out, Ref{Label: label, Detail: d, Page: page, Field: label, Tab: tab})
	}
	for _, f := range projectFields() {
		add(f, "project", 0, "")
	}
	var custom map[string][]i18n.Text
	if p.d.Pages != nil {
		custom = map[string][]i18n.Text{}
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
			add(k.String(), sec.ID, 0, "")
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
				add(d.Label(), "registry", 0, i18n.Tr("editor.projectsettings.player_setting", "player setting {id}", i18n.Str("id", d.ID)))
			case sectionOf[d.Owner] != "":
				add(d.Label(), sectionOf[d.Owner], 1, i18n.Tr("editor.projectsettings.default_for_players", "default for players"))
			default:
				add(d.Label(), "defaults/"+d.Owner, 0, i18n.Tr("editor.projectsettings.default_for_players", "default for players"))
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
