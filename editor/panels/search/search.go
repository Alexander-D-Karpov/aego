package search

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/fuzzy"
	"aego/editor/core/services"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	CommandOpen = "search.everywhere"
	maxHits     = 200
	maxEmpty    = 60
	detailCost  = 30
)

type Tab uint8

const (
	TabAll Tab = iota
	TabFiles
	TabSymbols
	TabNodes
	TabActions
	tabCount
)

var tabNames = [tabCount]string{"All", "Files", "Symbols", "Nodes", "Actions"}

type Item struct {
	Label  string
	Detail string
	Kind   string
	Tab    Tab
	Icon   string
	Run    func()
	score  int
	pos    []int
}

type Overlay struct {
	ShowHierarchy func()
	id            core.ID
	open          bool
	focus         bool
	follow        bool
	query         string
	last          string
	lastTab       Tab
	tab           Tab
	sel           int
	scroll        float32
	items         []Item
	hits          []int
	dirty         bool
	d             Deps
}

type Deps struct {
	Pipeline  *pipeline.Pipeline
	Symbols   func() []context.SearchItem
	Docs      services.Documents
	Selection services.Selection
	Commands  *command.Registry
	Notices   services.Notices
	Nav       services.Navigation
}

func New(d Deps) *Overlay {
	return &Overlay{id: core.IDOf("search.everywhere"), d: d}
}

func (o *Overlay) Active() bool {
	return o.open
}

func (o *Overlay) Open(ui *core.Context, tab Tab) {
	o.open, o.focus, o.dirty = true, true, true
	o.tab, o.sel, o.scroll = tab, 0, 0
	o.collect()
	ui.OpenModal(o.id)
}

func (o *Overlay) Close(ui *core.Context) {
	if !o.open {
		return
	}
	o.open = false
	ui.CloseModal()
	ui.CancelFocus()
}

func fileIcon(k asset.ArtifactKind) string {
	switch k {
	case asset.ArtScene:
		return "file.scene"
	case asset.ArtScript:
		return "file.script"
	case asset.ArtPrefab:
		return "file.prefab"
	case asset.ArtMesh:
		return "cat.rendering"
	}
	return "file.texture"
}

func (o *Overlay) collect() {
	o.items = o.items[:0]
	if o.d.Pipeline != nil {
		for _, a := range o.d.Pipeline.Assets() {
			rel := a.Path
			o.items = append(o.items, Item{
				Label: path.Base(rel), Detail: path.Dir(rel), Kind: "file", Tab: TabFiles, Icon: fileIcon(a.Kind),
				Run: func() { o.d.Nav.Open(context.Location{Path: rel, Offset: -1}) },
			})
		}
	}
	if o.d.Symbols != nil {
		for _, s := range o.d.Symbols() {
			loc := s.Loc
			o.items = append(o.items, Item{
				Label: s.Label, Detail: s.Detail, Kind: s.Kind, Tab: TabSymbols, Icon: "cat.scripts",
				Run: func() { o.d.Nav.Open(loc) },
			})
		}
	}
	if d := o.d.Docs.Doc(); d != nil {
		s := d.Scene()
		root, doc := s.Root(), d.ID
		for _, h := range s.PreOrder() {
			if h == root || !s.IsAlive(h) {
				continue
			}
			id := s.ID(h)
			o.items = append(o.items, Item{
				Label: s.Name(h), Detail: s.Path(h), Kind: "node", Tab: TabNodes, Icon: "cat.node",
				Run: func() {
					o.d.Docs.Documents().SetActive(doc)
					o.d.Selection.Select(id)
					if o.ShowHierarchy != nil {
						o.ShowHierarchy()
					}
				},
			})
		}
	}
	for _, d := range o.d.Commands.All() {
		if d.ID == CommandOpen {
			continue
		}
		id := d.ID
		detail := d.Category
		if sc := d.Shortcut.String(); sc != "" {
			detail += "  " + sc
		}
		o.items = append(o.items, Item{
			Label: d.Name, Detail: detail, Kind: "action", Tab: TabActions, Icon: "tool.select",
			Run: func() {
				if o.d.Commands.Enabled(id) {
					o.d.Commands.Run(id)
				} else {
					o.d.Notices.Notify(context.LevelInfo, "This action is not available right now")
				}
			},
		})
	}
}

func (o *Overlay) rank() {
	o.hits = o.hits[:0]
	q := strings.ReplaceAll(strings.TrimSpace(o.query), " ", "")
	for i := range o.items {
		it := &o.items[i]
		if o.tab != TabAll && it.Tab != o.tab {
			continue
		}
		if q == "" {
			it.score, it.pos = 0, it.pos[:0]
			o.hits = append(o.hits, i)
			continue
		}
		s, pos, ok := fuzzy.Match(q, it.Label, it.pos)
		if !ok && it.Detail != "" {
			if ds, _, dok := fuzzy.Match(q, it.Detail, nil); dok {
				s, pos, ok = ds-detailCost, pos[:0], true
			}
		}
		if !ok {
			continue
		}
		it.score, it.pos = s, pos
		o.hits = append(o.hits, i)
	}
	if q != "" {
		sort.SliceStable(o.hits, func(a, b int) bool {
			x, y := &o.items[o.hits[a]], &o.items[o.hits[b]]
			if x.score != y.score {
				return x.score > y.score
			}
			return len(x.Label) < len(y.Label)
		})
	}
	limit := maxHits
	if q == "" {
		limit = maxEmpty
	}
	if len(o.hits) > limit {
		o.hits = o.hits[:limit]
	}
	o.sel, o.scroll = 0, 0
}

