package integration

import (
	"runtime"
	"testing"
	"time"

	"aego/editor/core/play"
	"aego/engine/problems"
)

// Replay allocates nothing in the controller beyond the game's own; 56 frames from the key at 300 take no scrub keys (D346).
func TestReplayAllocsPerFrame(t *testing.T) {
	skipUnderRace(t)
	s, pc := rewindSession(t)
	pc.SetKeyEvery(300)
	drive(s.Host.Engine, pc, 0, 600, nil)
	pc.Pause()
	scrubTo(t, pc, 300)
	cached := pc.Timeline().Cached
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	n := 0
	for to := 314; to <= 356; to += 14 {
		scrubTo(t, pc, to)
		n += pc.Replayed()
	}
	runtime.ReadMemStats(&b)
	if tl := pc.Timeline(); n != 56 || tl.Cached != cached {
		t.Fatalf("the seeks replayed %d frames and cached %d more keys, want 56 frames from the key at 300 and none", n, tl.Cached-cached)
	}
	per := float64(b.Mallocs-a.Mallocs) / float64(n)
	t.Logf("%.2f objects per replayed frame", per)
	if per > 1.5 {
		t.Fatalf("%.2f objects per replayed frame, want at most 1.5", per)
	}
}

// Dragging back through the scrub cache replays at most TransientEvery frames per step and lands on the recorded state.
func TestScrubCacheMatchesLinearRun(t *testing.T) {
	s, pc := rewindSession(t)
	pc.SetKeyEvery(300)
	marks := map[int][32]byte{}
	check := map[int]bool{1490: true, 1440: true, 1400: true, 1350: true, 1300: true}
	drive(s.Host.Engine, pc, 0, 1800, func(f int) {
		if check[f] {
			marks[f] = pausedHash(pc)
		}
	})
	pc.Pause()
	var path []int
	for n := 1500; n >= 1300; n -= 10 {
		path = append(path, n)
	}
	for n := 1310; n <= 1500; n += 10 {
		path = append(path, n)
	}
	long := 0
	for i, n := range path {
		scrubTo(t, pc, n)
		if back := i > 1 && n < path[i-1]; back && pc.Replayed() > play.TransientEvery {
			long++
		}
		if h, ok := marks[n]; ok && pc.StateHash() != h {
			t.Fatalf("state at %d after scrubbing differs from the linear run", n)
		}
	}
	if want := 200/play.TransientSpan + 1; long > want {
		t.Fatalf("%d steps of the drag back replayed more than %d frames, want at most %d", long, play.TransientEvery, want)
	}
	if tl := pc.Timeline(); tl.Cached == 0 {
		t.Fatal("the drag kept no keys for scrubbing")
	}
	pc.Pause()
	drive(s.Host.Engine, pc, 1500, 1501, nil)
	if tl := pc.Timeline(); tl.Cached != 0 || tl.CacheBytes != 0 {
		t.Fatalf("after Play the scrub cache holds %d keys, %d bytes", tl.Cached, tl.CacheBytes)
	}
}

func TestAdaptiveKeyEvery(t *testing.T) {
	s, pc := rewindSession(t)
	drive(s.Host.Engine, pc, 0, 900, nil)
	tl := pc.Timeline()
	if tl.KeyEvery < play.MinKeyEvery || tl.KeyEvery > play.DefaultKeyEvery {
		t.Fatalf("key every %d ticks, want %d..%d", tl.KeyEvery, play.MinKeyEvery, play.DefaultKeyEvery)
	}
	// the interval adapts during the run, so the keys are counted against the longest one it may have used
	if tl.Keys < int(900/play.DefaultKeyEvery) {
		t.Fatalf("%d keys over 900 ticks at one per %d", tl.Keys, tl.KeyEvery)
	}
}

