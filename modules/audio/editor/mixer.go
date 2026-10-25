package editor

import (
	"math"
	"strconv"

	"aego/audio/mix"
	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/engine/i18n"
	"aego/modules/audio"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	stripW   = 84
	stripGap = 6
	meterW   = 10
	floorDB  = -60
	holdFall = 0.6
	buttonW  = 120
)

type mixerPanel struct {
	p      *plugin
	pct    [101]string
	hold   [6]float32
	last   float64
	stats  string
	voices int
	under  uint64
}

func newMixer(p *plugin) *mixerPanel {
	m := &mixerPanel{p: p}
	for i := range m.pct {
		m.pct[i] = strconv.Itoa(i) + "%"
	}
	return m
}

func (m *mixerPanel) ID() string {
	return workspace.Mixer
}

func (m *mixerPanel) Title() string {
	return i18n.Tr("i18n.audio.mixer.title", "Audio Mixer")
}

func level(x float32) float32 {
	if x <= 0 {
		return 0
	}
	db := 20 * math.Log10(float64(x))
	return mathx.Clamp(float32((db-floorDB)/-floorDB), 0, 1)
}

func meterColor(lv float32) mathx.Color {
	switch {
	case lv > 0.95:
		return colHigh
	case lv > 0.8:
		return colMid
	}
	return colLow
}

func (m *mixerPanel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	mod := m.p.module()
	if mod == nil || mod.Mixer() == nil {
		widgets.LabelDim(ui, i18n.Tr("i18n.audio.the_audio_output_is_not", "The audio output is not open"))
		return
	}
	mx := mod.Mixer()
	var dt float32
	if m.last > 0 {
		dt = float32(f.Now - m.last)
	}
	m.last = f.Now
	m.header(ui, mod)
	pad := ui.Theme.Padding
	top := ui.Cursor().Y
	body := mathx.Rect{X: r.X + pad, Y: top, W: r.W - 2*pad, H: r.Y + r.H - top - pad}
	if body.H < 100 {
		return
	}
	ui.PushClip(body)
	for i := range audio.Buses {
		x := body.X + float32(i)*(stripW+stripGap)
		if i > 0 {
			x += stripGap
		}
		m.strip(ui, mod, mx, i, mathx.Rect{X: x, Y: body.Y, W: stripW, H: body.H}, dt)
	}
	ui.PopClip()
}

func (m *mixerPanel) header(ui *core.Context, mod *audio.Module) {
	st := mod.Stats()
	if m.stats == "" || st.Voices != m.voices || st.Underruns != m.under {
		m.voices, m.under = st.Voices, st.Underruns
		m.stats = "Voices " + strconv.Itoa(st.Voices) + "   Underruns " + strconv.FormatUint(st.Underruns, 10)
	}
	row := ui.Cursor().Next(0)
	ui.Text(m.stats, mathx.Vec2{X: row.X, Y: ui.TextY(row)}, ui.Theme.TextDim)
	save := mathx.Rect{X: row.X + row.W - buttonW, Y: row.Y, W: buttonW, H: row.H}
	reset := mathx.Rect{X: save.X - buttonW - 4, Y: row.Y, W: buttonW, H: row.H}
	if widgets.ButtonRect(ui, ui.ID("reset"), reset, i18n.Tr("i18n.audio.reset_session", "Reset Session")) {
		mod.ResetSession()
	}
	if widgets.ButtonRect(ui, ui.ID("save"), save, i18n.Tr("i18n.audio.save_to_profile", "Save to Profile")) {
		if err := mod.SaveSession(); err != nil {
			m.p.notify(err)
		} else {
			m.p.info("Bus volumes saved to the user settings")
		}
	}
}

func (m *mixerPanel) strip(ui *core.Context, mod *audio.Module, mx *mix.Mixer, i int, r mathx.Rect, dt float32) {
	th := &ui.Theme
	b := audio.Bus(i)
	border := th.Border
	if i == 0 {
		border = th.AccentDim
	}
	ui.Box(r, th.Radius.Button, th.Widget, border)
	rh := th.RowHeight
	name := mathx.Rect{X: r.X, Y: r.Y + 2, W: r.W, H: rh}
	ui.TextIn(name, audio.BusTitle(i), th.Text, core.AlignCenter)
	btn := mathx.Rect{X: r.X + 4, Y: r.Y + r.H - rh - 4, W: (r.W - 12) / 2, H: rh}
	muted := mod.BusMuted(b)
	if widgets.ToggleButton(ui, core.IDSeq(ui.ID("mute"), i), btn, "M", &muted) {
		mod.SetBusMuted(b, muted)
	}
	if i > 0 {
		btn.X += btn.W + 4
		solo := mod.BusSolo(b)
		if widgets.ToggleButton(ui, core.IDSeq(ui.ID("solo"), i), btn, "S", &solo) {
			mod.SetBusSolo(b, solo)
		}
	}
	v := mod.BusVolume(b)
	val := mathx.Rect{X: r.X, Y: btn.Y - rh, W: r.W, H: rh}
	ui.TextIn(val, m.pct[int(mathx.Clamp(v, 0, 1)*100+0.5)], th.TextDim, core.AlignCenter)
	area := mathx.Rect{X: r.X + 8, Y: name.Y + rh + 4, W: r.W - 16, H: val.Y - name.Y - rh - 8}
	if area.H < 20 {
		return
	}
	meter := mathx.Rect{X: area.X, Y: area.Y, W: meterW, H: area.H}
	peak, rms := mx.Meters(i)
	m.hold[i] = max(level(peak), m.hold[i]-dt*holdFall)
	ui.Rect(meter, th.Background)
	lv := level(rms)
	ui.Rect(mathx.Rect{X: meter.X, Y: meter.Y + meter.H*(1-lv), W: meter.W, H: meter.H * lv}, meterColor(lv))
	ui.Rect(mathx.Rect{X: meter.X, Y: meter.Y + meter.H*(1-m.hold[i]), W: meter.W, H: 2}, meterColor(m.hold[i]))
	ui.Outline(meter, th.Border)
	track := mathx.Rect{X: meter.X + meter.W + 12, Y: area.Y, W: area.W - meter.W - 12, H: area.H}
	m.fader(ui, mod, b, i, track, v)
}

func (m *mixerPanel) fader(ui *core.Context, mod *audio.Module, b audio.Bus, i int, r mathx.Rect, v float32) {
	th := &ui.Theme
	id := core.IDSeq(ui.ID("fader"), i)
	ui.RoundRect(mathx.Rect{X: r.X + r.W/2 - 2, Y: r.Y, W: 4, H: r.H}, 2, th.Background)
	if d, ok := ui.Drag(id, r); ok && d.Y != 0 && r.H > 0 {
		v = mathx.Clamp(v-d.Y/r.H, 0, 1)
		mod.SetBusVolume(b, v)
	}
	if ui.DoubleClicked(id) {
		v = 1
		mod.SetBusVolume(b, v)
	}
	ky := r.Y + r.H*(1-v)
	ui.Box(mathx.Rect{X: r.X, Y: ky - 5, W: r.W, H: 10}, th.Radius.Button, ui.WidgetColor(id), th.Border)
}
