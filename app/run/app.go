package run

import (
	"aego/asset"
	"aego/core/log"
	"aego/engine"
	"aego/platform"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
)

type sceneApp struct {
	app      *App
	unsub    func()
	reloads  int
	lastKind asset.ArtifactKind
}

func (s *sceneApp) Init(*engine.Engine) error {
	s.unsub = s.app.Loader.Subscribe(func(r asset.Reloaded) {
		s.reloads++
		s.lastKind = r.Kind
		switch {
		case r.Kind == asset.ArtScene && r.ID == s.app.sceneID:
			s.app.reloadScene()
		case r.Kind == asset.ArtScript:
			s.app.hotReload()
		}
	})
	return nil
}

func (s *sceneApp) Event(platform.Event) {}

func (s *sceneApp) FixedUpdate(float64) {}

func (s *sceneApp) Update(float64) {}

func (s *sceneApp) Render(float64) {}

func (s *sceneApp) Shutdown() {
	if s.unsub != nil {
		s.unsub()
		s.unsub = nil
	}
}

func (s *sceneApp) Reloads() int {
	return s.reloads
}

func (a *App) reloadScene() {
	src, ok := a.Loader.Scene(a.sceneID)
	if !ok {
		a.log.Error("scene artifact disappeared", log.F("asset", a.sceneID.String()))
		return
	}
	next, report, err := sceneio.Load(src, "main.aescene", a.Types, a.Loader, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		a.log.Error("scene reload failed, keeping the current scene", log.F("err", err.Error()))
		return
	}
	for _, w := range report.Warnings {
		a.log.Warn("scene warning", log.F("path", w.Path), log.F("line", w.Pos.Line), log.F("msg", w.Msg))
	}
	a.Scene = next
	if a.unrec != nil {
		a.unrec()
	}
	a.unrec = frame.Record(next, a.Engine.MainWorld().Recorder)
	a.Runner.Bind(next, a.Renderer, a.Loader)
	a.Engine.SetService(scene.ServiceID, next)
	if err := a.Scripts.Instantiate(next); err != nil {
		a.log.Error("scripts could not start on the reloaded scene", log.F("err", err.Error()))
	}
	if err := a.attach(); err != nil {
		a.log.Error("modules could not attach to the reloaded scene", log.F("err", err.Error()))
	}
	a.log.Info("scene reloaded")
}
