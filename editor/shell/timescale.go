package shell

import (
	"strconv"
	"strings"

	"aego/core/clock"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/play"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

var timeScales = [...]float64{0.1, 0.25, 0.5, 1, 2, 4}

func scaleLabel(s float64) string {
	return "×" + strconv.FormatFloat(s, 'g', 3, 64)
}

func (a *App) registerTime() error {
	reg := a.ctx.Commands
	list := []command.Desc{{
		ID: command.PlayStepFixed, Name: "Step Fixed Tick", Category: "Play", Order: 125,
		Shortcut: command.DefaultShortcuts()[command.PlayStepFixed],
		Enabled:  func() bool { return a.play.State() == play.Paused },
		Run: func() error {
			a.play.StepFixed()
			return nil
		},
	}}
	for i, s := range timeScales {
		list = append(list, command.Desc{
			ID: command.PlayScalePrefix + strconv.Itoa(i), Name: "Time Scale " + scaleLabel(s), Category: "Play", Order: 200 + i,
			Checked: func() bool { return a.play.TimeScale() == s },
			Run: func() error {
				a.play.SetTimeScale(s)
				return nil
			},
		})
	}
	list = append(list, command.Desc{
		ID: command.PlayScalePrefix + "custom", Name: "Time Scale…", Category: "Play", Order: 290,
		Run: func() error {
			a.askTimeScale()
			return nil
		},
	})
	list = append(list, command.Desc{
		ID: command.PlayReleaseInput, Name: "Release Game Input", Category: "Play", Order: 150,
		Shortcut: command.Key(input.KeyEscape, command.ModShift),
		Enabled:  a.play.Captured,
		Run: func() error {
			a.gameView.Release()
			return nil
		},
	})
	for _, d := range list {
		if err := reg.Register(d); err != nil {
			return err
		}
	}
	a.play.SetRelease(
		func(in *input.State) bool { return reg.Pressed(command.PlayReleaseInput, in) },
		func() string {
			d, ok := reg.Get(command.PlayReleaseInput)
			if !ok {
				return ""
			}
			return d.KeysText()
		},
	)
	return nil
}

func (a *App) askTimeScale() {
	a.ctx.Ask(context.Question{
		Title:   i18n.Tr("editor.shell.time_scale", "Time scale"),
		Message: i18n.Tr("editor.shell.from_0_to_8_1", "From 0 to 8, 1 is normal speed"),
		Fields:  []string{i18n.Tr("editor.shell.scale", "Scale")},
		Values:  []string{strconv.FormatFloat(a.play.TimeScale(), 'g', -1, 64)},
		Buttons: []string{i18n.Tr("editor.shell.set", "Set"), i18n.Tr("editor.shell.cancel", "Cancel")},
		Submit: func(v []string) {
			s, err := strconv.ParseFloat(strings.TrimSpace(v[0]), 64)
			if err != nil || s < 0 || s > clock.MaxScale {
				a.ctx.Notify(context.LevelWarn, i18n.Note("editor.shell.time_scale_must_be_a", "Time scale must be a number from 0 to 8"))
				return
			}
			a.play.SetTimeScale(s)
		},
	})
}

func (a *App) scalePopup(id core.ID) {
	ui := a.ui
	if !ui.BeginPopup(id, 120) {
		return
	}
	cur := a.play.TimeScale()
	for _, s := range timeScales {
		if widgets.MenuItem(ui, scaleLabel(s), "", true, cur == s) {
			a.play.SetTimeScale(s)
		}
	}
	widgets.Separator(ui)
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.shell.custom", "Custom…"), "", "play.speed", true, false) {
		a.askTimeScale()
	}
	ui.EndPopup()
}

func (a *App) playStatus() (string, mathx.Color) {
	th := &a.ui.Theme
	st := a.play.Stats()
	tail := " · Tick " + strconv.FormatUint(st.Tick, 10) + " · Frame " + strconv.FormatUint(st.Updates, 10) + " · " + scaleLabel(a.play.TimeScale())
	switch a.play.State() {
	case play.Playing:
		return i18n.Tr("editor.shell.play_simtime_tail", "Play {simtime}{tail}", i18n.Str("simtime", clockText(a.play.SimTime())), i18n.Str("tail", tail)), th.Accent
	case play.Paused:
		return i18n.Tr("editor.shell.paused_simtime_tail", "Paused {simtime}{tail}", i18n.Str("simtime", clockTextFine(a.play.SimTime())), i18n.Str("tail", tail)), th.Warn
	}
	return i18n.Tr("editor.shell.state_edit", "Edit"), th.TextDim
}
