package playground

import (
	"aego/core/log"
	"aego/engine"
	"aego/engine/module"
	"aego/scene"
	"aego/scene/frame"
)

// sim is a windowless engine that steps the module systems of a console run, so tweens, timers and physics work there as in Play.
func (p *Playground) sim() (*engine.Engine, error) {
	if p.simEngine != nil {
		return p.simEngine, nil
	}
	b := engine.NewBuilder(engine.Config{Headless: true, HeadlessFrameTime: 1.0 / 60, Seed: 1, AppID: "aego-playground-console"}).WithLogger(log.Nop())
	if err := frame.Register(b.Registry()); err != nil {
		return nil, err
	}
	e, err := b.Use(p.man.Engine()).Build()
	if err != nil {
		return nil, err
	}
	if err := p.man.Init(&module.InitContext{Engine: e, Log: log.Nop(), Sections: p.sections}); err != nil {
		e.Close()
		return nil, err
	}
	p.simEngine = e
	return e, nil
}

func (p *Playground) attach(w *engine.World, s *scene.Scene, lg *log.Logger) (*engine.Engine, func(), error) {
	e, err := p.sim()
	if err != nil {
		return nil, nil, err
	}
	w.Time.Configure(e.TimeConfig())
	detach, err := p.man.Attach(&module.AttachContext{World: w, Scene: s, Log: lg, Play: true})
	if err != nil {
		return nil, nil, err
	}
	return e, detach, nil
}
