package shell

import (
	"path/filepath"
	"strings"

	"aego/editor/core/context"
	"aego/engine/i18n"
)

func (a *App) emptyProject() bool {
	return a.ctx.Docs.Active() == nil && a.opt.Project.MainScene.IsZero()
}

func (a *App) askNewProject() {
	a.ctx.Ask(context.Question{
		Title:   i18n.Tr("editor.shell.create_a_project", "Create a project"),
		Message: i18n.Tr("editor.shell.template_empty_2d_2_5d", "Template: empty, 2d, 2.5d or 3d"),
		Fields:  []string{i18n.Tr("editor.shell.folder", "Folder"), i18n.Tr("editor.shell.template", "Template")},
		Values:  []string{filepath.Join(filepath.Dir(a.opt.Project.Root), "new-game"), "2d"},
		Buttons: []string{i18n.Tr("editor.shell.create", "Create"), i18n.Tr("editor.shell.cancel", "Cancel")},
		Submit: func(v []string) {
			root, err := a.opt.NewProject(strings.TrimSpace(v[0]), strings.TrimSpace(v[1]))
			if err != nil {
				a.ctx.Notify(context.LevelError, err.Error())
				return
			}
			a.ctx.Notify(context.LevelInfo, i18n.Note("editor.shell.created_root_open_it_with", "Created {root}, open it with: aego edit {root2}", i18n.Str("root", root), i18n.Str("root2", root)))
		},
	})
}
