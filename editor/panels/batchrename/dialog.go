package batchrename

import (
	"aego/asset/pipeline"
	"aego/engine/i18n"
	"path"
	"strconv"
	"strings"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/platform/input"
	"aego/project/refactor"
	"aego/project/rename"
	"aego/ui/core"
	"aego/ui/widgets"
)

type target uint8

const (
	targetAssets target = iota
	targetNodes
)

func caseNames() []string {
	return []string{i18n.Tr("editor.batchrename.case.keep", "Keep case"), i18n.Tr("editor.batchrename.case.lower", "lower"), i18n.Tr("editor.batchrename.case.upper", "UPPER"), i18n.Tr("editor.batchrename.case.title", "Title")}
}

type Deps struct {
	Pipeline *pipeline.Pipeline
	Docs     services.Documents
	Exists   func(rel string) bool
	Move     func(ops []assetops.FileOp) error
	Notices  services.Notices
}

type Dialog struct {
	d       Deps
	id      core.ID
	open    bool
	what    target
	assets  []ids.AssetID
	paths   []string
	doc     document.ID
	nodes   []ids.NodeID
	rule    rename.Rule
	caseIdx int
	start   float64
	step    float64
	rows    []rename.Row
	err     string
	dirty   bool
	scroll  float32
}

func New(d Deps) *Dialog {
	return &Dialog{d: d, id: core.IDOf("batchrename")}
}

func (g *Dialog) Active() bool {
	return g.open
}

func (g *Dialog) reset(ui *core.Context) {
	g.rule = rename.Rule{Pattern: "{name}"}
	g.caseIdx, g.start, g.step, g.scroll = 0, 1, 1, 0
	g.dirty, g.open = true, true
	ui.OpenModal(g.id)
}

func (g *Dialog) OpenAssets(ui *core.Context, list []ids.AssetID) {
	pl := g.d.Pipeline
	g.assets, g.paths, g.what = g.assets[:0], g.paths[:0], targetAssets
	for _, id := range list {
		if rel, ok := pl.PathOf(id); ok {
			g.assets = append(g.assets, id)
			g.paths = append(g.paths, rel)
		}
	}
	if len(g.assets) > 0 {
		g.reset(ui)
	}
}

func (g *Dialog) OpenNodes(ui *core.Context, d *document.SceneDocument, nodes []ids.NodeID) {
	if d == nil || len(nodes) == 0 {
		return
	}
	g.doc, g.nodes, g.what = d.ID, append(g.nodes[:0], nodes...), targetNodes
	g.reset(ui)
}

func (g *Dialog) items() []rename.Item {
	var out []rename.Item
	switch g.what {
	case targetAssets:
		for _, rel := range g.paths {
			ext := path.Ext(rel)
			out = append(out, rename.Item{Name: strings.TrimSuffix(path.Base(rel), ext), Ext: ext, Parent: path.Dir(rel)})
		}
	case targetNodes:
		d := g.d.Docs.Documents().Get(g.doc)
		if d == nil {
			return nil
		}
		s := d.Scene()
		for _, id := range g.nodes {
			h := s.Handle(id)
			if h.IsZero() || !s.IsAlive(h) {
				continue
			}
			parent := ""
			if ph := s.Parent(h); !ph.IsZero() && ph != s.Root() {
				parent = s.Name(ph)
			}
			out = append(out, rename.Item{Name: s.Name(h), Parent: parent})
		}
	}
	return out
}

func (g *Dialog) plan() {
	g.rule.Case = rename.Case(g.caseIdx)
	g.rule.Start, g.rule.Step = int(g.start), int(g.step)
	var rows []rename.Row
	var err error
	if g.what == targetAssets {
		exists := g.d.Exists
		rows, err = rename.Plan(g.rule, g.items(), assetops.ValidName, func(parent, name string) bool {
			return exists != nil && exists(path.Join(parent, name))
		}, true)
	} else {
		rows, err = rename.Plan(g.rule, g.items(), nil, nil, false)
		for i := range rows {
			if strings.HasPrefix(rows[i].Conflict, "same name") {
				rows[i].Conflict = ""
			}
		}
	}
	g.rows, g.err, g.dirty = rows, "", false
	if err != nil {
		g.rows, g.err = nil, err.Error()
	}
}

func (g *Dialog) close(ui *core.Context) {
	g.open = false
	ui.CloseModal()
}

