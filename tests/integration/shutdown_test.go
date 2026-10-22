package integration

import (
	"sync"
	"testing"

	"aego/app"
	"aego/core/log"
	"aego/engine"
	"aego/engine/module"
	"aego/platform"
	"aego/platform/headless"
	"aego/project"
)

// shutdownLog records the order in which modules and the platform go down.
type shutdownLog struct {
	mu     sync.Mutex
	events []string
	late   int
}

func (l *shutdownLog) add(e string) {
	l.mu.Lock()
	l.events = append(l.events, e)
	l.mu.Unlock()
}

// liveOut stands for a device output: once the platform shuts down, any Write is a write into a closed device.
type liveOut struct {
	log    *shutdownLog
	spec   platform.AudioSpec
	mu     sync.Mutex
	dead   bool
	writes int
}

func (o *liveOut) Spec() platform.AudioSpec { return o.spec }
func (o *liveOut) Queued() int              { return 0 }
func (o *liveOut) Pause(bool)               {}

func (o *liveOut) Write(s []float32) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.dead {
		o.log.mu.Lock()
		o.log.late++
		o.log.mu.Unlock()
		return 0
	}
	o.writes++
	return len(s) / o.spec.Channels
}

func (o *liveOut) Close() {}

func (o *liveOut) kill() {
	o.mu.Lock()
	o.dead = true
	o.mu.Unlock()
}

type shutdownPlatform struct {
	*headless.Platform
	log  *shutdownLog
	live bool
	out  *liveOut
}

func (p *shutdownPlatform) OpenAudio(want platform.AudioSpec) (platform.AudioOutput, error) {
	if !p.live {
		return p.Platform.OpenAudio(want)
	}
	p.out = &liveOut{log: p.log, spec: want.WithDefaults()}
	return p.out, nil
}

func (p *shutdownPlatform) Shutdown() {
	p.log.add("platform")
	if p.out != nil {
		p.out.kill()
	}
	p.Platform.Shutdown()
}

type shutdownProbe struct{ log *shutdownLog }

func (shutdownProbe) Info() module.Info                { return module.Info{ID: "shutdownprobe"} }
func (shutdownProbe) Register(*module.Registry) error  { return nil }
func (p shutdownProbe) Init(*module.InitContext) error { return nil }
func (p shutdownProbe) Shutdown()                      { p.log.add("modules") }

type nopApp struct{}

func (nopApp) Init(*engine.Engine) error { return nil }
func (nopApp) Event(platform.Event)      {}
func (nopApp) FixedUpdate(float64)       {}
func (nopApp) Update(float64)            {}
func (nopApp) Render(float64)            {}
func (nopApp) Shutdown()                 {}

func shutdownRun(t *testing.T, live bool, frames int) *shutdownLog {
	t.Helper()
	p, err := project.Load(copyExample(t, "platformer"))
	if err != nil {
		t.Fatal(err)
	}
	l := &shutdownLog{}
	plat := &shutdownPlatform{Platform: headless.New(headless.Options{FrameTime: 1.0 / 60}), log: l, live: live}
	h, err := app.Build(p, app.Config{Engine: engine.Config{Headless: true, HeadlessFrameTime: 1.0 / 60}, Logger: log.Nop(), Platform: plat}, shutdownProbe{log: l})
	if err != nil {
		t.Fatal(err)
	}
	for range frames {
		h.Engine.Step(nopApp{})
	}
	if live && plat.out == nil {
		t.Fatal("the audio module did not open the platform output")
	}
	h.Engine.Close()
	h.Close()
	if live {
		plat.out.mu.Lock()
		w := plat.out.writes
		plat.out.mu.Unlock()
		if w == 0 {
			t.Fatal("the pump never wrote to the live output")
		}
	}
	return l
}

func checkShutdown(t *testing.T, l *shutdownLog) {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.events) != 2 || l.events[0] != "modules" || l.events[1] != "platform" {
		t.Fatalf("shutdown order %v, want modules before platform", l.events)
	}
	if l.late != 0 {
		t.Fatalf("%d write(s) reached the output after the platform shut down", l.late)
	}
}

// Engine.Close shuts the modules down before the platform, even when the engine is closed directly.
func TestShutdownOrder(t *testing.T) {
	checkShutdown(t, shutdownRun(t, false, 30))
}

// With a device-like output the pump runs on its own goroutine; it must stop before the platform closes the device.
func TestShutdownOrderWithPump(t *testing.T) {
	checkShutdown(t, shutdownRun(t, true, 30))
}
