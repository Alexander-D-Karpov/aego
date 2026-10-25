package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"aego/app/edit"
	"aego/core/log"
	"aego/editor/core/play"
	"aego/sdk"
)

const scrubFrames = 3600

// scrubSession plays the platformer for a minute of recorded input.
func scrubSession(tb testing.TB) (*edit.Session, *play.Controller) {
	tb.Helper()
	cfg := edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()}
	if os.Getenv("AEGO_REWIND_GL") != "" {
		cfg.Headless, cfg.Hidden = false, true
	}
	example, mods := "platformer", []sdk.Module{combatModule{}}
	if ex := os.Getenv("AEGO_REWIND_EXAMPLE"); ex != "" && ex != example {
		example, mods = ex, nil
	}
	if os.Getenv("AEGO_REWIND_KEYS") == "off" {
		mods = append(mods, opaqueModule{})
	}
	s, pc := rewindSessionCfg(tb, copyExample(tb, example), 0, cfg, mods...)
	drive(s.Host.Engine, pc, 0, scrubFrames, nil)
	pc.Pause()
	return s, pc
}

func scrubTo(tb testing.TB, pc *play.Controller, n int) {
	tb.Helper()
	if err := pc.Scrub(n); err != nil {
		tb.Fatal(err)
	}
}

// BenchmarkRewindScrub measures one Scrub over a minute of play: back and forward by fixed steps, and a drag.
func BenchmarkRewindScrub(b *testing.B) {
	_, pc := scrubSession(b)
	for _, k := range []int{1, 10, 60, 600} {
		for _, dir := range []string{"Back", "Forward"} {
			b.Run(fmt.Sprintf("%s%d", dir, k), func(b *testing.B) {
				b.ReportAllocs()
				replayed := 0
				for i := 0; i < b.N; i++ {
					at := 900 + (i*137)%(scrubFrames-1800)
					from, to := at, at-k
					if dir == "Forward" {
						from, to = at-k, at
					}
					b.StopTimer()
					scrubTo(b, pc, from)
					b.StartTimer()
					scrubTo(b, pc, to)
					replayed += pc.Replayed()
				}
				b.ReportMetric(float64(replayed)/float64(b.N), "replayed/op")
			})
		}
	}
	b.Run("Drag", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, n := range dragPath(1800) {
				scrubTo(b, pc, n)
			}
		}
		b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*len(dragPath(1800))), "ns/seek")
	})
}

// dragPath is 120 seeks of about 30 frames, back and then forward again.
func dragPath(from int) []int {
	out := make([]int, 0, 120)
	for i := 1; i <= 60; i++ {
		out = append(out, from-30*i)
	}
	for i := 59; i >= 0; i-- {
		out = append(out, from-30*i)
	}
	return out
}

type seekSample struct {
	d        time.Duration
	replayed int
	allocs   uint64
	bytes    uint64
}

func measureSeek(tb testing.TB, pc *play.Controller, n int) seekSample {
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	start := time.Now()
	scrubTo(tb, pc, n)
	d := time.Since(start)
	runtime.ReadMemStats(&b)
	return seekSample{d: d, replayed: pc.Replayed(), allocs: b.Mallocs - a.Mallocs, bytes: b.TotalAlloc - a.TotalAlloc}
}

func summarize(name string, ss []seekSample) string {
	ds := make([]time.Duration, len(ss))
	var rep, al, by uint64
	for i, s := range ss {
		ds[i] = s.d
		rep += uint64(s.replayed)
		al += s.allocs
		by += s.bytes
	}
	slices.Sort(ds)
	n := uint64(len(ss))
	q := func(p float64) time.Duration { return ds[min(int(float64(len(ds))*p), len(ds)-1)] }
	return fmt.Sprintf("%-12s n=%-4d p50=%-10v p95=%-10v max=%-10v replayed/seek=%-6d allocs/seek=%-7d bytes/seek=%d",
		name, n, q(0.5).Round(time.Microsecond), q(0.95).Round(time.Microsecond), ds[len(ds)-1].Round(time.Microsecond), rep/n, al/n, by/n)
}

