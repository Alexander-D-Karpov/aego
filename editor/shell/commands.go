package shell

import (
	"strings"

	"aego/core/ids"
	"aego/editor/core/assetops"
	edclip "aego/editor/core/clipboard"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/play"
	"aego/editor/core/templates"
	"aego/editor/core/workspace"
	"aego/editor/panels/assets"
	"aego/editor/panels/inspector"
	"aego/editor/panels/sceneview"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/ui/theme"
)

const (
	CommandQuit       = "app.quit"
	CommandCloseScene = "file.close_scene"
)

const (
	CommandPlayBack    = "play.back"
	CommandPlayForward = "play.forward"
)

const (
	CommandAssetsRename    = assets.CommandRename
	CommandAssetsDelete    = assets.CommandDelete
	CommandAssetsDuplicate = assets.CommandDuplicate
	CommandAssetsUndo      = assets.CommandUndo
	CommandAssetsRedo      = assets.CommandRedo
	CommandResetLayout     = "window.reset_layout"
	CommandToggleOrtho     = "scene.toggle_ortho"
)

func (a *App) registerCommands() error {
	reg := a.ctx.Commands
	ctx := a.ctx
	sc := command.DefaultShortcuts()
	hasDoc := func() bool { return ctx.Doc() != nil }
	inAssets := func() bool { return a.ws.Focused() == workspace.Assets }
	assetSel := func() bool { return inAssets() && a.assetsPanel.HasSelection() }
	hasSel := func() bool { return ctx.Doc() != nil && len(ctx.Selected()) > 0 }
	playing := func() bool { return a.play.State() != play.Stopped }
	historyLabel := func(undo bool) func() string {
		return func() string {
			base := i18n.Tr("editor.shell.redo", "Redo")
			if undo {
				base = i18n.Tr("editor.shell.undo", "Undo")
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
		{ID: command.FileOpen, Name: "Open Scene…", Category: "File", Order: 120, Run: a.openScene},
		{ID: command.FileSave, Name: "Save", Category: "File", Order: 200, Enabled: hasDoc, Run: a.save},
		{ID: command.FileSaveAs, Name: "Save As…", Category: "File", Order: 205, Enabled: hasDoc,
			Run: func() error {
				assetops.PromptSaveAs(ctx, ctx.Doc())
				return nil
			}},
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

		{ID: command.SceneToolSelect, Name: "Select Tool", Category: "Scene", Order: 100, Checked: tool(context.ToolSelect), Run: a.setTool(context.ToolSelect)},
		{ID: command.SceneToolMove, Name: "Move Tool", Category: "Scene", Order: 110, Checked: tool(context.ToolMove), Run: a.setTool(context.ToolMove)},
		{ID: command.SceneToolRotate, Name: "Rotate Tool", Category: "Scene", Order: 120, Checked: tool(context.ToolRotate), Run: a.setTool(context.ToolRotate)},
		{ID: command.SceneToolScale, Name: "Scale Tool", Category: "Scene", Order: 130, Checked: tool(context.ToolScale), Run: a.setTool(context.ToolScale)},
		{ID: command.SceneFrame, Name: "Frame Selection", Category: "Scene", Order: 200, Enabled: hasDoc, Run: func() error {
			a.ws.Show(workspace.Scene)
			a.sceneView.Frame()
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
		{ID: CommandToggleOrtho, Name: "Orthographic 3D View", Category: "Scene", Order: 320, Enabled: hasDoc,
			Checked: a.sceneView.Ortho, Run: func() error {
				a.sceneView.ToggleOrtho()
				return nil
			}},
		{ID: command.SceneCreateNode, Name: "Create Empty", Category: "Scene", Order: 400,
			Shortcut: command.Key(input.KeyN, command.ModAlt|command.ModShift), Enabled: hasDoc,
			Run: func() error {
				t, ok := a.templates.Get("engine.empty")
				if !ok {
					return nil
				}
				_, err := templates.Instantiate(ctx, t, ids.NodeID{}, -1, nil)
				return err
			}},

		{ID: command.PlayStart, Name: "Play", Category: "Play", Order: 100,
			Label: func() string {
				if playing() {
					return i18n.Tr("editor.shell.stop", "Stop")
				}
				return i18n.Tr("editor.shell.play", "Play")
			},
			Enabled: func() bool { return playing() || (a.scriptErrors() == 0 && (a.ctx.Docs.Edit() == nil || a.canPlay())) },
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
		{ID: CommandPlayBack, Name: "Step Back", Category: "Play", Order: 121,
			Shortcut: command.Key(input.KeyLeft, command.ModPrimary),
			Enabled:  a.play.CanSeekBack,
			Run:      a.play.StepBack},
		{ID: CommandPlayForward, Name: "Step Forward", Category: "Play", Order: 122,
			Shortcut: command.Key(input.KeyRight, command.ModPrimary),
			Enabled:  playing,
			Run:      a.play.StepForward},
		{ID: command.PlayStop, Name: "Stop", Category: "Play", Order: 130, Enabled: playing,
			Run: func() error {
				a.stopPlay()
				return nil
			}},

		{ID: command.AssetsReimport, Name: "Reimport Asset", Category: "Assets", Order: 100,
			Enabled: a.assetsPanel.HasSelection,
			Run:     func() error { return a.assetsPanel.ReimportSelected() }},
		{ID: CommandAssetsRename, Name: "Rename Asset", Category: "Assets", Order: 110, Scope: command.ScopeAssets,
			Shortcut: command.Key(input.KeyF2, 0),
			Enabled:  func() bool { return inAssets() && a.assetsPanel.Single() },
			Run: func() error {
				a.assetsPanel.StartRename()
				return nil
			}},
		{ID: CommandAssetsDuplicate, Name: "Duplicate Asset", Category: "Assets", Order: 115, Scope: command.ScopeAssets,
			Shortcut: command.Key(input.KeyD, command.ModPrimary), Enabled: assetSel,
			Run: func() error { return a.assetsPanel.DuplicateSelected() }},
		{ID: CommandAssetsDelete, Name: "Delete Asset", Category: "Assets", Order: 120, Scope: command.ScopeAssets,
			Shortcut: command.Key(input.KeyDelete, 0), Enabled: assetSel,
			Run: func() error {
				a.assetsPanel.DeleteSelected()
				return nil
			}},
		{ID: CommandAssetsUndo, Name: "Undo File Operation", Category: "Assets", Order: 200, Scope: command.ScopeAssets,
			Shortcut: command.Key(input.KeyZ, command.ModPrimary),
			Enabled:  func() bool { return inAssets() && a.assetsPanel.CanUndo() },
			Run:      func() error { return a.assetsPanel.Undo() }},
		{ID: CommandAssetsRedo, Name: "Redo File Operation", Category: "Assets", Order: 210, Scope: command.ScopeAssets,
			Shortcut: command.Key(input.KeyZ, command.ModPrimary|command.ModShift),
			Enabled:  func() bool { return inAssets() && a.assetsPanel.CanRedo() },
			Run:      func() error { return a.assetsPanel.Redo() }},
		{ID: assets.CommandCopy, Name: "Copy Asset", Category: "Assets", Order: 130, Scope: command.ScopeAssets,
			Shortcut: command.Key(input.KeyC, command.ModPrimary), Enabled: assetSel,
			Run: func() error { return a.assetsPanel.CopySelected() }},
		{ID: assets.CommandCut, Name: "Cut Asset", Category: "Assets", Order: 135, Scope: command.ScopeAssets,
			Shortcut: command.Key(input.KeyX, command.ModPrimary), Enabled: assetSel,
			Run: func() error { return a.assetsPanel.CutSelected() }},
		{ID: assets.CommandPaste, Name: "Paste into Folder", Category: "Assets", Order: 140, Scope: command.ScopeAssets,
			Shortcut: command.Key(input.KeyV, command.ModPrimary), Enabled: inAssets,
			Run: func() error { return a.assetsPanel.PasteHere() }},
		{ID: sceneview.CommandPasteImage, Name: "Paste Image as Sprite", Category: "Scene", Order: 410, Scope: command.ScopeScene,
			Shortcut: command.Key(input.KeyV, command.ModPrimary),
			Enabled:  func() bool { return a.sceneView.CanPasteImage() },
			Run:      func() error { return a.sceneView.PasteImage() }},

		{ID: inspector.CommandCopy, Name: "Copy Field", Scope: inspector.Scope,
			Shortcut: command.Key(input.KeyC, command.ModPrimary),
			Enabled:  a.inspectorPanel.CanCopy,
			Run:      func() error { return a.inspectorPanel.CopyHovered() }},
		{ID: inspector.CommandPaste, Name: "Paste Field", Scope: inspector.Scope,
			Shortcut: command.Key(input.KeyV, command.ModPrimary),
			Enabled:  hasDoc,
			Run:      func() error { return a.inspectorPanel.PasteHovered() }},
		{ID: inspector.CommandWatch, Name: "Add Field to Watch", Scope: inspector.Scope,
			Shortcut: command.Key(input.KeyW, command.ModPrimary|command.ModAlt),
			Enabled:  a.inspectorPanel.CanWatch,
			Run:      func() error { return a.inspectorPanel.WatchHovered() }},
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
	if a.agentP != nil {
		return reg.Register(command.Desc{ID: "agent.ask", Name: "Ask Agent…", Category: "Window", Run: func() error { a.agentP.Ask(""); return nil }})
	}
	return nil
}

func (a *App) registerWindowCommands() error {
	reg := a.ctx.Commands
	for _, d := range a.ws.Menu() {
		id := d.Panel.ID()
		if _, exists := reg.Get(command.WindowPrefix + id); exists {
			continue
		}
		if err := reg.Register(command.Desc{
			ID:       command.WindowPrefix + id,
			Name:     d.Panel.Title(),
			Category: "Window",
			Order:    d.Order,
			Icon:     panelIcon(d),
			Label:    d.Panel.Title,
			TipFunc: func() string {
				return i18n.Tr("editor.window.toggle.tip", "Show or hide the {panel} panel", i18n.Str("panel", d.Panel.Title()))
			},
			Checked: func() bool { return a.ws.Visible(id) },
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
	for i, m := range []theme.Mode{theme.ModeSystem, theme.ModeDark, theme.ModeLight} {
		if err := reg.Register(command.Desc{
			ID:       "window.theme." + strings.ToLower(m.String()),
			Name:     "Theme: " + m.String(),
			Category: themeCategory,
			Order:    i,
			Label:    func() string { return menuTitle(themeCategory) + ": " + themeName(m) },
			TipFunc:  func() string { return themeTip(m) },
			Checked:  func() bool { return a.ThemeMode() == m },
			Run: func() error {
				a.SetThemeMode(m)
				return nil
			},
		}); err != nil {
			return err
		}
	}
	if err := a.registerLanguageCommands(reg); err != nil {
		return err
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
	if a.opt.Code != nil {
		a.opt.Code.FlushAutosave()
	}
	var err error
	if a.ctx.Docs.Edit() != nil {
		src, perr := a.playSource()
		if perr != nil {
			return perr
		}
		err = a.play.Start(src)
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
	return a.newUnsaved("scenes/untitled.aescene", false)
}

func (a *App) saveDoc(d *document.SceneDocument) error {
	if strings.HasSuffix(d.Path, document.PrefabExt) {
		return a.prefabs.SaveDocument(d)
	}
	return a.ctx.Docs.Save(d.ID)
}

func (a *App) save() error {
	d := a.ctx.Doc()
	if d == nil {
		return nil
	}
	if d.IsPlay() {
		d = a.ctx.Docs.Get(d.ID)
	}
	if err := a.saveDoc(d); err != nil {
		return err
	}
	a.ctx.Notify(context.LevelInfo, i18n.Note("editor.shell.saved_path", "Saved {path}", i18n.Str("path", d.Path)))
	return nil
}

func (a *App) saveAll() error {
	var first error
	for _, d := range a.ctx.Docs.Docs() {
		if !d.Dirty() {
			continue
		}
		if err := a.saveDoc(d); err != nil && first == nil {
			first = err
		}
	}
	for _, d := range a.ctx.Docs.Scripts() {
		if !d.Dirty() {
			continue
		}
		if err := a.ctx.Docs.SaveScript(d); err != nil && first == nil {
			first = err
		}
	}
	if first != nil {
		return first
	}
	a.ctx.Notify(context.LevelInfo, i18n.Note("editor.shell.saved_all_scenes", "Saved all scenes"))
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

func panelIcon(d workspace.Desc) string {
	if ic, ok := d.Panel.(workspace.Iconed); ok {
		if s := ic.Icon(); s != "" {
			return s
		}
	}
	return d.Icon
}
