package shell

import (
	"aego/editor/core/command"
	pd "aego/editor/core/playdiff"
)

const CommandPlayDiffMode = "play.diff_mode"

func (a *App) beforeStop() {
	m := a.ctx.Docs
	play := m.PlayDoc()
	if play == nil || a.diffMode == pd.ModeNever {
		return
	}
	edit := m.Get(play.ID)
	if edit == nil {
		return
	}
	hist := pd.FromHistory(edit, play)
	if hist.Empty() && len(hist.Created)+len(hist.Removed) == 0 {
		return
	}
	a.diffPending, a.diffDoc, a.diffRes, a.diffFull = true, edit.ID, hist, pd.Full(edit, play, hist)
}

func (a *App) showPlayDiff() {
	if !a.diffPending || a.ctx.Docs.InPlay() {
		return
	}
	a.diffPending = false
	a.playDiff.Open(a.ctx.UI, a.diffDoc, a.diffRes, a.diffFull)
	a.diffRes, a.diffFull = pd.Result{}, pd.Result{}
}

func (a *App) registerPlayDiff() error {
	for i, m := range []pd.Mode{pd.ModeIfChanges, pd.ModeAlways, pd.ModeNever} {
		mode := m
		names := [...]string{"Show Play Changes When Something Changed", "Always Show Play Changes", "Never Show Play Changes"}
		if err := a.ctx.Commands.Register(command.Desc{
			ID: CommandPlayDiffMode + "." + mode.String(), Name: names[i], Category: "Play", Order: 520 + i,
			Checked: func() bool { return a.diffMode == mode },
			Run:     func() error { a.diffMode = mode; return nil },
		}); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) PlayDiffOpen() bool {
	return a.playDiff.Active() || a.diffPending
}
