package app

import (
	"aego/editor/command"
	"aego/editor/search"
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
	a.search.Open(a.ctx, tab)
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
	a.search.Draw(a.ctx)
}
