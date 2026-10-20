package display

import (
	"aego/engine"
	"aego/scene/frame"
)

const SystemScale = ID + ".scale"

type scaleSystem struct {
	m *Module
}

func (s scaleSystem) Run(ctx *engine.Context) {
	if r := frame.Of(ctx); r != nil {
		r.SetRenderScale(s.m.renderScale())
	}
}
