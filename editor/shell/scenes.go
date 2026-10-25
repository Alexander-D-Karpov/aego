package shell

import (
	"aego/editor/core/assetops"
	"aego/editor/core/fuzzy"
	"aego/engine/i18n"
	"path"
	"sort"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/templates"
	"aego/editor/core/workspace"
	"aego/platform/input"
	"aego/project"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/widgets"
)

const pickerRows = 14

type scenePicker struct {
	open  bool
	focus bool
	query string
	sel   int
	items []pipeline.AssetInfo
	view  []int
}

func (a *App) reportDirs() {
	if a.opt.DirsErr != nil {
		a.log.Warn("cannot create the project folders", log.F("err", a.opt.DirsErr.Error()))
	}
	if len(a.opt.CreatedDirs) > 0 {
		a.log.Info("created missing project folders: " + strings.Join(a.opt.CreatedDirs, ", "))
	}
}

func (a *App) three() bool {
	return a.opt.Project.DefaultSpace == project.Space3D
}

func (a *App) emptyScene(three bool) *scene.Scene {
	var res asset.Resolver
	if a.ctx.Loader != nil {
		res = a.ctx.Loader
	}
	opts := scene.Options{}
	if three {
		opts.DefaultSpace = scene.Space3D
	}
	return scene.New(a.opt.Types, res, opts)
}

func (a *App) sceneFor(three bool) *scene.Scene {
	s := a.emptyScene(three)
	if !three {
		a.templates.Spawn(s, s.Root(), "render2d.camera")
		s.Flush()
		return s
	}
	a.templates.Spawn(s, s.Root(), "render3d.camera")
	a.templates.Spawn(s, s.Root(), "render3d.plane")
	cube := a.templates.Spawn(s, s.Root(), "render3d.cube")
	s.SetPosition3D(cube, mathx.Vec3{Y: 0.5})
	s.Flush()
	return s
}

func (a *App) newUnsaved(rel string, three bool) error {
	d := a.ctx.Docs.New(a.ctx.Docs.UniquePath(rel), a.sceneFor(three))
	if three {
		d.View.Mode = document.View3D
	}
	a.ctx.Selection.Clear()
	return nil
}

func (a *App) createScene(rel string) error {
	three := a.three()
	if _, err := assetops.CreateFile(a.ctx, rel, sceneio.Save(a.sceneFor(three))); err != nil {
		return err
	}
	d, err := a.ctx.Docs.Open(rel)
	if err != nil {
		return err
	}
	if three {
		d.View.Mode = document.View3D
	}
	a.ctx.Selection.Clear()
	return nil
}

func (a *App) createPrefab(rel string) error {
	s := a.emptyScene(a.three())
	a.templates.Spawn(s, s.Root(), "engine.empty")
	s.Flush()
	if _, err := assetops.CreateFile(a.ctx, rel, sceneio.SavePrefab(s)); err != nil {
		return err
	}
	_, err := a.ctx.Docs.Open(rel)
	return err
}

func (a *App) instantiatePrefab(id ids.AssetID, name string) {
	d := a.ctx.Doc()
	if d == nil {
		return
	}
	t, ok := a.templates.ForAsset(asset.ArtPrefab, nil, id, name)
	if !ok {
		a.ctx.Notify(context.LevelWarn, i18n.Note("editor.shell.no_template_can_instantiate_name", "no template can instantiate {name}", i18n.Str("name", name)))
		return
	}
	parent := ids.NodeID{}
	if it, ok := a.ctx.Selection.Active(); ok && it.Doc == d.ID {
		parent = it.Node
	}
	if _, err := templates.Instantiate(a.ctx, t, parent, -1, nil); err != nil {
		a.ctx.Notify(context.LevelError, err.Error())
	}
}

func (a *App) openScene() error {
	if a.opt.Pipeline == nil {
		return nil
	}
	pk := &a.picker
	pk.items = pk.items[:0]
	for _, it := range a.opt.Pipeline.Assets() {
		if it.Kind == asset.ArtScene {
			pk.items = append(pk.items, it)
		}
	}
	sort.Slice(pk.items, func(i, j int) bool { return pk.items[i].Path < pk.items[j].Path })
	pk.open, pk.focus, pk.query = true, true, ""
	a.filterScenes()
	return nil
}

