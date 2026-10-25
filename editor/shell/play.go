package shell

import (
	"aego/core/errs"
	"aego/editor/core/document"
	"aego/engine/i18n"
)

// playPrefabHint says why Play does not run a prefab document.
func playPrefabHint() string {
	return i18n.Note("editor.shell.play_prefab_hint", "Play runs a scene: open this prefab from a scene to play it there")
}

func (a *App) playSource() (*document.SceneDocument, error) {
	d := a.ctx.Docs.Edit()
	if d == nil {
		return nil, errs.New(errs.EditorDocument, "no scene is open")
	}
	src, ok := a.ctx.Docs.PlaySource(d.ID)
	if !ok {
		return nil, errs.New(errs.EditorCommand, playPrefabHint())
	}
	if src.ID != d.ID {
		a.playReturn = d.ID
		a.ctx.Docs.SetActive(src.ID)
	}
	return src, nil
}

func (a *App) afterPlay() {
	if a.playReturn == 0 {
		return
	}
	if a.ctx.Docs.Get(a.playReturn) != nil {
		a.ctx.Docs.SetActive(a.playReturn)
	}
	a.playReturn = 0
}

func (a *App) canPlay() bool {
	d := a.ctx.Docs.Edit()
	if d == nil {
		return false
	}
	_, ok := a.ctx.Docs.PlaySource(d.ID)
	return ok
}
