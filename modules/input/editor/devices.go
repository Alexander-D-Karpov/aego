package editor

import (
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/engine"
	"aego/modules/input"
	raw "aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	DevicesID  = input.ID + ".devices"
	FocusID    = input.ID + ".focus"
	virtualPad = uint32(0xAE900001)
	stickR     = float32(44)
)

type Devices struct {
	world  func() *engine.World
	input  func() *raw.State
	virt   bool
	held   [raw.PadButtonCount]bool
	sticks [2]mathx.Vec2
	player int
}

func NewDevices(world func() *engine.World, in func() *raw.State) *Devices {
	return &Devices{world: world, input: in}
}

func (*Devices) ID() string {
	return DevicesID
}

func (*Devices) Title() string {
	return "Input Devices"
}

func vec(v mathx.Vec2) string {
	return "(" + strconv.FormatFloat(float64(v.X), 'f', 2, 32) + ", " + strconv.FormatFloat(float64(v.Y), 'f', 2, 32) + ")"
}

func point(v mathx.Vec2) string {
	return "(" + strconv.Itoa(int(v.X)) + ", " + strconv.Itoa(int(v.Y)) + ")"
}

func padState(in *raw.State, i int) string {
	var b strings.Builder
	for k := raw.PadButton(0); k < raw.PadButtonCount; k++ {
		if in.PadDown(i, k) {
			b.WriteString(k.String() + " ")
		}
	}
	for a := raw.PadAxis(0); a < raw.PadAxisCount; a++ {
		if v := in.PadAxisValue(i, a); mathx.Abs(v) > 0.05 {
			b.WriteString(a.String() + "=" + strconv.FormatFloat(float64(v), 'f', 2, 32) + " ")
		}
	}
	return b.String()
}

func deviceName(d int) string {
	switch {
	case d < 0:
		return "none"
	case d == 0:
		return "Keyboard & Mouse"
	}
	return "Pad " + strconv.Itoa(d)
}

func (d *Devices) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	var in *raw.State
	if d.input != nil {
		in = d.input()
	}
	widgets.Label(ui, "Devices")
	if in != nil {
		widgets.LabelDim(ui, "Keyboard & Mouse   cursor "+point(in.MousePos())+"   keys down "+strconv.Itoa(in.DownCount()))
		for i := 0; i < raw.MaxPads; i++ {
			if in.PadConnected(i) {
				widgets.LabelDim(ui, "Pad "+strconv.Itoa(i+1)+": "+in.PadName(i)+"   "+padState(in, i))
			}
		}
		d.virtual(ui, in)
	}
	widgets.Separator(ui)
	d.actions(ui)
}

type padKey struct {
	b     raw.PadButton
	label string
	r     mathx.Rect
}

func padLayout(area mathx.Rect) ([]padKey, [2]mathx.Vec2) {
	const cs, gap, row = float32(26), float32(2), float32(22)
	step := cs + gap
	mid := area.Y + row + 6 + stickR + 4
	lc := mathx.Vec2{X: area.X + stickR + 4, Y: mid}
	dc := mathx.Vec2{X: lc.X + stickR + 16 + 1.5*step, Y: mid}
	fc := mathx.Vec2{X: dc.X + 3*step + 24, Y: mid}
	rc := mathx.Vec2{X: fc.X + 1.5*step + 16 + stickR, Y: mid}
	end := rc.X + stickR + 4
	cell := func(c mathx.Vec2, dx, dy float32) mathx.Rect {
		return mathx.Rect{X: c.X + dx*step - cs/2, Y: c.Y + dy*step - cs/2, W: cs, H: cs}
	}
	top := func(x, w float32) mathx.Rect {
		return mathx.Rect{X: x, Y: area.Y, W: w, H: row}
	}
	cx := (dc.X + fc.X) / 2
	keys := []padKey{
		{raw.PadLB, "LB", top(area.X, 44)},
		{raw.PadL3, "L3", top(area.X+48, 44)},
		{raw.PadBack, "Back", top(cx-78, 52)},
		{raw.PadGuide, "Guide", top(cx-24, 48)},
		{raw.PadStart, "Start", top(cx+28, 52)},
		{raw.PadR3, "R3", top(end-92, 44)},
		{raw.PadRB, "RB", top(end-44, 44)},
		{raw.PadUp, "↑", cell(dc, 0, -1)},
		{raw.PadDown, "↓", cell(dc, 0, 1)},
		{raw.PadLeft, "←", cell(dc, -1, 0)},
		{raw.PadRight, "→", cell(dc, 1, 0)},
		{raw.PadY, "Y", cell(fc, 0, -1)},
		{raw.PadA, "A", cell(fc, 0, 1)},
		{raw.PadX, "X", cell(fc, -1, 0)},
		{raw.PadB, "B", cell(fc, 1, 0)},
	}
	return keys, [2]mathx.Vec2{lc, rc}
}

func (d *Devices) SetVirtual(in *raw.State, on bool) {
	if in == nil || on == d.virt {
		return
	}
	d.virt = on
	if on {
		in.ApplyPadAdded(virtualPad, "Virtual Gamepad")
		return
	}
	in.ApplyPadRemoved(virtualPad)
	d.held = [raw.PadButtonCount]bool{}
	d.sticks = [2]mathx.Vec2{}
}

