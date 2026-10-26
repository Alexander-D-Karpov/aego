package shell

import (
	"time"

	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/sysopen"
	"aego/editor/core/tasks"
	"aego/editor/core/updateui"
	"aego/engine/i18n"
	"aego/update"
)

const (
	CommandCheckUpdates   = "help.updates.check"
	CommandRestoreVersion = "help.updates.restore"
	CommandUpdatesNightly = "help.updates.nightly"
	commandUpdatesMode    = "help.updates.mode"
)

// tickUpdates lets the update service check on time, shows its download in the Task Center and says when an update was rolled back.
func (a *App) tickUpdates() {
	u := a.opt.Updates
	if u == nil {
		return
	}
	u.Tick(time.Now())
	st := u.State()
	if st.RolledBack != "" && !a.rollbackSaid {
		a.rollbackSaid = true
		a.ctx.Notify(context.LevelWarn, i18n.Note("editor.updates.rolled_back", "Aego {version} stopped right after it started, so the previous version was restored", i18n.Str("version", st.RolledBack)))
	}
	if st.Phase == updateui.Downloading && a.updateTask == nil && a.tasks != nil {
		a.updateTask = a.tasks.Work(tasks.Spec{Title: i18n.Tr("editor.updates.task", "Downloading Aego {version}", i18n.Str("version", st.Version)), Kind: "update"}, func(t *tasks.Task) error {
			for {
				s := u.State()
				if s.Total > 0 {
					t.SetProgress(float32(s.Done) / float32(s.Total))
				}
				if s.Phase != updateui.Downloading {
					return nil
				}
				time.Sleep(200 * time.Millisecond)
			}
		})
	}
	if st.Phase != updateui.Downloading {
		a.updateTask = nil
	}
}

// updateStatus is the status bar's word on an update: empty when there is nothing to say.
func (a *App) updateStatus() string {
	u := a.opt.Updates
	if u == nil {
		return ""
	}
	st := u.State()
	if st.Hidden {
		return ""
	}
	switch st.Phase {
	case updateui.Available:
		return i18n.Tr("editor.updates.available", "Aego {version} is available", i18n.Str("version", st.Version))
	case updateui.Ready:
		return i18n.Tr("editor.updates.ready", "Aego {version} downloaded, restart to update", i18n.Str("version", st.Version))
	}
	return ""
}

// askUpdate offers the downloaded version: Restart to Update, Later, Skip This Version, Release Notes.
func (a *App) askUpdate() {
	u := a.opt.Updates
	st := u.State()
	restart := i18n.Tr("editor.updates.restart", "Restart to Update")
	if st.Phase == updateui.Available {
		restart = i18n.Tr("editor.updates.download", "Download")
		if !st.Installable {
			restart = i18n.Tr("editor.updates.page", "Get It")
		}
	}
	buttons := []string{restart, i18n.Tr("editor.updates.later", "Later"), i18n.Tr("editor.updates.skip", "Skip This Version")}
	msg := st.Notes
	if msg == "" {
		msg = i18n.Tr("editor.updates.no_notes", "The release has no notes.")
	}
	a.ctx.Ask(context.Question{
		Title:   i18n.Tr("editor.updates.title", "Aego {version}", i18n.Str("version", st.Version)),
		Message: msg,
		Buttons: buttons,
		Then: func(choice int) {
			switch choice {
			case 0:
				switch {
				case st.Phase == updateui.Ready:
					a.restartForUpdate(true)
				case st.Installable:
					u.Download()
				default:
					if err := sysopen.Open(u.Page()); err != nil {
						a.ctx.Notify(context.LevelError, err.Error())
					}
				}
			case 1:
				u.Later()
			case 2:
				u.Skip()
			}
		},
	})
}

// restartForUpdate installs the downloaded version, or goes back to the previous one, and opens this project again on it; unsaved work is asked about first.
func (a *App) restartForUpdate(install bool) {
	u := a.opt.Updates
	do := func() {
		relaunch := true
		var err error
		if install {
			relaunch, err = u.Install()
		} else {
			relaunch, err = u.Restore()
		}
		if err != nil {
			a.ctx.Notify(context.LevelError, i18n.Note("editor.updates.failed", "Cannot update Aego: {error}", i18n.Str("error", err.Error())))
			return
		}
		if !relaunch {
			a.quitting = true
			a.eng.Quit()
			return
		}
		root := ""
		if a.ctx.Project != nil {
			root = a.ctx.Project.Root
		}
		a.leave(root, "")
	}
	if !a.ctx.Docs.Dirty() {
		do()
		return
	}
	a.ctx.Ask(context.Question{
		Title:   i18n.Tr("editor.shell.unsaved_changes", "Unsaved changes"),
		Message: i18n.Tr("editor.updates.save_first", "Save the changed scenes and scripts before Aego restarts?"),
		Buttons: []string{i18n.Tr("editor.shell.save_and_continue", "Save and Continue"), i18n.Tr("editor.shell.discard", "Discard"), i18n.Tr("editor.shell.cancel", "Cancel")},
		Then: func(choice int) {
			switch choice {
			case 0:
				if err := a.saveAll(); err != nil {
					a.ctx.Notify(context.LevelError, err.Error())
					return
				}
				do()
			case 1:
				do()
			}
		},
	})
}

// updateCommands are Help's update choices: check now, the mode, the nightly channel and going back to the previous version.
func (a *App) updateCommands() []command.Desc {
	u := a.opt.Updates
	if u == nil {
		return nil
	}
	mode := func(m update.Mode) (func() bool, func() error) {
		return func() bool { return u.Policy().Mode == m }, func() error { u.SetMode(m); return nil }
	}
	autoOn, autoRun := mode(update.Auto)
	askOn, askRun := mode(update.Ask)
	offOn, offRun := mode(update.Off)
	return []command.Desc{
		{ID: CommandCheckUpdates, Name: "Check for Updates", Category: "Help", Order: 880, Run: func() error {
			u.Check()
			a.ctx.Notify(context.LevelInfo, i18n.Note("editor.updates.checking", "Checking for updates"))
			return nil
		}},
		{ID: CommandRestoreVersion, Name: "Restore Previous Version", Category: "Help", Order: 885, Enabled: u.CanRestore,
			Tip: "Go back to the version this one updated from", Run: func() error { a.restartForUpdate(false); return nil }},
		{ID: CommandUpdatesNightly, Name: "Nightly Updates", Category: "Help", Order: 889,
			Tip:     "Update to a build of every master commit instead of stable versions",
			Checked: func() bool { return u.Policy().Channel == update.Nightly },
			Run: func() error {
				c := update.Nightly
				if u.Policy().Channel == update.Nightly {
					c = update.Stable
				}
				u.SetChannel(c)
				return nil
			}},
		{ID: commandUpdatesMode + ".auto", Name: "Install Updates on Restart", Category: "Help", Order: 886, Checked: autoOn, Run: autoRun},
		{ID: commandUpdatesMode + ".ask", Name: "Ask Before Downloading Updates", Category: "Help", Order: 887, Checked: askOn, Run: askRun},
		{ID: commandUpdatesMode + ".off", Name: "Check for Updates Only When Asked", Category: "Help", Order: 888, Checked: offOn, Run: offRun},
	}
}
