package input

import (
	"aego/core/mathx"
	raw "aego/platform/input"
)

type sample struct {
	v                       float32
	down, pressed, released bool
}

type reader struct {
	in   *raw.State
	kbm  bool
	pads uint8
}

func press(d, p, r bool) sample {
	s := sample{down: d, pressed: p, released: r}
	if d {
		s.v = 1
	}
	return s
}

func (r *reader) read(c Control) sample {
	in := r.in
	if in == nil {
		return sample{}
	}
	switch c.Device {
	case DevKey:
		if r.kbm {
			k := raw.Scancode(c.Code)
			return press(in.KeyDown(k), in.KeyPressed(k), in.KeyReleased(k))
		}
	case DevMouse:
		if r.kbm {
			b := raw.Button(c.Code)
			return press(in.ButtonDown(b), in.ButtonPressed(b), in.ButtonReleased(b))
		}
	case DevMouseAxis:
		if r.kbm {
			var v float32
			switch c.Code {
			case MouseX:
				v = in.MouseDelta().X
			case MouseY:
				v = in.MouseDelta().Y
			default:
				v = in.Wheel().Y
			}
			return sample{v: v, down: v != 0}
		}
	case DevPadButton:
		var out sample
		b := raw.PadButton(c.Code)
		for i := 0; i < raw.MaxPads; i++ {
			if r.pads&(1<<i) != 0 {
				out.down = out.down || in.PadDown(i, b)
				out.pressed = out.pressed || in.PadPressed(i, b)
				out.released = out.released || in.PadReleased(i, b)
			}
		}
		if out.down {
			out.v = 1
		}
		return out
	case DevPadAxis:
		var out sample
		a := raw.PadAxis(c.Code)
		for i := 0; i < raw.MaxPads; i++ {
			if r.pads&(1<<i) != 0 {
				if v := in.PadAxisValue(i, a); mathx.Abs(v) > mathx.Abs(out.v) {
					out.v = v
				}
			}
		}
		out.down = mathx.Abs(out.v) >= PressPoint
		return out
	}
	return sample{}
}

func (r *reader) stick(code uint16) mathx.Vec2 {
	var best mathx.Vec2
	if r.in == nil {
		return best
	}
	ax, ay := raw.PadAxis(code*2), raw.PadAxis(code*2+1)
	for i := 0; i < raw.MaxPads; i++ {
		if r.pads&(1<<i) == 0 {
			continue
		}
		v := mathx.Vec2{X: r.in.PadAxisValue(i, ax), Y: r.in.PadAxisValue(i, ay)}
		if v.Len() > best.Len() {
			best = v
		}
	}
	return best
}

type mask struct {
	keys  [raw.ScancodeCount]int8
	mouse [raw.ButtonCount]int8
	maxis [3]int8
	pbtn  [raw.MaxPads][raw.PadButtonCount]int8
	paxis [raw.MaxPads][raw.PadAxisCount]int8
}

func (m *mask) owner(c Control, pads uint8) int8 {
	switch c.Device {
	case DevNone:
		return 0
	case DevKey:
		return m.keys[c.Code]
	case DevMouse:
		return m.mouse[c.Code]
	case DevMouseAxis:
		return m.maxis[c.Code]
	}
	for i := 0; i < raw.MaxPads; i++ {
		if pads&(1<<i) == 0 {
			continue
		}
		var o int8
		switch c.Device {
		case DevPadButton:
			o = m.pbtn[i][c.Code]
		case DevPadAxis:
			o = m.paxis[i][c.Code]
		case DevStick:
			o = max(m.paxis[i][c.Code*2], m.paxis[i][c.Code*2+1])
		}
		if o != 0 {
			return o
		}
	}
	return 0
}

func claim(slot *int8, owner int8) {
	if *slot == 0 {
		*slot = owner
	}
}

func (m *mask) set(c Control, pads uint8, owner int8) {
	switch c.Device {
	case DevNone:
		return
	case DevKey:
		claim(&m.keys[c.Code], owner)
		return
	case DevMouse:
		claim(&m.mouse[c.Code], owner)
		return
	case DevMouseAxis:
		claim(&m.maxis[c.Code], owner)
		return
	}
	for i := 0; i < raw.MaxPads; i++ {
		if pads&(1<<i) == 0 {
			continue
		}
		switch c.Device {
		case DevPadButton:
			claim(&m.pbtn[i][c.Code], owner)
		case DevPadAxis:
			claim(&m.paxis[i][c.Code], owner)
		case DevStick:
			claim(&m.paxis[i][c.Code*2], owner)
			claim(&m.paxis[i][c.Code*2+1], owner)
		}
	}
}

func (m *mask) blocked(b *Binding, pads uint8) int8 {
	if o := m.owner(b.Mod, pads); o != 0 {
		return o
	}
	for i := 0; i < b.count(); i++ {
		if o := m.owner(b.Parts[i], pads); o != 0 {
			return o
		}
	}
	return 0
}

func (m *mask) take(a *Action, pads uint8, owner int8) {
	for i := range a.Bindings {
		b := &a.Bindings[i]
		m.set(b.Mod, pads, owner)
		for j := 0; j < b.count(); j++ {
			m.set(b.Parts[j], pads, owner)
		}
	}
}