func (g *Dialog) Draw(ui *core.Context) {
	if !g.open {
		return
	}
	th := &ui.Theme
	sz := ui.Size()
	w := min(720, sz.X-40)
	h := min(520, sz.Y-60)
	r := mathx.Rect{X: (sz.X - w) / 2, Y: max(20, sz.Y*0.1), W: w, H: h}
	if !ui.BeginModal(g.id, r) {
		g.open = false
		return
	}
	if ui.KeyPressed(input.KeyEscape) {
		ui.EndModal()
		g.close(ui)
		return
	}
	title := i18n.Tr("editor.batchrename.batch_rename_items_asset_s", "Batch Rename {items} asset(s)", i18n.Str("items", strconv.Itoa(len(g.items()))))
	if g.what == targetNodes {
		title = i18n.Tr("editor.batchrename.batch_rename_items_node_s", "Batch Rename {items} node(s)", i18n.Str("items", strconv.Itoa(len(g.items()))))
	}
	widgets.Label(ui, title)
	field := func(label string, s *string) {
		lr, fr := widgets.FieldRects(ui, label)
		ui.Text(label, mathx.Vec2{X: lr.X, Y: ui.TextY(lr)}, th.TextDim)
		before := *s
		widgets.TextFieldRect(ui, ui.ID("f/"+label), fr, s, false)
		if *s != before {
			g.dirty = true
		}
	}
	field(i18n.Tr("editor.batchrename.pattern", "Pattern"), &g.rule.Pattern)
	widgets.LabelDim(ui, "{name}  {n}  {n:03}  {parent}  {ext}")
	field(i18n.Tr("editor.batchrename.find", "Find"), &g.rule.Find)
	field(i18n.Tr("editor.batchrename.replace", "Replace"), &g.rule.Replace)
	row := ui.Cursor().Next(0)
	rx := row.X
	regex := g.rule.Regex
	if widgets.ToggleButton(ui, ui.ID("regex"), mathx.Rect{X: rx, Y: row.Y, W: 70, H: row.H}, i18n.Tr("editor.batchrename.regex", ".* Regex"), &regex) {
		g.rule.Regex, g.dirty = regex, true
	}
	rx += 76
	if widgets.Combo(ui, ui.ID("case"), mathx.Rect{X: rx, Y: row.Y, W: 120, H: row.H}, &g.caseIdx, caseNames(), false) {
		g.dirty = true
	}
	rx += 126
	for _, n := range []struct {
		label string
		v     *float64
	}{{i18n.Tr("editor.batchrename.start", "Start"), &g.start}, {i18n.Tr("editor.batchrename.step", "Step"), &g.step}} {
		lw := ui.TextSize(n.label).X + 6
		ui.Text(n.label, mathx.Vec2{X: rx, Y: ui.TextY(row)}, th.TextDim)
		if e := widgets.DragNumber(ui, ui.ID("n/"+n.label), mathx.Rect{X: rx + lw, Y: row.Y, W: 60, H: row.H}, n.v, widgets.NumberOpts{Int: true, Speed: 0.1}); e.Changed {
			g.dirty = true
		}
		rx += lw + 70
	}
	if g.dirty {
		g.plan()
	}
	if g.err != "" {
		widgets.LabelWrapped(ui, g.err, true)
	}
	top := ui.Cursor().Y
	list := mathx.Rect{X: r.X + th.Padding, Y: top, W: r.W - 2*th.Padding, H: max(0, r.Y+r.H-top-th.RowHeight*2-th.Padding)}
	widgets.VirtualList(ui, ui.ID("rows"), list, len(g.rows), th.RowHeight, &g.scroll, func(i int, rr mathx.Rect) {
		rw := g.rows[i]
		half := rr.W / 2
		col := th.Text
		if !rw.Changed() {
			col = th.TextDim
		}
		ui.PushClip(mathx.Rect{X: rr.X, Y: rr.Y, W: half - 8, H: rr.H})
		ui.Text(rw.From, mathx.Vec2{X: rr.X, Y: ui.TextY(rr)}, th.TextDim)
		ui.PopClip()
		to := rw.To
		if rw.Conflict != "" {
			col, to = th.Error, rw.To+"   ("+rw.Conflict+")"
		}
		ui.PushClip(mathx.Rect{X: rr.X + half, Y: rr.Y, W: half, H: rr.H})
		ui.Text("→ "+to, mathx.Vec2{X: rr.X + half, Y: ui.TextY(rr)}, col)
		ui.PopClip()
	})
	ui.Cursor().Y = list.Y + list.H + 4
	btn := ui.Cursor().Next(0)
	bw := float32(120)
	if widgets.ButtonRect(ui, ui.ID("cancel"), mathx.Rect{X: btn.X + btn.W - bw, Y: btn.Y, W: bw, H: btn.H}, i18n.Tr("editor.batchrename.cancel", "Cancel")) {
		ui.EndModal()
		g.close(ui)
		return
	}
	changed := 0
	for _, rw := range g.rows {
		if rw.Changed() {
			changed++
		}
	}
	ok := changed > 0 && !rename.Blocked(g.rows) && g.err == ""
	ar := mathx.Rect{X: btn.X + btn.W - 2*bw - 6, Y: btn.Y, W: bw, H: btn.H}
	if ok {
		if widgets.ButtonRect(ui, ui.ID("apply"), ar, i18n.Tr("editor.batchrename.rename_changed", "Rename {changed}", i18n.Str("changed", strconv.Itoa(changed)))) {
			if err := g.apply(); err != nil {
				g.d.Notices.Notify(context.LevelError, err.Error())
			}
			ui.EndModal()
			g.close(ui)
			return
		}
	} else {
		ui.Rect(ar, th.Widget)
		ui.TextIn(ar, i18n.Tr("editor.batchrename.rename", "Rename"), th.TextDisabled, core.AlignCenter)
	}
	ui.EndModal()
}

func (g *Dialog) apply() error {
	if g.what == targetNodes {
		return g.applyNodes()
	}
	return g.applyAssets()
}

func (g *Dialog) applyNodes() error {
	d := g.d.Docs.Documents().Get(g.doc)
	if d == nil {
		return nil
	}
	s := d.Scene()
	tx := d.History.Begin(i18n.Tr("editor.history.batch_rename", "Batch Rename"))
	k := 0
	for _, id := range g.nodes {
		h := s.Handle(id)
		if h.IsZero() || !s.IsAlive(h) {
			continue
		}
		rw := g.rows[k]
		k++
		if !rw.Changed() {
			continue
		}
		if err := d.Props.Rename(tx, id, rw.To); err != nil {
			tx.Abort()
			return err
		}
	}
	tx.Commit()
	return nil
}

func (g *Dialog) applyAssets() error {
	ops := refactor.RenameOps(g.paths, g.assets, g.rows)
	list := make([]assetops.FileOp, len(ops))
	for i, op := range ops {
		list[i] = assetops.FileOp{ID: op.ID, From: op.From, To: op.To}
	}
	return g.d.Move(list)
}
