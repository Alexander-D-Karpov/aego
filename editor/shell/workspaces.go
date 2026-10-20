package shell

import (
	"strconv"

	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/play"
	"aego/editor/core/workspace"
	"aego/editor/panels/events"
	"aego/editor/panels/watch"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	CommandWorkspacePrefix  = "window.workspace."
	CommandWorkspaceSaveAs  = "window.workspace_save_as"
	CommandWorkspaceRename  = "window.workspace_rename"
	CommandWorkspaceDelete  = "window.workspace_delete"
	CommandWorkspaceToolbar = "window.toolbar"
	CommandMaximize         = "window.maximize"
	CommandFullscreenPlay   = "play.fullscreen"
)

var workspaceKeys = [...]input.Scancode{input.Key1, input.Key2, input.Key3, input.Key4}

func WorkspaceCommand(i int) string {
	return CommandWorkspacePrefix + strconv.Itoa(i+1)
}

func BuiltinWorkspaces() []workspace.Preset {
	off := workspace.SlotNone
	return []workspace.Preset{
		{ID: workspace.Design, Title: "Design", Order: 0},
		{ID: workspace.Programming, Title: "Programming", Order: 10, Slots: map[string]workspace.Slot{
			workspace.Hierarchy: off, workspace.Game: off, workspace.Profiler: off, events.ID: off, watch.ID: off,
		}},
		{ID: workspace.Art, Title: "Art", Order: 20, Slots: map[string]workspace.Slot{
			workspace.Assets: workspace.SlotBottom, workspace.Game: off, workspace.Console: off, workspace.Problems: off,
			workspace.Profiler: off, events.ID: off, watch.ID: off,
		}},
		{ID: workspace.Audio, Title: "Audio", Order: 30, Slots: map[string]workspace.Slot{
			workspace.Hierarchy: off, workspace.Game: off, workspace.Problems: off, workspace.Profiler: off,
			events.ID: off, watch.ID: off,
		}},
	}
}

func (a *App) presetAt(i int) (workspace.Preset, bool) {
	list := a.ws.Presets()
	if i < 0 || i >= len(list) {
		return workspace.Preset{}, false
	}
	return list[i], true
}

func (a *App) registerWorkspaces() error {
	for _, p := range BuiltinWorkspaces() {
		if err := a.ws.RegisterPreset(p); err != nil {
			return err
		}
	}
	reg := a.ctx.Commands
	var list []command.Desc
	for i, k := range workspaceKeys {
		list = append(list, command.Desc{
			ID: WorkspaceCommand(i), Name: "Workspace " + strconv.Itoa(i+1), Category: "Window", Order: 10 + i,
			Shortcut: command.Key(k, command.ModPrimary),
			Label: func() string {
				if p, ok := a.presetAt(i); ok {
					return p.Title
				}
				return "Workspace " + strconv.Itoa(i+1)
			},
			Enabled: func() bool {
				_, ok := a.presetAt(i)
				return ok
			},
			Checked: func() bool {
				p, ok := a.presetAt(i)
				return ok && p.ID == a.ws.Current().ID
			},
			Run: func() error {
				if p, ok := a.presetAt(i); ok {
					a.ws.Switch(p.ID)
				}
				return nil
			},
		})
	}
	list = append(list,
		command.Desc{ID: CommandWorkspaceSaveAs, Name: "Save as Workspace…", Category: "Window", Order: 890,
			Run: func() error {
				a.askWorkspaceName("Save as workspace", "", func(title string) error {
					_, err := a.ws.SaveAs(title)
					return err
				})
				return nil
			}},
		command.Desc{ID: CommandWorkspaceRename, Name: "Rename Workspace…", Category: "Window", Order: 891,
			Run: func() error {
				cur := a.ws.Current()
				a.askWorkspaceName("Rename workspace", cur.Title, func(title string) error { return a.ws.Rename(cur.ID, title) })
				return nil
			}},
		command.Desc{ID: CommandWorkspaceDelete, Name: "Delete Workspace", Category: "Window", Order: 892,
			Enabled: func() bool { return a.ws.Current().User },
			Run:     func() error { return a.ws.Delete(a.ws.Current().ID) }},
		command.Desc{ID: CommandWorkspaceToolbar, Name: "Toolbar", Category: "Window", Order: 880,
			Checked: a.ws.Toolbar,
			Run: func() error {
				a.ws.SetToolbar(!a.ws.Toolbar())
				return nil
			}},
		command.Desc{ID: CommandMaximize, Name: "Maximize Panel", Category: "Window", Order: 870,
			Shortcut: command.Key(input.KeySpace, command.ModShift),
			Checked:  func() bool { return a.ws.Tree.Maximized() != "" },
			Run: func() error {
				if a.ws.Tree.Maximized() != "" {
					a.ws.Tree.Restore()
					return nil
				}
				id := a.ws.Focused()
				if id == "" {
					id = a.ws.Hovered()
				}
				a.ws.ToggleMaximize(id)
				return nil
			}},
		command.Desc{ID: CommandFullscreenPlay, Name: "Full Screen Play", Category: "Play", Order: 140,
			Shortcut: command.Key(input.KeyF11, 0),
			Checked:  func() bool { return a.full },
			Enabled: func() bool {
				return a.full || a.play.State() != play.Stopped || (a.scriptErrors() == 0 && (a.ctx.Docs.Edit() == nil || a.canPlay()))
			},
			Run: func() error {
				if a.full {
					a.exitFull()
					return nil
				}
				return a.enterFull()
			}},
	)
	for _, d := range list {
		if err := reg.Register(d); err != nil {
			return err
		}
	}
	a.play.Subscribe(func(st play.State) {
		if st == play.Stopped {
			a.exitFull()
		}
	})
	return nil
}

