package shell

import (
	"aego/editor/core/command"
	core "aego/editor/core/health"
	exportpanel "aego/editor/panels/export"
	"aego/engine/i18n"
	"aego/engine/problems"
)

const CommandExport = "file.export"

func (a *App) initExport() error {
	x := a.opt.Exporter
	if x == nil {
		return nil
	}
	a.exportP = exportpanel.New(exportpanel.Deps{Exporter: x, Tasks: a.tasks, PickFolder: a.pickFolder})
	return a.ctx.Commands.Register(command.Desc{
		ID: CommandExport, Name: "Export…", Category: "File", Order: 820, Icon: "file.package",
		Run: func() error {
			a.ws.Show(exportpanel.ID)
			return nil
		},
	})
}

func (a *App) exportWork() func() []core.Item {
	x := a.opt.Exporter
	if x == nil {
		return nil
	}
	prep := x.Prepare()
	return func() []core.Item {
		var out []core.Item
		for _, d := range x.Check(prep, exportpanel.Release) {
			switch d.Severity {
			case problems.Error:
				out = append(out, core.Item{Code: d.Code, Title: i18n.Tr("editor.shell.blocks_a_release_export_message", "Blocks a release export: {message}", i18n.Str("message", d.Message)), Severity: core.Error})
			case problems.Warning:
				out = append(out, core.Item{Code: d.Code, Title: i18n.Tr("editor.shell.export_message", "Export: {message}", i18n.Str("message", d.Message)), Severity: core.Warning})
			}
		}
		return out
	}
}
