//go:build js

package web

import (
	"syscall/js"

	"aego/platform/input"
)

func (p *Platform) anyPad() bool {
	for i := range p.pads {
		if p.pads[i].on {
			return true
		}
	}
	return false
}

func (p *Platform) pollPads() {
	if !p.gamepads.Truthy() || (p.padLive == 0 && !p.anyPad()) {
		return
	}
	list := p.gamepads.Invoke()
	n := 0
	if list.Truthy() {
		n = list.Length()
	}
	for i := range p.pads {
		f := &p.pad
		*f = padFrame{}
		if i < n {
			readPad(list.Index(i), f, !p.pads[i].on)
		}
		p.queue = p.pads[i].diff(uint32(i+1), f, p.queue)
	}
}

func readPad(gp js.Value, f *padFrame, named bool) {
	if !gp.Truthy() || !gp.Get("connected").Truthy() {
		return
	}
	f.connected = true
	if named {
		f.name = gp.Get("id").String()
	}
	buttons := gp.Get("buttons")
	nb := buttons.Length()
	for _, m := range standardPad {
		if m.index < nb {
			f.buttons[m.button] = buttons.Index(m.index).Get("pressed").Truthy()
		}
	}
	if padLT < nb {
		f.axes[input.PadLT] = float32(buttons.Index(padLT).Get("value").Float())
	}
	if padRT < nb {
		f.axes[input.PadRT] = float32(buttons.Index(padRT).Get("value").Float())
	}
	axes := gp.Get("axes")
	na := axes.Length()
	for k, a := range standardAxes {
		if k < na {
			f.axes[a] = float32(axes.Index(k).Float())
		}
	}
}
