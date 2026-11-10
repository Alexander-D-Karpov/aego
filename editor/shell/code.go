package shell

import (
	"path"
	"strconv"
	"strings"

	"aego/asset/pipeline"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/play"
	"aego/editor/panels/scripteditor"
	"aego/engine/i18n"
	"aego/ui/widgets"
)

func (a *App) attachCode() {
	// Source Save, Autosave and Save All share the import transaction even without watching; scripts compile through the host, shader/material diagnostics through the asset pipeline.
	a.ctx.Docs.Subscribe(func(ev document.Event) {
		if ev.Kind != document.EvSourceSaved || a.ctx.Pipeline == nil {
			return
		}
		if source, metadata := strings.CutSuffix(ev.Path, ".meta"); metadata {
			if id, ok := a.ctx.Pipeline.Lookup(source); ok {
				batch, err := a.ctx.Pipeline.ReloadMetadata(id)
				a.ctx.ApplyBatch(batch)
				a.reportSavedImport(batch, err)
			}
			return
		}
		id, ok := a.ctx.Pipeline.Lookup(ev.Path)
		if !ok {
			return
		}
		switch strings.ToLower(path.Ext(ev.Path)) {
		case ".aeshader", ".glsl", ".aemat", ".aeart", ".aepreset":
		default:
			return
		}
		var batch *pipeline.Batch
		var err error
		if strings.EqualFold(path.Ext(ev.Path), ".aeart") {
			// Linked PNGs depend on the saved art source even without a file
			// watcher. Publish them only after the art import succeeds.
			batch, err = a.ctx.Pipeline.ReimportWithDependents(id)
		} else {
			batch, err = a.ctx.Pipeline.Reimport(id)
		}
		a.ctx.ApplyBatch(batch)
		a.reportSavedImport(batch, err)
	})
	a.consolePanel.OnOpen = func(rel string, line int) {
		a.ctx.Open(context.At(rel, line, 1))
	}
	if a.opt.Code == nil {
		return
	}
	e := a.eng
	a.ctx.Code = a.opt.Code
	a.opt.Code.Attach(scripteditor.Deps{
		Docs:      a.ctx,
		Selection: a.ctx,
		Commands:  a.ctx.Commands,
		Notices:   a.ctx,
		Nav:       a.ctx,
		Problems:  a.ctx,
		Scripts:   a.ctx.Scripts,
		Viewport:  a.viewport,
		Now:       e.Platform().Now,
		Submit:    func(fn func()) { e.Jobs().Submit(fn) },
		OnMain:    func(fn func()) { e.Jobs().OnMain(fn) },
		Presence:  a.presence,
	}, a.ws)
	a.ctx.OpenScript = a.opt.Code.Open
}

// Save publishes bytes before importing. Do not silently describe the result
// as current when the pipeline asks to retry or reports failed subassets.
func (a *App) reportSavedImport(batch *pipeline.Batch, err error) {
	if batch != nil && len(batch.Retry) != 0 {
		a.ctx.Notify(context.LevelWarn, i18n.Note("editor.shell.saved_import_retry", "Saved source has {count} pending imports; reimport the asset after the source settles.", i18n.Str("count", strconv.Itoa(len(batch.Retry)))))
	}
	if err != nil {
		a.ctx.Notify(context.LevelError, err.Error())
	} else if batch == nil {
		a.ctx.Notify(context.LevelError, i18n.Tr("editor.shell.saved_import_missing", "Saved source did not produce an import result."))
	} else if len(batch.Failed) != 0 {
		a.ctx.Notify(context.LevelError, i18n.Note("editor.shell.saved_import_failed", "Saved source has {count} failed imports; see the import diagnostics.", i18n.Str("count", strconv.Itoa(len(batch.Failed)))))
	}
}

func (a *App) drawScriptsBanner(body mathx.Rect) {
	if a.play.State() == play.Stopped || a.ctx.Scripts == nil {
		return
	}
	gen, failed := a.ctx.Scripts.Status()
	if !failed {
		return
	}
	ui := a.ui
	th := &ui.Theme
	msg := i18n.Tr("editor.shell.running_script_build_formatuint_the", "Running script build {formatuint}, the latest source failed to compile", i18n.Str("formatuint", strconv.FormatUint(gen, 10)))
	h := th.RowHeight + 8
	w := min(body.W-16, ui.TextSize(msg).X+2*th.Padding+100)
	r := mathx.Rect{X: body.X + (body.W-w)/2, Y: body.Y + 6, W: w, H: h}
	ui.Rect(r, th.PanelAlt)
	ui.Outline(r, th.Warn)
	ui.PushClip(r)
	ui.Text(msg, mathx.Vec2{X: r.X + th.Padding, Y: ui.TextY(r)}, th.Warn)
	ui.PopClip()
	br := mathx.Rect{X: r.X + r.W - 90, Y: r.Y + 4, W: 84, H: h - 8}
	if widgets.ButtonRect(ui, ui.ID("scripts.problems"), br, i18n.Tr("editor.shell.problems", "Problems")) && a.ctx.ShowProblems != nil {
		a.ctx.ShowProblems("")
	}
}
