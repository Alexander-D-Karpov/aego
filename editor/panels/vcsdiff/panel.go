package vcsdiff

import (
	"aego/core/ids"
	"aego/engine/i18n"
	"path"
	"strconv"

	"aego/asset/pipeline"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/scenediff"
	"aego/editor/core/services"
	"aego/editor/core/tasks"
	"aego/editor/core/vcs"
	"aego/ui/core"
	"aego/ui/widgets"
)

const ID = "structural_diff"

type Deps struct {
	VCS           vcs.Provider
	Pipeline      *pipeline.Pipeline
	Docs          services.Documents
	Selection     services.Selection
	Notices       services.Notices
	Tasks         *tasks.Queue
	ShowHierarchy func()
}

type Panel struct {
	d       Deps
	rel     string
	revs    []vcs.Commit
	rev     int
	res     scenediff.Result
	err     string
	loading *tasks.Task
	scroll  float32
}

func New(d Deps) *Panel {
	return &Panel{d: d}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return i18n.Tr("editor.vcsdiff.title", "Diff")
}

func (p *Panel) Icon() string {
	return "panel.diff"
}

func Diffable(rel string) bool {
	switch path.Ext(rel) {
	case ".aescene", ".aeprefab", ".aedata", ".aetable", ".aepreset":
		return true
	}
	return false
}

func (p *Panel) Show(rel string) {
	p.rel, p.rev, p.revs, p.scroll = rel, 0, nil, 0
	p.load()
}

func (p *Panel) revision() string {
	if p.rev > 0 && p.rev <= len(p.revs) {
		return p.revs[p.rev-1].Hash
	}
	return "HEAD"
}

func (p *Panel) load() {
	if p.d.VCS == nil || p.rel == "" || p.loading != nil {
		return
	}
	rel, rev, abs, prov := p.rel, p.revision(), p.d.Pipeline.Abs(p.rel), p.d.VCS
	needLog := p.revs == nil
	var res scenediff.Result
	var log []vcs.Commit
	var failed string
	p.err = ""
	p.loading = p.d.Tasks.Work(tasks.Spec{Title: i18n.Tr("editor.vcsdiff.diff_rel_with_rev", "Diff {rel} with {rev}", i18n.Str("rel", path.Base(rel)), i18n.Str("rev", shortRev(rev))), Kind: "vcs"}, func(*tasks.Task) error {
		if needLog {
			log, _ = prov.Log(rel, 50)
		}
		before, err := prov.Show(rev, rel)
		if err != nil {
			before = nil
			failed = "not in " + shortRev(rev) + ", shown as new"
		}
		after, err := vcs.ReadWorking(abs)
		if err != nil {
			return err
		}
		res, err = scenediff.Compute(before, after, rel)
		return err
	}).Then(func(t *tasks.Task) {
		p.loading = nil
		if t.Err() != nil {
			p.err = t.Err().Error()
			return
		}
		if needLog {
			p.revs = log
		}
		p.res = res
		if failed != "" {
			p.err = failed
		}
	})
}

