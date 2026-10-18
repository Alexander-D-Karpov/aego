package integration

import (
	"aego/contracts/persistence"
	"aego/save"
	"aego/script/runtime"
	"aego/script/vm"
	"os"
	"slices"
	"testing"
	"time"

	"aego/core/mathx"
	"aego/editor/core/history/commands"
	"aego/editor/core/play"
	"aego/engine"
	"aego/platform"
	"aego/platform/input"
	"aego/scene"
	"aego/sdk"
)

func keyHold(s interface{ Input() *input.State }, k input.Scancode, down bool) {
	kind := platform.EvKeyUp
	if down {
		kind = platform.EvKeyDown
	}
	platform.ApplyInput(s.Input(), platform.Event{Kind: kind, Key: input.KeyEvent{Scan: k}})
}

func drive(e interface{ Input() *input.State }, pc *play.Controller, from, to int, at func(frame int)) {
	for i := from; i < to; i++ {
		keyHold(e, input.KeyD, i%400 >= 20 && i%400 < 200)
		keyHold(e, input.KeyA, i%400 >= 220 && i%400 < 380)
		keyHold(e, input.KeySpace, i%90 >= 60 && i%90 < 64)
		pc.Tick(1.0 / 60)
		if at != nil {
			at(i + 1)
		}
	}
}

func movePlayer(t *testing.T, pc *play.Controller, to mathx.Vec2) {
	t.Helper()
	sc := pc.Scene()
	h := sc.Find(sc.Root(), "Player")
	before := sc.Local(h)
	after := before
	after.Position = to
	if err := pc.PlayDoc().History.Exec(&commands.SetTransform{Node: sc.ID(h), Before: before, After: after}); err != nil {
		t.Fatal(err)
	}
}

func playerPos(pc *play.Controller) mathx.Vec2 {
	sc := pc.Scene()
	return sc.Local(sc.Find(sc.Root(), "Player")).Position
}

func pausedHash(pc *play.Controller) [32]byte {
	pc.Pause()
	defer pc.Pause()
	return pc.StateHash()
}

func TestKeyframeSeekMatchesLinearRun(t *testing.T) {
	s, pc := rewindSession(t)
	pc.SetKeyEvery(300)
	marks := map[int][32]byte{}
	drive(s.Host.Engine, pc, 0, 10000, func(f int) {
		if f == 5000 || f == 7777 {
			marks[f] = pausedHash(pc)
		}
	})
	if tl := pc.Timeline(); tl.Keys < 30 || tl.KeysOff != "" || tl.Usable != 10000 {
		t.Fatalf("timeline %+v, want 10000 frames with keys", tl)
	}
	for _, n := range []int{5000, 7777, 5000} {
		if err := pc.Seek(n); err != nil {
			t.Fatal(err)
		}
		if pc.StateHash() != marks[n] {
			t.Fatalf("state after seeking to %d differs from the linear run", n)
		}
		if r := pc.Replayed(); r > 300 {
			t.Fatalf("seek to %d replayed %d frames, want at most 300", n, r)
		}
	}
}

func TestKeyframeBudgetRaisesTheFloor(t *testing.T) {
	const limit = 1 << 20
	s, pc := rewindSessionWith(t, limit, combatModule{})
	pc.SetKeyEvery(60)
	drive(s.Host.Engine, pc, 0, 3000, nil)
	tl := pc.Timeline()
	if tl.Floor == 0 || tl.Keys == 0 || tl.KeyBytes > limit {
		t.Fatalf("timeline %+v, want a raised floor within %d bytes", tl, limit)
	}
	if err := pc.Seek(0); err != nil {
		t.Fatal(err)
	}
	if c := pc.Timeline().Cursor; c != tl.Floor {
		t.Fatalf("seek left of the floor landed on %d, want %d", c, tl.Floor)
	}
}

func TestKeyframeEditAfterRewindEndsHistory(t *testing.T) {
	s, pc := rewindSession(t)
	drive(s.Host.Engine, pc, 0, 600, nil)
	if err := pc.Seek(200); err != nil {
		t.Fatal(err)
	}
	movePlayer(t, pc, mathx.Vec2{X: -3, Y: 0})
	pc.Pause()
	drive(s.Host.Engine, pc, 200, 220, nil)
	if tl := pc.Timeline(); tl.Usable != 200 || tl.Why == "" || !tl.Beyond {
		t.Fatalf("timeline %+v, an edit at frame 200 must end the history there", tl)
	}
}

func TestKeyframeEditAtFrameZeroRebases(t *testing.T) {
	s, pc := rewindSession(t)
	pc.Pause()
	want := mathx.Vec2{X: -3, Y: 0}
	movePlayer(t, pc, want)
	if err := pc.Seek(0); err != nil {
		t.Fatal(err)
	}
	if p := playerPos(pc); p != want {
		t.Fatalf("player at %v after seeking to 0, want the edit %v", p, want)
	}
	pc.Pause()
	drive(s.Host.Engine, pc, 0, 30, nil)
	if tl := pc.Timeline(); tl.Why != "" || tl.Usable != 30 {
		t.Fatalf("timeline %+v, a rebased history must keep recording", tl)
	}
	if err := pc.Seek(0); err != nil {
		t.Fatal(err)
	}
	if p := playerPos(pc); p != want {
		t.Fatalf("player at %v after 30 frames and a seek to 0, want %v", p, want)
	}
}

