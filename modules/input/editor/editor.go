package editor

import (
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/engine"
	"aego/engine/i18n"
	"aego/modules/input"
	raw "aego/platform/input"
)

func Hook(e *ext.Registry) error {
	ed := e.Editor()
	if err := e.SettingsPage(input.ID, NewPage(ed).Draw, i18n.T("i18n.input.players", "Players"), i18n.T("i18n.input.dead_zone", "Dead Zone"), i18n.T("i18n.input.start_stack", "Start Stack"), i18n.T("i18n.input.actions", "Actions"), i18n.T("i18n.input.bindings", "Bindings"), i18n.T("i18n.input.contexts", "Contexts")); err != nil {
		return err
	}
	world := func() *engine.World {
		if ed == nil {
			return nil
		}
		return ed.PlayWorld()
	}
	rawInput := func() *raw.State {
		if ed == nil || ed.Engine == nil {
			return nil
		}
		return ed.Engine.Input()
	}
	if err := e.Panel(workspace.Desc{Panel: NewDevices(world, rawInput), Near: workspace.Console, Icon: "panel.input_devices"}); err != nil {
		return err
	}
	return e.Panel(workspace.Desc{Panel: NewFocus(world), Near: workspace.Console, Icon: "panel.input_focus"})
}
