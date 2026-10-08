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
	"aego/editor/command"
	"aego/editor/context"
	"aego/editor/workspace"
	"aego/ui/core"
	"aego/ui/dnd"
	"aego/ui/widgets"
)

const tileW = 88

type dirRow struct {
	path  string
	depth int
	has   bool
}

type Panel struct {
	thumbs         *Thumbs
	folder         string
	expanded       map[string]bool
	dirs           []string
	rows           []dirRow
	items          []pipeline.AssetInfo
	view           []int
	subdirs        []string
	rev            uint64
	dirsAt         float64
	selected       ids.AssetID
	search         string
	split          float32
	treeScroll     float32
	gridScroll     float32
	renaming       ids.AssetID
	renameStart    bool
	renameText     string
	renamingDir    string
	renameDirStart bool
	renameDirText  string
	menuAsset      ids.AssetID
	menuDir        string
	assetMenuID    core.ID
	dirMenuID      core.ID
}

func New(t *Thumbs) *Panel {
	return &Panel{
		thumbs:   t,
		folder:   "assets",
		expanded: map[string]bool{"assets": true, "scenes": true},
		split:    0.28,
		rev:      ^uint64(0),
	}
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
	return p.selected
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

func (p *Panel) refresh(ctx *context.Editor) {
	pl := ctx.Pipeline
	now := ctx.UI.Time
	if rev := pl.Revision(); rev != p.rev {
		p.rev = rev
		p.items = pl.Assets()
		p.dirs = pl.Dirs()
		p.dirsAt = now
	} else if now-p.dirsAt > 2 {
		p.dirs = pl.Dirs()
		p.dirsAt = now
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

func (p *Panel) Draw(ctx *context.Editor, r mathx.Rect) {
	ui := ctx.UI
	th := &ui.Theme
	if ctx.Pipeline == nil {
		widgets.LabelDim(ui, "Asset pipeline is not available")
		return
	}
	p.assetMenuID = ui.ID("amenu")
	p.dirMenuID = ui.ID("dmenu")
	p.refresh(ctx)
	bar := ui.Cursor().Next(0)
	bw, label := float32(100), "New Folder"
	if bar.W < 300 {
		bw, label = bar.H, "+"
	}
	newID := ui.ID("newdir")
	widgets.SearchField(ui, ui.ID("search"), mathx.Rect{X: bar.X, Y: bar.Y, W: max(0, bar.W-bw-4), H: bar.H}, &p.search)
	if widgets.ButtonRect(ui, newID, mathx.Rect{X: bar.X + bar.W - bw, Y: bar.Y, W: bw, H: bar.H}, label) {
		p.newFolder(ctx, p.folder)
	}
	widgets.Tooltip(ui, newID, "New Folder")
	top := bar.Y + bar.H + th.Spacing
	body := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	if body.W < 260 {
		p.drawGrid(ctx, body)
	} else {
		left, right := widgets.Splitter(ui, ui.ID("split"), body, false, &p.split)
		p.drawTree(ctx, left)
		p.drawGrid(ctx, right)
	}
	p.menus(ctx)
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

func (p *Panel) drawTree(ctx *context.Editor, r mathx.Rect) {
	ui := ctx.UI
	th := &ui.Theme
	p.buildRows()
	widgets.VirtualList(ui, ui.ID("tree"), r, len(p.rows), th.RowHeight, &p.treeScroll, func(i int, rr mathx.Rect) {
		row := p.rows[i]
		if p.renamingDir == row.path {
			p.drawDirRename(ctx, row, rr)
			return
		}
		id := core.IDOf("assets.dir/" + row.path)
		ev := widgets.TreeRowRect(ui, id, rr, widgets.TreeRow{
			Depth:       row.depth,
			Label:       path.Base(row.path),
			HasChildren: row.has,
			Open:        p.expanded[row.path],
			Selected:    p.folder == row.path && p.search == "",
			Focused:     ctx.FocusedPanel == workspace.Assets,
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
			p.menuAsset = ids.AssetID{}
			widgets.OpenContextMenu(ui, p.dirMenuID)
		}
		p.dropOnDir(ctx, row.path, rr)
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

func (p *Panel) drawGrid(ctx *context.Editor, r mathx.Rect) {
	ui := ctx.UI
	th := &ui.Theme
	p.filter()
	count := len(p.subdirs) + len(p.view)
	cols := max(1, int(r.W/tileW))
	tileH := float32(ThumbSize) + th.RowHeight + 12
	rows := (count + cols - 1) / cols
	widgets.VirtualList(ui, ui.ID("grid"), r, rows, tileH, &p.gridScroll, func(row int, rr mathx.Rect) {
		for c := 0; c < cols; c++ {
			i := row*cols + c
			if i >= count {
				break
			}
			tr := mathx.Rect{X: rr.X + float32(c)*tileW + 2, Y: rr.Y + 2, W: tileW - 4, H: tileH - 4}
			if i < len(p.subdirs) {
				p.dirTile(ctx, p.subdirs[i], tr)
				continue
			}
			p.assetTile(ctx, &p.items[p.view[i-len(p.subdirs)]], tr)
		}
	})
	if count == 0 {
		ui.TextIn(r, "Drop files here to import", th.TextDisabled, core.AlignCenter)
	}
	id := ui.ID("gridbg")
	if ui.RightClicked(id, r) {
		p.menuDir = p.folder
		p.menuAsset = ids.AssetID{}
		widgets.OpenContextMenu(ui, p.dirMenuID)
	}
	if ctx.DnD.Hovering(ui, r) {
		if pl, _ := ctx.DnD.Payload(); pl.Kind == context.KindFiles {
			if got, ok := ctx.DnD.Target(ui, r, context.KindFiles); ok {
				files, _ := got.Data.([]string)
				p.importFiles(ctx, files, p.folder)
			}
		}
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

func (p *Panel) assetTile(ctx *context.Editor, a *pipeline.AssetInfo, r mathx.Rect) {
	ui := ctx.UI
	th := &ui.Theme
	id := assetTileID(a.ID)
	if ui.Pressed(id, r) {
		p.selected = a.ID
		if ui.DoubleClicked(id) {
			p.open(ctx, a)
		}
	}
	if ui.RightClicked(id, r) {
		p.selected = a.ID
		p.menuAsset = a.ID
		widgets.OpenContextMenu(ui, p.assetMenuID)
	}
	if ui.Active() == id {
		ctx.DnD.Source(ui, id, dnd.Payload{
			Kind:  context.KindAsset,
			Label: path.Base(a.Path),
			Data:  context.AssetPayload{ID: a.ID, Kind: a.Kind, Name: a.Name},
		})
	}
	switch {
	case p.selected == a.ID:
		ui.Rect(r, th.Selection)
	case ui.Hot() == id:
		ui.Rect(r, th.WidgetHot)
	}
	img := mathx.Rect{X: r.X + (r.W-ThumbSize)/2, Y: r.Y + 4, W: ThumbSize, H: ThumbSize}
	if reg, ok := p.thumbs.Get(*a); ok {
		ui.List().Texture(reg, fitRect(img, reg.W, reg.H), mathx.Color{R: 1, G: 1, B: 1, A: 1})
	} else {
		ui.Rect(img, th.Widget)
		if a.Kind == asset.ArtScript {
			widgets.IconAt(ui, iconRect(img, 10), "file.script", th.Text)
		} else {
			ui.TextIn(img, kindLabel(a), th.TextDim, core.AlignCenter)
		}
	}
	nameR := mathx.Rect{X: r.X + 2, Y: img.Y + img.H + 2, W: r.W - 4, H: th.RowHeight}
	if p.renaming == a.ID {
		p.drawAssetRename(ctx, a, nameR)
	} else {
		ui.PushClip(nameR)
		ui.TextIn(nameR, strings.TrimSuffix(path.Base(a.Path), path.Ext(a.Path)), th.Text, core.AlignCenter)
		ui.PopClip()
	}
	if a.Error != "" {
		ui.List().RectOutline(r, th.Error, 1)
		widgets.Tooltip(ui, id, a.Error)
	} else {
		widgets.Tooltip(ui, id, a.Path)
	}
}

func (p *Panel) dirTile(ctx *context.Editor, dir string, r mathx.Rect) {
	ui := ctx.UI
	th := &ui.Theme
	id := core.IDOf("assets.tile/" + dir)
	if ui.Pressed(id, r) && ui.DoubleClicked(id) {
		p.folder = dir
		p.expanded[path.Dir(dir)] = true
		p.gridScroll = 0
	}
	if ui.RightClicked(id, r) {
		p.menuDir = dir
		p.menuAsset = ids.AssetID{}
		widgets.OpenContextMenu(ui, p.dirMenuID)
	}
	if ui.Hot() == id {
		ui.Rect(r, th.WidgetHot)
	}
	img := mathx.Rect{X: r.X + (r.W-ThumbSize)/2, Y: r.Y + 4, W: ThumbSize, H: ThumbSize}
	widgets.IconAt(ui, iconRect(img, 6), "file.folder", th.Accent)
	nameR := mathx.Rect{X: r.X + 2, Y: img.Y + img.H + 2, W: r.W - 4, H: th.RowHeight}
	ui.PushClip(nameR)
	ui.TextIn(nameR, path.Base(dir), th.Text, core.AlignCenter)
	ui.PopClip()
	p.dropOnDir(ctx, dir, r)
}

func (p *Panel) dropOnDir(ctx *context.Editor, dir string, r mathx.Rect) {
	ui := ctx.UI
	dn := ctx.DnD
	if !dn.Hovering(ui, r) {
		return
	}
	pl, _ := dn.Payload()
	switch pl.Kind {
	case context.KindAsset:
		if got, ok := dn.Target(ui, r, context.KindAsset); ok {
			p.moveInto(ctx, got.Data.(context.AssetPayload).ID, dir)
		}
	case context.KindFiles:
		if got, ok := dn.Target(ui, r, context.KindFiles); ok {
			files, _ := got.Data.([]string)
			p.importFiles(ctx, files, dir)
		}
	default:
		dn.Target(ui, r, "reject")
	}
}

func (p *Panel) moveInto(ctx *context.Editor, id ids.AssetID, dir string) {
	a := p.find(id)
	if a == nil {
		return
	}
	dst := path.Join(dir, path.Base(a.Path))
	if dst == a.Path {
		return
	}
	old := a.Path
	if err := ctx.Pipeline.Move(id, dst); err != nil {
		ctx.Notify(context.LevelError, err.Error())
		return
	}
	ctx.Docs.Renamed(old, dst)
	p.invalidate()
}

func (p *Panel) importFiles(ctx *context.Editor, files []string, dir string) {
	created, batch, err := ctx.Pipeline.ImportExternal(files, dir)
	ctx.ApplyBatch(batch)
	if err != nil {
		ctx.Notify(context.LevelError, err.Error())
		return
	}
	ctx.Notify(context.LevelInfo, "Imported "+strconv.Itoa(len(created))+" file(s) into "+dir)
	if len(created) > 0 {
		p.selected = created[0]
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

func (p *Panel) open(ctx *context.Editor, a *pipeline.AssetInfo) {
	switch a.Kind {
	case asset.ArtScene:
		if _, err := ctx.Docs.Open(a.Path); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
	case asset.ArtScript:
		if err := openExternal(ctx.Pipeline.Abs(a.Path)); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
	}
}

func (p *Panel) Reveal(ctx *context.Editor, rel string) {
	if ctx.Pipeline == nil {
		return
	}
	p.refresh(ctx)
	p.search = ""
	p.gridScroll = 0
	folder := path.Dir(rel)
	for i := range p.items {
		if p.items[i].Path == rel {
			p.selected = p.items[i].ID
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
	if !p.selected.IsZero() {
		p.renaming = p.selected
		p.renameStart = true
	}
}

func (p *Panel) drawAssetRename(ctx *context.Editor, a *pipeline.AssetInfo, r mathx.Rect) {
	ui := ctx.UI
	fid := ui.ID("arename")
	ext := path.Ext(a.Path)
	if p.renameStart {
		p.renameStart = false
		p.renameText = strings.TrimSuffix(path.Base(a.Path), ext)
		ui.SetFocus(fid)
		ui.EditBuffer(fid, p.renameText).SelectAll()
	}
	if _, committed := widgets.TextFieldRect(ui, fid, r, &p.renameText, false); committed {
		name := strings.TrimSpace(p.renameText)
		if name != "" && !strings.ContainsAny(name, "/\\") {
			if !strings.HasSuffix(name, ext) {
				name += ext
			}
			dst := path.Join(path.Dir(a.Path), name)
			old := a.Path
			if err := ctx.Pipeline.Move(a.ID, dst); err != nil {
				ctx.Notify(context.LevelError, err.Error())
			} else {
				ctx.Docs.Renamed(old, dst)
				p.invalidate()
			}
		}
	}
	if ui.Focus() != fid {
		p.renaming = ids.AssetID{}
	}
}

func (p *Panel) drawDirRename(ctx *context.Editor, row dirRow, rr mathx.Rect) {
	ui := ctx.UI
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
		if name != "" && !strings.ContainsAny(name, "/\\") {
			dst := path.Join(path.Dir(row.path), name)
			if err := ctx.Pipeline.MoveDir(row.path, dst); err != nil {
				ctx.Notify(context.LevelError, err.Error())
			} else {
				ctx.Docs.Renamed(row.path, dst)
				if p.folder == row.path || strings.HasPrefix(p.folder, row.path+"/") {
					p.folder = dst + strings.TrimPrefix(p.folder, row.path)
				}
				p.invalidate()
			}
		}
	}
	if ui.Focus() != fid {
		p.renamingDir = ""
	}
}

func (p *Panel) newFolder(ctx *context.Editor, parent string) {
	dir := ctx.Pipeline.UniqueDir(path.Join(parent, "New Folder"))
	if err := ctx.Pipeline.Mkdir(dir); err != nil {
		ctx.Notify(context.LevelError, err.Error())
		return
	}
	p.expanded[parent] = true
	p.renamingDir = dir
	p.renameDirStart = true
	p.invalidate()
}

func (p *Panel) ReimportSelected(ctx *context.Editor) error {
	if p.selected.IsZero() || ctx.Pipeline == nil {
		return nil
	}
	batch, err := ctx.Pipeline.Reimport(p.selected)
	ctx.ApplyBatch(batch)
	if err == nil {
		ctx.Notify(context.LevelInfo, "Reimported")
	}
	return err
}

func (p *Panel) DeleteSelected(ctx *context.Editor) {
	a := p.find(p.selected)
	if a == nil {
		return
	}
	id, rel := a.ID, a.Path
	ctx.Ask(context.Question{
		Title:   "Delete asset",
		Message: "Delete " + rel + "? Scenes that reference it keep the reference and show it as missing.",
		Buttons: []string{"Delete", "Cancel"},
		Then: func(choice int) {
			if choice != 0 {
				return
			}
			batch, err := ctx.Pipeline.Delete(id)
			if err != nil {
				ctx.Notify(context.LevelError, err.Error())
				return
			}
			ctx.ApplyBatch(batch)
			p.thumbs.Forget(id)
			if p.selected == id {
				p.selected = ids.AssetID{}
			}
			p.invalidate()
		},
	})
}

func (p *Panel) duplicate(ctx *context.Editor, id ids.AssetID) {
	dup, batch, err := ctx.Pipeline.Duplicate(id)
	ctx.ApplyBatch(batch)
	if err != nil {
		ctx.Notify(context.LevelError, err.Error())
		return
	}
	p.selected = dup
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

func (p *Panel) menus(ctx *context.Editor) {
	ui := ctx.UI
	if widgets.ContextMenu(ui, p.assetMenuID) {
		if a := p.find(p.menuAsset); a != nil {
			id := a.ID
			if widgets.MenuItem(ui, "Open", "", a.Kind == asset.ArtScene || a.Kind == asset.ArtScript, false) {
				p.open(ctx, a)
			}
			if widgets.MenuItem(ui, "Rename", "F2", true, false) {
				p.selected = id
				p.StartRename()
			}
			if widgets.MenuItem(ui, "Duplicate", "", true, false) {
				p.duplicate(ctx, id)
			}
			if widgets.MenuItem(ui, "Reimport", "", true, false) {
				p.selected = id
				if err := p.ReimportSelected(ctx); err != nil {
					ctx.Notify(context.LevelError, err.Error())
				}
			}
			if widgets.MenuItem(ui, "Reveal in File Manager", "", true, false) {
				if err := reveal(ctx.Pipeline.Abs(a.Path), false); err != nil {
					ctx.Notify(context.LevelError, err.Error())
				}
			}
			widgets.Separator(ui)
			if widgets.MenuItem(ui, "Delete", "Delete", true, false) {
				p.selected = id
				p.DeleteSelected(ctx)
			}
		}
		widgets.EndContextMenu(ui)
	}
	if widgets.ContextMenu(ui, p.dirMenuID) {
		dir := p.menuDir
		if widgets.MenuItem(ui, "New Folder", "", true, false) {
			p.newFolder(ctx, dir)
		}
		isRoot := path.Dir(dir) == "."
		if widgets.MenuItem(ui, "Rename", "", !isRoot, false) {
			p.renamingDir = dir
			p.renameDirStart = true
		}
		if widgets.MenuItem(ui, "Delete Empty Folder", "", !isRoot, false) {
			if err := ctx.Pipeline.RemoveDir(dir); err != nil {
				ctx.Notify(context.LevelError, err.Error())
			} else {
				if p.folder == dir {
					p.folder = path.Dir(dir)
				}
				p.invalidate()
			}
		}
		if widgets.MenuItem(ui, "Reveal in File Manager", "", true, false) {
			if err := reveal(ctx.Pipeline.Abs(dir), true); err != nil {
				ctx.Notify(context.LevelError, err.Error())
			}
		}
		widgets.EndContextMenu(ui)
	}
}
