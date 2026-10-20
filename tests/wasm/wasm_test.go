package wasm

import (
	"testing"

	"aego/core/log"
	"aego/core/mathx"
	"aego/engine"
	"aego/platform"
	"aego/platform/headless"
	"aego/platform/input"
	"aego/render"
	renderhost "aego/render/host"
)

type probe struct {
	events int
	frames int
}

func (p *probe) Init(*engine.Engine) error { return nil }
func (p *probe) Event(platform.Event)      { p.events++ }
func (p *probe) FixedUpdate(float64)       {}
func (p *probe) Update(float64)            { p.frames++ }
func (p *probe) Render(float64)            {}
func (p *probe) Shutdown()                 {}

type driven struct {
	*headless.Platform
	limit int
	steps int
}

func (d *driven) Drive(step func() bool) {
	for d.steps < d.limit && step() {
		d.steps++
	}
}

func build(t *testing.T, plat platform.Platform) (*engine.Engine, *renderhost.Module) {
	t.Helper()
	mod := renderhost.New(render.Options{})
	e, err := engine.NewBuilder(engine.Config{
		Headless:          true,
		HeadlessFrameTime: 1.0 / 60,
		ResRoot:           t.TempDir(),
		UserRoot:          t.TempDir(),
		AppID:             "aego-wasm",
	}).WithLogger(log.Nop()).WithPlatform(plat).Use(mod).Build()
	if err != nil {
		t.Fatal(err)
	}
	return e, mod
}

func TestInjectedEvents(t *testing.T) {
	plat := headless.New(headless.Options{FrameTime: 1.0 / 60})
	e, mod := build(t, plat)
	defer e.Close()
	if mod.Renderer() == nil {
		t.Fatal("renderer did not start on the null backend")
	}
	app := &probe{}
	plat.Push(platform.Event{Kind: platform.EvKeyDown, Key: input.KeyEvent{Scan: input.KeyA}})
	plat.Push(platform.Event{Kind: platform.EvMouseWheel, Mouse: input.MouseEvent{Wheel: mathx.Vec2{Y: -1}}})
	if !e.Step(app) {
		t.Fatalf("engine stopped: %v", e.Err())
	}
	if !e.Input().KeyDown(input.KeyA) || e.Input().Wheel().Y != -1 {
		t.Fatal("injected input did not reach the input state")
	}
	for i := 1; i < 120; i++ {
		if !e.Step(app) {
			t.Fatalf("engine stopped at frame %d: %v", i, e.Err())
		}
	}
	if app.events != 2 || app.frames != 120 {
		t.Fatalf("events=%d frames=%d", app.events, app.frames)
	}
	if !e.Input().KeyDown(input.KeyA) || e.Input().Wheel().Y != 0 {
		t.Fatal("held key or per-frame wheel is wrong after 120 frames")
	}
}

func TestRunUsesDriver(t *testing.T) {
	plat := &driven{Platform: headless.New(headless.Options{FrameTime: 1.0 / 60}), limit: 10}
	e, _ := build(t, plat)
	app := &probe{}
	if err := e.Run(app); err != nil {
		t.Fatal(err)
	}
	if plat.steps != 10 || app.frames != 10 {
		t.Fatalf("driver steps=%d frames=%d", plat.steps, app.frames)
	}
}
