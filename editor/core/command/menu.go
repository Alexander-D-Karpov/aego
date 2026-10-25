package command

import (
	"aego/ui/core"
	"aego/ui/widgets"
)

func MenuItem(ui *core.Context, reg *Registry, id, label string) bool {
	d, ok := reg.Get(id)
	if !ok {
		return false
	}
	if label == "" {
		label = d.Title()
	}
	it := widgets.Item{
		Label:    label,
		Shortcut: d.Shortcut.String(),
		Icon:     d.Icon,
		Tip:      d.TipText(),
		Enabled:  reg.Enabled(id),
		Checked:  d.Checked != nil && d.Checked(),
	}
	if widgets.Entry(ui, it) {
		reg.Run(id)
		return true
	}
	return false
}
