package assets

import (
	"aego/editor/core/code/highlight"
	favs "aego/editor/core/favorites"
	"aego/editor/core/refs"
	"aego/engine/i18n"
	"os/exec"
	"path"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/assetfiles"
	"aego/editor/core/assetops"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/preview"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/platform/input"
	"aego/project"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/dnd"
	"aego/ui/widgets"
)

const (
	CommandRename         = "assets.rename"
	CommandDelete         = "assets.delete"
	CommandDuplicate      = "assets.duplicate"
	CommandUndo           = "assets.undo"
	CommandRedo           = "assets.redo"
	CommandCopy           = "assets.copy"
	CommandCut            = "assets.cut"
	CommandPaste          = "assets.paste"
	CommandFindReferences = "assets.find_references"
)

type dirRow struct {
	path  string
	depth int
	has   bool
}

type Deps struct {
	Files          *assetfiles.Files
	Pipeline       *pipeline.Pipeline
	Thumbs         *preview.Service
	Docs           services.Documents
	Selection      services.Selection
	Scenes         services.SceneOps
	Prefabs        services.Prefabs
	Commands       *command.Registry
	Notices        services.Notices
	Nav            services.Navigation
	Project        *project.Project
	Schema         *schema.Registry
	Inspect        func(id ids.AssetID)
	Viewable       func(kind asset.ArtifactKind) bool
	OpenTable      func(id ids.AssetID)
	DataType       func(id ids.AssetID) (string, bool)
	Favorites      *favs.Store
	Refs           *refs.Service
	ShowReferences func(id ids.AssetID, deleting []ids.AssetID)
	BatchRename    func(list []ids.AssetID)
	Compare        func(rel string)
}

type Panel struct {
	Instantiate    func(id ids.AssetID, name string)
	d              Deps
	f              *services.Frame
	now            float64
	folder         string
	expanded       map[string]bool
	dirs           []string
	rows           []dirRow
	items          []pipeline.AssetInfo
	view           []int
	subdirs        []string
	rev            uint64
	dirsAt         float64
	sel            map[ids.AssetID]bool
	active         ids.AssetID
	anchor         ids.AssetID
	pending        ids.AssetID
	inspectPending bool
	selDir         string
	search         string
	split          float32
	treeScroll     float32
	gridScroll     float32
	overDir        bool
	renaming       ids.AssetID
	renameStart    bool
	renameText     string
	renamingDir    string
	renameDirStart bool
	renameDirText  string
	menuDir        string
	assetMenuID    core.ID
	dirMenuID      core.ID
	cut            cutSet
	scrollTo       ids.AssetID
	tile           float32
	stacks         map[string][]int
	stackRev       uint64
	spans          []highlight.Span
	sheetOpen      map[ids.AssetID]bool
	drawn          []drawnTile
	cellsOf        map[ids.AssetID]int
	sheetOf        map[ids.AssetID]ids.AssetID
}

func New(d Deps) *Panel {
	return &Panel{
		d:         d,
		folder:    "assets",
		expanded:  map[string]bool{"assets": true, "scenes": true, "scripts": true, "prefabs": true},
		sel:       map[ids.AssetID]bool{},
		sheetOpen: map[ids.AssetID]bool{},
		cellsOf:   map[ids.AssetID]int{},
		sheetOf:   map[ids.AssetID]ids.AssetID{},
		split:     0.28,
		rev:       ^uint64(0),
	}
}

func (p *Panel) report(err error) {
	if err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
	}
}

func (p *Panel) CanUndo() bool {
	return p.d.Files.CanUndo()
}

func (p *Panel) CanRedo() bool {
	return p.d.Files.CanRedo()
}

func (p *Panel) ID() string {
	return workspace.Assets
}

func (p *Panel) Title() string {
	return i18n.Tr("editor.assets.title", "Assets")
}

func (p *Panel) Scope() string {
	return command.ScopeAssets
}

func (p *Panel) Selected() ids.AssetID {
	return p.active
}

func (p *Panel) HasSelection() bool {
	return len(p.sel) > 0
}

func (p *Panel) Single() bool {
	return len(p.sel) == 1
}

func (p *Panel) Selection() []ids.AssetID {
	out := make([]ids.AssetID, 0, len(p.sel))
	for i := range p.items {
		if p.sel[p.items[i].ID] {
			out = append(out, p.items[i].ID)
		}
	}
	return out
}

