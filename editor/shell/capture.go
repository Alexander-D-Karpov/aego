package shell

import (
	"time"

	"aego/editor/core/capture"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/panels/sceneview"
	"aego/engine/i18n"
	"aego/gpu"
	"aego/platform/input"
)

const captureFlip = true

func (a *App) captureTexture(tex gpu.TextureHandle, w, h int, name string) {
	dev := a.ctx.Engine.GPU()
	if dev == nil {
		return
	}
	buf := make([]byte, w*h*4)
	if err := dev.ReadPixels(tex, 0, 0, w, h, buf); err != nil {
		a.ctx.Notify(context.LevelError, i18n.Note("editor.shell.cannot_read_the_scene_view", "cannot read the Scene View: {error}", i18n.Str("error", err.Error())))
		return
	}
	rel, err := capture.Save(a.opt.Project.Root, name, w, h, buf, captureFlip, time.Now())
	if err != nil {
		a.ctx.Notify(context.LevelError, err.Error())
		return
	}
	a.ctx.Notify(context.LevelInfo, i18n.Note("editor.shell.saved_rel", "Saved {rel}", i18n.Str("rel", rel)))
}

func (a *App) registerCapture() error {
	return a.ctx.Commands.Register(command.Desc{
		ID: sceneview.CommandCapture, Name: "Capture Scene View", Category: "Scene", Order: 340,
		Shortcut: command.Key(input.KeyF12, command.ModPrimary|command.ModShift),
		Enabled:  func() bool { return a.ctx.Doc() != nil },
		Run:      a.sceneView.RequestCapture,
	})
}