// TestRewindScrubMeasure writes the scrub cost table to AEGO_REWIND_MEASURE; AEGO_REWIND_PROF=<dir> adds profiles, AEGO_REWIND_GL=1 opens a window, AEGO_REWIND_KEYS=off turns keyframes off, AEGO_REWIND_EXAMPLE picks another example.
func TestRewindScrubMeasure(t *testing.T) {
	out := os.Getenv("AEGO_REWIND_MEASURE")
	if out == "" {
		t.Skip("set AEGO_REWIND_MEASURE to a file to measure scrubbing")
	}
	prof := os.Getenv("AEGO_REWIND_PROF")
	if v, err := strconv.Atoi(os.Getenv("AEGO_REWIND_MEMRATE")); err == nil {
		runtime.MemProfileRate = v
	}
	var w strings.Builder
	start := time.Now()
	s, pc := scrubSession(t)
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	tl := pc.Timeline()
	fmt.Fprintf(&w, "record: %d frames, tick %d, %v to play\n", tl.Frames, tl.Tick, time.Since(start).Round(time.Millisecond))
	fmt.Fprintf(&w, "keyframes: %d (every %d ticks), %.1f MB, last %d KB, capture %v\n", tl.Keys, tl.KeyEvery, float64(tl.KeyBytes)/(1<<20), tl.LastKey>>10, tl.KeyTime)
	fmt.Fprintf(&w, "recording: %.2f MB (scene data, frames, inputs), floor %d\n", float64(tl.Bytes)/(1<<20), tl.Floor)
	fmt.Fprintf(&w, "heap in use after GC: %.1f MB\n\n", float64(ms.HeapAlloc)/(1<<20))
	var first, again []seekSample
	for i := range 40 {
		at := 900 + (i*137)%(scrubFrames-1800)
		scrubTo(t, pc, scrubFrames)
		first = append(first, measureSeek(t, pc, at))
		again = append(again, measureSeek(t, pc, at-1))
	}
	fmt.Fprintln(&w, summarize("back first", first))
	fmt.Fprintln(&w, summarize("back 1 again", again))
	tl = pc.Timeline()
	fmt.Fprintf(&w, "scrub cache: %d keys, %.1f MB\n", tl.Cached, float64(tl.CacheBytes)/(1<<20))
	for _, k := range []int{1, 10, 60, 600} {
		samples := 40
		if k == 600 {
			samples = 12
		}
		var back, fwd []seekSample
		for i := range samples {
			at := 900 + (i*137)%(scrubFrames-1800)
			scrubTo(t, pc, at)
			back = append(back, measureSeek(t, pc, at-k))
			fwd = append(fwd, measureSeek(t, pc, at))
		}
		fmt.Fprintln(&w, summarize(fmt.Sprintf("back %d", k), back))
		fmt.Fprintln(&w, summarize(fmt.Sprintf("forward %d", k), fwd))
	}
	scrubTo(t, pc, 1800)
	var cpu *os.File
	var heapBase string
	if prof != "" {
		heapBase = filepath.Join(prof, "drag-heap-base.pb.gz")
		writeHeap(t, heapBase)
		f, err := os.Create(filepath.Join(prof, "drag-cpu.pb.gz"))
		if err != nil {
			t.Fatal(err)
		}
		cpu = f
		if err := pprof.StartCPUProfile(cpu); err != nil {
			t.Fatal(err)
		}
	}
	var drag []seekSample
	for range 3 {
		for _, n := range dragPath(1800) {
			drag = append(drag, measureSeek(t, pc, n))
		}
	}
	if cpu != nil {
		pprof.StopCPUProfile()
		cpu.Close()
		writeHeap(t, filepath.Join(prof, "drag-heap.pb.gz"))
	}
	fmt.Fprintln(&w, summarize("drag 30", drag))
	scrubTo(t, pc, 1800)
	var frames, idle, stepOnly []time.Duration
	stop := cpuProfile(t, prof, "idle-cpu.pb.gz")
	idleCPU := cpuTime()
	for range 120 {
		st := time.Now()
		s.Step()
		idle = append(idle, time.Since(st))
	}
	idleCPU = (cpuTime() - idleCPU) / 120
	stop()
	stop = cpuProfile(t, prof, "frame-cpu.pb.gz")
	frameCPU := cpuTime()
	for _, n := range dragPath(1800) {
		st := time.Now()
		scrubTo(t, pc, n)
		mid := time.Now()
		s.Step()
		frames = append(frames, time.Since(st))
		stepOnly = append(stepOnly, time.Since(mid))
	}
	frameCPU = (cpuTime() - frameCPU) / time.Duration(len(dragPath(1800)))
	stop()
	med := func(ds []time.Duration) time.Duration {
		ds = slices.Clone(ds)
		slices.Sort(ds)
		return ds[len(ds)/2].Round(time.Microsecond)
	}
	fmt.Fprintf(&w, "\neditor frame, paused, no seek: wall median %v, process CPU %v per frame\n", med(idle), idleCPU.Round(time.Microsecond))
	fmt.Fprintf(&w, "editor frame with one scrub of 30 (Apply's one seek per frame): wall median %v, of which the frame after the seek %v; process CPU %v per frame\n", med(frames), med(stepOnly), frameCPU.Round(time.Microsecond))
	t.Log("\n" + w.String())
	if err := os.WriteFile(out, []byte(w.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// cpuProfile profiles until the returned function runs; with no directory it does nothing.
func cpuProfile(t *testing.T, dir, name string) func() {
	t.Helper()
	if dir == "" {
		return func() {}
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		t.Fatal(err)
	}
	return func() {
		pprof.StopCPUProfile()
		f.Close()
	}
}

func writeHeap(t *testing.T, path string) {
	t.Helper()
	runtime.GC()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := pprof.Lookup("allocs").WriteTo(f, 0); err != nil {
		t.Fatal(err)
	}
}
