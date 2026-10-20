package integration

import (
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/command"
	inputmod "aego/modules/input"
	"aego/platform/input"
	"aego/project"
)

func TestShiftEscapeReleasesGameView(t *testing.T) {
	p, err := project.Create(t.TempDir(), "Capture", false)
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	ctx := s.App.Context()
	ctx.Commands.Run(command.FileNewScene)
	pc := s.App.Play()
	if err := pc.Start(ctx.Doc()); err != nil {
		t.Fatal(err)
	}
	pc.SetCapture(true, mathx.Vec2{}, mathx.Vec2{X: 640, Y: 360})
	in := s.Engine.Input()

	in.ApplyKeyDown(input.KeyEscape, 0, 0, false)
	pc.Tick(1.0 / 60)
	if !pc.Captured() {
		t.Fatal("Escape released the Game View")
	}
	if !pc.World().Input.KeyPressed(input.KeyEscape) {
		t.Fatal("the game did not get Escape")
	}

	in.ApplyKeyUp(input.KeyEscape, 0, 0)
	in.BeginFrame()
	in.ApplyKeyDown(input.KeyLShift, 0, input.ModShift, false)
	in.ApplyKeyDown(input.KeyEscape, 0, input.ModShift, false)
	pc.Tick(1.0 / 60)
	if pc.Captured() {
		t.Fatal("Shift+Esc did not release the Game View")
	}
	if got := pc.ReleaseHint(); got == "" {
		t.Fatal("no release hint")
	}
	d, _ := ctx.Commands.Get(command.PlayReleaseInput)
	t.Logf("hint %q, keys %q", pc.ReleaseHint(), d.KeysText())
}

func TestEditorPlayReadsActions(t *testing.T) {
	p, err := project.Load(copyExample(t, "platformer"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	pc := s.App.Play()
	if err := pc.StartProject(); err != nil {
		t.Fatal(err)
	}
	pc.SetCapture(true, mathx.Vec2{}, mathx.Vec2{X: 640, Y: 360})
	for range 5 {
		pc.Tick(1.0 / 60)
	}
	w := pc.World()
	sc := pc.Scene()
	player := sc.Find(sc.Root(), "Player")
	x0 := sc.Local(player).Position.X
	s.Engine.Input().ApplyKeyDown(input.KeyD, 0, 0, false)
	for range 10 {
		pc.Tick(1.0 / 60)
	}
	if v := inputmod.Of(w).Axis("Move"); v != 1 {
		t.Fatalf("Move = %v in editor Play, want 1", v)
	}
	if dx := sc.Local(player).Position.X - x0; dx <= 0 {
		t.Fatalf("player did not move right: dx=%v", dx)
	}
}

func TestStepTickAdvancesFrameAndTime(t *testing.T) {
	p, err := project.Load(copyExample(t, "platformer"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	pc := s.App.Play()
	if err := pc.StartProject(); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		pc.Tick(1.0 / 60)
	}
	pc.Pause()
	pc.Tick(1.0 / 60)
	before, sim := pc.Stats(), pc.SimTime()
	for range 10 {
		pc.Tick(1.0 / 60)
	}
	if st := pc.Stats(); st.Tick != before.Tick || st.Updates != before.Updates || pc.SimTime() != sim {
		t.Fatalf("paused frames advanced the game: %+v -> %+v", before, st)
	}
	pc.StepFixed()
	pc.Tick(1.0 / 60)
	st := pc.Stats()
	if st.Tick != before.Tick+1 || st.Updates != before.Updates+1 || pc.SimTime() <= sim {
		t.Fatalf("+1 tick: tick %d->%d frame %d->%d time %v->%v", before.Tick, st.Tick, before.Updates, st.Updates, sim, pc.SimTime())
	}
}