func (a *App) filterScenes() {
	pk := &a.picker
	pk.view = pk.view[:0]
	pk.sel = 0
	q := strings.TrimSpace(pk.query)
	if q == "" {
		for i := range pk.items {
			pk.view = append(pk.view, i)
		}
		return
	}
	type hit struct{ i, score int }
	var hits []hit
	for i, it := range pk.items {
		if s, _, ok := fuzzy.Match(q, it.Path, nil); ok {
			hits = append(hits, hit{i, s})
		}
	}
	sort.SliceStable(hits, func(x, y int) bool { return hits[x].score > hits[y].score })
	for _, h := range hits {
		pk.view = append(pk.view, h.i)
	}
}

func (a *App) pickScene(k int) {
	pk := &a.picker
	pk.open = false
	a.ui.SetFocus(0)
	if k < 0 || k >= len(pk.view) {
		return
	}
	if _, err := a.ctx.Docs.Open(pk.items[pk.view[k]].Path); err != nil {
		a.ctx.Notify(context.LevelError, err.Error())
		return
	}
	a.ws.Show(workspace.Scene)
}

func (a *App) drawScenePicker() {
	pk := &a.picker
	if !pk.open {
		return
	}
	if len(a.modals) > 0 {
		pk.open = false
		return
	}
	ui := a.ui
	th := &ui.Theme
	qid := core.IDOf("editor.scenepicker")
	if pk.focus {
		ui.SetFocus(qid)
		ui.EditBuffer(qid, pk.query).SelectAll()
		pk.focus = false
	}
	if ui.Focus() != qid {
		pk.open = false
		return
	}
	switch {
	case ui.KeyPressed(input.KeyEscape):
		pk.open = false
		ui.SetFocus(0)
		return
	case ui.KeyPressed(input.KeyEnter), ui.KeyPressed(input.KeyKPEnter):
		a.pickScene(pk.sel)
		return
	case ui.KeyRepeated(input.KeyDown):
		pk.sel = min(pk.sel+1, len(pk.view)-1)
	case ui.KeyRepeated(input.KeyUp):
		pk.sel = max(pk.sel-1, 0)
	}
	rh := ui.LineHeight() + 6
	rows := min(len(pk.view), pickerRows)
	sz := ui.Size()
	w := min(sz.X-40, 520)
	r := mathx.Rect{X: (sz.X - w) / 2, Y: 2*th.BarHeight + 8, W: w, H: th.RowHeight + 12 + float32(max(rows, 1))*rh}
	picked := -1
	ui.BeginOverlay(r, draw.LayerTop)
	ui.Rect(r, th.PanelAlt)
	ui.Outline(r, th.Border)
	fr := mathx.Rect{X: r.X + 4, Y: r.Y + 4, W: r.W - 8, H: th.RowHeight}
	if changed, _ := widgets.TextFieldRect(ui, qid, fr, &pk.query, true); changed {
		a.filterScenes()
	}
	start := max(0, min(pk.sel-rows+1, len(pk.view)-rows))
	for i := 0; i < rows; i++ {
		k := start + i
		it := pk.items[pk.view[k]]
		rr := mathx.Rect{X: r.X + 2, Y: fr.Y + fr.H + 4 + float32(i)*rh, W: r.W - 4, H: rh}
		id := core.IDSeq(qid, 1+i)
		if ui.Pressed(id, rr) {
			picked = k
		}
		switch {
		case k == pk.sel:
			ui.Rect(rr, th.Selection)
		case ui.Hot() == id:
			ui.Rect(rr, th.WidgetHot)
		}
		name := path.Base(it.Path)
		ui.Text(name, mathx.Vec2{X: rr.X + 6, Y: ui.TextY(rr)}, th.Text)
		ui.Text(path.Dir(it.Path), mathx.Vec2{X: rr.X + 18 + ui.TextSize(name).X, Y: ui.TextY(rr)}, th.TextDim)
	}
	if len(pk.view) == 0 {
		ui.TextIn(mathx.Rect{X: r.X, Y: fr.Y + fr.H + 4, W: r.W, H: rh}, i18n.Tr("editor.shell.no_scenes_match", "No scenes match"), th.TextDisabled, core.AlignCenter)
	}
	ui.EndOverlay()
	if picked >= 0 {
		a.pickScene(picked)
	}
}
