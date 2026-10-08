package app

import (
	"aego/asset"
	"aego/core/ids"
	edclip "aego/editor/clipboard"
	"aego/editor/command"
	"aego/editor/context"
	"aego/editor/inspector"
	"aego/editor/play"
	"aego/editor/sceneview"
	"aego/editor/templates"
	"aego/editor/workspace"
	"aego/platform/input"
	"aego/scene"
	"aego/scene/components"
)

const (
	CommandQuit       = "app.quit"
	CommandCloseScene = "file.close_scene"
)

const (
	CommandAssetsRename = "assets.rename"
	CommandAssetsDelete = "assets.delete"
	CommandResetLayout  = "window.reset_layout"
)

func (a *App) registerCommands() error {
	reg := a.ctx.Commands
	ctx := a.ctx
	sc := command.DefaultShortcuts()
	hasDoc := func() bool { return ctx.Doc() != nil }
	hasSel := func() bool { return ctx.Doc() != nil && len(ctx.Selected()) > 0 }
	playing := func() bool { return a.play.State() != play.Stopped }
	historyLabel := func(undo bool) func() string {
		return func() string {
			base := "Redo"
			if undo {
				base = "Undo"
			}
			h := ctx.History()
			if h == nil {
				return base
			}
			u, r := h.Labels(1)
			l := r
			if undo {
				l = u
			}
			if len(l) > 0 {
				return base + " " + l[0]
			}
			return base
		}
	}
	tool := func(t uint8) func() bool { return func() bool { return ctx.Settings.Tool == t } }
	list := []command.Desc{
		{ID: command.FileNewScene, Name: "New Scene", Category: "File", Order: 100, Run: a.newScene},
		{ID: CommandNewScene3D, Name: "New 3D Scene", Category: "File", Order: 110, Run: a.newScene3D},
		{ID: command.FileSave, Name: "Save", Category: "File", Order: 200, Enabled: hasDoc, Run: a.save},
		{ID: command.FileSaveAll, Name: "Save All", Category: "File", Order: 210, Enabled: ctx.Docs.Dirty, Run: a.saveAll},
		{ID: CommandCloseScene, Name: "Close Scene", Category: "File", Order: 300,
			Shortcut: command.Key(input.KeyW, command.ModPrimary), Enabled: hasDoc, Run: a.closeActive},
		{ID: CommandQuit, Name: "Quit", Category: "File", Order: 900,
			Shortcut: command.Key(input.KeyQ, command.ModPrimary), Run: func() error {
				a.requestQuit()
				return nil
			}},

		{ID: command.EditUndo, Name: "Undo", Category: "Edit", Order: 100, Label: historyLabel(true),
			Enabled: func() bool {
				h := ctx.History()
				return h != nil && h.CanUndo()
			}, Run: ctx.Undo},
		{ID: command.EditRedo, Name: "Redo", Category: "Edit", Order: 110, Label: historyLabel(false),
			Enabled: func() bool {
				h := ctx.History()
				return h != nil && h.CanRedo()
			}, Run: ctx.Redo},
		{ID: command.EditCut, Name: "Cut", Category: "Edit", Order: 200, Enabled: hasSel, Run: func() error { return edclip.Cut(ctx) }},
		{ID: command.EditCopy, Name: "Copy", Category: "Edit", Order: 210, Enabled: hasSel, Run: func() error { return edclip.Copy(ctx) }},
		{ID: command.EditPaste, Name: "Paste", Category: "Edit", Order: 220, Enabled: hasDoc, Run: func() error { return edclip.Paste(ctx) }},
		{ID: command.EditDuplicate, Name: "Duplicate", Category: "Edit", Order: 230, Enabled: hasSel, Run: func() error { return edclip.Duplicate(ctx) }},
		{ID: command.EditDelete, Name: "Delete", Category: "Edit", Order: 240, Enabled: hasSel, Run: func() error { return edclip.Delete(ctx) }},
		{ID: command.EditRename, Name: "Rename", Category: "Edit", Order: 300, Enabled: hasSel, Run: func() error {
			if it, ok := ctx.Selection.Active(); ok {
				a.ws.Show(workspace.Hierarchy)
				a.hierarchy.StartRename(it.Node)
			}
			return nil
		}},
		{ID: command.EditSelectAll, Name: "Select All", Category: "Edit", Order: 310, Enabled: hasDoc, Run: a.selectAll},

		{ID: command.SceneToolSelect, Name: "Select Tool", Category: "Scene", Order: 100, Checked: tool(sceneview.ToolSelect), Run: a.setTool(sceneview.ToolSelect)},
		{ID: command.SceneToolMove, Name: "Move Tool", Category: "Scene", Order: 110, Checked: tool(sceneview.ToolMove), Run: a.setTool(sceneview.ToolMove)},
		{ID: command.SceneToolRotate, Name: "Rotate Tool", Category: "Scene", Order: 120, Checked: tool(sceneview.ToolRotate), Run: a.setTool(sceneview.ToolRotate)},
		{ID: command.SceneToolScale, Name: "Scale Tool", Category: "Scene", Order: 130, Checked: tool(sceneview.ToolScale), Run: a.setTool(sceneview.ToolScale)},
		{ID: command.SceneFrame, Name: "Frame Selection", Category: "Scene", Order: 200, Enabled: hasDoc, Run: func() error {
			a.ws.Show(workspace.Scene)
			a.sceneView.Frame(ctx)
			return nil
		}},
		{ID: command.SceneToggleGrid, Name: "Grid", Category: "Scene", Order: 300,
			Checked: func() bool { return ctx.Settings.Grid }, Run: func() error {
				ctx.Settings.Grid = !ctx.Settings.Grid
				return nil
			}},
		{ID: command.SceneToggleSnap, Name: "Snap", Category: "Scene", Order: 310,
			Checked: func() bool { return ctx.Settings.Snap }, Run: func() error {
				ctx.Settings.Snap = !ctx.Settings.Snap
				return nil
			}},
		{ID: command.SceneCreateNode, Name: "Create Empty", Category: "Scene", Order: 400,
			Shortcut: command.Key(input.KeyN, command.ModPrimary|command.ModShift), Enabled: hasDoc,
			Run: func() error {
				_, err := templates.Instantiate(ctx, templates.Empty(), ids.NodeID{}, -1, nil)
				return err
			}},

		{ID: command.PlayStart, Name: "Play", Category: "Play", Order: 100,
			Label: func() string {
				if playing() {
					return "Stop"
				}
				return "Play"
			},
			Enabled: func() bool { return playing() || a.scriptErrors() == 0 },
			Run:     a.togglePlay},
		{ID: command.PlayPause, Name: "Pause", Category: "Play", Order: 110, Enabled: playing,
			Checked: func() bool { return a.play.State() == play.Paused },
			Run: func() error {
				a.play.Pause()
				return nil
			}},
		{ID: command.PlayStep, Name: "Step Frame", Category: "Play", Order: 120,
			Enabled: func() bool { return a.play.State() == play.Paused },
			Run: func() error {
				a.play.Step()
				return nil
			}},
		{ID: command.PlayStop, Name: "Stop", Category: "Play", Order: 130, Enabled: playing,
			Run: func() error {
				a.stopPlay()
				return nil
			}},

		{ID: command.AssetsReimport, Name: "Reimport Asset", Category: "Assets", Order: 100,
			Enabled: func() bool { return !a.assetsPanel.Selected().IsZero() },
			Run:     func() error { return a.assetsPanel.ReimportSelected(ctx) }},
		{ID: CommandAssetsRename, Name: "Rename Asset", Category: "Assets", Order: 110, Scope: command.ScopeAssets,
			Shortcut: command.Key(input.KeyF2, 0),
			Enabled:  func() bool { return !a.assetsPanel.Selected().IsZero() },
			Run: func() error {
				a.assetsPanel.StartRename()
				return nil
			}},
		{ID: CommandAssetsDelete, Name: "Delete Asset", Category: "Assets", Order: 120, Scope: command.ScopeAssets,
			Shortcut: command.Key(input.KeyDelete, 0),
			Enabled:  func() bool { return !a.assetsPanel.Selected().IsZero() },
			Run: func() error {
				a.assetsPanel.DeleteSelected(ctx)
				return nil
			}},

		{ID: inspector.CommandCopy, Name: "Copy Field", Scope: inspector.Scope,
			Shortcut: command.Key(input.KeyC, command.ModPrimary),
			Enabled:  a.inspectorPanel.CanCopy,
			Run:      func() error { return a.inspectorPanel.CopyHovered(ctx) }},
		{ID: inspector.CommandPaste, Name: "Paste Field", Scope: inspector.Scope,
			Shortcut: command.Key(input.KeyV, command.ModPrimary),
			Enabled:  hasDoc,
			Run:      func() error { return a.inspectorPanel.PasteHovered(ctx) }},
	}
	for _, d := range list {
		if d.Shortcut.IsZero() {
			d.Shortcut = sc[d.ID]
		}
		if d.Scope == "" {
			d.Scope = command.ScopeOf(d.ID)
		}
		if err := reg.Register(d); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) registerWindowCommands() error {
	reg := a.ctx.Commands
	for i, p := range a.ws.Panels() {
		id := p.ID()
		if _, exists := reg.Get(command.WindowPrefix + id); exists {
			continue
		}
		if err := reg.Register(command.Desc{
			ID:       command.WindowPrefix + id,
			Name:     p.Title(),
			Category: "Window",
			Order:    100 + i,
			Checked:  func() bool { return a.ws.Visible(id) },
			Run: func() error {
				a.ws.Toggle(id)
				return nil
			},
		}); err != nil {
			return err
		}
	}
	if _, exists := reg.Get(CommandResetLayout); exists {
		return nil
	}
	return reg.Register(command.Desc{
		ID:       CommandResetLayout,
		Name:     "Reset Layout",
		Category: "Window",
		Order:    900,
		Run: func() error {
			a.ws.ResetLayout()
			return nil
		},
	})
}

func (a *App) stopPlay() {
	if a.play.State() == play.Stopped {
		return
	}
	a.play.Stop()
	a.ws.Show(workspace.Scene)
}

func (a *App) togglePlay() error {
	if a.play.State() != play.Stopped {
		a.stopPlay()
		return nil
	}
	var err error
	if d := a.ctx.Docs.Edit(); d != nil {
		err = a.play.Start(d)
	} else {
		err = a.play.StartProject()
	}
	if err == nil {
		a.ws.Show(workspace.Game)
	}
	return err
}

func (a *App) selectAll() error {
	s := a.ctx.Scene()
	if s == nil {
		return nil
	}
	root := s.Root()
	var all []ids.NodeID
	for _, h := range s.PreOrder() {
		if h != root && s.IsAlive(h) {
			all = append(all, s.ID(h))
		}
	}
	a.ctx.Select(all...)
	return nil
}

func (a *App) setTool(t uint8) func() error {
	return func() error {
		a.ctx.Settings.Tool = t
		return nil
	}
}

func (a *App) newScene() error {
	var res asset.Resolver
	if a.ctx.Loader != nil {
		res = a.ctx.Loader
	}
	s := scene.New(a.opt.Types, res, scene.Options{})
	cam := s.Create(s.Root(), "Camera")
	*scene.Add[components.Camera2D](s, cam) = components.DefaultCamera2D()
	s.Flush()
	rel := a.ctx.Docs.UniquePath("scenes/untitled.aescene")
	a.ctx.Docs.New(rel, s)
	a.ctx.Selection.Clear()
	return nil
}

func (a *App) save() error {
	d := a.ctx.Doc()
	if d == nil {
		return nil
	}
	if err := a.ctx.Docs.Save(d.ID); err != nil {
		return err
	}
	a.ctx.Notify(context.LevelInfo, "Saved "+d.Path)
	return nil
}

func (a *App) saveAll() error {
	if err := a.ctx.Docs.SaveAll(); err != nil {
		return err
	}
	a.ctx.Notify(context.LevelInfo, "Saved all scenes")
	return nil
}

func (a *App) closeActive() error {
	d := a.ctx.Doc()
	if d == nil {
		return nil
	}
	if d.Dirty() {
		a.push(modal{kind: modalClose, doc: d.ID, path: d.Path})
		return nil
	}
	a.ctx.Selection.DropDoc(d.ID)
	return a.ctx.Docs.Close(d.ID, false)
}
