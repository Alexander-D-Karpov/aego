package pipeline

import (
	"aego/engine"
	"aego/engine/builtin"
	"aego/engine/module"
	"aego/render"
	"aego/scene"
	"aego/scene/frame"
	"aego/script/stdlib"
)

func Types() (*scene.Types, *module.Manifest, error) {
	t, err := builtin.NewTypes()
	if err != nil {
		return nil, nil, err
	}
	man, err := builtin.Load(t, nil, stdlib.Options{})
	if err != nil {
		return nil, nil, err
	}
	if err := t.Schema().Freeze(); err != nil {
		return nil, nil, err
	}
	return t, man, nil
}

type Pipeline struct {
	Runner *frame.Runner
	ctx    engine.Context
	prep   []engine.System
	draw   []engine.System
}

func New(man *module.Manifest) *Pipeline {
	p := &Pipeline{Runner: frame.New()}
	w := engine.NewWorld("demo")
	w.SetService(frame.ServiceID, p.Runner)
	p.ctx.World = w
	for _, d := range man.Systems {
		switch d.Phase {
		case engine.PhaseRenderPrepare:
			p.prep = append(p.prep, d.System)
		case engine.PhaseRender:
			p.draw = append(p.draw, d.System)
		}
	}
	return p
}

func (p *Pipeline) Render(e *engine.Engine, r *render.Renderer) {
	m := e.Metrics()
	p.Runner.Prepare(m.PixelW, m.PixelH, m.Density())
	p.ctx.Engine = e
	for _, s := range p.prep {
		s.Run(&p.ctx)
	}
	for _, s := range p.draw {
		s.Run(&p.ctx)
	}
	p.Runner.Submit(r)
}
