package shell

import (
	"aego/editor/core/workspace"
)

func (a *App) panelCatalog() []workspace.Desc {
	return []workspace.Desc{
		{Panel: a.hierarchy, Slot: workspace.SlotLeft, Order: 100},
		{Panel: a.assetsPanel, Slot: workspace.SlotBrowser, Order: 110},
		{Panel: a.sceneView, Slot: workspace.SlotCenter, Order: 120},
		{Panel: a.gameView, Slot: workspace.SlotCenter, Order: 130},
		{Panel: a.inspectorPanel, Slot: workspace.SlotRight, Order: 140},
		{Panel: a.consolePanel, Slot: workspace.SlotBottom, Order: 150},
		{Panel: a.problemsPanel, Slot: workspace.SlotBottom, Order: 160},
		{Panel: a.profiler, Slot: workspace.SlotBottom, Order: 170},
		{Panel: a.settings, Near: workspace.Inspector, Order: 200},
		{Panel: a.docs, Near: workspace.Scene, Order: 210},
		{Panel: a.welcome, Near: workspace.Inspector, Order: 220},
		{Panel: a.keymap, Near: workspace.Inspector, Order: 230},
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
