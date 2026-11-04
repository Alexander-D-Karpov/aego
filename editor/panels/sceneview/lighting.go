package sceneview

import (
	"aego/core/ids"
	"aego/engine"
	"aego/render/lighting"
	"aego/render/view"
)

func (p *Panel) SetLightingEnvironmentPreview(id ids.NodeID) {
	if p.d.Docs != nil {
		if d := p.d.Docs.Doc(); d != nil {
			p.worldFor(d).runner.SetLightingEnvironmentPreview(id)
		}
	}
}

func (p *Panel) LightingEnvironmentPreview() ids.NodeID {
	if p.d.Docs != nil {
		if d := p.d.Docs.Doc(); d != nil {
			return p.worldFor(d).runner.LightingEnvironmentPreview()
		}
	}
	return ids.NodeID{}
}

// SetLightingView affects only this document's Scene View runner, not its scene.
func (p *Panel) SetLightingView(enabled bool, mode lighting.DebugMode) {
	if p.d.Docs == nil {
		return
	}
	if d := p.d.Docs.Doc(); d != nil {
		p.worldFor(d).runner.SetLightingView(enabled, mode)
	}
}

func (p *Panel) LightingView() (bool, lighting.DebugMode) {
	if p.d.Docs != nil {
		if d := p.d.Docs.Doc(); d != nil {
			return p.worldFor(d).runner.LightingView()
		}
	}
	return true, lighting.DebugLighting
}

func (p *Panel) LightingOutput() view.ID {
	if p.d.Docs != nil {
		if d := p.d.Docs.Doc(); d != nil {
			return p.worldFor(d).runner.OutputID()
		}
	}
	return view.ID{}
}

func (p *Panel) LightingWorld() *engine.World {
	if p.d.Docs != nil {
		if d := p.d.Docs.Doc(); d != nil {
			return p.worldFor(d).w
		}
	}
	return nil
}