func TestThinningKeepsDepth(t *testing.T) {
	recent, old := play.RecentFrames, play.OldFrames
	play.RecentFrames, play.OldFrames = 600, 1800
	t.Cleanup(func() { play.RecentFrames, play.OldFrames = recent, old })
	s, pc := rewindSession(t)
	pc.SetKeyEvery(30)
	var mark [32]byte
	drive(s.Host.Engine, pc, 0, 3600, func(f int) {
		if f == 700 {
			mark = pausedHash(pc)
		}
	})
	pc.Pause()
	keys := pc.KeyFrames()
	newest := keys[len(keys)-1]
	for i := 1; i < len(keys)-1; i++ {
		gap, age := keys[i]-keys[i-1], newest-keys[i]
		switch {
		case age >= play.OldFrames && gap < int(play.OldSpacing):
			t.Fatalf("keys %d and %d are %d apart at age %d, want at least %d", keys[i-1], keys[i], gap, age, play.OldSpacing)
		case age >= play.RecentFrames && age < play.OldFrames && gap < int(play.MidSpacing):
			t.Fatalf("keys %d and %d are %d apart at age %d, want at least %d", keys[i-1], keys[i], gap, age, play.MidSpacing)
		}
	}
	if tl := pc.Timeline(); tl.Floor != 0 {
		t.Fatalf("floor %d, thinning must keep the whole history", tl.Floor)
	}
	if err := pc.Seek(700); err != nil {
		t.Fatal(err)
	}
	if pc.StateHash() != mark {
		t.Fatal("a seek into the thinned history differs from the linear run")
	}
}

// Play and seeks reuse the asset diagnostics: a source is asked again only when its revision changes.
func TestPlayDoesNotRevalidateAssets(t *testing.T) {
	s, pc := rewindSession(t)
	calls := 0
	drop := s.App.Context().Problems.Add(problems.Source{
		Revision: func() uint64 { return 1 },
		Append: func(dst []problems.Diagnostic) []problems.Diagnostic {
			calls++
			return dst
		},
	})
	defer drop()
	s.Step()
	start := calls
	for range 120 {
		s.Step()
	}
	pc.Pause()
	for i := range 20 {
		scrubTo(t, pc, 100-i*3)
		s.Step()
	}
	if n := calls - start; n > 1 {
		t.Fatalf("a source with a constant revision was asked %d times during Play and seeks", n)
	}
}

// A pause the editor holds idles like Stopped once input stops for a second; a running game never does (D346).
func TestHeldPauseIdles(t *testing.T) {
	s, pc := rewindSession(t)
	drive(s.Host.Engine, pc, 0, 30, nil)
	if s.App.Idle() {
		t.Fatal("the editor idles while the game runs")
	}
	pc.Pause()
	if !pc.Held() {
		t.Fatal("the pause button does not hold the game")
	}
	time.Sleep(1100 * time.Millisecond)
	for _, task := range s.App.Tasks().Tasks() {
		t.Logf("task at start of idle wait: kind=%q title=%q state=%v lane=%v", task.Kind, task.Title, task.State(), task.Lane())
	}
	start := time.Now()
	for deadline := time.Now().Add(10 * time.Second); !s.App.Idle(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Logf("play state=%v held=%v now=%g drag-and-drop=%v workspace-drag=%v tasks-busy=%v",
				pc.State(), pc.Held(), s.App.Context().Now(), s.App.Context().DnD.Active(), s.App.Workspace().Dragging(), s.App.Tasks().Busy())
			for _, task := range s.App.Tasks().Tasks() {
				t.Logf("unfinished task: kind=%q title=%q state=%v lane=%v canceled=%v", task.Kind, task.Title, task.State(), task.Lane(), task.Canceled())
			}
			t.Fatal("a held pause without input does not idle once background tasks end")
		}
		s.Step()
	}
	t.Logf("idle reached after %v", time.Since(start))
	pc.Pause()
	s.Step()
	if s.App.Idle() {
		t.Fatal("the editor still idles after the game resumed")
	}
}