func (p *Panel) Undo() error {
	err := p.d.Files.Undo()
	p.invalidate()
	return err
}

func (p *Panel) Redo() error {
	err := p.d.Files.Redo()
	p.invalidate()
	return err
}

func (p *Panel) find(id ids.AssetID) *pipeline.AssetInfo {
	for i := range p.items {
		if p.items[i].ID == id {
			return &p.items[i]
		}
	}
	return nil
}

func (p *Panel) invalidate() {
	p.rev = ^uint64(0)
}

func (p *Panel) refresh() {
	pl := p.d.Pipeline
	rev := pl.Revision()
	if rev != p.rev || p.now-p.dirsAt > 2 {
		changed := rev != p.rev
		p.rev = rev
		p.dirsAt = p.now
		p.dirs = pl.Dirs()
		var extras []pipeline.AssetInfo
		if p.d.Thumbs != nil {
			changed = p.d.Thumbs.RefreshSources() || changed
			extras = p.d.Thumbs.SourceFiles()
			for _, s := range extras {
				for d := path.Dir(s.Path); ; d = path.Dir(d) {
					if !slices.Contains(p.dirs, d) {
						p.dirs = append(p.dirs, d)
					}
					if d == "." {
						break
					}
				}
			}
			slices.Sort(p.dirs)
		}
		if changed {
			p.items = append(pl.Assets(), extras...)
			p.addCells()
			p.stacks = nil
			p.prune()
		}
	}

	found := false
	for _, d := range p.dirs {
		if d == p.folder {
			found = true
			break
		}
	}
	if !found && len(p.dirs) > 0 {
		p.folder = p.dirs[0]
		if slices.Contains(p.dirs, "assets") {
			p.folder = "assets"
		}
	}
}

// addCells lists the cells of every sliced sheet right after it, as sprites (D339).
func (p *Panel) addCells() {
	clear(p.cellsOf)
	clear(p.sheetOf)
	n := len(p.items)
	for i := range n {
		a := p.items[i]
		if a.Kind != asset.ArtTexture {
			continue
		}
		m, ok := p.d.Pipeline.MetaOf(a.ID)
		if !ok || len(m.Subs) == 0 {
			continue
		}
		p.cellsOf[a.ID] = len(m.Subs)
		for _, s := range m.Subs {
			p.sheetOf[s.ID] = a.ID
			p.items = append(p.items, pipeline.AssetInfo{ID: s.ID, Path: a.Path + pipeline.SubSep + s.Name, Name: s.Name, Kind: asset.ArtSpriteRegion})
		}
	}
	if len(p.items) > n {
		sort.SliceStable(p.items, func(i, j int) bool { return p.items[i].Path < p.items[j].Path })
	}
}

// isCell reports whether id is a cell of a sliced sheet, which file operations skip.
func (p *Panel) isCell(id ids.AssetID) bool {
	_, ok := p.sheetOf[id]
	return ok
}

// fileSelection is the selection without sheet cells, for copy, move, rename and delete.
func (p *Panel) fileSelection() []ids.AssetID {
	out := make([]ids.AssetID, 0, len(p.sel))
	for i := range p.items {
		if id := p.items[i].ID; p.sel[id] && p.items[i].Kind != preview.SourceKind && !p.isCell(id) {
			out = append(out, id)
		}
	}
	return out
}

type drawnTile struct {
	id          ids.AssetID
	tile, arrow mathx.Rect
}

// TileRect is where an asset's tile and, for a sliced sheet, its expand arrow were drawn on the last frame.
func (p *Panel) TileRect(id ids.AssetID) (tile, arrow mathx.Rect, ok bool) {
	for _, d := range p.drawn {
		if d.id == id {
			return d.tile, d.arrow, true
		}
	}
	return mathx.Rect{}, mathx.Rect{}, false
}

// ToggleSheet opens or closes the cells of a sliced sheet in the grid.
func (p *Panel) ToggleSheet(id ids.AssetID) {
	p.sheetOpen[id] = !p.sheetOpen[id]
}

func (p *Panel) prune() {
	alive := make(map[ids.AssetID]bool, len(p.items))
	for i := range p.items {
		alive[p.items[i].ID] = true
	}
	for id := range p.sel {
		if !alive[id] {
			delete(p.sel, id)
		}
	}
	if !alive[p.active] {
		p.active = ids.AssetID{}
	}
}

func (p *Panel) selectOnly(id ids.AssetID) {
	clear(p.sel)
	if !id.IsZero() {
		p.sel[id] = true
	}
	p.active, p.anchor = id, id
}

