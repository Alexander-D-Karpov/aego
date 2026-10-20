package tween

import (
	"testing"

	"aego/core/log"
	"aego/core/mathx"
	"aego/engine"
)

func pausedEngine(t *testing.T) *engine.Engine {
	t.Helper()
	b := engine.NewBuilder(engine.Config{Headless: true, ResRoot: t.TempDir(), UserRoot: t.TempDir()}).WithLogger(log.Nop())
	sys := b.Registry().Systems()
	if err := sys.Register(engine.SystemDesc{ID: SystemGame, Phase: engine.PhasePostUpdate, Scope: engine.ScopePlay, System: step{}}); err != nil {
		t.Fatal(err)
	}
	if err := sys.Register(engine.SystemDesc{
		ID: SystemUnscaled, Phase: engine.PhasePostUpdate, Scope: engine.ScopePlay, Pause: engine.PauseAlways,
		After: []string{SystemGame}, System: step{unscaled: true},
	}); err != nil {
		t.Fatal(err)
	}
	e, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}

func TestUnscaledRunsWhileWorldIsPaused(t *testing.T) {
	e := pausedEngine(t)
	svc, s, h := setup(t)
	w := engine.NewWorld("play")
	w.Time.Configure(e.TimeConfig())
	w.SetService(ServiceID, svc)
	game, _ := svc.ToFloat(h, "Probe.value", 1, 0.25, Options{})
	real, _ := svc.ToVec2(h, "Probe.pos", mathx.Vec2{X: 2, Y: 2}, 0.25, Options{Unscaled: true})
	w.Time.SetPaused(true)
	for range 16 {
		e.TickWorld(w, engine.ScopePlay, float64(dt))
	}
	p := get(s, h)
	if p.Value != 0 || svc.Done(game) {
		t.Fatalf("a game tween moved while paused: %v", p.Value)
	}
	if p.Pos != (mathx.Vec2{X: 2, Y: 2}) || !svc.Done(real) {
		t.Fatalf("an unscaled tween must finish while paused, pos %+v", p.Pos)
	}
	w.Time.SetPaused(false)
	for range 16 {
		e.TickWorld(w, engine.ScopePlay, float64(dt))
	}
	if v := get(s, h).Value; v != 1 || !svc.Done(game) {
		t.Fatalf("game tween after resume %v", v)
	}
}