func (a *App) askWorkspaceName(title, value string, fn func(string) error) {
	a.ctx.Ask(context.Question{
		Title:   title,
		Fields:  []string{"Name"},
		Values:  []string{value},
		Buttons: []string{"Save", "Cancel"},
		Submit: func(v []string) {
			if err := fn(v[0]); err != nil {
				a.ctx.Notify(context.LevelError, err.Error())
			}
		},
	})
}

func (a *App) globalKeys(in *input.State) {
	reg := a.ctx.Commands
	for i := range workspaceKeys {
		if id := WorkspaceCommand(i); reg.Pressed(id, in) {
			reg.Run(id)
			return
		}
	}
}

func (a *App) workspacePopup(id core.ID) {
	ui := a.ui
	if !ui.BeginPopup(id, 240) {
		return
	}
	reg := a.ctx.Commands
	cur := a.ws.Current().ID
	for i, p := range a.ws.Presets() {
		sc := ""
		if i < len(workspaceKeys) {
			sc = reg.Shortcut(WorkspaceCommand(i))
		}
		ui.PushIDInt(i)
		if widgets.MenuItem(ui, p.Title, sc, true, p.ID == cur) {
			a.ws.Switch(p.ID)
			ui.ClosePopup(id)
		}
		ui.PopID()
	}
	widgets.Separator(ui)
	for _, cid := range []string{CommandWorkspaceSaveAs, CommandWorkspaceRename, CommandWorkspaceDelete, CommandResetLayout, CommandWorkspaceToolbar} {
		if a.ctx.CommandItem(cid, "") {
			ui.ClosePopup(id)
		}
	}
	ui.EndPopup()
}

func (a *App) enterFull() error {
	if a.play.State() == play.Stopped {
		if err := a.togglePlay(); err != nil {
			return err
		}
	}
	if a.play.State() == play.Stopped {
		return nil
	}
	a.eng.Window().SetFullscreen(true)
	m := a.eng.Metrics()
	a.play.SetCapture(true, mathx.Vec2{}, mathx.Vec2{X: float32(m.LogicalW), Y: float32(m.LogicalH)})
	a.full = true
	return nil
}

func (a *App) exitFull() {
	if !a.full {
		return
	}
	a.full = false
	a.eng.Window().SetFullscreen(false)
	a.gameView.Release()
}

func (a *App) FullScreen() bool {
	return a.full
}
