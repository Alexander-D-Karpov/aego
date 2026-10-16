package inspector

import (
	"path"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/render/texture"
	"aego/scene"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/widgets"
)

type refKind uint8

const (
	refNil refKind = iota
	refAlive
	refExternal
	refMissing
	refNoComponent
)

type refInfo struct {
	kind    refKind
	label   string
	tooltip string
}

func (k refKind) color(th *core.Theme) mathx.Color {
	switch k {
	case refNil:
		return th.TextDisabled
	case refMissing:
		return th.Error
	case refNoComponent:
		return th.Warn
	}
	return th.Text
}

func shortType(full string) string {
	return full[strings.LastIndexByte(full, '.')+1:]
}

func hasComponent(s *scene.Scene, h scene.NodeHandle, name string) bool {
	td, ok := s.Registry().ByName(name)
	if !ok {
		return false
	}
	_, has := s.GetByType(h, td.ID)
	return has
}

func nodeState(s *scene.Scene, id ids.NodeID, need string) refInfo {
	if id.IsZero() {
		return refInfo{kind: refNil, label: "None"}
	}
	h := s.Handle(id)
	if h.IsZero() || !s.IsAlive(h) {
		return refInfo{kind: refMissing, label: "Missing", tooltip: "Node " + id.String() + " is not in this scene"}
	}
	where := s.Path(h)
	if need != "" && !hasComponent(s, h, need) {
		return refInfo{kind: refNoComponent, label: s.Name(h) + " · no " + shortType(need), tooltip: where + " has no " + need}
	}
	return refInfo{kind: refAlive, label: s.Name(h), tooltip: where}
}

func assetState(pl *pipeline.Pipeline, id ids.AssetID) refInfo {
	if id.IsZero() {
		return refInfo{kind: refNil, label: "None"}
	}
	if b, ok := asset.LookupBuiltin(id); ok {
		return refInfo{kind: refExternal, label: path.Base(b.Name), tooltip: "Engine builtin " + b.Name}
	}
	if pl != nil {
		if e, ok := pl.Entry(id); ok && !e.Missing {
			return refInfo{kind: refAlive, label: path.Base(e.Name), tooltip: e.Name}
		}
	}
	return refInfo{kind: refMissing, label: "Missing asset", tooltip: "Asset " + id.String() + " is not imported"}
}

func fitThumb(box mathx.Rect, reg texture.Region) mathx.Rect {
	if reg.W == 0 || reg.H == 0 {
		return box
	}
	s := min(box.W/float32(reg.W), box.H/float32(reg.H))
	w, h := float32(reg.W)*s, float32(reg.H)*s
	return mathx.Rect{X: box.X + (box.W-w)/2, Y: box.Y + (box.H-h)/2, W: w, H: h}
}

func preview(ui *core.Context, reg texture.Region) {
	th := &ui.Theme
	const size = float32(128)
	m := ui.Mouse()
	sz := ui.Size()
	r := mathx.Rect{X: m.X + 16, Y: m.Y + 16, W: size + 8, H: size + 8}
	if r.X+r.W > sz.X {
		r.X = max(0, m.X-r.W-8)
	}
	if r.Y+r.H > sz.Y {
		r.Y = max(0, m.Y-r.H-8)
	}
	l := ui.List()
	l.PushLayer(draw.LayerTop)
	l.Rect(r, th.PanelAlt)
	l.RectOutline(r, th.Border, 1)
	l.Texture(reg, fitThumb(mathx.Rect{X: r.X + 4, Y: r.Y + 4, W: size, H: size}, reg), mathx.Color{R: 1, G: 1, B: 1, A: 1})
	l.PopLayer()
}

