package search

import (
	favs "aego/editor/core/favorites"
	"aego/engine/i18n"
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
	"aego/editor/core/props"
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
	favBonus    = 40
)

type Tab uint8

const (
	TabAll Tab = iota
	TabFiles
	TabSymbols
	TabNodes
	TabActions
	TabSettings
	tabCount
)

var tabNames = [tabCount]i18n.Text{
	i18n.T("editor.search.tab.all", "All"), i18n.T("editor.search.tab.files", "Files"), i18n.T("editor.search.tab.symbols", "Symbols"),
	i18n.T("editor.search.tab.nodes", "Nodes"), i18n.T("editor.search.tab.actions", "Actions"), i18n.T("editor.search.tab.settings", "Settings"),
}

type Item struct {
	Label  string
	Detail string
	Kind   string
	Tab    Tab
	Icon   string
	Run    func()
	Fav    favs.Item
	HasFav bool
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
	Thumbs    context.ThumbSource
	Pipeline  *pipeline.Pipeline
	Symbols   func() []context.SearchItem
	Docs      services.Documents
	Selection services.Selection
	Commands  *command.Registry
	Notices   services.Notices
	Nav       services.Navigation
	Settings  func() []Item
	Favorites *favs.Store
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
		return "file.mesh"
	}
	return "file.texture"
}