func (p *Panel) toggle(id ids.AssetID) {
	if p.sel[id] {
		delete(p.sel, id)
		if p.active == id {
			p.active = ids.AssetID{}
		}
	} else {
		p.sel[id] = true
		p.active = id
	}
	p.anchor = id
}

func (p *Panel) viewIndex(id ids.AssetID) int {
	for k, i := range p.view {
		if p.items[i].ID == id {
			return k
		}
	}
	return -1
}

func (p *Panel) selectRange(id ids.AssetID) {
	a, b := p.viewIndex(p.anchor), p.viewIndex(id)
	if a < 0 || b < 0 {
		p.selectOnly(id)
		return
	}
	clear(p.sel)
	for k := min(a, b); k <= max(a, b); k++ {
		p.sel[p.items[p.view[k]].ID] = true
	}
	p.active = id
}

func (p *Panel) press(id ids.AssetID) {
	mods := p.f.UI.Mods()
	p.selDir = ""
	switch {
	case mods.Primary():
		p.toggle(id)
	case mods.Has(input.ModShift):
		p.selectRange(id)
	case p.sel[id] && len(p.sel) > 1:
		p.pending, p.active = id, id
	default:
		p.selectOnly(id)
	}
	// Keep the current Inspector available as a drop target until this press
	// is known to be a click. A texture drag must not replace its material.
	p.inspectPending = true
}

func (p *Panel) inspectActive() {
	if p.d.Inspect == nil {
		return
	}
	var target ids.AssetID
	if a := p.find(p.active); a != nil && len(p.sel) == 1 && (a.Kind == asset.ArtData || p.viewable(a.Kind)) {
		target = a.ID
	}
	p.d.Inspect(target)
}

func (p *Panel) viewable(k asset.ArtifactKind) bool {
	return p.d.Viewable != nil && p.d.Viewable(k)
}

func (p *Panel) settle() {
	if p.f.DnD.Active() {
		p.pending = ids.AssetID{}
		p.inspectPending = false
		return
	}
	if !p.f.UI.MouseDown(input.MouseLeft) {
		if !p.pending.IsZero() {
			p.selectOnly(p.pending)
			p.pending = ids.AssetID{}
		}
		if p.inspectPending {
			p.inspectPending = false
			p.inspectActive()
		}
	}
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	p.f, p.now = f, f.Now
	ui := f.UI
	th := &ui.Theme
	if p.d.Pipeline == nil {
		widgets.LabelDim(ui, i18n.Tr("editor.assets.asset_pipeline_is_not_available", "Asset pipeline is not available"))
		return
	}
	p.assetMenuID = ui.ID("amenu")
	p.dirMenuID = ui.ID("dmenu")
	p.refresh()
	p.settle()
	bar := ui.Cursor().Next(0)
	bw, label := float32(100), i18n.Tr("editor.assets.new_folder", "New Folder")
	if bar.W < 300 {
		bw, label = bar.H, "+"
	}
	sw := float32(0)
	if bar.W >= 260 {
		sw = 3 * (bar.H + 4)
	}
	newID := ui.ID("newdir")
	widgets.SearchField(ui, ui.ID("search"), mathx.Rect{X: bar.X, Y: bar.Y, W: max(0, bar.W-bw-sw-8), H: bar.H}, &p.search)
	if sw > 0 {
		p.sizeButtons(mathx.Rect{X: bar.X + bar.W - bw - sw - 4, Y: bar.Y, W: sw, H: bar.H})
	}
	if widgets.ButtonRect(ui, newID, mathx.Rect{X: bar.X + bar.W - bw, Y: bar.Y, W: bw, H: bar.H}, label) {
		p.newFolder(p.folder)
	}
	widgets.Tooltip(ui, newID, i18n.Tr("editor.assets.new_folder", "New Folder"))
	top := bar.Y + bar.H + th.Spacing
	body := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	if body.W < 260 {
		p.drawGrid(body)
	} else {
		left, right := widgets.Splitter(ui, ui.ID("split"), body, false, &p.split)
		p.drawTree(left)
		p.drawGrid(right)
	}
	p.menus()
}

