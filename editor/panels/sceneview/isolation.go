package sceneview

import (
	"strconv"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/scene"
	"aego/ui/widgets"
)

const CommandIsolate = "scene.isolate"

type isolation struct {
	doc document.ID
	rev uint64
	ver uint64
	vis map[scene.NodeHandle]bool
}

func Visible(s *scene.Scene, roots map[ids.NodeID]bool) map[scene.NodeHandle]bool {
	vis := map[scene.NodeHandle]bool{}
	root := s.Root()
	for _, h := range s.PreOrder() {
		if h == root || !s.IsAlive(h) {
			continue
		}
		for p := h; !p.IsZero() && p != root; p = s.Parent(p) {
			if roots[s.ID(p)] {
				vis[h] = true
				break
			}
		}
	}
	return vis
}

func (p *Panel) updateIsolation(d *document.SceneDocument) {
	if len(d.View.Isolated) == 0 {
		p.iso.vis = nil
		return
	}
	s := d.Scene()
	rev := s.StructureRevision()
	if p.iso.vis != nil && p.iso.doc == d.ID && p.iso.rev == rev && p.iso.ver == p.isoVer {
		return
	}
	p.iso = isolation{doc: d.ID, rev: rev, ver: p.isoVer, vis: Visible(s, d.View.Isolated)}
}

func (p *Panel) hidden(_ *scene.Scene, h scene.NodeHandle) bool {
	return p.iso.vis != nil && !p.iso.vis[h]
}

func (p *Panel) runnerHide() func(scene.NodeHandle) bool {
	if p.iso.vis == nil {
		return nil
	}
	vis := p.iso.vis
	return func(h scene.NodeHandle) bool { return !vis[h] }
}

func (p *Panel) Isolated() bool {
	d := p.d.Docs.Doc()
	return d != nil && len(d.View.Isolated) > 0
}

func (p *Panel) ToggleIsolation() error {
	d := p.d.Docs.Doc()
	if d == nil {
		return nil
	}
	p.isoVer++
	if len(d.View.Isolated) > 0 {
		d.View.Isolated = nil
		return nil
	}
	sel := p.d.Selection.Selected()
	if len(sel) == 0 {
		p.d.Notices.Notify(context.LevelInfo, "Select the nodes to isolate")
		return nil
	}
	d.View.Isolated = make(map[ids.NodeID]bool, len(sel))
	for _, id := range sel {
		d.View.Isolated[id] = true
	}
	return nil
}

func (p *Panel) isolationBar(d *document.SceneDocument, r mathx.Rect) {
	n := len(d.View.Isolated)
	if n == 0 {
		return
	}
	ui := p.ui()
	th := &ui.Theme
	text := "Isolated: " + strconv.Itoa(n) + " node(s)"
	bw := ui.TextSize("Show All").X + 16
	h := th.BarHeight
	w := ui.TextSize(text).X + bw + 24
	bar := mathx.Rect{X: r.X + 6, Y: r.Y + r.H - h - 6, W: min(w, r.W-12), H: h}
	ui.Shadow(bar, th.Radius.Popup)
	ui.RoundRect(bar, th.Radius.Popup, th.BarBg)
	ui.Outline(bar, th.Accent)
	ui.PushClip(bar)
	ui.Text(text, mathx.Vec2{X: bar.X + 8, Y: ui.TextY(bar)}, th.Text)
	id := ui.ID("isolation.exit")
	if widgets.ButtonRect(ui, id, mathx.Rect{X: bar.X + bar.W - bw - 4, Y: bar.Y + 3, W: bw, H: h - 6}, "Show All") {
		p.ToggleIsolation()
	}
	ui.PopClip()
	widgets.Tooltip(ui, id, "Leave isolation ("+p.d.Commands.Shortcut(CommandIsolate)+")")
}
