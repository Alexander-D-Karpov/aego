package references

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/refs"
	"aego/editor/core/services"
	"aego/editor/core/tasks"
	"aego/engine/i18n"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	ID          = "references"
	MissingID   = "missing_references"
	CommandFind = "assets.find_references"
)

type Deps struct {
	Refs          *refs.Service
	Pipeline      *pipeline.Pipeline
	Docs          services.Documents
	Selection     services.Selection
	Nav           services.Navigation
	Notices       services.Notices
	Tasks         *tasks.Queue
	Delete        func([]ids.AssetID)
	Show          func(id ids.AssetID)
	ShowHierarchy func()
}

func kindIcon(k asset.ArtifactKind, project bool) string {
	switch {
	case project:
		return "cat.settings"
	case k == asset.ArtScene:
		return "file.scene"
	case k == asset.ArtPrefab:
		return "file.prefab"
	case k == asset.ArtData:
		return "file.data"
	case k == asset.ArtTable:
		return "file.table"
	case k == asset.ArtPreset:
		return "file.preset"
	}
	return "file.unknown"
}

func openPlace(d Deps, r refs.Ref, pl refs.Place) {
	switch {
	case r.Project:
		d.Notices.Notify(context.LevelInfo, i18n.Note("editor.references.the_startup_scene_is_set", "The startup scene is set in Project Settings"))
	case r.Kind == asset.ArtScene || r.Kind == asset.ArtPrefab:
		if _, err := d.Docs.Documents().Open(r.Path); err != nil {
			d.Notices.Notify(context.LevelError, err.Error())
			return
		}
		if !pl.Node.IsZero() {
			d.Selection.Select(pl.Node)
			if d.ShowHierarchy != nil {
				d.ShowHierarchy()
			}
		}
	default:
		d.Nav.Open(context.Location{Path: r.Path, Offset: -1})
	}
}

func pickAsset(ui *core.Context, pid core.ID, pl *pipeline.Pipeline, kind asset.ArtifactKind, q *string, skip ids.AssetID) (ids.AssetID, bool) {
	if !ui.BeginPopup(pid, 360) {
		return ids.AssetID{}, false
	}
	widgets.SearchField(ui, core.IDSeq(pid, 1), ui.Cursor().Next(0), q)
	needle := strings.ToLower(strings.TrimSpace(*q))
	list := pl.Assets()
	sort.Slice(list, func(i, j int) bool { return list[i].Path < list[j].Path })
	var got ids.AssetID
	picked, n := false, 0
	for _, a := range list {
		if n >= 200 {
			break
		}
		if a.ID == skip || (kind != "" && a.Kind != kind) {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(a.Path), needle) {
			continue
		}
		n++
		if widgets.Selectable(ui, core.IDSeq(pid, 10+n), a.Path, false) {
			got, picked = a.ID, true
			ui.ClosePopup(pid)
		}
	}
	if n == 0 {
		widgets.LabelDim(ui, i18n.Tr("editor.references.no_matching_assets", "No matching assets"))
	}
	ui.EndPopup()
	return got, picked
}

func runReplace(d Deps, from, to ids.AssetID, label string, done func()) {
	job, err := d.Refs.Begin(from, to)
	if err != nil {
		d.Notices.Notify(context.LevelError, err.Error())
		return
	}
	finish := func(err error) {
		if err == nil {
			err = d.Refs.Finish(job)
		}
		switch {
		case err != nil:
			d.Notices.Notify(context.LevelError, i18n.Note("editor.references.error_originals_in_backup", "{error} (originals in {backup})", i18n.Str("error", err.Error()), i18n.Str("backup", job.Backup)))
		default:
			n := len(job.Changed()) + len(job.Docs)
			msg := i18n.Tr("editor.references.label_n_file_s_updated", "{label}: {n} file(s) updated", i18n.Str("label", label), i18n.Str("n", strconv.Itoa(n)))
			if len(job.Changed()) > 0 {
				msg += i18n.Tr("editor.references.originals_in_backup", ", originals in {backup}", i18n.Str("backup", job.Backup))
			}
			d.Notices.Notify(context.LevelInfo, msg)
		}
		if done != nil {
			done()
		}
	}
	if len(job.Files) == 0 {
		finish(nil)
		return
	}
	if d.Tasks == nil {
		finish(job.Rewrite())
		return
	}
	d.Tasks.Work(tasks.Spec{Title: label, Kind: "refs", Detail: strings.Join(job.Files, ", ")}, func(*tasks.Task) error {
		return job.Rewrite()
	}).Then(func(t *tasks.Task) { finish(t.Err()) })
}

