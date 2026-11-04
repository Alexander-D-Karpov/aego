package shell

import (
	"aego/editor/core/services"
	"errors"
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

func (a *App) AgentExporter() services.Exporter { return a.opt.Exporter }

func (a *App) CaptureAgentView(view string) ([]byte, uint64, string, error) {
	d := a.ctx.Doc()
	if d == nil {
		return nil, 0, "", errors.New("no open scene")
	}
	t, tick := a.sceneView.LastRender()
	if view == "game" {
		t, tick = a.gameView.LastRender()
		if playDoc := a.play.PlayDoc(); playDoc != nil {
			d = playDoc
		}
	} else if view != "scene" {
		return nil, 0, "", errors.New("unknown view")
	}
	if t == nil || d == nil {
		return nil, 0, "", errors.New("view has no frame for this scene; show the view and start Play for a game capture, or use scene_run with shot")
	}
	if t.W <= 0 || t.H <= 0 || int64(t.W)*int64(t.H) > 16<<20 {
		return nil, 0, "", errors.New("invalid capture size")
	}
	dev := a.ctx.Engine.GPU()
	if dev == nil {
		return nil, 0, "", errors.New("view has no graphics device")
	}
	buf := make([]byte, t.W*t.H*4)
	if err := dev.ReadPixels(t.Texture, 0, 0, t.W, t.H, buf); err != nil {
		return nil, 0, "", err
	}
	png, err := capture.Encode(t.W, t.H, buf, captureFlip)
	return png, tick, d.Path, err
}
