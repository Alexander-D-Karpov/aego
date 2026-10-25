package settings

import (
	"aego/engine/i18n"
	"slices"

	"aego/core/mathx"
	"aego/engine"
	es "aego/engine/settings"
	"aego/engine/settings/settingsui"
	raw "aego/platform/input"
	"aego/render"
	"aego/render/text"
	"aego/render/view"
	"aego/scene/frame"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/theme"
	"aego/ui/widgets"
)

const uiScaleKey = "display.uiScale"

type View struct {
	st      *es.Store
	world   *engine.World
	ui      *core.Context
	in      raw.State
	ed      settingsui.Editor
	bar     widgets.ScrollState
	open    bool
	paused  bool
	cat     string
	keys    []es.Key
	draft   map[es.Key]schema.Value
	scroll  float32
	content float32
	note    string
	uiKey   es.Key
}

func NewView(st *es.Store, w *engine.World) *View {
	return &View{st: st, world: w, draft: map[es.Key]schema.Value{}, uiKey: st.Key(uiScaleKey)}
}

func (v *View) Opened() bool {
	return v.open
}

func (v *View) Open() {
	if v.open {
		return
	}
	v.open = true
	if t := v.world.Time; t != nil {
		v.paused = t.Paused()
		t.SetPaused(true)
	}
	if cats := v.cats(); v.cat == "" && len(cats) > 0 {
		v.cat = cats[0]
	}
	v.refresh()
}

func (v *View) Close() {
	if !v.open {
		return
	}
	v.open = false
	v.ed.Cancel()
	clear(v.draft)
	v.note = ""
	if t := v.world.Time; t != nil {
		t.SetPaused(v.paused)
	}
}

func (v *View) Toggle() {
	if v.open {
		v.Close()
	} else {
		v.Open()
	}
}

func (v *View) shows(d *es.Desc, cat string) bool {
	return d.Flags.Has(es.User) && (d.Category == cat || (cat == es.CategoryAccessibility && d.Flags.Has(es.Accessibility)))
}

func (v *View) cats() []string {
	var out []string
	for _, c := range es.Categories {
		has := false
		v.st.Each(func(_ es.Key, d *es.Desc) { has = has || v.shows(d, c) })
		if has {
			out = append(out, c)
		}
	}
	return out
}

func (v *View) refresh() {
	v.keys = v.keys[:0]
	v.st.Each(func(k es.Key, d *es.Desc) {
		if v.shows(d, v.cat) {
			v.keys = append(v.keys, k)
		}
	})
	slices.SortStableFunc(v.keys, func(a, b es.Key) int { return v.st.Desc(a).Order - v.st.Desc(b).Order })
	v.scroll = 0
}

func (v *View) setup(e *engine.Engine, r *render.Renderer) {
	ui := core.New(r.UIFont())
	ui.Theme = theme.Editor()
	if e != nil {
		if set, err := text.LoadSet(e.VFS(), r.Textures(), "engine://fonts", "ui", []int{12, 14, 16, 20}, []int{1, 2}); err == nil {
			ui.SetFonts(set)
		}
		if sh, err := draw.NewShapes(e.GPU(), r.Textures()); err == nil {
			ui.SetShapes(sh)
		}
	}
	v.ui = ui
}

func (v *View) draw(ctx *engine.Context, fr *frame.Runner) {
	rnd := fr.Renderer()
	out := fr.Output()
	if out.W <= 0 || out.H <= 0 {
		return
	}
	if v.ui == nil {
		v.setup(ctx.Engine, rnd)
	}
	scale := out.Scale
	if scale <= 0 {
		scale = 1
	}
	us := float32(v.st.Float(v.uiKey))
	if us <= 0 {
		us = 1
	}
	px := scale * us
	size := mathx.Vec2{X: float32(out.W) / px, Y: float32(out.H) / px}
	f := ctx.Input().Snapshot()
	f.MousePos = f.MousePos.Scale(1 / us)
	v.in.Restore(f)
	v.ui.Begin(&v.in, size, px)
	v.layout(size)
	list := v.ui.End()
	sv := view.ScreenView(out.W, out.H, px)
	sv.Target, sv.TargetW, sv.TargetH = out.Target, out.TargetW, out.TargetH
	sv.DoClear = false
	sv.Label = SystemDraw
	vf := rnd.View(sv)
	white := rnd.White()
	if font := v.ui.Font(); font != nil && !font.White.IsZero() {
		white = font.White
	}
	list.Flush(vf, v.ui.Font(), white, 0)
	vf.End()
}

