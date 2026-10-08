package app

import (
	"fmt"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/context"
	"aego/editor/document"
	"aego/ui/core"
	"aego/ui/widgets"
)

type modalKind uint8

const (
	modalRecovery modalKind = iota
	modalConflict
	modalSaveAs
	modalQuit
	modalClose
	modalLock
	modalAsk
)

type modal struct {
	kind    modalKind
	doc     document.ID
	path    string
	removed bool
	rec     document.Recovery
	lock    document.LockInfo
	input   string
	ask     context.Question
}

func (a *App) push(m modal) {
	if m.kind != modalAsk {
		for _, o := range a.modals {
			if o.kind == m.kind && o.doc == m.doc && o.rec.File == m.rec.File {
				return
			}
		}
	}
	a.modals = append(a.modals, m)
}

func (a *App) pop() {
	if len(a.modals) > 0 {
		a.modals = a.modals[1:]
	}
	if len(a.modals) == 0 {
		a.ui.CloseModal()
	}
}

func mineSuggestion(p string) string {
	return strings.TrimSuffix(p, document.SceneExt) + "-mine" + document.SceneExt
}

func (a *App) drawModals() {
	if len(a.modals) == 0 {
		return
	}
	ui := a.ui
	docs := a.ctx.Docs
	id := core.IDOf("editor.modal")
	ui.OpenModal(id)
	md := a.modals[0]
	switch md.kind {
	case modalRecovery:
		msg := "An autosave of " + md.rec.Path + " from " + md.rec.Time.Format("2006-01-02 15:04") + " was found."
		switch widgets.Confirm(ui, id, "Recover unsaved changes", msg, "Restore", "Discard", "Later") {
		case 0:
			if _, err := docs.RestoreRecovery(md.rec); err != nil {
				a.ctx.Notify(context.LevelError, err.Error())
			}
			a.pop()
		case 1:
			docs.DiscardRecovery(md.rec)
			a.pop()
		case 2:
			a.pop()
		}
	case modalConflict:
		msg := md.path + " was changed on disk while it has unsaved edits."
		first := "Reload"
		if md.removed {
			msg = md.path + " was deleted on disk."
			first = "Close"
		}
		switch widgets.Confirm(ui, id, "Scene changed outside the editor", msg, first, "Keep mine", "Save As…") {
		case 0:
			if md.removed {
				a.ctx.Selection.DropDoc(md.doc)
				docs.Close(md.doc, true)
			} else if err := docs.Reload(md.doc); err != nil {
				a.ctx.Notify(context.LevelError, err.Error())
			} else {
				a.ctx.PruneSelectionOf(md.doc)
			}
			a.pop()
		case 1:
			docs.KeepMine(md.doc)
			a.pop()
		case 2:
			a.modals[0] = modal{kind: modalSaveAs, doc: md.doc, input: mineSuggestion(md.path)}
		}
	case modalSaveAs:
		a.drawSaveAs(id)
	case modalQuit:
		msg := strconv.Itoa(docs.DirtyCount()) + " scene(s) have unsaved changes."
		switch widgets.Confirm(ui, id, "Quit Aego Editor", msg, "Save All", "Discard", "Cancel") {
		case 0:
			if err := docs.SaveAll(); err != nil {
				a.ctx.Notify(context.LevelError, err.Error())
				a.pop()
				return
			}
			a.pop()
			a.quitting = true
			a.eng.Quit()
		case 1:
			a.discard = true
			a.pop()
			a.quitting = true
			a.eng.Quit()
		case 2:
			a.pop()
		}
	case modalClose:
		switch widgets.Confirm(ui, id, "Close scene", md.path+" has unsaved changes.", "Save", "Discard", "Cancel") {
		case 0:
			if err := docs.Save(md.doc); err != nil {
				a.ctx.Notify(context.LevelError, err.Error())
			} else {
				a.ctx.Selection.DropDoc(md.doc)
				docs.Close(md.doc, false)
			}
			a.pop()
		case 1:
			a.ctx.Selection.DropDoc(md.doc)
			docs.Close(md.doc, true)
			a.pop()
		case 2:
			a.pop()
		}
	case modalLock:
		msg := fmt.Sprintf("editor.lock belongs to pid %d on %s (%s). Another editor may be running, or the previous one crashed.",
			md.lock.PID, md.lock.Host, md.lock.Time.Format("2006-01-02 15:04"))
		if widgets.Confirm(ui, id, "Project may be open elsewhere", msg, "Continue") == 0 {
			a.pop()
		}
	case modalAsk:
		if c := widgets.Confirm(ui, id, md.ask.Title, md.ask.Message, md.ask.Buttons...); c >= 0 {
			a.pop()
			if md.ask.Then != nil {
				md.ask.Then(c)
			}
		}
	}
}

func (a *App) drawSaveAs(id core.ID) {
	ui := a.ui
	th := &ui.Theme
	sz := ui.Size()
	w := min(float32(460), sz.X-40)
	r := mathx.Rect{X: (sz.X - w) / 2, Y: sz.Y / 3, W: w, H: th.RowHeight*4 + th.Padding*4}
	if !ui.BeginModal(id, r) {
		return
	}
	widgets.Label(ui, "Save scene as")
	in := a.modals[0].input
	widgets.TextField(ui, ui.ID("saveas.path"), &in)
	a.modals[0].input = in
	row := ui.Cursor().Next(0)
	ok := widgets.ButtonRect(ui, ui.ID("saveas.ok"), mathx.Rect{X: row.X + row.W - 184, Y: row.Y, W: 88, H: row.H}, "Save")
	cancel := widgets.ButtonRect(ui, ui.ID("saveas.cancel"), mathx.Rect{X: row.X + row.W - 90, Y: row.Y, W: 88, H: row.H}, "Cancel")
	ui.EndModal()
	doc := a.modals[0].doc
	switch {
	case ok:
		if err := a.ctx.Docs.SaveAs(doc, in); err != nil {
			a.ctx.Notify(context.LevelError, err.Error())
			return
		}
		a.pop()
	case cancel:
		a.pop()
	}
}
