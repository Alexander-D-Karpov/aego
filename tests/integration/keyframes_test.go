package integration

import (
	"aego/contracts/debugdraw"
	"aego/contracts/persistence"
	"aego/modules/debug"
	"aego/modules/physics"
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
		e.Input().BeginFrame()
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
	e.Input().BeginFrame()
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
	_, pc := rewindSessionAt(t, brainProject(t), 0, combatModule{})
	pc.SetKeyEvery(10)
	hashes := map[int][32]byte{}
	for i := 0; i < 120; i++ {
		pc.Tick(1.0 / 60)
		hashes[i+1] = pausedHash(pc)
	}
	for _, n := range []int{33, 39, 45} {
		if err := pc.Seek(n); err != nil {
			t.Fatal(err)
		}
		if p := brainField(t, pc, "pulses"); p.Int() <= 0 {
			t.Fatalf("frame %d is not inside the Beat wait, pulses %d", n, p.Int())
		}
		if pc.StateHash() != hashes[n] {
			t.Fatalf("state after seeking into the Beat wait at %d differs", n)
		}
	}
}

func brainField(t *testing.T, pc *play.Controller, name string) vm.Value {
	t.Helper()
	rt, ok := engine.ServiceOf[*runtime.Runtime](pc.World(), runtime.ServiceID)
	if !ok {
		t.Fatal("no script runtime in the play world")
	}
	sc := pc.Scene()
	id, ok := rt.InstanceOf(sc.Find(sc.Root(), "Brain"), brainType)
	if !ok {
		t.Fatal("Brain has no script instance")
	}
	v, _ := rt.Field(id, name)
	return v
}

func TestKeyframeCaptureTime(t *testing.T) {
	s, pc := rewindSession(t)
	drive(s.Host.Engine, pc, 0, 300, nil)
	pc.SetKeyEvery(1)
	var times []time.Duration
	cpuStart := cpuTime()
	start := time.Now()
	for range 100 {
		pc.Tick(1.0 / 60)
		times = append(times, pc.Timeline().KeyTime)
	}
	wall := time.Since(start)
	cpuEnd := cpuTime()
	slices.Sort(times)
	med := times[len(times)/2]
	p95 := times[(len(times)*95-1)/100]
	t.Logf("median keyframe capture: %v", med)
	if cpuStart != 0 && cpuEnd >= cpuStart {
		t.Logf("100 ticks: wall %v, process CPU %v; capture p95 %v, max %v", wall, cpuEnd-cpuStart, p95, times[len(times)-1])
	} else {
		t.Logf("100 ticks: wall %v, process CPU unavailable; capture p95 %v, max %v", wall, p95, times[len(times)-1])
	}
	if med > time.Millisecond {
		t.Fatalf("median keyframe capture took %v, want at most 1 ms", med)
	}
}

func TestKeyframeScrubFollowsTheJump(t *testing.T) {
	s, pc := rewindSession(t)
	e := s.Host.Engine
	ys := map[int]float32{}
	for i := 0; i < 400; i++ {
		keyHold(e, input.KeySpace, i%120 >= 60 && i%120 < 64)
		pc.Tick(1.0 / 60)
		ys[i+1] = playerPos(pc).Y
	}
	lo, hi := ys[1], ys[1]
	for _, y := range ys {
		lo, hi = min(lo, y), max(hi, y)
	}
	if hi-lo < 0.5 {
		t.Fatalf("the player never jumped: y in [%v, %v]", lo, hi)
	}
	for _, n := range []int{70, 80, 90, 75, 62, 85, 300, 190, 195, 185, 70} {
		if err := pc.Scrub(n); err != nil {
			t.Fatal(err)
		}
		if y := playerPos(pc).Y; y != ys[n] {
			t.Fatalf("scrubbed to %d: player y %v, recorded %v", n, y, ys[n])
		}
	}
}

func shapeLines(t *testing.T, pc *play.Controller) int {
	t.Helper()
	d, ok := engine.ServiceOf[*debug.Service](pc.World(), debugdraw.ServiceID)
	if !ok {
		t.Fatal("no debug draw service in the play world")
	}
	n := 0
	d.EachChannel(func(name string, count int, _ uint64) {
		if name == physics.ChannelShapes {
			n = count
		}
	})
	return n
}

func TestKeyframeSeekKeepsOverlayVisibility(t *testing.T) {
	s, pc := rewindSession(t)
	pc.SetKeyEvery(60)
	drive(s.Host.Engine, pc, 0, 400, nil)
	if n := shapeLines(t, pc); n != 0 {
		t.Fatalf("%d physics shape primitives while the overlay is off", n)
	}
	d, _ := engine.ServiceOf[*debug.Service](pc.World(), debugdraw.ServiceID)
	d.Hide(physics.ChannelShapes, false)
	drive(s.Host.Engine, pc, 400, 401, nil)
	for _, n := range []int{100, 120, 60, 300} {
		if err := pc.Seek(n); err != nil {
			t.Fatal(err)
		}
		if shapeLines(t, pc) == 0 {
			t.Fatalf("no physics shapes at frame %d after turning the overlay on at frame 400", n)
		}
	}
}

func TestLoadWhilePaused(t *testing.T) {
	s, pc := rewindSession(t)
	e := s.Host.Engine
	drive(e, pc, 0, 120, nil)
	svc, ok := engine.ServiceOf[persistence.Service](pc.World(), persistence.ServiceID)
	if !ok {
		t.Fatal("no save service in the play world")
	}
	if err := svc.Save("paused"); err != nil {
		t.Fatal(err)
	}
	pc.Tick(1.0 / 60)
	saved := playerPos(pc)
	st, _ := engine.Service[*save.Store](e, persistence.StoreID)
	deadline := time.Now().Add(5 * time.Second)
	for !st.Exists("paused") {
		s.Step()
		if time.Now().After(deadline) {
			t.Fatal("the save was never written")
		}
	}
	drive(e, pc, 121, 240, nil)
	if playerPos(pc) == saved {
		t.Fatal("the player did not move after saving")
	}
	if pc.State() == play.Playing {
		pc.Pause()
	}
	before := pc.Scene()
	svc, _ = engine.ServiceOf[persistence.Service](pc.World(), persistence.ServiceID)
	if err := svc.Load("paused"); err != nil {
		t.Fatal(err)
	}
	for deadline = time.Now().Add(5 * time.Second); pc.Scene() == before; {
		pc.Tick(1.0 / 60)
		s.Step()
		if time.Now().After(deadline) {
			t.Fatal("loading while paused never happened")
		}
	}
	if pc.State() != play.Paused {
		t.Fatalf("state %v after loading while paused, want paused", pc.State())
	}
	if p := playerPos(pc); p.Sub(saved).Len() > 1e-3 {
		t.Fatalf("player at %v after loading, saved at %v", p, saved)
	}
}
