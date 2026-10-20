package shell

import (
	"aego/core/ids"
	"aego/editor/core/command"
	"aego/editor/panels/references"
	"aego/platform/input"
)

func (a *App) showReferences(id ids.AssetID, deleting []ids.AssetID) {
	a.refsPanel.Show(id, deleting)
	a.ws.Show(references.ID)
}

func (a *App) registerReferences() error {
	return a.ctx.Commands.Register(command.Desc{
		ID: references.CommandFind, Name: "Find References", Category: "Assets", Order: 220,
		Shortcut: command.Key(input.KeyF12, command.ModShift),
		Enabled:  func() bool { return len(a.assetsPanel.Selection()) == 1 },
		Run: func() error {
			if sel := a.assetsPanel.Selection(); len(sel) == 1 {
				a.showReferences(sel[0], nil)
			}
			return nil
		},
	})
}