func (v *View) layout(size mathx.Vec2) {
	ui := v.ui
	th := &ui.Theme
	if ui.KeyPressed(raw.KeyEscape) && !v.ed.Capturing() {
		v.Close()
		return
	}
	ui.Rect(mathx.Rect{W: size.X, H: size.Y}, mathx.Color{A: 0.55})
	w, h := min(860, size.X-32), min(620, size.Y-32)
	if w < 240 || h < 180 {
		return
	}
	r := mathx.Rect{X: (size.X - w) / 2, Y: (size.Y - h) / 2, W: w, H: h}
	ui.Shadow(r, th.Radius.Popup)
	ui.Box(r, th.Radius.Popup, th.Panel, th.Border)
	pad := th.Padding * 2
	head := mathx.Rect{X: r.X + pad, Y: r.Y + pad, W: r.W - 2*pad, H: th.RowHeight}
	ui.Text(i18n.Tr("settings.title", "Settings"), mathx.Vec2{X: head.X, Y: ui.TextY(head)}, th.Text)
	tabs := mathx.Rect{X: head.X, Y: head.Y + head.H + th.Spacing*3, W: head.W, H: th.RowHeight}
	tabs.H = v.tabs(ui, tabs)
	foot := mathx.Rect{X: head.X, Y: r.Y + r.H - pad - th.RowHeight, W: head.W, H: th.RowHeight}
	body := mathx.Rect{X: head.X, Y: tabs.Y + tabs.H + th.Spacing*4, W: head.W}
	body.H = max(0, foot.Y-th.Spacing*3-body.Y)
	if left, ok := v.st.Pending(); ok {
		banner := mathx.Rect{X: body.X, Y: body.Y + body.H - th.RowHeight, W: body.W, H: th.RowHeight}
		body.H = max(0, body.H-th.RowHeight-th.Spacing*2)
		v.confirm(ui, banner, left)
	}
	v.list(ui, body)
	v.footer(ui, foot)
}

// tabs draws the category tabs, wrapping them onto more rows on a narrow screen, and returns their height.
func (v *View) tabs(ui *core.Context, r mathx.Rect) float32 {
	x, y := r.X, r.Y
	for i, c := range v.cats() {
		label := es.CategoryText(c)
		w := min(ui.TextSize(label).X+24, r.W)
		if x > r.X && x+w > r.X+r.W {
			x, y = r.X, y+r.H+4
		}
		on := c == v.cat
		if widgets.ToggleButton(ui, core.IDSeq(ui.ID("tab"), i), mathx.Rect{X: x, Y: y, W: w, H: r.H}, label, &on) && c != v.cat {
			v.cat = c
			v.ed.Cancel()
			v.refresh()
		}
		x += w + 4
	}
	return y + r.H - r.Y
}

