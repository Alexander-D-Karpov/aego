package scripteditor

import (
	"aego/editor/core/command"
	"aego/editor/core/context"
)

const CommandZen = "code.zen"

func (h *Host) registerZen() {
	err := h.d.Commands.Register(command.Desc{
		ID: CommandZen, Name: "Zen Mode", Category: "Scripts", Order: 370,
		Enabled: func() bool { return h.ZenOn() || h.owns(h.active) },
		Checked: h.ZenOn,
		Run: func() error {
			h.ToggleZen()
			return nil
		},
	})
	if err != nil {
		h.d.Notices.Notify(context.LevelError, err.Error())
	}
}

func (h *Host) ZenOn() bool {
	if h.zen == "" || h.dock == nil {
		return false
	}
	if h.dock.Tree.Maximized() != h.zen {
		h.zen = ""
		return false
	}
	return true
}

func (h *Host) ToggleZen() {
	if h.ZenOn() {
		h.dock.Tree.Restore()
		h.zen = ""
		return
	}
	p := h.active
	if !h.owns(p) {
		return
	}
	h.dock.Show(p.ID())
	if h.dock.Tree.Maximize(p.ID()) {
		h.zen = p.ID()
		p.wantFocus = true
	}
}

func (p *Panel) inZen() bool {
	return p.host.ZenOn() && p.host.zen == p.ID()
}
