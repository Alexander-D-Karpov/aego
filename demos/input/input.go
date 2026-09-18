package input

import (
	"strings"

	"aego/core/log"
	"aego/demos"
	"aego/engine"
	"aego/platform"
	pinput "aego/platform/input"
)

func init() {
	demos.Register("input", func() demos.Demo { return New() })
}

type Demo struct {
	demos.Base
	e     *engine.Engine
	log   *log.ChannelLogger
	sb    strings.Builder
	title string
}

func New() *Demo {
	return &Demo{}
}

func (d *Demo) Name() string {
	return "input"
}

func (d *Demo) Init(e *engine.Engine) error {
	d.e = e
	d.log = e.Log().With(log.Game)
	d.log.Info("press keys, move the mouse, alt-tab away to verify the focus reset")
	if w, ok := e.Window().(interface{ SetTextInput(bool) }); ok {
		w.SetTextInput(true)
	}
	return nil
}

func (d *Demo) Event(ev platform.Event) {
	switch ev.Kind {
	case platform.EvKeyDown, platform.EvKeyUp:
		d.log.Info(ev.Kind.String(),
			log.F("key", ev.Key.Scan.String()),
			log.F("scancode", uint32(ev.Key.Scan)),
			log.F("keycode", uint32(ev.Key.Key)),
			log.F("mods", uint32(ev.Key.Mods)),
			log.F("repeat", ev.Key.Repeat))
	case platform.EvTextInput:
		d.log.Info("text", log.F("value", string(ev.TextBytes())))
	case platform.EvMouseDown, platform.EvMouseUp:
		d.log.Info(ev.Kind.String(),
			log.F("button", uint32(ev.Mouse.Button)),
			log.F("x", ev.Mouse.Pos.X), log.F("y", ev.Mouse.Pos.Y))
	case platform.EvMouseWheel:
		d.log.Info("wheel", log.F("x", ev.Mouse.Wheel.X), log.F("y", ev.Mouse.Wheel.Y))
	case platform.EvWindowFocusGained:
		d.log.Info("focus gained")
	case platform.EvWindowFocusLost:
		down := 0
		d.e.Input().EachDown(func(pinput.Scancode) { down++ })
		d.log.Info("focus lost", log.F("keys_down_after_reset", down))
	}
}

func (d *Demo) Update(float64) {
	in := d.e.Input()
	if in.KeyPressed(pinput.KeyEscape) {
		d.e.Quit()
	}
	d.sb.Reset()
	d.sb.WriteString("input | mouse ")
	writeFloat(&d.sb, in.MousePos().X)
	d.sb.WriteByte(',')
	writeFloat(&d.sb, in.MousePos().Y)
	d.sb.WriteString(" | keys:")
	in.EachDown(func(s pinput.Scancode) {
		d.sb.WriteByte(' ')
		d.sb.WriteString(s.String())
	})
	if m := in.Mods(); m != 0 {
		d.sb.WriteString(" | mods:")
		for _, mod := range [...]struct {
			bit  pinput.Mods
			name string
		}{
			{pinput.ModShift, " shift"}, {pinput.ModCtrl, " ctrl"}, {pinput.ModAlt, " alt"},
			{pinput.ModSuper, " super"}, {pinput.ModCaps, " caps"}, {pinput.ModNum, " num"},
		} {
			if m.Has(mod.bit) {
				d.sb.WriteString(mod.name)
			}
		}
	}
	if t := d.sb.String(); t != d.title {
		d.title = t
		d.e.Window().SetTitle(t)
	}
}

func writeFloat(sb *strings.Builder, v float32) {
	n := int(v)
	if n < 0 {
		sb.WriteByte('-')
		n = -n
	}
	var digits [12]byte
	i := len(digits)
	for {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
		if n == 0 {
			break
		}
	}
	sb.Write(digits[i:])
}
