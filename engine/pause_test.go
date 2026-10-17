package engine

import "testing"

type counter struct{ n *int }

func (c counter) Run(*Context) { *c.n++ }

func TestPauseModes(t *testing.T) {
	b := NewBuilder(Config{Headless: true, HeadlessFrameTime: 1.0 / 60})
	var normal, always, when, fixed int
	sys := b.Registry().Systems()
	for _, d := range []SystemDesc{
		{ID: "t.normal", Phase: PhaseUpdate, System: counter{&normal}},
		{ID: "t.always", Phase: PhaseUpdate, Pause: PauseAlways, System: counter{&always}},
		{ID: "t.when", Phase: PhaseUpdate, Pause: PauseWhenPaused, System: counter{&when}},
		{ID: "t.fixed", Phase: PhasePhysics, System: counter{&fixed}},
	} {
		if err := sys.Register(d); err != nil {
			t.Fatal(err)
		}
	}
	e, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	w := NewWorld("test")
	w.Time.Configure(e.TimeConfig())
	for range 3 {
		e.TickWorld(w, ScopePlay, 1.0/60)
	}
	if normal != 3 || always != 3 || when != 0 || fixed != 3 {
		t.Fatalf("running: %d %d %d %d", normal, always, when, fixed)
	}
	w.Time.SetPaused(true)
	for range 3 {
		e.TickWorld(w, ScopePlay, 1.0/60)
	}
	if normal != 3 || always != 6 || when != 3 || fixed != 3 {
		t.Fatalf("paused: %d %d %d %d", normal, always, when, fixed)
	}
	w.Time.StepFixed()
	e.TickWorld(w, ScopePlay, 1.0/60)
	if fixed != 4 || normal != 3 {
		t.Fatalf("step fixed: fixed %d normal %d", fixed, normal)
	}
	w.Time.StepFrame()
	e.TickWorld(w, ScopePlay, 1.0/60)
	if normal != 4 || fixed > 5 {
		t.Fatalf("step frame: normal %d fixed %d", normal, fixed)
	}
}
