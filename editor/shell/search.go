package shell

import (
	"aego/editor/core/command"
	"aego/editor/panels/search"
	"aego/platform"
	"aego/platform/input"
)

const doubleShift = 0.4

func (a *App) registerSearch() error {
	return a.ctx.Commands.Register(command.Desc{
		ID:       search.CommandOpen,
		Name:     "Search Everywhere",
		Category: "Edit",
		Order:    400,
		Shortcut: command.Key(input.KeyP, command.ModPrimary|command.ModShift),
		Hint:     "Shift Shift",
		Run: func() error {
			a.openSearch(search.TabAll)
			return nil
		},
	})
}

func (a *App) openSearch(tab search.Tab) {
	if len(a.modals) > 0 || a.play.Captured() || a.search.Active() {
		return
	}
	a.search.Open(a.ui, tab)
}

func (a *App) paletteKey(ev platform.Event) {
	if ev.Key.Repeat {
		return
	}
	d, ok := a.ctx.Commands.Get(search.CommandOpen)
	if ok && d.Matches(command.Shortcut{Key: ev.Key.Scan, Mods: command.FromInput(ev.Key.Mods)}) {
		a.openSearch(search.TabAll)
	}
}

func (a *App) shiftTap(ev platform.Event) {
	k := ev.Key.Scan
	if k != input.KeyLShift && k != input.KeyRShift {
		a.shiftArmed = false
		return
	}
	if ev.Key.Repeat {
		return
	}
	now := a.eng.Platform().Now()
	if a.shiftArmed && now-a.shiftAt < doubleShift {
		a.shiftArmed = false
		a.openSearch(search.TabAll)
		return
	}
	a.shiftArmed, a.shiftAt = true, now
}

func (a *App) drawSearch() {
	if len(a.modals) > 0 {
		a.search.Close(a.ui)
		return
	}
	a.search.Draw(a.ui)
}
