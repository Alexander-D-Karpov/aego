package engine_test

import (
	"testing"

	"aego/core/log"
	"aego/engine"
	"aego/platform"
)

type nopApp struct{}

func (nopApp) Init(*engine.Engine) error { return nil }
func (nopApp) Event(platform.Event)      {}
func (nopApp) FixedUpdate(float64)       {}
func (nopApp) Update(float64)            {}
func (nopApp) Render(float64)            {}
func (nopApp) Shutdown()                 {}

func TestTracedFrameAllocatesNothing(t *testing.T) {
	e, err := engine.NewBuilder(engine.Config{Headless: true, Trace: true}).WithLogger(log.Nop()).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	var app nopApp
	for i := 0; i < 30; i++ {
		e.Step(app)
	}
	if n := testing.AllocsPerRun(100, func() { e.Step(app) }); n != 0 {
		t.Fatalf("%v allocations per traced frame", n)
	}
	f := e.Trace().Frame(0)
	if f == nil || len(f.Events) == 0 {
		t.Fatal("no zones recorded")
	}
}