func (p *Panel) buildRows() {
	p.rows = p.rows[:0]
	for i, d := range p.dirs {
		visible := true
		for parent := path.Dir(d); parent != "." && parent != "/"; parent = path.Dir(parent) {
			if !p.expanded[parent] {
				visible = false
				break
			}
		}
		if !visible {
			continue
		}
		has := false
		for _, other := range p.dirs[i+1:] {
			if strings.HasPrefix(other, d+"/") {
				has = true
				break
			}
		}
		p.rows = append(p.rows, dirRow{path: d, depth: strings.Count(d, "/"), has: has})
	}
}

func sourceFolder(dir string) bool {
	return dir == "." || dir == "native" || strings.HasPrefix(dir, "native/")
}
func dirLabel(dir string) string {
	if dir == "." {
		return i18n.Tr("editor.assets.project_files", "Project files")
	}
	return path.Base(dir)
}

func dirPayload(dir string) dnd.Payload {
	return dnd.Payload{Kind: context.KindAssets, Label: path.Base(dir), Data: context.AssetsPayload{Dirs: []string{dir}}}
}

func (p *Panel) drawTree(r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	p.buildRows()
	widgets.VirtualList(ui, ui.ID("tree"), r, len(p.rows), th.RowHeight, &p.treeScroll, func(i int, rr mathx.Rect) {
		row := p.rows[i]
		if p.renamingDir == row.path {
			p.drawDirRename(row, rr)
			return
		}
		id := core.IDOf("assets.dir/" + row.path)
		ev := widgets.TreeRowRect(ui, id, rr, widgets.TreeRow{
			Depth:       row.depth,
			Label:       dirLabel(row.path),
			HasChildren: row.has,
			Open:        p.expanded[row.path],
			Selected:    p.folder == row.path && p.search == "",
			Focused:     p.f.Focused,
		})
		if ev.Toggled || (ev.DoubleClicked && row.has) {
			p.expanded[row.path] = !p.expanded[row.path]
		}
		if ev.Pressed {
			p.folder = row.path
			p.gridScroll = 0
			p.search = ""
		}
		if ev.RightClicked && !sourceFolder(row.path) {
			p.menuDir = row.path
			widgets.OpenContextMenu(ui, p.dirMenuID)
		}
		if ui.Active() == id && !sourceFolder(row.path) && path.Dir(row.path) != "." {
			p.f.DnD.Source(ui, id, dirPayload(row.path))
		}
		p.dropOnDir(row.path, rr)
	})
}

func (p *Panel) filter() {
	p.view = p.view[:0]
	p.subdirs = p.subdirs[:0]
	q := strings.ToLower(strings.TrimSpace(p.search))
	if q == "" {
		for _, d := range p.dirs {
			if d != p.folder && path.Dir(d) == p.folder {
				p.subdirs = append(p.subdirs, d)
			}
		}
	}
	for i := range p.items {
		a := &p.items[i]
		if q != "" {
			if strings.Contains(strings.ToLower(a.Path), q) {
				p.view = append(p.view, i)
			}
			continue
		}
		if sheet, ok := p.sheetOf[a.ID]; ok && !p.sheetOpen[sheet] {
			continue
		}
		if path.Dir(a.Path) == p.folder {
			p.view = append(p.view, i)
		}
	}
}

func (p *Panel) drawGrid(r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	p.filter()
	p.drawn = p.drawn[:0]
	count := len(p.subdirs) + len(p.view)
	tw := p.tileW()
	cols := max(1, int(r.W/tw))
	tileH := p.tileSize() + th.RowHeight + 12
	rows := (count + cols - 1) / cols
	p.scrollToTile(cols, tileH, r.H)
	p.overDir = false
	widgets.VirtualList(ui, ui.ID("grid"), r, rows, tileH, &p.gridScroll, func(row int, rr mathx.Rect) {
		for c := 0; c < cols; c++ {
			i := row*cols + c
			if i >= count {
				break
			}
			tr := mathx.Rect{X: rr.X + float32(c)*tw + 2, Y: rr.Y + 2, W: tw - 4, H: tileH - 4}
			if i < len(p.subdirs) {
				p.dirTile(p.subdirs[i], tr)
				continue
			}
			p.assetTile(&p.items[p.view[i-len(p.subdirs)]], tr)
		}
	})
	first := max(0, int(p.gridScroll/tileH)-1)
	last := min(rows, int((p.gridScroll+r.H)/tileH)+2)
	for _, row := range []int{first, last - 1} {
		for c := 0; c < cols; c++ {
			idx := row*cols + c - len(p.subdirs)
			if idx >= 0 && idx < len(p.view) {
				p.d.Thumbs.Prefetch(p.items[p.view[idx]])
			}
		}
	}
	if count == 0 {
		lines := widgets.WrapLines(ui, i18n.Tr("editor.assets.drop_files_here", "Drop files here to import"), r.W-2*th.Padding)
		lh := ui.LineHeight()
		y := r.Y + (r.H-float32(len(lines))*lh)/2
		for i, l := range lines {
			ui.TextIn(mathx.Rect{X: r.X, Y: y + float32(i)*lh, W: r.W, H: lh}, l, th.TextDisabled, core.AlignCenter)
		}
	}
	used := mathx.Clamp(float32(rows)*tileH-p.gridScroll, 0, r.H)
	empty := mathx.Rect{X: r.X, Y: r.Y + used, W: r.W, H: r.H - used}
	id := ui.ID("gridbg")
	if empty.H > 0 {
		if ui.Pressed(id, empty) && !ui.Primary() && !ui.Mods().Has(input.ModShift) {
			p.selectOnly(ids.AssetID{})
			p.selDir = ""
		}
		if ui.RightClicked(id, empty) {
			p.menuDir = p.folder
			widgets.OpenContextMenu(ui, p.dirMenuID)
		}
	}
	if !p.overDir {
		p.dropOnDir(p.folder, r)
	}
}