func shortRev(r string) string {
	if len(r) > 8 {
		return r[:8]
	}
	return r
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	if p.d.VCS == nil {
		widgets.LabelWrapped(ui, i18n.Tr("editor.vcsdiff.the_project_is_not_inside", "The project is not inside a git repository"), true)
		return
	}
	if p.rel == "" {
		widgets.LabelWrapped(ui, i18n.Tr("editor.vcsdiff.use_compare_with_head_in", "Use Compare with HEAD in the menu of a scene, prefab or data asset"), true)
		return
	}
	head := ui.Cursor().Next(0)
	names := []string{"HEAD"}
	for _, c := range p.revs {
		names = append(names, c.Short+"  "+c.Subject)
	}
	cw := min(260, head.W*0.45)
	ui.PushClip(mathx.Rect{X: head.X, Y: head.Y, W: head.W - cw - 6, H: head.H})
	ui.Text(i18n.Tr("editor.vcsdiff.rel_working_copy_vs", "{rel}  ·  working copy vs", i18n.Str("rel", p.rel)), mathx.Vec2{X: head.X, Y: ui.TextY(head)}, th.Text)
	ui.PopClip()
	if widgets.Combo(ui, ui.ID("rev"), mathx.Rect{X: head.X + head.W - cw, Y: head.Y, W: cw, H: head.H}, &p.rev, names, false) {
		p.load()
	}
	if d := p.d.Docs.Documents().Find(p.rel); d != nil && d.Dirty() {
		widgets.LabelDim(ui, i18n.Tr("editor.vcsdiff.unsaved_changes_in_the_open", "Unsaved changes in the open document are not included"))
	}
	if p.loading != nil {
		widgets.LabelDim(ui, i18n.Tr("editor.vcsdiff.loading", "Loading…"))
		return
	}
	if p.err != "" {
		widgets.LabelDim(ui, p.err)
	}
	if p.res.Empty() {
		widgets.LabelDim(ui, i18n.Tr("editor.vcsdiff.no_differences", "No differences"))
		return
	}
	widgets.LabelDim(ui, i18n.Tr("editor.vcsdiff.changes_change_s", "{changes} change(s)", i18n.Str("changes", strconv.Itoa(len(p.res.Changes)))))
	top := ui.Cursor().Y
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	widgets.VirtualList(ui, ui.ID("rows"), list, len(p.res.Changes), th.RowHeight+2, &p.scroll, func(i int, rr mathx.Rect) {
		p.row(ui, i, rr)
	})
}

func kindColor(th *core.Theme, k scenediff.Kind) mathx.Color {
	switch k {
	case scenediff.NodeAdded, scenediff.CompAdded, scenediff.FieldAdded:
		return th.AxisY
	case scenediff.NodeRemoved, scenediff.CompRemoved, scenediff.FieldRemoved:
		return th.Error
	}
	return th.Warn
}

func (p *Panel) row(ui *core.Context, i int, rr mathx.Rect) {
	th := &ui.Theme
	c := p.res.Changes[i]
	id := core.IDSeq(ui.ID("row"), i)
	if ui.Clicked(id, rr) && p.res.Scene && c.Node != "" {
		p.reveal(c.Node)
	}
	if ui.Hot() == id {
		ui.Rect(rr, th.WidgetHot)
	}
	ui.Rect(mathx.Rect{X: rr.X, Y: rr.Y + 2, W: 3, H: rr.H - 4}, kindColor(th, c.Kind))
	where := c.Path
	if c.Owner != "" {
		if where != "" {
			where += " · "
		}
		where += c.Owner
	}
	if c.Field != "" {
		where += " · " + c.Field
	}
	text := c.Kind.String()
	switch {
	case c.Before != "" && c.After != "":
		text += ": " + c.Before + "  →  " + c.After
	case c.After != "":
		text += ": " + c.After
	case c.Before != "":
		text += ": " + c.Before
	}
	ww := min(rr.W*0.45, ui.TextSize(where).X+12)
	ui.PushClip(mathx.Rect{X: rr.X + 8, Y: rr.Y, W: ww, H: rr.H})
	ui.Text(where, mathx.Vec2{X: rr.X + 8, Y: ui.TextY(rr)}, th.Text)
	ui.PopClip()
	ui.PushClip(mathx.Rect{X: rr.X + 12 + ww, Y: rr.Y, W: max(0, rr.W-ww-12), H: rr.H})
	ui.Text(text, mathx.Vec2{X: rr.X + 12 + ww, Y: ui.TextY(rr)}, th.TextDim)
	ui.PopClip()
	ui.MouseOver(id, rr)
	widgets.Tooltip(ui, id, where+"\n"+text)
}

func (p *Panel) reveal(node string) {
	id, err := parseNode(node)
	if err != nil {
		return
	}
	d, err := p.d.Docs.Documents().Open(p.rel)
	if err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
		return
	}
	s := d.Scene()
	if h := s.Handle(id); h.IsZero() || !s.IsAlive(h) {
		p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.vcsdiff.the_node_is_not_in", "The node is not in the working copy"))
		return
	}
	p.d.Selection.Select(id)
	if p.d.ShowHierarchy != nil {
		p.d.ShowHierarchy()
	}
}

func parseNode(s string) (ids.NodeID, error) {
	return ids.ParseNodeID(s)
}
