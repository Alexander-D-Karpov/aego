package shell

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/sceneops"
	"aego/editor/core/templates"
	"aego/editor/shell/addpalette"
	"aego/platform/input"
)

func (a *App) wirePalette(sops *sceneops.Ops) {
	a.addPal = addpalette.New(addpalette.Deps{Ctx: a.ctx, Meta: a.inspectors, Templates: a.templates, Root: a.opt.Project.Root, Actions: a.animActions})
	sops.SetCreate(func(parent ids.NodeID, pl templates.Place, at *mathx.Vec2, created func([]ids.NodeID)) {
		a.addPal.CreateNode(parent, pl, at, created)
	})
}

// AddPalette is the Add Component and Create palette, for tests and screenshots.
func (a *App) AddPalette() *addpalette.Service {
	return a.addPal
}

func (a *App) openAdd(nodes []ids.NodeID, at *mathx.Vec2, query string) {
	if len(a.modals) > 0 || a.search.Active() {
		return
	}
	a.addPal.AddComponent(nodes, at)
	if query != "" {
		a.addPal.Palette().SetQuery(query)
	}
}

func (a *App) registerPalette() error {
	ctx := a.ctx
	hasSel := func() bool { return ctx.Doc() != nil && len(ctx.Selected()) > 0 }
	hasDoc := func() bool { return ctx.Doc() != nil }
	if err := ctx.Commands.Register(command.Desc{
		ID: command.EditAddComponent, Name: "Add Component…", Category: "Edit", Order: 420, Enabled: hasSel,
		Shortcut: command.Key(input.KeyA, command.ModPrimary|command.ModShift),
		Run: func() error {
			a.openAdd(ctx.Selected(), nil, "")
			return nil
		},
	}); err != nil {
		return err
	}
	return ctx.Commands.Register(command.Desc{
		ID: command.SceneCreate, Name: "Create…", Category: "Scene", Order: 399, Enabled: hasDoc,
		Shortcut: command.Key(input.KeyN, command.ModPrimary|command.ModShift),
		Run: func() error {
			if len(a.modals) > 0 || a.search.Active() {
				return nil
			}
			parent := ids.NodeID{}
			if it, ok := ctx.Selection.Active(); ok && ctx.Doc() != nil && it.Doc == ctx.Doc().ID {
				parent = it.Node
			}
			a.addPal.CreateNode(parent, templates.Place{}, nil, nil)
			return nil
		},
	})
}

func (a *App) drawPalette() {
	if len(a.modals) > 0 && a.addPal.Active() {
		a.addPal.Close()
	}
	a.addPal.Draw()
}