func (p *Panel) refBox(id core.ID, r mathx.Rect, info refInfo, mixed bool, thumb texture.Region, hasThumb bool) (mathx.Rect, bool, bool, core.ID) {
	ui := p.f.UI
	th := &ui.Theme
	bw := r.H
	set := mixed || info.kind != refNil
	right := bw + 2
	if set {
		right += bw + 2
	}
	box := mathx.Rect{X: r.X, Y: r.Y, W: r.W - right, H: r.H}
	hid := core.IDSeq(id, 3)
	picked := false
	if !mixed && (info.kind == refAlive || info.kind == refNoComponent) {
		picked = ui.Clicked(hid, box)
	} else {
		ui.MouseOver(hid, box)
	}
	ui.Rect(box, th.Widget)
	border := th.Border
	if !mixed && info.kind == refMissing {
		border = th.Error
	}
	ui.Outline(box, border)
	tx := box.X + th.Padding
	if hasThumb && !mixed {
		s := box.H - 4
		ir := mathx.Rect{X: box.X + 2, Y: box.Y + 2, W: s, H: s}
		ui.Rect(ir, th.Background)
		ui.List().Texture(thumb, fitThumb(ir, thumb), mathx.Color{R: 1, G: 1, B: 1, A: 1})
		tx = ir.X + ir.W + 6
		if ui.HoverDuration(hid) > 0.35 {
			preview(ui, thumb)
		}
	} else if !mixed {
		widgets.Tooltip(ui, hid, info.tooltip)
	}
	text, col := info.label, info.kind.color(th)
	if mixed {
		text, col = th.Mixed, th.Text
	}
	ui.PushClip(mathx.Rect{X: tx, Y: box.Y, W: max(0, box.X+box.W-tx-2), H: box.H})
	ui.Text(text, mathx.Vec2{X: tx, Y: ui.TextY(box)}, col)
	ui.PopClip()
	x := box.X + box.W + 2
	clear := false
	if set {
		clear = widgets.ButtonRect(ui, core.IDSeq(id, 1), mathx.Rect{X: x, Y: r.Y, W: bw, H: r.H}, widgets.Glyph(ui, '×', "x"))
		x += bw + 2
	}
	pr := mathx.Rect{X: x, Y: r.Y, W: bw, H: r.H}
	pid := core.IDSeq(id, 2)
	if ui.Pressed(pid, pr) {
		if ui.PopupOpen(pid) {
			ui.ClosePopup(pid)
		} else {
			ui.CloseAllPopups()
			ui.OpenPopup(pid, mathx.Vec2{X: box.X, Y: r.Y + r.H})
		}
	}
	ui.Rect(pr, ui.WidgetColor(pid))
	ui.TextIn(pr, widgets.Glyph(ui, '…', "..."), th.Text, core.AlignCenter)
	return box, picked, clear, pid
}

func (p *Panel) thumbRow(id core.ID, label string, asset ids.AssetID, selected bool) bool {
	ui := p.f.UI
	th := &ui.Theme
	h := th.RowHeight + 8
	r := ui.Cursor().Next(h)
	ui.FitPopup(h + ui.TextSize(label).X + 20)
	clicked := ui.Clicked(id, r)
	switch {
	case selected:
		ui.Rect(r, th.WidgetDown)
	case ui.Hot() == id:
		ui.Rect(r, th.WidgetHot)
	}
	ir := mathx.Rect{X: r.X + 4, Y: r.Y + 4, W: h - 8, H: h - 8}
	ui.Rect(ir, th.Background)
	if p.d.Thumbs != nil && !asset.IsZero() {
		if reg, ok := p.d.Thumbs.ByID(asset); ok {
			ui.List().Texture(reg, fitThumb(ir, reg), mathx.Color{R: 1, G: 1, B: 1, A: 1})
		}
	}
	ui.Text(label, mathx.Vec2{X: ir.X + ir.W + 8, Y: ui.TextY(r)}, th.Text)
	return clicked
}

