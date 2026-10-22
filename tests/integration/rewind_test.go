package integration

import (
	"os"
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/core/mathx"
	"aego/core/rewind"
	"aego/editor/core/play"
	"aego/modules/camera"
	inputmod "aego/modules/input"
	"aego/modules/physics"
	"aego/platform"
	"aego/platform/input"
	"aego/project"
	"aego/sdk"
)

func rewindSession(t *testing.T) (*edit.Session, *play.Controller) {
	t.Helper()
	return rewindSessionWith(t, 0, combatModule{})
}

func rewindSessionWith(t *testing.T, limit int, mods ...sdk.Module) (*edit.Session, *play.Controller) {
	t.Helper()
	p, err := project.Load(copyExample(t, "platformer"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()}, mods...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	pc := s.App.Play()
	if limit > 0 {
		pc.SetHistoryLimit(limit)
	}
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

func TestEditorPauseRecordsNothing(t *testing.T) {
	_, pc := rewindSession(t)
	for range 60 {
		pc.Tick(1.0 / 60)
	}
	pc.Pause()
	want := pc.Timeline()
	hash := playHash(pc)
	for range 300 {
		pc.Tick(1.0 / 60)
	}
	if tl := pc.Timeline(); tl.Frames != want.Frames || tl.Cursor != want.Cursor || playHash(pc) != hash {
		t.Fatalf("timeline %+v while paused, want %+v and an unchanged world", tl, want)
	}
	pc.Step()
	pc.Tick(1.0 / 60)
	if tl := pc.Timeline(); tl.Frames != want.Frames+1 || pc.State() != play.Paused {
		t.Fatalf("timeline %+v after a step, want one more frame and still paused", tl)
	}
	pc.Pause()
	pc.Tick(1.0 / 60)
	if tl := pc.Timeline(); tl.Frames != want.Frames+2 {
		t.Fatalf("timeline %+v after resuming, want recording to go on", tl)
	}
}

func cameraHash(t *testing.T, pc *play.Controller) [32]byte {
	t.Helper()
	s := camera.Of(pc.World())
	if s == nil {
		t.Fatal("the platformer play world has no camera service")
	}
	h := rewind.NewHasher()
	s.Hash(h)
	return h.Sum()
}

func TestEditorRewindKeepsTheCamera(t *testing.T) {
	_, pc := rewindSession(t)
	data, err := os.ReadFile(runFile)
	if err != nil {
		t.Fatal(err)
	}
	actions := inputmod.Of(pc.World())
	if actions == nil {
		t.Fatal("the play world has no input service")
	}
	if _, err := actions.Replay(data); err != nil {
		t.Fatal(err)
	}
	const before, inside = 20, 330
	marks := map[int][32]byte{}
	for i := 1; i <= 360; i++ {
		pc.Tick(1.0 / 60)
		if i == before || i == inside {
			marks[i] = cameraHash(t, pc)
		}
	}
	if camera.Of(pc.World()).Active("").IsZero() {
		t.Fatal("the platformer camera has no active virtual camera")
	}
	if marks[before] == marks[inside] {
		t.Fatal("the camera state is the same before and inside the arena zone")
	}
	for _, f := range []int{inside, before, inside} {
		if err := pc.Seek(f); err != nil {
			t.Fatal(err)
		}
		if cameraHash(t, pc) != marks[f] {
			t.Fatalf("camera state differs after seeking to frame %d", f)
		}
	}
}
