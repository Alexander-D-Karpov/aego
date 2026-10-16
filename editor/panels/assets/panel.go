package assets

import (
	"os/exec"
	"path"
	"runtime"
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
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/platform/input"
	"aego/project"
	"aego/ui/core"
	"aego/ui/dnd"
	"aego/ui/widgets"
)

const tileW = 88

const (
	CommandRename    = "assets.rename"
	CommandDelete    = "assets.delete"
	CommandDuplicate = "assets.duplicate"
	CommandUndo      = "assets.undo"
	CommandRedo      = "assets.redo"
	CommandCopy      = "assets.copy"
	CommandCut       = "assets.cut"
	CommandPaste     = "assets.paste"
)

type dirRow struct {
	path  string
	depth int
	has   bool
}

type Deps struct {
	Files     *assetfiles.Files
	Pipeline  *pipeline.Pipeline
	Thumbs    *Thumbs
	Docs      services.Documents
	Selection services.Selection
	Scenes    services.SceneOps
	Prefabs   services.Prefabs
	Commands  *command.Registry
	Notices   services.Notices
	Nav       services.Navigation
	Project   *project.Project
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
}

func New(d Deps) *Panel {
	return &Panel{
		d:        d,
		folder:   "assets",
		expanded: map[string]bool{"assets": true, "scenes": true, "scripts": true, "prefabs": true},
		sel:      map[ids.AssetID]bool{},
		split:    0.28,
		rev:      ^uint64(0),
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
	return "Assets"
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
	if rev := pl.Revision(); rev != p.rev {
		p.rev = rev
		p.items = pl.Assets()
		p.dirs = pl.Dirs()
		p.dirsAt = p.now
		p.prune()
	} else if p.now-p.dirsAt > 2 {
		p.dirs = pl.Dirs()
		p.dirsAt = p.now
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
	}
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
}

func (p *Panel) settle() {
	if p.pending.IsZero() {
		return
	}
	switch {
	case p.f.DnD.Active():
		p.pending = ids.AssetID{}
	case !p.f.UI.MouseDown(input.MouseLeft):
		p.selectOnly(p.pending)
		p.pending = ids.AssetID{}
	}
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	p.f, p.now = f, f.Now
	ui := f.UI
	th := &ui.Theme
	if p.d.Pipeline == nil {
		widgets.LabelDim(ui, "Asset pipeline is not available")
		return
	}
	p.assetMenuID = ui.ID("amenu")
	p.dirMenuID = ui.ID("dmenu")
	p.refresh()
	p.settle()
	bar := ui.Cursor().Next(0)
	bw, label := float32(100), "New Folder"
	if bar.W < 300 {
		bw, label = bar.H, "+"
	}
	newID := ui.ID("newdir")
	widgets.SearchField(ui, ui.ID("search"), mathx.Rect{X: bar.X, Y: bar.Y, W: max(0, bar.W-bw-4), H: bar.H}, &p.search)
	if widgets.ButtonRect(ui, newID, mathx.Rect{X: bar.X + bar.W - bw, Y: bar.Y, W: bw, H: bar.H}, label) {
		p.newFolder(p.folder)
	}
	widgets.Tooltip(ui, newID, "New Folder")
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
			Label:       path.Base(row.path),
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
		if ev.RightClicked {
			p.menuDir = row.path
			widgets.OpenContextMenu(ui, p.dirMenuID)
		}
		if ui.Active() == id && path.Dir(row.path) != "." {
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
			if path.Dir(d) == p.folder {
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
		if path.Dir(a.Path) == p.folder {
			p.view = append(p.view, i)
		}
	}
}

func (p *Panel) drawGrid(r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	p.filter()
	count := len(p.subdirs) + len(p.view)
	cols := max(1, int(r.W/tileW))
	tileH := float32(ThumbSize) + th.RowHeight + 12
	rows := (count + cols - 1) / cols
	p.scrollToTile(cols, tileH, r.H)
	p.overDir = false
	widgets.VirtualList(ui, ui.ID("grid"), r, rows, tileH, &p.gridScroll, func(row int, rr mathx.Rect) {
		for c := 0; c < cols; c++ {
			i := row*cols + c
			if i >= count {
				break
			}
			tr := mathx.Rect{X: rr.X + float32(c)*tileW + 2, Y: rr.Y + 2, W: tileW - 4, H: tileH - 4}
			if i < len(p.subdirs) {
				p.dirTile(p.subdirs[i], tr)
				continue
			}
			p.assetTile(&p.items[p.view[i-len(p.subdirs)]], tr)
		}
	})
	if count == 0 {
		ui.TextIn(r, "Drop files here to import", th.TextDisabled, core.AlignCenter)
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

func iconRect(img mathx.Rect, pad float32) mathx.Rect {
	return mathx.Rect{X: img.X + pad, Y: img.Y + pad, W: img.W - 2*pad, H: img.H - 2*pad}
}

func kindLabel(a *pipeline.AssetInfo) string {
	if a.Kind == asset.ArtScene {
		return "SCENE"
	}
	return strings.ToUpper(strings.TrimPrefix(path.Ext(a.Path), "."))
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
	return dnd.Payload{Kind: context.KindAssets, Label: strconv.Itoa(len(items)) + " assets", Data: context.AssetsPayload{Items: items}}
}

func (p *Panel) assetTile(a *pipeline.AssetInfo, r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	id := assetTileID(a.ID)
	if ui.Pressed(id, r) {
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
		widgets.OpenContextMenu(ui, p.assetMenuID)
	}
	if ui.Active() == id {
		p.f.DnD.Source(ui, id, p.payload(a))
	}
	switch {
	case p.sel[a.ID]:
		ui.Rect(r, th.Selection)
	case ui.Hot() == id:
		ui.Rect(r, th.WidgetHot)
	}
	img := mathx.Rect{X: r.X + (r.W-ThumbSize)/2, Y: r.Y + 4, W: ThumbSize, H: ThumbSize}
	if reg, ok := p.d.Thumbs.Get(*a); ok {
		ui.List().Texture(reg, fitRect(img, reg.W, reg.H), mathx.Color{R: 1, G: 1, B: 1, A: 1})
	} else {
		ui.Rect(img, th.Widget)
		if a.Kind == asset.ArtScript {
			widgets.IconAt(ui, iconRect(img, 10), "file.script", th.Text)
		} else {
			ui.TextIn(img, kindLabel(a), th.TextDim, core.AlignCenter)
		}
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
		ui.TextIn(nameR, assetops.Stem(a.Path), th.Text, core.AlignCenter)
		ui.PopClip()
	}
	tip := a.Path
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
	img := mathx.Rect{X: r.X + (r.W-ThumbSize)/2, Y: r.Y + 4, W: ThumbSize, H: ThumbSize}
	widgets.IconAt(ui, iconRect(img, 6), "file.folder", th.Accent)
	nameR := mathx.Rect{X: r.X + 2, Y: img.Y + img.H + 2, W: r.W - 4, H: th.RowHeight}
	ui.PushClip(nameR)
	ui.TextIn(nameR, path.Base(dir), th.Text, core.AlignCenter)
	ui.PopClip()
	if p.f.DnD.Hovering(ui, r) {
		p.overDir = true
	}
	p.dropOnDir(dir, r)
}

func (p *Panel) accepts(pl dnd.Payload, dir string) bool {
	switch pl.Kind {
	case context.KindFiles, context.KindAsset:
		return true
	case context.KindNodes:
		return p.d.Prefabs != nil
	case context.KindAssets:
		ap, ok := pl.Data.(context.AssetsPayload)
		if !ok {
			return false
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
	p.d.Notices.Notify(context.LevelInfo, "Imported "+strconv.Itoa(len(created))+" file(s) into "+dir)
	if len(created) > 0 {
		p.selectOnly(created[0])
		p.folder = dir
	}
	p.invalidate()
}

func openExternal(abs string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", abs).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", abs).Start()
	}
	return exec.Command("xdg-open", abs).Start()
}

func openable(k asset.ArtifactKind) bool {
	return k == asset.ArtScene || k == asset.ArtScript || k == asset.ArtPrefab
}

func (p *Panel) open(a *pipeline.AssetInfo) {
	switch a.Kind {
	case asset.ArtScene, asset.ArtPrefab:
		if _, err := p.d.Docs.Documents().Open(a.Path); err != nil {
			p.d.Notices.Notify(context.LevelError, err.Error())
		}
	case asset.ArtScript:
		if p.d.Files.ScriptEditor() {
			p.d.Nav.Open(context.At(a.Path, 0, 1))
			return
		}
		if err := openExternal(p.d.Pipeline.Abs(a.Path)); err != nil {
			p.d.Notices.Notify(context.LevelError, err.Error())
		}
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
	if len(p.sel) == 1 && p.sel[p.active] {
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
