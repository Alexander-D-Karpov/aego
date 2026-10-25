package animation

import (
	"math"
	"slices"
	"strconv"

	"aego/anim"
	"aego/core/curve"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/animedit"
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/render/texture"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	thumbSize = 16
	thumbGap  = 18
	zoomStep  = 0.85
)

var (
	laneXYZW = [...]string{"x", "y", "z", "w"}
	laneRGBA = [...]string{"r", "g", "b", "a"}
)

type curveKey struct {
	clip        *anim.Clip
	track, lane int
}

// frame maps clip time to the dope sheet, zoomed with the wheel and panned with Shift+wheel.
func (p *Panel) frame(area mathx.Rect) frame {
	ui := p.f.UI
	length := max(p.clip.Length, 1e-3)
	if p.span <= 0 {
		p.t0, p.span = 0, length
	}
	g := frame{x0: area.X + pad, w: max(area.W-2*pad, 1), length: length, step: p.step(), t0: p.t0, span: p.span}
	if !area.Contains(ui.Mouse()) || ui.MouseInOverlay() {
		return g
	}
	z := ui.ScrollY(1)
	if z == 0 {
		return g
	}
	if ui.Mods().Has(input.ModShift) {
		p.t0 -= z * p.span * 0.1
	} else {
		at := g.t(ui.Mouse().X)
		p.span = mathx.Clamp(p.span*float32(math.Pow(zoomStep, float64(z))), g.step*4, length*2)
		p.t0 = at - (ui.Mouse().X-g.x0)/g.w*p.span
	}
	p.t0 = mathx.Clamp(p.t0, -p.span/2, length-p.span/2)
	g.t0, g.span = p.t0, p.span
	return g
}

func (p *Panel) resetView() {
	p.span, p.ctrack, p.cof = 0, -1, curveKey{}
	clear(p.thumbs)
}

func laneNames(k schema.Kind) []string {
	if k == schema.KindColor {
		return laneRGBA[:]
	}
	return laneXYZW[:]
}

func (p *Panel) curveView(r mathx.Rect, g frame) {
	ui := p.f.UI
	th := &ui.Theme
	svc := p.svc
	if p.ctrack < 0 || svc.Lanes(p.ctrack) == 0 {
		p.ctrack = -1
		for i := range p.clip.Tracks {
			if svc.Lanes(i) > 0 {
				p.ctrack, p.lane = i, 0
				break
			}
		}
	}
	if p.ctrack < 0 {
		ui.TextIn(r, "The clip has no numeric tracks to draw as curves", th.TextDisabled, core.AlignCenter)
		return
	}
	lanes := svc.Lanes(p.ctrack)
	p.lane = min(p.lane, lanes-1)
	tabs := mathx.Rect{X: r.X + pad, Y: r.Y + 2, W: 26, H: th.RowHeight - 4}
	if lanes > 1 {
		names := laneNames(p.clip.Tracks[p.ctrack].Kind)
		for i := range lanes {
			if widgets.ButtonRect(ui, core.IDSeq(ui.ID("lane"), i+1), tabs, names[i]) {
				p.lane = i
			}
			if i == p.lane {
				ui.Rect(mathx.Rect{X: tabs.X + 2, Y: tabs.Y + tabs.H - 2, W: tabs.W - 4, H: 2}, th.Accent)
			}
			tabs.X += tabs.W + 4
		}
	}
	gr := mathx.Rect{X: r.X + pad, Y: r.Y + th.RowHeight, W: max(r.W-2*pad, 1), H: max(r.H-th.RowHeight-pad, 1)}
	key := curveKey{clip: p.clip, track: p.ctrack, lane: p.lane}
	if key != p.cof && !p.cst.Dragging() {
		p.cof = key
		p.cbase = svc.LaneCurve(p.ctrack, p.lane)
		p.ccur = curve.Curve{Keys: slices.Clone(p.cbase.Keys)}
		p.cst.Key = -1
	}
	p.cst.SetTimeRange(g.t(gr.X+widgets.CurvePad), g.t(gr.X+gr.W-widgets.CurvePad))
	e := widgets.CurveGraph(ui, ui.ID("curve"), gr, &p.ccur, p.cst)
	d := p.d.Docs.Doc()
	if e.Begin && p.ctx == nil && d != nil {
		p.ctx = d.History.Begin("Edit Curve")
	}
	if e.Changed {
		ops := svc.CurveOps(p.ctrack, p.lane, p.cbase, p.ccur)
		if err := svc.EditClip("Edit Curve", ops...); err != nil {
			p.notify(err)
			p.cof = curveKey{}
		} else {
			p.cbase = curve.Curve{Keys: slices.Clone(p.ccur.Keys)}
			p.cof.clip = svc.Clip()
		}
	}
	if e.End && p.ctx != nil {
		p.ctx.Commit()
		p.ctx = nil
	}
}