func fitRect(box mathx.Rect, w, h uint16) mathx.Rect {
	if w == 0 || h == 0 {
		return box
	}
	s := min(box.W/float32(w), box.H/float32(h))
	fw, fh := float32(w)*s, float32(h)*s
	return mathx.Rect{X: box.X + (box.W-fw)/2, Y: box.Y + (box.H-fh)/2, W: fw, H: fh}
}

func assetTileID(id ids.AssetID) core.ID {
	return core.IDSeq(core.ID(id.Hi), int(uint32(id.Lo)))
}

func (p *Panel) payload(a *pipeline.AssetInfo) dnd.Payload {
	if !p.sel[a.ID] || len(p.sel) < 2 {
		return dnd.Payload{
			Kind:  context.KindAsset,
			Label: path.Base(a.Path),
			Data:  context.AssetPayload{ID: a.ID, Kind: a.Kind, Name: a.Name},
		}
	}
	items := make([]context.AssetPayload, 0, len(p.sel))
	for i := range p.items {
		if it := &p.items[i]; p.sel[it.ID] {
			items = append(items, context.AssetPayload{ID: it.ID, Kind: it.Kind, Name: it.Name})
		}
	}
	return dnd.Payload{Kind: context.KindAssets, Label: i18n.Tr("editor.assets.items_assets", "{items} assets", i18n.Str("items", strconv.Itoa(len(items)))), Data: context.AssetsPayload{Items: items}}
}