func (d *Devices) virtual(ui *core.Context, in *raw.State) {
	th := &ui.Theme
	on := d.virt
	label := "Virtual Gamepad"
	row := ui.Cursor().Next(0)
	br := mathx.Rect{X: row.X, Y: row.Y, W: ui.TextSize(label).X + 24, H: row.H}
	if widgets.ToggleButton(ui, ui.ID("vpad"), br, label, &on) {
		d.SetVirtual(in, on)
	}
	if !d.virt {
		return
	}
	area := ui.Cursor().Next(22 + 6 + 2*stickR + 8)
	keys, centers := padLayout(area)
	for _, k := range keys {
		id := core.IDSeq(ui.ID("vb"), int(k.b))
		if ui.Pressed(id, k.r) {
			ui.CaptureMouse(id)
		}
		held := ui.Active() == id && ui.MouseDown(raw.MouseLeft)
		if ui.Active() == id && !ui.MouseDown(raw.MouseLeft) {
			ui.ReleaseMouse()
		}
		col := ui.WidgetColor(id)
		if held {
			col = th.Accent
		}
		ui.Box(k.r, th.Radius.Button, col, th.Border)
		ui.TextIn(k.r, k.label, th.Text, core.AlignCenter)
		if held != d.held[k.b] {
			d.held[k.b] = held
			in.ApplyPadButton(virtualPad, k.b, held)
		}
	}
	for s := 0; s < 2; s++ {
		c := centers[s]
		box := mathx.Rect{X: c.X - stickR, Y: c.Y - stickR, W: 2 * stickR, H: 2 * stickR}
		id := core.IDSeq(ui.ID("vs"), s)
		if ui.Pressed(id, box) {
			ui.CaptureMouse(id)
		}
		var v mathx.Vec2
		if ui.Active() == id {
			if ui.MouseDown(raw.MouseLeft) {
				v = ui.Mouse().Sub(c).Scale(1 / stickR)
				if l := v.Len(); l > 1 {
					v = v.Scale(1 / l)
				}
			} else {
				ui.ReleaseMouse()
			}
		}
		ui.Box(box, stickR, th.Widget, th.Border)
		k := c.Add(v.Scale(stickR - 12))
		ui.RoundRect(mathx.Rect{X: k.X - 12, Y: k.Y - 12, W: 24, H: 24}, 12, th.Accent)
		if v != d.sticks[s] {
			d.sticks[s] = v
			ax := raw.PadAxis(s * 2)
			in.ApplyPadAxis(virtualPad, ax, v.X)
			in.ApplyPadAxis(virtualPad, ax+1, v.Y)
		}
	}
}

func valueText(i input.Info) string {
	switch i.Kind {
	case input.Button:
		if i.Down {
			return "held"
		}
		return ""
	case input.Axis1D:
		return strconv.FormatFloat(float64(i.Value.X), 'f', 2, 32)
	}
	return vec(i.Value)
}

func (d *Devices) actions(ui *core.Context) {
	th := &ui.Theme
	var s *input.Service
	if d.world != nil {
		s = input.Of(d.world())
	}
	if s == nil {
		widgets.LabelDim(ui, "Live action values appear here while Play runs")
		return
	}
	if n := s.Players(); n > 1 {
		names := make([]string, n)
		for i := range names {
			names[i] = "Player " + strconv.Itoa(i+1)
		}
		widgets.Combo(ui, ui.ID("player"), widgets.Field(ui, "Player"), &d.player, names, false)
	}
	d.player = min(d.player, s.Players()-1)
	widgets.LabelDim(ui, "Active device: "+deviceName(s.ActiveDevice(d.player)))
	s.Each(d.player, func(i input.Info) {
		r := ui.Cursor().Next(0)
		col := th.Text
		if !i.Active {
			col = th.TextDisabled
		}
		ui.PushClip(r)
		ui.Text(i.Name, mathx.Vec2{X: r.X + 4, Y: ui.TextY(r)}, col)
		bar := mathx.Rect{X: r.X + r.W*0.3, Y: r.Y + 4, W: r.W * 0.35, H: r.H - 8}
		ui.RoundRect(bar, 3, th.Widget)
		switch i.Kind {
		case input.Axis1D:
			v := mathx.Clamp(i.Value.X, -1, 1)
			mid := bar.X + bar.W/2
			w := bar.W / 2 * mathx.Abs(v)
			x := mid
			if v < 0 {
				x = mid - w
			}
			ui.Rect(mathx.Rect{X: x, Y: bar.Y, W: w, H: bar.H}, th.Accent)
		case input.Axis2D:
			ui.Rect(mathx.Rect{X: bar.X, Y: bar.Y, W: bar.W * mathx.Clamp(i.Value.Len(), 0, 1), H: bar.H}, th.Accent)
		default:
			if i.Down {
				ui.RoundRect(bar, 3, th.Accent)
			}
		}
		ui.Text(valueText(i)+"   "+i.Glyph, mathx.Vec2{X: bar.X + bar.W + 8, Y: ui.TextY(r)}, th.TextDim)
		ui.PopClip()
	})
}