type opaqueModule struct{}

type opaqueState struct{}

func (opaqueModule) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "opaque", Version: "0.1.0"}
}

func (opaqueModule) Register(*sdk.Registry) error {
	return nil
}

func (opaqueModule) AttachWorld(ctx *sdk.AttachContext) error {
	if ctx.Play && ctx.World != nil {
		ctx.World.SetService("opaque.state", &opaqueState{})
	}
	return nil
}

func (opaqueModule) DetachWorld(ctx *sdk.AttachContext) {
	if ctx.Play && ctx.World != nil {
		ctx.World.RemoveService("opaque.state")
	}
}

func TestKeyframesOffFallsBackToReboot(t *testing.T) {
	s, pc := rewindSessionWith(t, 0, combatModule{}, opaqueModule{})
	var mark [32]byte
	drive(s.Host.Engine, pc, 0, 400, func(f int) {
		if f == 150 {
			mark = playHash(pc)
		}
	})
	if tl := pc.Timeline(); tl.KeysOff != "opaque.state" || tl.Keys != 0 {
		t.Fatalf("timeline %+v, want keyframes off because of opaque.state", tl)
	}
	if err := pc.Seek(150); err != nil {
		t.Fatal(err)
	}
	if playHash(pc) != mark {
		t.Fatal("physics differs after a reboot seek with keyframes off")
	}
}

func TestKeyframeReplayWritesNoSaves(t *testing.T) {
	s, pc := rewindSession(t)
	e := s.Host.Engine
	drive(e, pc, 0, 99, nil)
	keyHold(e, input.KeyF5, true)
	pc.Tick(1.0 / 60)
	keyHold(e, input.KeyF5, false)
	drive(e, pc, 100, 160, nil)
	st, ok := engine.Service[*save.Store](e, persistence.StoreID)
	if !ok {
		t.Skip("no save store in the editor session")
	}
	path, err := st.Path("quick")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.Step()
		if _, err := os.Stat(path); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the live quick save was never written")
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := pc.Seek(50); err != nil {
		t.Fatal(err)
	}
	if err := pc.Seek(150); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		s.Step()
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a replayed frame wrote the save slot again")
	}
}

func TestKeyframeSeekInsideCoroutineWait(t *testing.T) {
	s, pc := rewindSession(t)
	pc.Pause()
	sc := pc.Scene()
	enemy := sc.Local(sc.Find(sc.Root(), "EnemyLeft")).Position
	movePlayer(t, pc, mathx.Vec2{X: enemy.X - 0.74, Y: enemy.Y - 0.05})
	pc.Pause()
	pc.SetKeyEvery(10)
	hashes := map[int][32]byte{}
	hit := 0
	for i := 0; i < 240; i++ {
		pc.Tick(1.0 / 60)
		hashes[i+1] = pausedHash(pc)
		if h, ok := scene.Get[Health](pc.Scene(), pc.Scene().Find(pc.Scene().Root(), "Player")); ok && h.Hits > 0 && hit == 0 {
			hit = i + 1
		}
	}
	if hit == 0 {
		t.Skip("the player was never hit, so the blink coroutine never ran")
	}
	_ = s
	for _, n := range []int{hit + 3, hit + 9, hit + 15} {
		if err := pc.Seek(n); err != nil {
			t.Fatal(err)
		}
		if f := playerField(t, pc, "flash"); f.Float() <= 0 {
			t.Fatalf("frame %d is not inside the blink wait, flash %v", n, f.Float())
		}
		if pc.StateHash() != hashes[n] {
			t.Fatalf("state after seeking into the blink wait at %d differs", n)
		}
	}
}

func playerField(t *testing.T, pc *play.Controller, name string) vm.Value {
	t.Helper()
	rt, ok := engine.ServiceOf[*runtime.Runtime](pc.World(), runtime.ServiceID)
	if !ok {
		t.Fatal("no script runtime in the play world")
	}
	sc := pc.Scene()
	id, ok := rt.InstanceOf(sc.Find(sc.Root(), "Player"), playerType)
	if !ok {
		t.Fatal("player has no PlayerController instance")
	}
	v, _ := rt.Field(id, name)
	return v
}

func TestKeyframeCaptureTime(t *testing.T) {
	s, pc := rewindSession(t)
	drive(s.Host.Engine, pc, 0, 300, nil)
	pc.SetKeyEvery(1)
	var times []time.Duration
	for range 100 {
		pc.Tick(1.0 / 60)
		times = append(times, pc.Timeline().KeyTime)
	}
	slices.Sort(times)
	med := times[len(times)/2]
	t.Logf("median keyframe capture: %v", med)
	if med > time.Millisecond {
		t.Fatalf("median keyframe capture took %v, want at most 1 ms", med)
	}
}
