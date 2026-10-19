package shell

import (
	"aego/asset/pipeline"
	"aego/editor/core/favorites"
	"aego/editor/core/workspace"
	favpanel "aego/editor/panels/favorites"
	"aego/editor/panels/scenes"
	"aego/editor/panels/undohistory"
)

func (a *App) panelCatalog() []workspace.Desc {
	return []workspace.Desc{
		{Panel: a.hierarchy, Slot: workspace.SlotLeft, Order: 100, Icon: "panel.hierarchy"},
		{Panel: a.assetsPanel, Slot: workspace.SlotBrowser, Order: 110, Icon: "panel.assets"},
		{Panel: scenes.New(scenes.Deps{
			Assets: func() []pipeline.AssetInfo {
				if a.opt.Pipeline == nil {
					return nil
				}
				return a.opt.Pipeline.Assets()
			},
			Previews: a.thumbs,
			Files:    a.files,
			Docs:     a.ctx,
			Notices:  a.ctx,
			Project:  a.opt.Project,
			Reveal: func(rel string) {
				if a.ctx.RevealPath != nil {
					a.ctx.RevealPath(rel)
				}
			},
		}), Near: workspace.Assets, Order: 112},
		{Panel: favpanel.New(favpanel.Deps{Store: a.favs, Notices: a.ctx, Env: favorites.Env{Pipeline: a.opt.Pipeline, Docs: a.ctx, Selection: a.ctx, Commands: a.ctx.Commands, Nav: a.ctx, ShowHierarchy: func() { a.ws.Show(workspace.Hierarchy) }}}), Near: workspace.Assets, Order: 113, Icon: "panel.favorites"},
		{Panel: a.sceneView, Slot: workspace.SlotCenter, Order: 120},
		{Panel: a.gameView, Slot: workspace.SlotCenter, Order: 130, Icon: "panel.game"},
		{Panel: a.inspectorPanel, Slot: workspace.SlotRight, Order: 140, Icon: "panel.inspector"},
		{Panel: a.consolePanel, Slot: workspace.SlotBottom, Order: 150, Icon: "panel.console"},
		{Panel: a.problemsPanel, Slot: workspace.SlotBottom, Order: 160, Icon: "panel.problems"},
		{Panel: a.profiler, Slot: workspace.SlotBottom, Order: 170, Icon: "panel.profiler"},
		{Panel: a.eventsPanel, Slot: workspace.SlotBottom, Order: 175, Icon: "panel.events"},
		{Panel: a.watchPanel, Slot: workspace.SlotBottom, Order: 176, Icon: "panel.watch"},
		{Panel: a.settings, Near: workspace.Inspector, Order: 200, Icon: "panel.settings"},
		{Panel: a.docs, Near: workspace.Scene, Order: 210, Icon: "help.book"},
		{Panel: a.tablePanel, Near: workspace.Scene, Order: 215, Icon: "panel.table"},
		{Panel: a.welcome, Near: workspace.Inspector, Order: 220, Icon: "panel.welcome"},
		{Panel: a.keymap, Near: workspace.Inspector, Order: 230, Icon: "panel.keymap"},
		{Panel: undohistory.New(undohistory.Deps{History: a.ctx.History, Notices: a.ctx}), Near: workspace.Inspector, Order: 232, Icon: "panel.history"},
		{Panel: a.crashesPanel, Near: workspace.Console, Order: 240, Icon: "panel.crashes"},
		{Panel: a.savesPanel, Near: workspace.Console, Order: 245, Icon: "panel.saves"},
		{Panel: a.refsPanel, Near: workspace.Console, Order: 250, Icon: "panel.references"},
		{Panel: a.missing, Near: workspace.Console, Order: 251, Icon: "status.warning"},
		{Panel: a.taskPanel, Near: workspace.Console, Order: 252, Icon: "panel.tasks"},
		{Panel: a.healthP, Near: workspace.Console, Order: 253, Icon: "panel.health"},
		{Panel: a.diffPanel, Near: workspace.Scene, Order: 216, Icon: "panel.diff"},
	}
}

func (a *App) registerPanels() error {
	for _, d := range a.opt.Panels {
		if err := a.ws.Register(d); err != nil {
			return err
		}
	}
	for _, d := range a.panelCatalog() {
		if a.ws.Has(d.Panel.ID()) {
			continue
		}
		if err := a.ws.Register(d); err != nil {
			return err
		}
	}
	return nil
}