type row struct {
	hit   int
	place int
}

type Panel struct {
	d        Deps
	target   ids.AssetID
	kind     asset.ArtifactKind
	hits     []refs.Hit
	rows     []row
	deleting []ids.AssetID
	pick     string
	scroll   float32
}

func New(d Deps) *Panel {
	return &Panel{d: d}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return i18n.Tr("editor.references.title", "References")
}

func (p *Panel) Icon() string {
	return "panel.references"
}

func (p *Panel) Show(id ids.AssetID, deleting []ids.AssetID) {
	p.target, p.deleting, p.scroll = id, deleting, 0
	p.kind = ""
	if e, ok := p.d.Pipeline.Entry(id); ok {
		p.kind = e.Kind
	} else if g, ok := p.d.Pipeline.Grave(id); ok {
		p.kind = g.Kind
	}
	p.refresh()
}

func (p *Panel) refresh() {
	p.hits = p.d.Refs.Find(p.target)
	p.rows = p.rows[:0]
	for i, h := range p.hits {
		p.rows = append(p.rows, row{hit: i, place: -1})
		for j := range h.Places {
			p.rows = append(p.rows, row{hit: i, place: j})
		}
	}
}

func (p *Panel) name() string {
	if rel, ok := p.d.Pipeline.PathOf(p.target); ok {
		return rel
	}
	if g, ok := p.d.Pipeline.Grave(p.target); ok {
		return i18n.Tr("editor.references.path_deleted", "{path} (deleted)", i18n.Str("path", g.Path))
	}
	return p.target.String()
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	if p.target.IsZero() {
		widgets.LabelWrapped(ui, "Select an asset and press "+"Shift+F12"+" or use Find References in its menu.", true)
		return
	}
	head := ui.Cursor().Next(0)
	rw := ui.TextSize(i18n.Tr("editor.references.refresh", "Refresh")).X + 16
	ui.PushClip(mathx.Rect{X: head.X, Y: head.Y, W: head.W - rw - 4, H: head.H})
	ui.Text(path.Base(p.name()), mathx.Vec2{X: head.X, Y: ui.TextY(head)}, th.Text)
	ui.Text(path.Dir(p.name()), mathx.Vec2{X: head.X + ui.TextSize(path.Base(p.name())).X + 10, Y: ui.TextY(head)}, th.TextDim)
	ui.PopClip()
	if widgets.ButtonRect(ui, ui.ID("refresh"), mathx.Rect{X: head.X + head.W - rw, Y: head.Y, W: rw, H: head.H}, i18n.Tr("editor.references.refresh", "Refresh")) {
		p.refresh()
	}
	n := len(p.hits)
	if n == 0 {
		widgets.LabelDim(ui, i18n.Tr("editor.references.nothing_references_this_asset", "Nothing references this asset"))
	} else {
		widgets.LabelDim(ui, i18n.Tr("editor.references.n_file_s_reference_it", "{n} file(s) reference it", i18n.Str("n", strconv.Itoa(n))))
	}
	foot := th.RowHeight + th.Padding
	top := ui.Cursor().Y
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top-foot)}
	widgets.VirtualList(ui, ui.ID("rows"), list, len(p.rows), th.RowHeight, &p.scroll, func(i int, rr mathx.Rect) {
		p.drawRow(ui, i, rr)
	})
	ui.Cursor().Y = list.Y + list.H + 4
	p.footer(ui)
}

