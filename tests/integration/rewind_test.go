package integration

import (
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/play"
	"aego/modules/physics"
	"aego/platform"
	"aego/platform/input"
	"aego/project"
)

func rewindSession(t *testing.T) (*edit.Session, *play.Controller) {
	t.Helper()
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
	pc.SetCapture(true, mathx.Vec2{}, mathx.Vec2{X: 1e6, Y: 1e6})
	return s, pc
}

func playHash(pc *play.Controller) [32]byte {
	return physics.SimOf(pc.World()).World().Snapshot().Hash()
}

func TestEditorRewindReplaysTheRecording(t *testing.T) {
	s, pc := rewindSession(t)
	in := s.Host.Engine.Input()
	hold := func(k input.Scancode, down bool) {
		kind := platform.EvKeyUp
		if down {
			kind = platform.EvKeyDown
		}
		platform.ApplyInput(in, platform.Event{Kind: kind, Key: input.KeyEvent{Scan: k}})
	}
	var marks = map[int][32]byte{}
	for i := 0; i < 200; i++ {
		hold(input.KeyD, i >= 20 && i < 90)
		hold(input.KeySpace, i >= 60 && i < 64)
		pc.Tick(1.0 / 60)
		if i+1 == 100 || i+1 == 200 {
			marks[i+1] = playHash(pc)
		}
	}
	if tl := pc.Timeline(); tl.Frames != 200 || tl.Usable != 200 {
		t.Fatalf("timeline %+v, want 200 recorded frames", tl)
	}
	if err := pc.Seek(100); err != nil {
		t.Fatal(err)
	}
	if pc.State() != play.Paused || pc.Timeline().Cursor != 100 {
		t.Fatalf("state %v cursor %d after seeking back", pc.State(), pc.Timeline().Cursor)
	}
	if playHash(pc) != marks[100] {
		t.Fatal("physics differs after rewinding to frame 100")
	}
	pc.Tick(1.0 / 60)
	if pc.Timeline().Cursor != 100 || playHash(pc) != marks[100] {
		t.Fatal("a paused world in the past must not tick")
	}
	if err := pc.Seek(200); err != nil {
		t.Fatal(err)
	}
	if playHash(pc) != marks[200] {
		t.Fatal("physics differs after seeking forward to frame 200")
	}
	if err := pc.Seek(150); err != nil {
		t.Fatal(err)
	}
	pc.Pause()
	pc.Tick(1.0 / 60)
	if tl := pc.Timeline(); tl.Frames != 151 || tl.Cursor != 151 {
		t.Fatalf("timeline %+v, playing from the past must drop the future", tl)
	}
	pc.Taint("test")
	pc.Tick(1.0 / 60)
	if tl := pc.Timeline(); tl.Usable != 151 || tl.Why != "test" || !tl.Beyond {
		t.Fatalf("timeline %+v after a taint", tl)
	}
	if err := pc.StepBack(); err != nil || pc.Timeline().Cursor != 151 {
		t.Fatalf("step back past a taint lands on the cut, got %d (%v)", pc.Timeline().Cursor, err)
	}
	pc.Invalidate("settings")
	if tl := pc.Timeline(); tl.Usable != 0 || tl.Why != "settings" {
		t.Fatalf("timeline %+v, a changed setting must drop the whole recording", tl)
	}
}

func TestEditorRewindReplaysBreaks(t *testing.T) {
	_, pc := rewindSession(t)
	fired := false
	pc.SetBreak(func() (string, bool) {
		if !fired && pc.World().Time.Tick() == 31 {
			fired = true
			return "test break", true
		}
		return "", false
	})
	for i := 0; i < 40; i++ {
		pc.Tick(3.0 / 60)
		if pc.State() == play.Paused {
			pc.Pause()
		}
	}
	if !fired {
		t.Fatal("the break never fired")
	}
	want, frames := playHash(pc), pc.Timeline().Frames
	if err := pc.Seek(0); err != nil {
		t.Fatal(err)
	}
	if err := pc.Seek(frames); err != nil {
		t.Fatal(err)
	}
	if playHash(pc) != want {
		t.Fatal("replay of a frame cut short by a break differs")
	}
}