func (o *Overlay) collect() {
	o.items = o.items[:0]
	o.favorites()
	if o.d.Pipeline != nil {
		files := o.d.Pipeline.Assets()
		if src, ok := o.d.Thumbs.(interface{ SourceFiles() []pipeline.AssetInfo }); ok {
			files = append(files, src.SourceFiles()...)
		}
		for _, a := range files {
			rel := a.Path
			o.items = append(o.items, Item{
				Label: path.Base(rel), Detail: path.Dir(rel), Kind: "file", Tab: TabFiles, Icon: fileIcon(a.Kind),
				Fav: favs.Item{Kind: favs.Asset, Asset: a.ID, Label: path.Base(rel)}, HasFav: true,
				Run: func() { o.d.Nav.Open(context.AssetDocumentAt(rel)) },
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
		root, doc, scenePath := s.Root(), d.ID, d.Path
		for _, h := range s.PreOrder() {
			if h == root || !s.IsAlive(h) {
				continue
			}
			id := s.ID(h)
			o.items = append(o.items, Item{
				Label: s.Name(h), Detail: strings.TrimPrefix(s.Path(h), "/"), Kind: "node", Tab: TabNodes, Icon: props.NodeIcon(s, h),
				Fav: favs.Item{Kind: favs.Node, Scene: scenePath, Node: id, Label: s.Name(h)}, HasFav: true,
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
	if o.d.Settings != nil {
		for _, it := range o.d.Settings() {
			it.Tab, it.Kind = TabSettings, "setting"
			if it.Icon == "" {
				it.Icon = "ui.more"
			}
			o.items = append(o.items, it)
		}
	}
	for _, d := range o.d.Commands.All() {
		if d.ID == CommandOpen {
			continue
		}
		id := d.ID
		detail := d.Category
		if t := i18n.Lookup("editor.menu."+strings.ToLower(d.Category), ""); t != "" {
			detail = t
		}
		if sc := d.Shortcut.String(); sc != "" {
			detail += "  " + sc
		}
		o.items = append(o.items, Item{
			Label: d.Title(), Detail: detail, Kind: "action", Tab: TabActions, Icon: "tool.select",
			Fav: favs.Item{Kind: favs.Command, Command: id, Label: d.Name}, HasFav: true,
			Run: func() {
				if o.d.Commands.Enabled(id) {
					o.d.Commands.Run(id)
				} else {
					o.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.search.this_action_is_not_available", "This action is not available right now"))
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
		if it.Kind == "favorite" && o.tab != TabAll {
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
		if it.Kind == "favorite" {
			s += favBonus
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
		case ui.Primary() && ui.KeyPressed(input.KeyD):
			o.star()
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
		bw := ui.TextSize(tabNames[t].String()).X + 18
		on := o.tab == t
		if widgets.ToggleButton(ui, core.IDSeq(o.id, 10+int(t)), mathx.Rect{X: x, Y: tabs.Y, W: bw, H: tabs.H}, tabNames[t].String(), &on) {
			o.tab = t
		}
		x += bw + 4
	}
	ui.TextIn(tabs, i18n.Tr("editor.search.hint_tabs", "Double Shift · Tab switches"), th.TextDisabled, core.AlignRight)
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
		ui.TextIn(list, i18n.Tr("editor.search.nothing_found", "Nothing found"), th.TextDisabled, core.AlignCenter)
	}
	foot := mathx.Rect{X: r.X + th.Padding, Y: r.Y + r.H - th.RowHeight - th.Padding/2, W: r.W - 2*th.Padding, H: th.RowHeight}
	ui.TextIn(foot, i18n.Tr("editor.search.hits_result_s", "{hits} result(s)", i18n.Str("hits", strconv.Itoa(len(o.hits)))), th.TextDim, core.AlignLeft)
	ui.TextIn(foot, i18n.Tr("editor.search.hint", "Enter open · Ctrl+D favorite · Esc close"), th.TextDim, core.AlignRight)
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
	box := mathx.Rect{X: rr.X + 4, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}
	pictured := false
	if o.d.Thumbs != nil && !it.Fav.Asset.IsZero() {
		if reg, ok := o.d.Thumbs.ByID(it.Fav.Asset); ok {
			ui.List().Texture(reg, box, mathx.White)
			pictured = true
		}
	}
	if !pictured {
		widgets.IconAt(ui, box, it.Icon, th.TextDim)
	}
	tx := rr.X + 26
	ty := ui.TextY(rr)
	kw := ui.TextSize(it.Kind).X + 12
	starred := it.Kind != "favorite" && it.HasFav && o.d.Favorites != nil && o.d.Favorites.Has(it.Fav)
	if starred {
		kw += 20
	}
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
	if starred {
		widgets.IconAt(ui, mathx.Rect{X: rr.X + rr.W - kw + 2, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}, "ui.star", th.Accent)
	}
	ui.TextIn(rr, it.Kind, th.TextDisabled, core.AlignRight)
	return pressed
}

func (o *Overlay) env() favs.Env {
	return favs.Env{Pipeline: o.d.Pipeline, Docs: o.d.Docs, Selection: o.d.Selection, Commands: o.d.Commands, Nav: o.d.Nav, ShowHierarchy: o.ShowHierarchy}
}

func (o *Overlay) favorites() {
	if o.d.Favorites == nil {
		return
	}
	env := o.env()
	for _, f := range o.d.Favorites.Items() {
		it := f
		label, detail, _, ok := favs.Describe(env, it)
		if !ok {
			continue
		}
		o.items = append(o.items, Item{
			Label: label, Detail: detail, Kind: "favorite", Tab: TabAll, Icon: "ui.star", Fav: it, HasFav: true,
			Run: func() {
				if err := favs.Open(env, it); err != nil {
					o.d.Notices.Notify(context.LevelInfo, err.Error())
				}
			},
		})
	}
}

func (o *Overlay) star() {
	if o.d.Favorites == nil || o.sel < 0 || o.sel >= len(o.hits) {
		return
	}
	it := &o.items[o.hits[o.sel]]
	if !it.HasFav {
		o.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.search.only_files_nodes_and_actions", "Only files, nodes and actions can be favorites"))
		return
	}
	added, err := o.d.Favorites.Toggle(it.Fav)
	switch {
	case err != nil:
		o.d.Notices.Notify(context.LevelError, err.Error())
	case added:
		o.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.search.fav_added", "Added {name} to Favorites", i18n.Str("name", it.Fav.Label)))
	default:
		o.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.search.fav_removed", "Removed {name} from Favorites", i18n.Str("name", it.Fav.Label)))
	}
}