func (p *Panel) assetTile(a *pipeline.AssetInfo, r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	id := assetTileID(a.ID)
	cell := p.isCell(a.ID)
	ts := p.tileSize()
	img := mathx.Rect{X: r.X + (r.W-ts)/2, Y: r.Y + 4, W: ts, H: ts}
	var arrow mathx.Rect
	if p.cellsOf[a.ID] > 0 {
		arrow = mathx.Rect{X: r.X + r.W - 18, Y: img.Y + img.H/2 - 8, W: 16, H: 16}
		if arrow.Contains(ui.Mouse()) && ui.MouseOver(id, r) && ui.MousePressed(input.MouseLeft) && !ui.Mods().Primary() {
			p.ToggleSheet(a.ID)
			arrow.W = -arrow.W
		}
	}
	p.drawn = append(p.drawn, drawnTile{id: a.ID, tile: r, arrow: mathx.Rect{X: arrow.X, Y: arrow.Y, W: mathx.Abs(arrow.W), H: arrow.H}})
	if arrow.W >= 0 && ui.Pressed(id, r) {
		p.press(a.ID)
		if ui.DoubleClicked(id) {
			p.selectOnly(a.ID)
			p.open(a)
		}
	}
	if ui.RightClicked(id, r) {
		if !p.sel[a.ID] {
			p.selectOnly(a.ID)
		}
		p.active = a.ID
		if !cell {
			widgets.OpenContextMenu(ui, p.assetMenuID)
		}
	}
	if ui.Active() == id && a.Kind != preview.SourceKind {
		p.f.DnD.Source(ui, id, p.payload(a))
		if p.f.DnD.Active() {
			p.inspectPending = false
		}
	}
	switch {
	case p.sel[a.ID]:
		ui.Rect(r, th.Selection)
	case ui.Hot() == id:
		ui.Rect(r, th.WidgetHot)
	case cell:
		ui.RoundRect(r, th.Radius.Field, th.CardBg)
	}
	reg, ok := p.d.Thumbs.Get(*a)
	if ok && ui.Hot() == id {
		if frame, found := p.d.Thumbs.HoverFrame(a.ID, p.now); found {
			if pic, ready := p.d.Thumbs.ByID(frame); ready {
				reg = pic
			}
		}
	}
	switch {
	case ok:
		if preview.Image(a.Kind) || a.Kind == asset.ArtPrefab || a.Kind == asset.ArtMaterial || a.Kind == asset.ArtShader || a.Kind == asset.ArtAnim {
			p.checker(img)
		}
		ui.List().Texture(reg, fitRect(img, reg.W, reg.H), mathx.White)
	default:
		p.schematic(a, img)
	}
	if arrow.W != 0 {
		icon := "ui.chevron_right"
		if p.sheetOpen[a.ID] {
			icon = "ui.chevron_down"
		}
		ar := arrow
		ar.W = 16
		ui.RoundRect(ar, 3, th.Panel)
		widgets.IconAt(ui, ar, icon, th.Text)
	}
	startup := p.d.Project != nil && a.ID == p.d.Project.MainScene
	if startup {
		widgets.IconAt(ui, mathx.Rect{X: img.X + img.W - 16, Y: img.Y, W: 16, H: 16}, "play.start", th.Accent)
	}
	nameR := mathx.Rect{X: r.X + 2, Y: img.Y + img.H + 2, W: r.W - 4, H: th.RowHeight}
	if p.renaming == a.ID {
		p.drawAssetRename(a, nameR)
	} else {
		ui.PushClip(nameR)
		name := assetops.Stem(a.Path)
		if cell {
			name = a.Name
		}
		ui.TextIn(nameR, widgets.Elide(ui, name, nameR.W), th.Text, core.AlignCenter)
		ui.PopClip()
	}
	tip := a.Path
	if role := p.d.Thumbs.Role(a.ID); role != "" && role != asset.RoleColor {
		tip += "\n" + string(role)
		ui.PushFont(ui.MonoFontAt(10))
		tag := widgets.Elide(ui, string(role), img.W-3)
		br := mathx.Rect{X: img.X, Y: img.Y + img.H - ui.LineHeight(), W: min(ui.TextSize(tag).X+3, img.W), H: ui.LineHeight()}
		ui.Rect(br, th.Panel)
		ui.TextIn(br, tag, th.Text, core.AlignCenter)
		ui.PopFont()
	}
	if reason := p.d.Thumbs.Reason(a.ID); reason != "" {
		tip += "\n" + reason
		widgets.IconAt(ui, mathx.Rect{X: img.X + 1, Y: img.Y + 1, W: 16, H: 16}, "status.warning", th.Warn)
	}
	if startup {
		tip += " · startup scene"
	}
	if a.Error != "" {
		ui.List().RectOutline(r, th.Error, 1)
		tip = a.Error
	}
	widgets.Tooltip(ui, id, tip)
}

func (p *Panel) enter(dir string) {
	p.folder = dir
	p.expanded[path.Dir(dir)] = true
	p.gridScroll = 0
	p.selDir = ""
}

func (p *Panel) dirTile(dir string, r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	id := core.IDOf("assets.tile/" + dir)
	if ui.Pressed(id, r) {
		p.selDir = dir
		if ui.DoubleClicked(id) {
			p.enter(dir)
		}
	}
	if ui.RightClicked(id, r) {
		p.menuDir = dir
		widgets.OpenContextMenu(ui, p.dirMenuID)
	}
	if ui.Active() == id && path.Dir(dir) != "." {
		p.f.DnD.Source(ui, id, dirPayload(dir))
	}
	switch {
	case p.selDir == dir:
		ui.Rect(r, th.Selection)
	case ui.Hot() == id:
		ui.Rect(r, th.WidgetHot)
	}
	ts := p.tileSize()
	img := mathx.Rect{X: r.X + (r.W-ts)/2, Y: r.Y + 4, W: ts, H: ts}
	p.folderStack(dir, img)
	nameR := mathx.Rect{X: r.X + 2, Y: img.Y + img.H + 2, W: r.W - 4, H: th.RowHeight}
	ui.PushClip(nameR)
	ui.TextIn(nameR, widgets.Elide(ui, path.Base(dir), nameR.W), th.Text, core.AlignCenter)
	ui.PopClip()
	if p.f.DnD.Hovering(ui, r) {
		p.overDir = true
	}
	p.dropOnDir(dir, r)
}