func (p *Panel) drawRow(ui *core.Context, i int, rr mathx.Rect) {
	th := &ui.Theme
	rw := p.rows[i]
	h := &p.hits[rw.hit]
	id := core.IDSeq(ui.ID("row"), i)
	if ui.Clicked(id, rr) {
		pl := refs.Place{}
		if rw.place >= 0 {
			pl = h.Places[rw.place]
		}
		openPlace(p.d, h.Ref, pl)
	}
	if ui.Hot() == id {
		ui.Rect(rr, th.WidgetHot)
	}
	ui.PushClip(rr)
	if rw.place < 0 {
		widgets.IconAt(ui, mathx.Rect{X: rr.X + 2, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}, kindIcon(h.Ref.Kind, h.Ref.Project), th.TextDim)
		text := h.Ref.Path
		if h.Open {
			text += "  (open)"
		}
		ui.Text(text, mathx.Vec2{X: rr.X + 24, Y: ui.TextY(rr)}, th.Text)
		if h.Err != nil {
			ui.TextIn(rr, h.Err.Error(), th.Error, core.AlignRight)
		}
	} else {
		text, tip := placeLabel(h.Places[rw.place])
		ui.Text(text, mathx.Vec2{X: rr.X + 24 + th.Indent, Y: ui.TextY(rr)}, th.TextDim)
		widgets.Tooltip(ui, id, tip)
	}
	ui.PopClip()
}

func (p *Panel) footer(ui *core.Context) {
	row := ui.Cursor().Next(0)
	x := row.X
	btn := func(id, label string, enabled bool) (bool, mathx.Rect) {
		w := ui.TextSize(label).X + 18
		r := mathx.Rect{X: x, Y: row.Y, W: w, H: row.H}
		x += w + 6
		if !enabled {
			ui.TextIn(r, label, ui.Theme.TextDisabled, core.AlignCenter)
			return false, r
		}
		return widgets.ButtonRect(ui, ui.ID(id), r, label), r
	}
	has := len(p.hits) > 0
	pid := ui.ID("pick")
	if ok, r := btn("replace", i18n.Tr("editor.references.replace_with", "Replace with…"), has); ok {
		p.pick = ""
		ui.CloseAllPopups()
		ui.OpenPopup(pid, mathx.Vec2{X: r.X, Y: r.Y - 300})
	}
	if ok, _ := btn("clear", i18n.Tr("editor.references.clear_references_button", "Clear References"), has); ok {
		target := p.target
		p.d.Notices.Ask(context.Question{
			Title:   i18n.Tr("editor.references.clear_references", "Clear references"),
			Message: i18n.Tr("editor.references.every_reference_to_name_becomes", "Every reference to {name} becomes None. Files that are not open are rewritten on disk; their originals go to .aego/recovery.", i18n.Str("name", p.name())),
			Buttons: []string{i18n.Tr("editor.references.clear", "Clear"), i18n.Tr("editor.references.cancel", "Cancel")},
			Then: func(c int) {
				if c == 0 {
					runReplace(p.d, target, ids.AssetID{}, "Clear references to "+path.Base(p.name()), p.refresh)
				}
			},
		})
	}
	if len(p.deleting) > 0 && p.d.Delete != nil {
		label := i18n.Tr("editor.references.delete_deleting_asset_s", "Delete {deleting} asset(s)", i18n.Str("deleting", strconv.Itoa(len(p.deleting))))
		if ok, _ := btn("delete", label, true); ok {
			list := p.deleting
			p.deleting = nil
			p.d.Delete(list)
			p.refresh()
		}
	}
	if to, ok := pickAsset(ui, pid, p.d.Pipeline, p.kind, &p.pick, p.target); ok {
		target := p.target
		rel, _ := p.d.Pipeline.PathOf(to)
		p.d.Notices.Ask(context.Question{
			Title:   i18n.Tr("editor.references.replace_references", "Replace references"),
			Message: i18n.Tr("editor.references.replace_every_reference_to_name", "Replace every reference to {name} with {rel}. Files that are not open are rewritten on disk; their originals go to .aego/recovery.", i18n.Str("name", p.name()), i18n.Str("rel", rel)),
			Buttons: []string{i18n.Tr("editor.references.replace", "Replace"), i18n.Tr("editor.references.cancel", "Cancel")},
			Then: func(c int) {
				if c == 0 {
					runReplace(p.d, target, to, "Replace references to "+path.Base(p.name()), p.refresh)
				}
			},
		})
	}
}

// placeLabel names the node by its last path segment and keeps the path for the tooltip.
func placeLabel(pl refs.Place) (text, tip string) {
	p := strings.TrimPrefix(pl.Path, "/")
	short := pl
	short.Path = path.Base(p)
	if p == "" {
		short.Path = ""
	}
	return short.String(), p
}
