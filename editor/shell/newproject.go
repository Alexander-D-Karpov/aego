package shell

import (
	"path/filepath"
	"strings"

	"aego/editor/core/context"
)

func (a *App) emptyProject() bool {
	return a.ctx.Docs.Active() == nil && a.opt.Project.MainScene.IsZero()
}

func (a *App) askNewProject() {
	a.ctx.Ask(context.Question{
		Title:   "Create a project",
		Message: "Template: empty, 2d, 2.5d or 3d",
		Fields:  []string{"Folder", "Template"},
		Values:  []string{filepath.Join(filepath.Dir(a.opt.Project.Root), "new-game"), "2d"},
		Buttons: []string{"Create", "Cancel"},
		Submit: func(v []string) {
			root, err := a.opt.NewProject(strings.TrimSpace(v[0]), strings.TrimSpace(v[1]))
			if err != nil {
				a.ctx.Notify(context.LevelError, err.Error())
				return
			}
			a.ctx.Notify(context.LevelInfo, "Created "+root+", open it with: aego edit "+root)
		},
	})
}
