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

// virtualSlot is the pad slot of the page's touch gamepad, globalThis.aegoVirtualPad in the standard mapping.
const virtualSlot = input.MaxPads - 1

func (p *Platform) pollPads() {
	vp := p.global.Get("aegoVirtualPad")
	virt := vp.Truthy() && vp.Get("connected").Truthy()
	if !virt && (!p.gamepads.Truthy() || (p.padLive == 0 && !p.anyPad())) {
		return
	}
	var list js.Value
	n := 0
	if p.gamepads.Truthy() && (p.padLive > 0 || p.anyPad()) {
		if list = p.gamepads.Invoke(); list.Truthy() {
			n = list.Length()
		}
	}
	for i := range p.pads {
		f := &p.pad
		*f = padFrame{}
		switch {
		case i == virtualSlot && virt:
			readPad(vp, f, !p.pads[i].on)
		case i < n:
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