func (p *Panel) assetField(id core.ID, r mathx.Rect, fd *schema.FieldDescriptor, v *schema.Value, mixed bool) widgets.Edit {
	ui := p.f.UI
	cur := v.Asset
	var e widgets.Edit
	info := assetState(p.d.Pipeline, cur)
	var thumb texture.Region
	hasThumb := false
	if p.d.Thumbs != nil && (info.kind == refAlive || info.kind == refExternal) {
		thumb, hasThumb = p.d.Thumbs.ByID(cur)
	}
	box, picked, clear, pid := p.refBox(id, r, info, mixed, thumb, hasThumb)
	if picked && p.d.Pipeline != nil {
		if rel, ok := p.d.Pipeline.PathOf(cur); ok {
			p.d.Nav.Open(context.AssetAt(rel))
		}
	}
	if clear {
		*v = schema.Asset(ids.AssetID{})
		e = all
	}
	if pl, ok := p.f.DnD.Payload(); ok && p.f.DnD.Hovering(ui, box) {
		ap, isAsset := pl.Data.(context.AssetPayload)
		if pl.Kind == context.KindAsset && isAsset && context.AssetMatches(fd.AssetKind, ap.Kind) {
			if got, ok := p.f.DnD.Target(ui, box, context.KindAsset); ok {
				*v = schema.Asset(got.Data.(context.AssetPayload).ID)
				e = all
			}
		} else {
			p.f.DnD.Target(ui, box, "reject")
		}
	}
	if !ui.BeginPopup(pid, max(r.W, 280)) {
		return e
	}
	widgets.SearchField(ui, core.IDSeq(pid, 1), ui.Cursor().Next(0), &p.pick)
	q := strings.ToLower(p.pick)
	pickRow := func(rid core.ID, label string, aid ids.AssetID) {
		if p.thumbRow(rid, label, aid, aid == cur) {
			*v = schema.Asset(aid)
			e = all
			ui.ClosePopup(pid)
		}
	}
	pickRow(core.IDSeq(pid, 2), "None", ids.AssetID{})
	for i, b := range asset.Builtins() {
		if !context.AssetMatches(fd.AssetKind, b.Kind) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(b.Name), q) {
			continue
		}
		pickRow(core.IDSeq(pid, 5000+i), b.Name, b.ID)
	}
	if p.d.Pipeline != nil {
		count := 0
		for _, a := range p.d.Pipeline.Assets() {
			if count >= 200 {
				break
			}
			if !context.AssetMatches(fd.AssetKind, a.Kind) {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(a.Name), q) {
				continue
			}
			count++
			pickRow(core.IDSeq(pid, 10+count), a.Name, a.ID)
		}
	}
	ui.EndPopup()
	return e
}

func (p *Panel) nodeField(d *document.SceneDocument, id core.ID, r mathx.Rect, v *schema.Value, mixed bool, need string) widgets.Edit {
	ui := p.f.UI
	s := d.Scene()
	cur := v.Node
	var e widgets.Edit
	box, picked, clear, pid := p.refBox(id, r, nodeState(s, cur, need), mixed, texture.Region{}, false)
	if picked {
		p.d.Selection.Select(cur)
	}
	if clear {
		*v = schema.Node(ids.NodeID{})
		e = all
	}
	if pl, ok := p.f.DnD.Payload(); ok && p.f.DnD.Hovering(ui, box) {
		np, isNodes := pl.Data.(context.NodesPayload)
		if pl.Kind == context.KindNodes && isNodes && np.Doc == d.ID && len(np.Nodes) == 1 {
			if got, ok := p.f.DnD.Target(ui, box, context.KindNodes); ok {
				*v = schema.Node(got.Data.(context.NodesPayload).Nodes[0])
				e = all
			}
		} else {
			p.f.DnD.Target(ui, box, "reject")
		}
	}
	if !ui.BeginPopup(pid, max(r.W, 280)) {
		return e
	}
	widgets.SearchField(ui, core.IDSeq(pid, 1), ui.Cursor().Next(0), &p.pick)
	if widgets.Selectable(ui, core.IDSeq(pid, 2), "None", cur.IsZero()) {
		*v = schema.Node(ids.NodeID{})
		e = all
		ui.ClosePopup(pid)
	}
	q := strings.ToLower(p.pick)
	root := s.Root()
	count := 0
	for _, h := range s.PreOrder() {
		if count >= 200 {
			break
		}
		if h == root || !s.IsAlive(h) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(s.Name(h)), q) {
			continue
		}
		if need != "" && !hasComponent(s, h, need) {
			continue
		}
		count++
		nid := s.ID(h)
		if widgets.Selectable(ui, core.IDSeq(pid, 10+count), s.Path(h), nid == cur) {
			*v = schema.Node(nid)
			e = all
			ui.ClosePopup(pid)
		}
	}
	ui.EndPopup()
	return e
}
