package engine

import (
	"testing"

	"aego/core/errs"
	"aego/core/log"
	"aego/platform"
	"aego/platform/headless"
	"aego/platform/input"
)

type testApp struct {
	e       *Engine
	fixed   int
	updates int
	renders int
	events  int
	aDown   bool
	aUp     bool
}

func (a *testApp) Init(e *Engine) error {
	a.e = e
	return nil
}

func (a *testApp) Event(platform.Event) {
	a.events++
}

func (a *testApp) FixedUpdate(float64) {
	a.fixed++
}

func (a *testApp) Update(float64) {
	a.updates++
	a.aDown = a.e.Input().KeyDown(input.KeyA)
	a.aUp = a.e.Input().KeyReleased(input.KeyA)
}

func (a *testApp) Render(float64) {
	a.renders++
}

func (a *testApp) Shutdown() {}

func newTestEngine(t *testing.T) (*Engine, *headless.Platform) {
	t.Helper()
	hp := headless.New(headless.Options{FrameTime: 1.0 / 60})
	e, err := NewBuilder(Config{
		Headless:          true,
		HeadlessFrameTime: 1.0 / 60,
		ResRoot:           t.TempDir(),
		UserRoot:          t.TempDir(),
		AppID:             "aego-test",
	}).WithLogger(log.Nop()).WithPlatform(hp).Build()
	if err != nil {
		t.Fatal(err)
	}
	return e, hp
}

func TestFixedStepCount(t *testing.T) {
	e, _ := newTestEngine(t)
	defer e.Close()
	app := &testApp{}
	for i := 0; i < 120; i++ {
		if !e.Step(app) {
			t.Fatalf("stopped at frame %d: %v", i, e.Err())
		}
	}
	if app.updates != 120 || app.renders != 120 {
		t.Fatalf("updates=%d renders=%d", app.updates, app.renders)
	}
	if app.fixed < 118 || app.fixed > 120 {
		t.Fatalf("fixed=%d", app.fixed)
	}
	if e.Stats().DrawCalls != 0 {
		t.Fatalf("null device reported draws: %d", e.Stats().DrawCalls)
	}
}

func TestEventsAndFocusReset(t *testing.T) {
	e, hp := newTestEngine(t)
	defer e.Close()
	app := &testApp{}
	e.Step(app)

	hp.Push(platform.Event{Kind: platform.EvKeyDown, Key: input.KeyEvent{Scan: input.KeyA}})
	e.Step(app)
	if !app.aDown {
		t.Fatal("key down not visible in update")
	}
	e.Step(app)
	if !app.aDown {
		t.Fatal("key must stay down across frames")
	}
	hp.Push(platform.Event{Kind: platform.EvWindowFocusLost})
	e.Step(app)
	if app.aDown || !app.aUp {
		t.Fatalf("focus loss did not reset input: down=%v released=%v", app.aDown, app.aUp)
	}
	if app.events == 0 {
		t.Fatal("app did not receive events")
	}
}

func TestQuitEvent(t *testing.T) {
	e, hp := newTestEngine(t)
	defer e.Close()
	app := &testApp{}
	e.Step(app)
	hp.Push(platform.Event{Kind: platform.EvQuit})
	if e.Step(app) {
		t.Fatal("engine kept running after quit")
	}
}

func TestResizeUpdatesMetrics(t *testing.T) {
	e, hp := newTestEngine(t)
	defer e.Close()
	app := &testApp{}
	e.Step(app)
	hp.Push(platform.Event{
		Kind: platform.EvWindowResized,
		Window: platform.WindowMetrics{
			LogicalW: 640, LogicalH: 480,
			PixelW: 1280, PixelH: 960,
			PixelDensity: 2, DisplayScale: 2,
		},
	})
	e.Step(app)
	if m := e.Metrics(); m.PixelW != 1280 || m.LogicalW != 640 || m.PixelDensity != 2 {
		t.Fatalf("metrics = %+v", m)
	}
}

func TestJobsOnMain(t *testing.T) {
	e, _ := newTestEngine(t)
	defer e.Close()
	app := &testApp{}
	e.Step(app)
	done := make(chan struct{})
	ran := 0
	e.Jobs().Submit(func() {
		e.Jobs().OnMain(func() { ran++ })
		close(done)
	})
	<-done
	e.Step(app)
	if ran != 1 {
		t.Fatalf("main queue ran %d times", ran)
	}
}

type testModule struct {
	id      string
	log     *[]string
	initErr error
	gpuOK   bool
}

func (m *testModule) Info() ModuleInfo { return ModuleInfo{ID: m.id, Version: "1"} }

func (m *testModule) Register(*Registry) error {
	*m.log = append(*m.log, "register:"+m.id)
	return nil
}

func (m *testModule) Init(e *Engine) error {
	*m.log = append(*m.log, "init:"+m.id)
	m.gpuOK = e.GPU() != nil && e.VFS() != nil && e.Jobs() != nil
	return m.initErr
}

// bareModule has no Init; Build must skip it rather than fail.
type bareModule struct{ log *[]string }

func (bareModule) Info() ModuleInfo { return ModuleInfo{ID: "bare", Version: "1"} }

func (m bareModule) Register(*Registry) error {
	*m.log = append(*m.log, "register:bare")
	return nil
}

func buildWithModules(t *testing.T, mods ...Module) (*Engine, error) {
	t.Helper()
	b := NewBuilder(Config{
		Headless: true, HeadlessFrameTime: 1.0 / 60,
		ResRoot: t.TempDir(), UserRoot: t.TempDir(), AppID: "aego-test",
	}).WithLogger(log.Nop()).WithPlatform(headless.New(headless.Options{FrameTime: 1.0 / 60}))
	for _, m := range mods {
		b.Use(m)
	}
	return b.Build()
}

func TestModuleInitRunsAfterBackends(t *testing.T) {
	var order []string
	a := &testModule{id: "a", log: &order}
	c := &testModule{id: "c", log: &order}
	e, err := buildWithModules(t, a, bareModule{log: &order}, c)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	want := []string{"register:a", "register:bare", "register:c", "init:a", "init:c"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
	if !a.gpuOK || !c.gpuOK {
		t.Fatal("Init ran before the device/vfs/jobs were available")
	}
}

func TestModuleInitErrorFailsBuild(t *testing.T) {
	var order []string
	bad := &testModule{id: "bad", log: &order, initErr: errBoom}
	e, err := buildWithModules(t, bad)
	if err == nil {
		e.Close()
		t.Fatal("Build must fail when a module Init fails")
	}
	if !errs.Has(err, errs.EngineModule) {
		t.Fatalf("want EngineModule code, got %v", err)
	}
}

var errBoom = errs.New(errs.GPUShaderCompile, "boom")
