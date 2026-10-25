package projectsettings

import (
	"strings"

	"aego/core/mathx"
	"aego/engine/i18n"
	es "aego/engine/settings"
	"aego/engine/settings/settingsui"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

const narrowRegistry = 380

type ownerPage struct {
	owner string
	title string
}

func (p *Panel) projectOwners() []ownerPage {
	st := p.d.Settings
	if st == nil {
		return nil
	}
	var out []ownerPage
	st.Each(func(_ es.Key, d *es.Desc) {
		if !d.Flags.Has(es.Project) {
			return
		}
		for _, o := range out {
			if o.owner == d.Owner {
				return
			}
		}
		title := d.Category
		if title == "" {
			title = d.Owner
		}
		out = append(out, ownerPage{owner: d.Owner, title: title})
	})
	return out
}

func (p *Panel) hasDefaults(owner string) bool {
	for _, o := range p.projectOwners() {
		if o.owner == owner {
			return true
		}
	}
	return false
}

func (p *Panel) tabs(ui *core.Context, id string) int {
	row := ui.Cursor().Next(0)
	w := min(140, row.W/2-2)
	for i, label := range [2]string{"Settings", "Defaults"} {
		on := p.tab[id] == i
		if widgets.ToggleButton(ui, core.IDSeq(ui.ID("tab"), i), mathx.Rect{X: row.X + float32(i)*(w+4), Y: row.Y, W: w, H: row.H}, label, &on) {
			p.tab[id] = i
		}
	}
	return p.tab[id]
}

func (p *Panel) drawDefaults(ui *core.Context, owner string) bool {
	st := p.d.Settings
	if st == nil {
		return false
	}
	widgets.LabelWrapped(ui, i18n.Tr("editor.projectsettings.starting_values_of_player_settings", "Starting values of player settings. Players change them in the game settings; this page sets what a new player gets."), true)
	widgets.Separator(ui)
	changed := false
	value := func(k es.Key) schema.Value {
		if v, ok := p.proj[k]; ok {
			return v
		}
		return st.Desc(k).Default
	}
	put := func(k es.Key, v schema.Value) {
		if es.Equal(v, st.Desc(k).Default) {
			delete(p.proj, k)
		} else {
			p.proj[k] = v
		}
		changed = true
	}
	st.Each(func(k es.Key, d *es.Desc) {
		if d.Owner != owner || !d.Flags.Has(es.Project) || settingsui.Paired(st, k) {
			return
		}
		ui.PushID(d.ID)
		v := value(k)
		if m := settingsui.Partner(st, k); m != 0 {
			mv := value(m)
			vc, mc := p.ed.Pair(ui, st, k, &v, m, &mv)
			if vc {
				put(k, v)
			}
			if mc {
				put(m, mv)
			}
		} else if p.ed.Field(ui, st, k, &v) {
			put(k, v)
		}
		ui.PopID()
	})
	return changed
}

func sourceColor(th *core.Theme, l es.Layer) mathx.Color {
	switch l {
	case es.LayerProject:
		return th.Text
	case es.LayerUser:
		return th.Accent
	case es.LayerSession:
		return th.Warn
	}
	return th.TextDim
}

func (p *Panel) drawRegistry(ui *core.Context) bool {
	st := p.d.Settings
	th := &ui.Theme
	widgets.SearchField(ui, ui.ID("filter"), ui.Cursor().Next(0), &p.filter)
	q := strings.ToLower(strings.TrimSpace(p.filter))
	srcW := ui.TextSize("session").X + 16
	cell := func(r mathx.Rect, s string, col mathx.Color) {
		ui.Text(widgets.Elide(ui, s, r.W-8), mathx.Vec2{X: r.X + 4, Y: ui.TextY(r)}, col)
	}
	split := func(r mathx.Rect) (mathx.Rect, mathx.Rect, mathx.Rect) {
		rest := max(0, r.W-srcW)
		nameW := rest * 0.55
		return mathx.Rect{X: r.X, Y: r.Y, W: nameW, H: r.H},
			mathx.Rect{X: r.X + nameW, Y: r.Y, W: rest - nameW, H: r.H},
			mathx.Rect{X: r.X + rest, Y: r.Y, W: srcW, H: r.H}
	}
	hr := ui.Cursor().Next(0)
	a, b, c := split(hr)
	cell(a, "Setting", th.TextDim)
	if hr.W >= narrowRegistry {
		cell(b, "Value", th.TextDim)
	}
	cell(c, "Source", th.TextDim)
	owner, shown := "", 0
	st.Each(func(k es.Key, d *es.Desc) {
		label := d.Label()
		if q != "" && !strings.Contains(strings.ToLower(d.ID), q) && !strings.Contains(strings.ToLower(label), q) && !strings.Contains(strings.ToLower(d.Category), q) {
			return
		}
		if d.Owner != owner {
			owner = d.Owner
			if shown > 0 {
				ui.Cursor().Next(4)
			}
			widgets.Label(ui, inspectTitle(owner))
		}
		shown++
		r := ui.Cursor().Next(0)
		stacked := r.W < narrowRegistry
		if stacked {
			r.H += th.RowHeight - 4
			ui.Cursor().Y += th.RowHeight - 4
		}
		id := core.IDSeq(ui.ID("reg"), int(k))
		if ui.MouseOver(id, r) {
			ui.RoundRect(r, th.Radius.Menu, th.WidgetHot)
		}
		if ui.Flash != "" && strings.EqualFold(ui.Flash, label) {
			if k := 1 - float32(ui.Time-ui.FlashAt)/2; k > 0 {
				fc := th.Accent
				fc.A = 0.28 * k
				ui.RoundRect(r, th.Radius.Menu, fc)
			}
			if !ui.FlashSeen {
				ui.FlashY, ui.FlashSeen = r.Y, true
			}
		}
		line := mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: th.RowHeight}
		a, b, c := split(line)
		src := st.Source(k)
		val := st.Format(k)
		if val == "" {
			val = "—"
		}
		if stacked {
			a.W = line.W - srcW
			cell(a, label, th.Text)
			cell(mathx.Rect{X: r.X + 10, Y: r.Y + th.RowHeight - 4, W: r.W - 10, H: th.RowHeight}, val, th.TextDim)
		} else {
			cell(a, label, th.Text)
			cell(b, val, th.Text)
		}
		badge := mathx.Rect{X: c.X + 2, Y: line.Y + 3, W: c.W - 4, H: line.H - 6}
		if src != es.LayerDefault {
			bg := sourceColor(th, src)
			bg.A = 0.18
			ui.RoundRect(badge, th.Radius.Button, bg)
		}
		ui.TextIn(badge, src.String(), sourceColor(th, src), core.AlignCenter)
		widgets.Tooltip(ui, id, d.ID)
	})
	if shown == 0 {
		widgets.LabelDim(ui, i18n.Tr("editor.projectsettings.no_setting_matches", "No setting matches"))
	}
	return false
}

func inspectTitle(owner string) string {
	if owner == "" {
		return ""
	}
	return strings.ToUpper(owner[:1]) + owner[1:]
}
