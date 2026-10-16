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
		label = d.Name
	}
	if widgets.MenuItem(ui, label, d.Shortcut.String(), reg.Enabled(id), false) {
		reg.Run(id)
		return true
	}
	return false
}
