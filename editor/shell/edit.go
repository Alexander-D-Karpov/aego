package shell

import (
	"aego/editor/core/command"
	"aego/editor/core/favorites"
	"aego/editor/core/workspace"
	"aego/editor/panels/inspector"
	"aego/editor/panels/sceneview"
	"aego/platform/input"
)

const (
	CommandSelectionBack    = "edit.selection_back"
	CommandSelectionForward = "edit.selection_forward"
)

func (a *App) nodeFavorite() (favorites.Item, bool) {
	d := a.ctx.Doc()
	it, ok := a.ctx.Selection.Active()
	if d == nil || !ok || it.Doc != d.ID {
		return favorites.Item{}, false
	}
	s := d.Scene()
	h := s.Handle(it.Node)
	if h.IsZero() || !s.IsAlive(h) {
		return favorites.Item{}, false
	}
	return favorites.Item{Kind: favorites.Node, Scene: d.Path, Node: it.Node, Label: s.Name(h)}, true
}

func (a *App) stepSelection(back bool) error {
	sel := a.ctx.Selection
	for {
		moved := false
		if back {
			moved = sel.Back()
		} else {
			moved = sel.Forward()
		}
		if !moved {
			return nil
		}
		it, ok := sel.Active()
		if !ok {
			continue
		}
		d := a.ctx.Docs.Get(it.Doc)
		if d == nil {
			continue
		}
		a.ctx.Docs.SetActive(d.ID)
		a.ctx.PruneSelectionOf(d.ID)
		if sel.Len() > 0 {
			return nil
		}
	}
}

func (a *App) registerEditExtras() error {
	reg := a.ctx.Commands
	sel := a.ctx.Selection
	list := []command.Desc{
		{ID: favorites.CommandNode, Name: "Add Node to Favorites", Category: "Edit", Order: 400,
			Label: func() string {
				if it, ok := a.nodeFavorite(); ok && a.favs.Has(it) {
					return "Remove from Favorites"
				}
				return "Add to Favorites"
			},
			Enabled: func() bool { _, ok := a.nodeFavorite(); return ok },
			Run: func() error {
				it, ok := a.nodeFavorite()
				if !ok {
					return nil
				}
				_, err := a.favs.Toggle(it)
				return err
			}},
		{ID: CommandSelectionBack, Name: "Previous Selection", Category: "Edit", Order: 410,
			Shortcut: command.Key(input.KeyLeft, command.ModAlt), Enabled: sel.CanBack,
			Run: func() error { return a.stepSelection(true) }},
		{ID: CommandSelectionForward, Name: "Next Selection", Category: "Edit", Order: 411,
			Shortcut: command.Key(input.KeyRight, command.ModAlt), Enabled: sel.CanForward,
			Run: func() error { return a.stepSelection(false) }},
		{ID: inspector.CommandLock, Name: "Lock Inspector", Category: "Window", Order: 865,
			Checked: a.inspectorPanel.Locked,
			Run: func() error {
				a.inspectorPanel.ToggleLock()
				a.ws.Show(workspace.Inspector)
				return nil
			}},
		{ID: sceneview.CommandIsolate, Name: "Isolate Selection", Category: "Scene", Order: 330, Scope: command.ScopeScene,
			Shortcut: command.Key(input.KeyH, command.ModShift), Checked: a.sceneView.Isolated,
			Run: a.sceneView.ToggleIsolation},
	}
	for _, d := range list {
		if err := reg.Register(d); err != nil {
			return err
		}
	}
	return nil
}