func (p *Panel) thumb(row *animedit.Row, k int, g frame) bool {
	x := g.x(row.Keys[k])
	if k > 0 && x-g.x(row.Keys[k-1]) < thumbGap {
		return false
	}
	if k+1 < len(row.Keys) && g.x(row.Keys[k+1])-x < thumbGap {
		return false
	}
	return p.d.Thumbs != nil
}

func (p *Panel) thumbOf(id ids.AssetID) texture.Region {
	if reg, ok := p.thumbs[id]; ok {
		return reg
	}
	reg, ok := p.d.Thumbs.ByID(id)
	if ok {
		p.thumbs[id] = reg
	}
	return reg
}

// TogglePlay plays or pauses the clip, for Space.
func (p *Panel) TogglePlay() {
	if p.svc == nil || p.clip == nil {
		return
	}
	if p.svc.Playing() {
		p.svc.Pause()
	} else {
		p.svc.Play()
	}
}

// DeleteKeys removes the selected keys, for Delete.
func (p *Panel) DeleteKeys() error {
	if p.svc == nil || len(p.sel) == 0 {
		return nil
	}
	err := p.svc.RemoveKeys(p.selected())
	clear(p.sel)
	return err
}

// KeyNode keys the fields of the selected node that differ from the clip, for K.
func (p *Panel) KeyNode() error {
	if p.svc == nil || p.clip == nil {
		return nil
	}
	sel := p.d.Selection.Selected()
	if len(sel) == 0 {
		return nil
	}
	p.svc.SetMode(max(p.svc.Mode(), animedit.Preview))
	n, err := p.svc.KeyChanged(sel[0])
	if err == nil && n == 0 && p.d.Notices != nil {
		p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.animation.nothing_to_key_the_node", "Nothing to key: the node matches the clip at this time"))
	}
	return err
}

// CopyKeys puts the selected keys on the animation clipboard, for Ctrl+C.
func (p *Panel) CopyKeys() {
	if p.svc == nil || len(p.sel) == 0 {
		return
	}
	if n := p.svc.CopyKeys(p.selected()); n > 0 && p.d.Notices != nil {
		p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.animation.copied_n_key_s", "Copied {n} key(s)", i18n.Str("n", strconv.Itoa(n))))
	}
}

// PasteKeys pastes the clipboard at the time cursor, for Ctrl+V.
func (p *Panel) PasteKeys() error {
	if p.svc == nil || p.clip == nil {
		return nil
	}
	return p.svc.PasteKeys()
}

func (p *Panel) Active() bool {
	return p.svc != nil && p.clip != nil
}

const (
	CommandPlay   = "animation.play"
	CommandKey    = "animation.key"
	CommandDelete = "animation.delete_keys"
	CommandCopy   = "animation.copy_keys"
	CommandPaste  = "animation.paste_keys"
)

// ShowCurves switches to the curve view of a track lane, for tests and screenshots.
func (p *Panel) ShowCurves(on bool, track, lane int) {
	p.curves, p.ctrack, p.lane = on, track, lane
}

// TogglePreview turns Preview on or off, for the Animation menu.
func (p *Panel) TogglePreview() {
	if p.svc == nil {
		return
	}
	if p.svc.Mode() == animedit.Normal {
		p.svc.SetMode(animedit.Preview)
	} else {
		p.svc.SetMode(animedit.Normal)
	}
}

// ToggleRecord turns Record on or off, for the Animation menu.
func (p *Panel) ToggleRecord() {
	if p.svc == nil {
		return
	}
	if p.svc.Mode() == animedit.Record {
		p.svc.SetMode(animedit.Preview)
	} else {
		p.svc.SetMode(animedit.Record)
	}
}

func (p *Panel) ToggleCurves()    { p.curves = !p.curves }
func (p *Panel) Curves() bool     { return p.curves }
func (p *Panel) Previewing() bool { return p.svc != nil && p.svc.Mode() != animedit.Normal }
func (p *Panel) Recording() bool  { return p.svc != nil && p.svc.Mode() == animedit.Record }

const (
	CommandPreview = "animation.preview"
	CommandRecord  = "animation.record"
	CommandCurves  = "animation.curves"
)