func (o *Overlay) Draw(ui *core.Context) {
	if !o.open {
		return
	}
	th := &ui.Theme
	if o.dirty || o.query != o.last || o.tab != o.lastTab {
		o.rank()
		o.last, o.lastTab, o.dirty = o.query, o.tab, false
	}
	sz := ui.Size()
	w := min(680, sz.X-40)
	h := min(460, sz.Y-60)
	r := mathx.Rect{X: (sz.X - w) / 2, Y: max(20, sz.Y*0.12), W: w, H: h}
	if ui.MousePressed(input.MouseLeft) && !r.Contains(ui.Mouse()) {
		o.Close(ui)
		return
	}
	if !ui.BeginModal(o.id, r) {
		o.open = false
		return
	}
	qid := core.IDSeq(o.id, 1)
	if o.focus || ui.Focus() == 0 {
		ui.SetFocus(qid)
		if o.focus {
			ui.EditBuffer(qid, o.query).SelectAll()
		}
		o.focus = false
	}
	run := -1
	if ui.Focus() == qid {
		switch {
		case ui.KeyPressed(input.KeyEscape):
			ui.EndModal()
			o.Close(ui)
			return
		case ui.KeyPressed(input.KeyEnter), ui.KeyPressed(input.KeyKPEnter):
			run = o.sel
		case ui.KeyPressed(input.KeyTab):
			if ui.Mods().Has(input.ModShift) {
				o.tab = (o.tab + tabCount - 1) % tabCount
			} else {
				o.tab = (o.tab + 1) % tabCount
			}
		case ui.KeyRepeated(input.KeyDown):
			o.sel = min(o.sel+1, len(o.hits)-1)
			o.follow = true
		case ui.KeyRepeated(input.KeyUp):
			o.sel = max(o.sel-1, 0)
			o.follow = true
		}
	}
	tabs := ui.Cursor().Next(0)
	x := tabs.X
	for t := Tab(0); t < tabCount; t++ {
		bw := ui.TextSize(tabNames[t]).X + 18
		on := o.tab == t
		if widgets.ToggleButton(ui, core.IDSeq(o.id, 10+int(t)), mathx.Rect{X: x, Y: tabs.Y, W: bw, H: tabs.H}, tabNames[t], &on) {
			o.tab = t
		}
		x += bw + 4
	}
	ui.TextIn(tabs, "Double Shift · Tab switches", th.TextDisabled, core.AlignRight)
	widgets.TextFieldRect(ui, qid, ui.Cursor().Next(0), &o.query, true)
	cur := ui.Cursor()
	rowH := th.RowHeight + 2
	list := mathx.Rect{X: cur.X, Y: cur.Y, W: cur.W, H: max(0, r.Y+r.H-cur.Y-th.RowHeight-th.Padding)}
	if o.follow {
		widgets.EnsureVisible(&o.scroll, o.sel, rowH, list.H)
		o.follow = false
	}
	widgets.VirtualList(ui, core.IDSeq(o.id, 2), list, len(o.hits), rowH, &o.scroll, func(i int, rr mathx.Rect) {
		if o.drawItem(ui, i, rr) {
			run = i
		}
	})
	if len(o.hits) == 0 {
		ui.TextIn(list, "Nothing found", th.TextDisabled, core.AlignCenter)
	}
	foot := mathx.Rect{X: r.X + th.Padding, Y: r.Y + r.H - th.RowHeight - th.Padding/2, W: r.W - 2*th.Padding, H: th.RowHeight}
	ui.TextIn(foot, strconv.Itoa(len(o.hits))+" result(s)", th.TextDim, core.AlignLeft)
	ui.TextIn(foot, "Enter open · Esc close", th.TextDim, core.AlignRight)
	ui.EndModal()
	if run >= 0 && run < len(o.hits) {
		it := o.items[o.hits[run]]
		o.Close(ui)
		if it.Run != nil {
			it.Run()
		}
	}
}

func (o *Overlay) drawItem(ui *core.Context, i int, rr mathx.Rect) bool {
	th := &ui.Theme
	it := &o.items[o.hits[i]]
	id := core.IDSeq(o.id, 100+i)
	pressed := ui.Pressed(id, rr)
	switch {
	case i == o.sel:
		ui.Rect(rr, th.Selection)
	case ui.Hot() == id:
		ui.Rect(rr, th.WidgetHot)
	}
	widgets.IconAt(ui, mathx.Rect{X: rr.X + 4, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}, it.Icon, th.TextDim)
	tx := rr.X + 26
	ty := ui.TextY(rr)
	kw := ui.TextSize(it.Kind).X + 12
	ui.PushClip(mathx.Rect{X: rr.X, Y: rr.Y, W: max(0, rr.W-kw), H: rr.H})
	ui.Text(it.Label, mathx.Vec2{X: tx, Y: ty}, th.Text)
	for _, p := range it.pos {
		if p >= len(it.Label) {
			continue
		}
		px := tx + ui.TextSize(it.Label[:p]).X
		ui.Rect(mathx.Rect{X: px, Y: ty + ui.LineHeight() - 1, W: ui.TextSize(it.Label[p : p+1]).X, H: 2}, th.Accent)
	}
	ui.Text(it.Detail, mathx.Vec2{X: tx + ui.TextSize(it.Label).X + 12, Y: ty}, th.TextDim)
	ui.PopClip()
	ui.TextIn(rr, it.Kind, th.TextDisabled, core.AlignRight)
	return pressed
}