func (p *Panel) accepts(pl dnd.Payload, dir string) bool {
	switch pl.Kind {
	case context.KindFiles:
		return true
	case context.KindAsset:
		ap, ok := pl.Data.(context.AssetPayload)
		return !ok || !p.isCell(ap.ID)
	case context.KindNodes:
		return p.d.Prefabs != nil
	case context.KindAssets:
		ap, ok := pl.Data.(context.AssetsPayload)
		if !ok {
			return false
		}
		for _, it := range ap.Items {
			if p.isCell(it.ID) {
				return false
			}
		}
		for _, d := range ap.Dirs {
			if d == dir || assetops.Under(dir, d) || path.Dir(d) == dir {
				return false
			}
		}
		return true
	}
	return false
}

func (p *Panel) dropOnDir(dir string, r mathx.Rect) {
	if sourceFolder(dir) {
		return
	}
	ui := p.f.UI
	dn := p.f.DnD
	if !dn.Hovering(ui, r) {
		return
	}
	pl, _ := dn.Payload()
	if !p.accepts(pl, dir) {
		dn.Target(ui, r, "reject")
		return
	}
	got, ok := dn.Target(ui, r, context.KindAsset, context.KindAssets, context.KindFiles, context.KindNodes)
	if !ok {
		return
	}
	switch got.Kind {
	case context.KindFiles:
		files, _ := got.Data.([]string)
		p.importFiles(files, dir)
	case context.KindAsset:
		ap, _ := got.Data.(context.AssetPayload)
		p.moveInto([]context.AssetPayload{ap}, nil, dir)
	case context.KindAssets:
		as, _ := got.Data.(context.AssetsPayload)
		p.moveInto(as.Items, as.Dirs, dir)
	case context.KindNodes:
		np, _ := got.Data.(context.NodesPayload)
		p.report(p.d.Prefabs.CreateFrom(p.d.Docs.Documents().Get(np.Doc), np.Nodes, dir))
		p.invalidate()
	}
}

func movedWith(src string, dirs []string) bool {
	for _, d := range dirs {
		if assetops.Under(src, d) {
			return true
		}
	}
	return false
}

func (p *Panel) moveInto(items []context.AssetPayload, dirs []string, target string) {
	var ops []assetops.FileOp
	for _, d := range dirs {
		dst := path.Join(target, path.Base(d))
		if dst == d || d == target || assetops.Under(target, d) {
			continue
		}
		ops = append(ops, assetops.FileOp{Dir: true, From: d, To: dst})
	}
	for _, it := range items {
		src, ok := p.d.Pipeline.PathOf(it.ID)
		if !ok || movedWith(src, dirs) {
			continue
		}
		if dst := path.Join(target, path.Base(src)); dst != src {
			ops = append(ops, assetops.FileOp{ID: it.ID, From: src, To: dst})
		}
	}
	if len(ops) == 0 {
		return
	}
	err := p.d.Files.Apply(ops)
	for _, op := range ops {
		if op.Dir && !p.d.Files.Exists(op.From) && (p.folder == op.From || assetops.Under(p.folder, op.From)) {
			p.folder = op.To + strings.TrimPrefix(p.folder, op.From)
		}
	}
	if err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
	}
	p.invalidate()
}

func (p *Panel) importFiles(files []string, dir string) {
	created, err := p.d.Files.ImportExternal(files, dir)
	if err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
		return
	}
	p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.assets.imported_created_file_s_into", "Imported {created} file(s) into {dir}", i18n.Str("created", strconv.Itoa(len(created))), i18n.Str("dir", dir)))
	if len(created) > 0 {
		p.selectOnly(created[0])
		p.folder = dir
	}
	p.invalidate()
}

func (p *Panel) open(a *pipeline.AssetInfo) {
	if p.d.Nav != nil {
		p.d.Nav.Open(context.AssetDocumentAt(a.Path))
	}
}

