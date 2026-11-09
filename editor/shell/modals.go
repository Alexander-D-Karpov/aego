package shell

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine"
	"aego/engine/i18n"
	"aego/platform/input"
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
	modalAbout
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
	vals    []string
	focus   bool
	scroll  widgets.ScrollState
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
		msg := i18n.Tr("editor.shell.an_autosave_of_path_from", "An autosave of {path} from {v} was found.", i18n.Str("path", md.rec.Path), i18n.Str("v", md.rec.Time.Format("2006-01-02 15:04")))
		switch widgets.Confirm(ui, id, i18n.Tr("editor.shell.recover_unsaved_changes", "Recover unsaved changes"), msg, i18n.Tr("editor.shell.restore", "Restore"), i18n.Tr("editor.shell.discard", "Discard"), i18n.Tr("editor.shell.later", "Later")) {
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
		msg := i18n.Tr("editor.shell.path_was_changed_on_disk", "{path} was changed on disk while it has unsaved edits.", i18n.Str("path", md.path))
		first := i18n.Tr("editor.shell.reload", "Reload")
		if md.removed {
			msg = i18n.Tr("editor.shell.path_was_deleted_on_disk", "{path} was deleted on disk.", i18n.Str("path", md.path))
			first = i18n.Tr("editor.shell.close", "Close")
		}
		switch widgets.Confirm(ui, id, i18n.Tr("editor.shell.scene_changed_outside_the_editor", "Scene changed outside the editor"), msg, first, i18n.Tr("editor.shell.keep_mine", "Keep mine"), i18n.Tr("editor.shell.save_as", "Save As…")) {
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
		msg := i18n.Tr("editor.shell.dirtycount_scene_s_have_unsaved", "{dirtycount} scene(s) have unsaved changes.", i18n.Str("dirtycount", strconv.Itoa(docs.DirtyCount())))
		switch widgets.Confirm(ui, id, i18n.Tr("editor.shell.quit_aego_editor", "Quit Aego Editor"), msg, i18n.Tr("editor.shell.save_all", "Save All"), i18n.Tr("editor.shell.discard", "Discard"), i18n.Tr("editor.shell.cancel", "Cancel")) {
		case 0:
			if err := docs.SaveAll(); err != nil {
				if !errors.Is(err, document.ErrSavePending) {
					a.ctx.Notify(context.LevelError, err.Error())
				}
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
		switch widgets.Confirm(ui, id, i18n.Tr("editor.shell.close_scene", "Close scene"), i18n.Tr("editor.shell.path_has_unsaved_changes", "{path} has unsaved changes.", i18n.Str("path", md.path)), i18n.Tr("editor.shell.save", "Save"), i18n.Tr("editor.shell.discard", "Discard"), i18n.Tr("editor.shell.cancel", "Cancel")) {
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
		if widgets.Confirm(ui, id, i18n.Tr("editor.shell.project_may_be_open_elsewhere", "Project may be open elsewhere"), msg, i18n.Tr("editor.shell.continue", "Continue")) == 0 {
			a.pop()
		}
	case modalAsk:
		if len(md.ask.Fields) > 0 {
			a.drawPrompt(id)
			return
		}
		if c := widgets.Confirm(ui, id, md.ask.Title, md.ask.Message, md.ask.Buttons...); c >= 0 {
			a.pop()
			if md.ask.Then != nil {
				md.ask.Then(c)
			}
		}
	case modalAbout:
		a.drawAbout(id)
	}
}

func (a *App) drawAbout(id core.ID) {
	ui, th := a.ui, &a.ui.Theme
	size := ui.Size()
	w := max(float32(1), min(float32(480), size.X-40))
	h := min(size.Y, 2*th.Padding+7*(th.RowHeight+th.Spacing))
	r := mathx.Rect{X: (size.X - w) / 2, Y: max(0, (size.Y-h)/3), W: w, H: h}
	if !ui.BeginModal(id, r) {
		return
	}
	widgets.Label(ui, i18n.Tr("editor.shell.about_aego", "About Aego"))
	widgets.LabelDim(ui, i18n.Tr("editor.shell.about_version", "Aego engine {version}", i18n.Str("version", engine.Version))+" · "+runtime.Version()+" · "+runtime.GOOS+"/"+runtime.GOARCH)
	ui.Cursor().Next(0)
	widgets.LabelDim(ui, i18n.Tr("editor.shell.created_by", "Created by: sanspie"))
	link := ui.Cursor().Next(0)
	link.W = min(link.W, ui.TextSize("about.akarpov.ru").X)
	linkID := ui.ID("about.developer")
	openDeveloper := ui.Clicked(linkID, link)
	color := th.Accent
	if ui.Hot() == linkID {
		ui.SetCursorShape(core.CursorPointer)
		ui.Rect(mathx.Rect{X: link.X, Y: link.Y + link.H - 2, W: link.W, H: 1}, color)
	}
	ui.Text("about.akarpov.ru", mathx.Vec2{X: link.X, Y: ui.TextY(link)}, color)
	widgets.LabelDim(ui, ":3")
	row := ui.Cursor().Next(0)
	closeLabel := i18n.Tr("editor.shell.close", "Close")
	websiteLabel := i18n.Tr("editor.shell.open_website", "Open website")
	closeW := ui.TextSize(closeLabel).X + 28
	websiteW := ui.TextSize(websiteLabel).X + 28
	closeRect := mathx.Rect{X: row.X + row.W - closeW, Y: row.Y, W: closeW, H: row.H}
	websiteRect := mathx.Rect{X: closeRect.X - websiteW - 6, Y: row.Y, W: websiteW, H: row.H}
	openRepo := widgets.ButtonRect(ui, ui.ID("about.repository"), websiteRect, websiteLabel)
	close := widgets.ButtonRect(ui, ui.ID("about.close"), closeRect, closeLabel) || ui.KeyPressed(input.KeyEscape)
	ui.EndModal()
	if openDeveloper {
		a.openSupportURL(developerURL)
	}
	if openRepo {
		a.openSupportURL(repositoryURL)
	}
	if close {
		a.pop()
	}
}

type promptLayout struct {
	window, body mathx.Rect
	content      float32
	fields       []float32
	buttons      [2]mathx.Rect
}

// Prompts share the existing modal, wrapping and scroll widgets. Keep actions
// outside the scrollable content so a long warning or translated label never
// hides confirmation, cancellation or a field below the window.
func layoutPrompt(ui *core.Context, q context.Question, labels [2]string) promptLayout {
	th, size := &ui.Theme, ui.Size()
	preferred := float32(460)
	buttonWidth := [2]float32{}
	for i, label := range labels {
		buttonWidth[i] = max(float32(88), ui.TextSize(label).X+28)
	}
	preferred = max(preferred, min(float32(680), ui.TextSize(q.Title).X+2*th.Padding), min(float32(680), buttonWidth[0]+buttonWidth[1]+th.Spacing+2*th.Padding))
	w := max(float32(1), min(preferred, size.X-40))
	inner := max(float32(1), w-2*th.Padding)
	buttonHeight := [2]float32{}
	for i, label := range labels {
		buttonWidth[i] = min(inner, buttonWidth[i])
		buttonHeight[i] = float32(max(1, len(widgets.WrapLines(ui, label, max(float32(1), buttonWidth[i]-28))))) * th.RowHeight
	}
	stacked := buttonWidth[0]+th.Spacing+buttonWidth[1] > inner
	buttonsH := max(buttonHeight[0], buttonHeight[1])
	if stacked {
		buttonsH = buttonHeight[0] + th.Spacing + buttonHeight[1]
	}
	contentSize := func(width float32) (float32, []float32) {
		rows := len(widgets.WrapLines(ui, q.Title, width)) + len(widgets.WrapLines(ui, q.Message, width))
		fields := make([]float32, len(q.Fields))
		for i, field := range q.Fields {
			rows += len(widgets.WrapLines(ui, field, width))
			fields[i] = float32(rows) * (th.RowHeight + th.Spacing)
			rows++
		}
		return max(float32(0), float32(rows)*(th.RowHeight+th.Spacing)-th.Spacing), fields
	}
	bodyW := inner
	content, fields := contentSize(bodyW)
	available := max(float32(0), size.Y-40-2*th.Padding-th.Spacing-buttonsH)
	if content > available {
		bodyW = max(float32(1), inner-th.ScrollbarW-th.Spacing)
		content, fields = contentSize(bodyW)
	}
	visible := min(content, available)
	h := 2*th.Padding + visible + th.Spacing + buttonsH
	window := mathx.Rect{X: (size.X - w) / 2, Y: max(float32(0), (size.Y-h)/3), W: w, H: h}
	l := promptLayout{window: window, body: mathx.Rect{X: window.X + th.Padding, Y: window.Y + th.Padding, W: bodyW, H: visible}, content: content, fields: fields}
	right, bottom := window.X+w-th.Padding, window.Y+h-th.Padding
	l.buttons[1] = mathx.Rect{X: right - buttonWidth[1], Y: bottom - buttonHeight[1], W: buttonWidth[1], H: buttonHeight[1]}
	l.buttons[0] = mathx.Rect{X: l.buttons[1].X - th.Spacing - buttonWidth[0], Y: bottom - buttonsH, W: buttonWidth[0], H: buttonHeight[0]}
	if stacked {
		l.buttons[0].X = right - buttonWidth[0]
	}
	return l
}

func promptButton(ui *core.Context, id core.ID, r mathx.Rect, label string, destructive bool) bool {
	textColor := ui.Theme.Text
	var clicked bool
	if destructive {
		clicked = ui.Clicked(id, r)
		bg := ui.Theme.DestructiveButton(ui.Hot() == id, ui.Active() == id)
		ui.Box(r, ui.Theme.Radius.Button, bg, ui.Theme.Border)
		textColor = mathx.White
	} else {
		clicked = widgets.ButtonRect(ui, id, r, "")
	}
	lines := widgets.WrapLines(ui, label, max(float32(1), r.W-28))
	y := r.Y + (r.H-float32(len(lines))*ui.Theme.RowHeight)/2
	for _, line := range lines {
		ui.TextIn(mathx.Rect{X: r.X, Y: y, W: r.W, H: ui.Theme.RowHeight}, line, textColor, core.AlignCenter)
		y += ui.Theme.RowHeight
	}
	return clicked
}

func (a *App) drawPrompt(id core.ID) {
	ui := a.ui
	th := &ui.Theme
	md := &a.modals[0]
	q := md.ask
	for len(md.vals) < len(q.Fields) {
		md.vals = append(md.vals, "")
	}
	labels := [2]string{i18n.Tr("editor.common.ok", "OK"), i18n.Tr("editor.common.cancel", "Cancel")}
	for i := range min(len(q.Buttons), len(labels)) {
		labels[i] = q.Buttons[i]
	}
	// Reserve room for either label before drawing the input. The callback is
	// evaluated again after text input so the red action follows the typed path
	// in the same frame, including when Enter submits it.
	layoutLabels := labels
	if q.DestructiveLabel != "" && ui.TextSize(q.DestructiveLabel).X > ui.TextSize(layoutLabels[0]).X {
		layoutLabels[0] = q.DestructiveLabel
	}
	l := layoutPrompt(ui, q, layoutLabels)
	if !ui.BeginModal(id, l.window) {
		return
	}
	base := ui.ID("prompt")
	submit, escape := false, false
	// The key that selected a Create action must not also submit the newly
	// opened prompt or remove focus from its text field in the same frame.
	opening := md.focus
	openingEnter := opening && (ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter))
	// Choice prompts support Tab and arrow selection without changing the
	// established submission behavior of existing text-only questions.
	if len(q.Choices) > 0 && !opening && ui.KeyPressed(input.KeyTab) && len(q.Fields) > 0 {
		at := 0
		for i := range q.Fields {
			if ui.Focus() == core.IDSeq(base, i) {
				at = i
				break
			}
		}
		delta := 1
		if ui.Mods().Has(input.ModShift) {
			delta = -1
		}
		ui.CloseAllPopups()
		ui.SetFocus(core.IDSeq(base, (at+delta+len(q.Fields))%len(q.Fields)))
	}
	scroll := ui.ModalScroll()
	if opening {
		*scroll = 0
	}
	maxScroll := max(float32(0), l.content-l.body.H)
	*scroll = mathx.Clamp(*scroll, 0, maxScroll)
	if ui.MouseOver(ui.ID("prompt.scroll"), l.body) {
		*scroll = mathx.Clamp(*scroll-ui.ScrollY(th.RowHeight*3), 0, maxScroll)
	}
	// Keyboard navigation and typing keep the active field reachable, while
	// opening a prompt initially shows the complete start of its warning.
	if !opening && (ui.KeyPressed(input.KeyTab) || len(ui.TextInput()) > 0) {
		for i, top := range l.fields {
			if ui.Focus() != core.IDSeq(base, i) {
				continue
			}
			if top < *scroll {
				*scroll = top
			} else if top+th.RowHeight > *scroll+l.body.H {
				*scroll = top + th.RowHeight - l.body.H
			}
			*scroll = mathx.Clamp(*scroll, 0, maxScroll)
		}
	}
	ui.PushClip(l.body)
	*ui.Cursor() = core.Cursor{X: l.body.X, Y: l.body.Y - *scroll, W: l.body.W, RowH: th.RowHeight, Space: th.Spacing}
	widgets.LabelWrapped(ui, q.Title, false)
	widgets.LabelWrapped(ui, q.Message, true)
	for i, f := range q.Fields {
		widgets.LabelWrapped(ui, f, true)
		fid := core.IDSeq(base, i)
		var choices []string
		if i < len(q.Choices) {
			choices = q.Choices[i]
		}
		if md.focus && i == 0 && !openingEnter {
			ui.SetFocus(fid)
			if len(choices) == 0 {
				ui.EditBuffer(fid, md.vals[i]).SelectAll()
			}
			md.focus = false
		}
		if len(choices) > 0 {
			selected := 0
			for n, value := range choices {
				if value == md.vals[i] {
					selected = n
					break
				}
			}
			if !opening && ui.Focus() == fid {
				if ui.KeyPressed(input.KeyDown) || ui.KeyPressed(input.KeyRight) {
					selected = (selected + 1) % len(choices)
				}
				if ui.KeyPressed(input.KeyUp) || ui.KeyPressed(input.KeyLeft) {
					selected = (selected + len(choices) - 1) % len(choices)
				}
				if ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter) {
					if ui.PopupOpen(fid) {
						ui.ClosePopup(fid)
					} else {
						submit = true
					}
				}
				escape = escape || (ui.KeyPressed(input.KeyEscape) && !ui.PopupOpen(fid))
			}
			widgets.Combo(ui, fid, ui.Cursor().Next(0), &selected, choices, false)
			if ui.Active() == fid {
				ui.SetFocus(fid)
			}
			md.vals[i] = choices[selected]
			continue
		}
		if !opening && ui.Focus() == fid {
			submit = submit || ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter)
			escape = escape || ui.KeyPressed(input.KeyEscape)
		}
		widgets.TextFieldRect(ui, fid, ui.Cursor().Next(0), &md.vals[i], true)
	}
	ui.PopClip()
	if maxScroll > 0 {
		track := mathx.Rect{X: l.body.X + l.body.W + th.Spacing, Y: l.body.Y, W: th.ScrollbarW, H: l.body.H}
		widgets.ScrollBar(ui, ui.ID("prompt.scrollbar"), track, scroll, l.content, l.body.H, &md.scroll)
	}
	destructive := q.DestructiveWhen != nil && q.DestructiveLabel != "" && q.DestructiveWhen(md.vals)
	if destructive {
		labels[0] = q.DestructiveLabel
	}
	ok := promptButton(ui, ui.ID("prompt.ok"), l.buttons[0], labels[0], destructive)
	cancel := promptButton(ui, ui.ID("prompt.cancel"), l.buttons[1], labels[1], false)
	ui.EndModal()
	switch {
	case ok || submit:
		vals := md.vals
		a.pop()
		if q.Submit != nil {
			q.Submit(vals)
		}
	case cancel || escape:
		a.pop()
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
	widgets.Label(ui, i18n.Tr("editor.shell.save_scene_as", "Save scene as"))
	in := a.modals[0].input
	widgets.TextField(ui, ui.ID("saveas.path"), &in)
	a.modals[0].input = in
	row := ui.Cursor().Next(0)
	ok := widgets.ButtonRect(ui, ui.ID("saveas.ok"), mathx.Rect{X: row.X + row.W - 184, Y: row.Y, W: 88, H: row.H}, i18n.Tr("editor.shell.save", "Save"))
	cancel := widgets.ButtonRect(ui, ui.ID("saveas.cancel"), mathx.Rect{X: row.X + row.W - 90, Y: row.Y, W: 88, H: row.H}, i18n.Tr("editor.shell.cancel", "Cancel"))
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
