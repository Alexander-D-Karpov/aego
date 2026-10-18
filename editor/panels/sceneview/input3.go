package sceneview

import (
	"path"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/templates"
	"aego/platform/input"
	"aego/scene"
	"aego/ui/widgets"
)

func (p *Panel) syncMode(d *document.SceneDocument) {
	if p.c3Doc != d.ID {
		p.c3Doc = d.ID
		p.c3.LoadOrbit(d.View.Orbit)
	}
	p.viewMode = ModeFor(d.View.Mode, d.Scene())
}

func (p *Panel) toggleMode(d *document.SceneDocument) {
	next := Mode3D
	if p.viewMode == Mode3D {
		next = Mode2D
	}
	d.View.Mode = next.ViewMode()
	p.viewMode = next
	if next == Mode3D && d.View.Orbit.Distance <= 0 && p.cur != nil {
		p.c3.FrameSelection(p.host3())
		d.View.Orbit = p.c3.Orbit()
	}
}

func (p *Panel) snapStep() float32 {
	if p.d.Tools.Snap != p.ui().Primary() {
		return p.d.Tools.SnapMove
	}
	return 0
}

func (p *Panel) input3(d *document.SceneDocument, r mathx.Rect) {
	if p.cur == nil {
		return
	}
	ui := p.ui()
	id := ui.ID("view3")
	over := ui.MouseOver(id, r) && !p.barRect.Contains(ui.Mouse())
	mods := ui.Mods()
	in := Input3{
		Mouse:   p.toPixels(ui.Mouse()),
		Delta:   ui.MouseDelta().Scale(p.scale),
		Hovered: over,
		Alt:     mods.Has(input.ModAlt),
		Shift:   mods.Has(input.ModShift) || mods.Primary(),
		Cancel:  ui.KeyPressed(input.KeyEscape),
	}
	if over {
		in.Wheel = ui.ZoomY()
	}
	for i, b := range [3]input.Button{input.MouseLeft, input.MouseRight, input.MouseMiddle} {
		in.Down[i] = ui.MouseDown(b)
		in.Pressed[i] = over && ui.MousePressed(b)
	}
	if in.Pressed[ButtonRight] {
		p.rightAt, p.rightArmed = in.Mouse, true
	}
	if p.rightArmed && in.Mouse.Sub(p.rightAt).Len() > 4*p.scale {
		p.rightArmed = false
	}
	if reason, locked := d.History.Locked(); locked && in.Pressed[ButtonLeft] && p.c3.Hot() != 0 {
		p.d.Notices.Notify(context.LevelWarn, reason)
		in.Pressed[ButtonLeft] = false
	}
	p.c3.Update(p.host3(), in)
	busy := p.c3.Busy()
	switch {
	case busy && ui.Captured() != id:
		ui.CaptureMouse(id)
	case !busy && ui.Captured() == id:
		ui.ReleaseMouse()
	}
	if p.rightArmed && ui.MouseReleased(input.MouseRight) {
		p.rightArmed = false
		p.openMenu3(in.Mouse)
	}
}

func (p *Panel) openMenu3(px mathx.Vec2) {
	h := p.host3()
	s := h.Scene()
	if s == nil {
		return
	}
	if n, ok := p.c3.PickAt(h, px); ok {
		if id := s.ID(n); !p.d.Selection.IsSelected(id) {
			p.d.Selection.Select(id)
		}
	}
	p.menuAt3 = p.c3.DropPoint(h, px, p.snapStep())
	widgets.OpenContextMenu(p.ui(), p.menuID)
}

func (p *Panel) createAt3(at mathx.Vec3) {
	if p.d.Templates == nil {
		return
	}
	p.d.Scenes.CreateMenuAt(p.d.Templates, templates.Place{At3: &at})
}

func (p *Panel) drop3(r mathx.Rect) {
	ui := p.ui()
	dn := p.f.DnD
	if p.cur == nil || !dn.Hovering(ui, r) {
		return
	}
	sp := scene.Space3D
	pl, _ := dn.Payload()
	ap, ok := pl.Data.(context.AssetPayload)
	if pl.Kind != context.KindAsset || !ok || p.d.Templates == nil || !p.d.Templates.Accepts(ap.Kind, &sp) {
		dn.Target(ui, r, "reject")
		return
	}
	got, ok := dn.Target(ui, r, context.KindAsset)
	if !ok {
		return
	}
	ap = got.Data.(context.AssetPayload)
	at := p.c3.DropPoint(p.host3(), p.toPixels(dn.Pos(ui)), p.snapStep())
	t, ok := p.d.Templates.ForAsset(ap.Kind, &sp, ap.ID, path.Base(ap.Name))
	if !ok {
		return
	}
	if err := p.d.Scenes.InstantiateAt3(t, at); err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
	}
}