func (p *Panel) Reveal(rel string) {
	if p.d.Pipeline == nil {
		return
	}
	p.refresh()
	p.search = ""
	p.gridScroll = 0
	folder := path.Dir(rel)
	for i := range p.items {
		if p.items[i].Path == rel {
			p.selectOnly(p.items[i].ID)
			break
		}
	}
	for _, d := range p.dirs {
		if d == rel {
			folder = rel
			break
		}
	}
	p.folder = folder
	for d := folder; d != "." && d != "/"; d = path.Dir(d) {
		p.expanded[d] = true
	}
}

func (p *Panel) StartRename() {
	if sourceFolder(p.selDir) {
		return
	}
	if a := p.find(p.active); a != nil && a.Kind == preview.SourceKind {
		return
	}
	if len(p.sel) == 1 && p.sel[p.active] && !p.isCell(p.active) {
		p.renaming, p.renameStart = p.active, true
	}
}

func (p *Panel) drawAssetRename(a *pipeline.AssetInfo, r mathx.Rect) {
	ui := p.f.UI
	fid := ui.ID("arename")
	ext := path.Ext(a.Path)
	if p.renameStart {
		p.renameStart = false
		p.renameText = assetops.Stem(a.Path)
		ui.SetFocus(fid)
		ui.EditBuffer(fid, p.renameText).SelectAll()
	}
	if _, committed := widgets.TextFieldRect(ui, fid, r, &p.renameText, false); committed {
		name := strings.TrimSpace(p.renameText)
		if assetops.ValidName(name) {
			if !strings.HasSuffix(name, ext) {
				name += ext
			}
			if dst := path.Join(path.Dir(a.Path), name); dst != a.Path {
				if err := p.d.Files.Apply([]assetops.FileOp{{ID: a.ID, From: a.Path, To: dst}}); err != nil {
					p.d.Notices.Notify(context.LevelError, err.Error())
				}
				p.invalidate()
			}
		}
	}
	if ui.Focus() != fid {
		p.renaming = ids.AssetID{}
	}
}

func (p *Panel) drawDirRename(row dirRow, rr mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	fid := ui.ID("drename")
	x := rr.X + float32(row.depth)*th.Indent + th.Indent + 2
	fr := mathx.Rect{X: x, Y: rr.Y + 1, W: max(40, rr.X+rr.W-x-2), H: rr.H - 2}
	if p.renameDirStart {
		p.renameDirStart = false
		p.renameDirText = path.Base(row.path)
		ui.SetFocus(fid)
		ui.EditBuffer(fid, p.renameDirText).SelectAll()
	}
	if _, committed := widgets.TextFieldRect(ui, fid, fr, &p.renameDirText, false); committed {
		name := strings.TrimSpace(p.renameDirText)
		dst := path.Join(path.Dir(row.path), name)
		if assetops.ValidName(name) && dst != row.path {
			if err := p.d.Files.Apply([]assetops.FileOp{{Dir: true, From: row.path, To: dst}}); err != nil {
				p.d.Notices.Notify(context.LevelError, err.Error())
			} else {
				if p.folder == row.path || assetops.Under(p.folder, row.path) {
					p.folder = dst + strings.TrimPrefix(p.folder, row.path)
				}
				p.expanded[dst] = p.expanded[row.path]
			}
			p.invalidate()
		}
	}
	if ui.Focus() != fid {
		p.renamingDir = ""
	}
}

func (p *Panel) newFolder(parent string) {
	dir := p.d.Pipeline.UniqueDir(path.Join(parent, "New Folder"))
	if err := p.d.Pipeline.Mkdir(dir); err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
		return
	}
	p.expanded[parent] = true
	p.renamingDir, p.renameDirStart = dir, true
	p.invalidate()
}

func reveal(abs string, isDir bool) error {
	switch runtime.GOOS {
	case "darwin":
		if isDir {
			return exec.Command("open", abs).Start()
		}
		return exec.Command("open", "-R", abs).Start()
	case "windows":
		if isDir {
			return exec.Command("explorer", abs).Start()
		}
		return exec.Command("explorer", "/select,", abs).Start()
	}
	if !isDir {
		abs = path.Dir(abs)
	}
	return exec.Command("xdg-open", abs).Start()
}

// Folder is the folder the grid shows.
func (p *Panel) Folder() string {
	return p.folder
}

// SetFolder shows a folder and unfolds its parents in the tree.
func (p *Panel) SetFolder(dir string) {
	if dir == "" || dir == p.folder {
		return
	}
	for d := path.Dir(dir); d != "." && d != "/"; d = path.Dir(d) {
		p.expanded[d] = true
	}
	p.enter(dir)
}