func (v *View) list(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	if ui.MouseOver(ui.ID("scroll"), r) {
		if d := ui.ScrollY(th.RowHeight * 3); d != 0 {
			v.scroll -= d
		}
	}
	v.scroll = mathx.Clamp(v.scroll, 0, max(0, v.content-r.H))
	ui.PushClip(r)
	ui.PushCursor(core.Cursor{X: r.X, Y: r.Y - v.scroll, W: r.W - th.ScrollbarW, RowH: th.RowHeight, Pad: th.Padding, Space: th.Spacing + 2})
	start := ui.Cursor().Y
	value := func(k es.Key) schema.Value {
		if val, ok := v.draft[k]; ok {
			return val
		}
		return v.st.Value(k)
	}
	for _, k := range v.keys {
		if settingsui.Paired(v.st, k) {
			continue
		}
		d := v.st.Desc(k)
		ui.PushID(d.ID)
		val := value(k)
		if m := settingsui.Partner(v.st, k); m != 0 {
			mv := value(m)
			vc, mc := v.ed.Pair(ui, v.st, k, &val, m, &mv)
			if vc {
				v.draft[k] = val
			}
			if mc {
				v.draft[m] = mv
			}
		} else if v.ed.Field(ui, v.st, k, &val) {
			v.draft[k] = val
		}
		ui.PopID()
	}
	v.content = ui.Cursor().Y - start
	ui.PopCursor()
	ui.PopClip()
	track := mathx.Rect{X: r.X + r.W - th.ScrollbarW, Y: r.Y, W: th.ScrollbarW, H: r.H}
	widgets.ScrollBar(ui, ui.ID("vbar"), track, &v.scroll, v.content, r.H, &v.bar)
}

func (v *View) confirm(ui *core.Context, r mathx.Rect, left float64) {
	th := &ui.Theme
	bw := float32(90)
	ui.Text(i18n.Tr("settings.keep_display", "Keep these display settings? Reverting in {n} s", i18n.Int("n", int64(left)+1)), mathx.Vec2{X: r.X, Y: ui.TextY(r)}, th.Warn)
	if widgets.ButtonRect(ui, ui.ID("keep"), mathx.Rect{X: r.X + r.W - 2*bw - 4, Y: r.Y, W: bw, H: r.H}, i18n.Tr("settings.keep", "Keep")) {
		v.st.Confirm()
	}
	if widgets.ButtonRect(ui, ui.ID("undo"), mathx.Rect{X: r.X + r.W - bw, Y: r.Y, W: bw, H: r.H}, i18n.Tr("settings.revert", "Revert")) {
		_ = v.st.RevertPending()
	}
}

func (v *View) footer(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	bw := min(90, (r.W-12)/4)
	x := r.X + r.W
	button := func(name, label string) bool {
		x -= bw
		hit := widgets.ButtonRect(ui, ui.ID(name), mathx.Rect{X: x, Y: r.Y, W: bw, H: r.H}, label)
		x -= 4
		return hit
	}
	if button("close", i18n.Tr("settings.close", "Close")) {
		v.Close()
		return
	}
	if button("defaults", i18n.Tr("settings.defaults", "Defaults")) {
		for _, k := range v.keys {
			v.draft[k] = v.st.Base(k)
		}
	}
	if button("revert", i18n.Tr("settings.revert", "Revert")) {
		clear(v.draft)
		v.note = ""
	}
	if button("apply", i18n.Tr("settings.apply", "Apply")) && len(v.draft) > 0 {
		v.apply()
	}
	if v.note != "" {
		ui.PushClip(mathx.Rect{X: r.X, Y: r.Y, W: max(0, x-r.X), H: r.H})
		ui.Text(v.note, mathx.Vec2{X: r.X, Y: ui.TextY(r)}, th.TextDim)
		ui.PopClip()
	}
}

func (v *View) apply() {
	tx := v.st.Begin()
	confirm := false
	for k, val := range v.draft {
		if err := tx.Set(k, val); err != nil {
			v.note = err.Error()
			return
		}
		confirm = confirm || v.st.Desc(k).Flags.Has(es.Confirm)
	}
	var err error
	if confirm {
		err = tx.ApplyWithConfirm(es.DefaultConfirm)
	} else {
		err = tx.Apply()
	}
	if err != nil {
		v.note = err.Error()
		return
	}
	clear(v.draft)
	v.note = ""
	if v.st.NeedsRestart() {
		v.note = i18n.Tr("settings.restart", "Some changes apply after a restart")
	}
}

func (v *View) DrawUI(ui *core.Context, size mathx.Vec2) {
	v.ui = ui
	v.layout(size)
}

func (v *View) Select(cat string) {
	v.cat = cat
	v.ed.Cancel()
	v.refresh()
}
