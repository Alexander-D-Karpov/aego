package sceneview

import (
	"aego/editor/core/context"
	"aego/engine/i18n"
)

const CommandCapture = "scene.capture"

func (p *Panel) RequestCapture() error {
	if p.d.Docs.Doc() == nil || p.target == nil {
		p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.sceneview.open_a_scene_to_capture", "Open a scene to capture the Scene View"))
		return nil
	}
	if p.d.Capture == nil {
		return nil
	}
	p.captureReq = true
	return nil
}

func (p *Panel) flushCapture() {
	if !p.captureReq || p.target == nil || p.d.Capture == nil {
		return
	}
	p.captureReq = false
	name := "scene"
	if d := p.d.Docs.Doc(); d != nil {
		name = d.Name()
	}
	p.d.Capture(p.target.Texture, p.target.W, p.target.H, name)
}
