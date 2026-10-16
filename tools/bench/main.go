package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"

	"aego/core/log"
	"aego/demos"
	_ "aego/demos/stress"
	_ "aego/demos/stress-scene"
	"aego/engine"
	renderhost "aego/render/host"
	"aego/tools/bench/stat"
)

type renderDemo interface {
	Module() *renderhost.Module
}

type record struct {
	Demo           string               `json:"demo"`
	Renderer       string               `json:"renderer"`
	Frames         int                  `json:"frames"`
	AllocsPerFrame int64                `json:"allocsPerFrame"`
	DrawCalls      float64              `json:"drawCalls"`
	Sprites        float64              `json:"sprites"`
	Metrics        map[string][]float64 `json:"metrics"`
}

func (r record) key() string {
	return r.Demo + "|" + r.Renderer
}

func main() {
	demoName := flag.String("demo", "stress", "demo to benchmark")
	frames := flag.Int("frames", 120, "frames per sample")
	count := flag.Int("count", 10, "samples")
	warmup := flag.Int("warmup", 120, "frames excluded from measurement")
	baseline := flag.String("baseline", ".cache/bench/baseline.json", "baseline file")
	write := flag.Bool("update", false, "write the current run into the baseline")
	alpha := flag.Float64("alpha", 0.05, "significance level")
	tolerance := flag.Float64("tolerance", 10, "allowed regression of the median in percent")
	memprofile := flag.String("memprofile", "", "write an allocation profile to this file")
	timing := flag.Bool("time", true, "compare timings; false checks allocations only")
	flag.Parse()

	if *memprofile != "" {
		runtime.MemProfileRate = 1
	}
	ctor, ok := demos.Registry[*demoName]
	if !ok {
		fail(fmt.Errorf("demo %q is not registered", *demoName))
	}
	demo := ctor()
	rd, ok := demo.(renderDemo)
	if !ok {
		fail(fmt.Errorf("demo %q does not expose a render module", *demoName))
	}
	m := rd.Module()
	e, err := engine.NewBuilder(engine.Config{
		Title:         "aego bench",
		Width:         1280,
		Height:        720,
		Hidden:        true,
		ResRoot:       "demos/res",
		AppID:         "aego-bench",
		MaxFixedSteps: 1,
	}).WithLogger(log.Nop()).Use(m).Build()
	if err != nil {
		fail(err)
	}
	reporter, _ := demo.(demos.MetricsReporter)
	var buf []demos.Metric
	for i := 0; i < *warmup; i++ {
		if !e.Step(demo) {
			fail(fmt.Errorf("engine stopped during warmup"))
		}
		if i == 0 {
			if o, ok := demo.(interface{ SetOverlay(bool) }); ok {
				o.SetOverlay(false)
			}
		}
	}
	rec := record{Demo: *demoName, Renderer: e.Caps().Renderer, Metrics: map[string][]float64{}}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	total := 0
	var draws, sprites float64
	for c := 0; c < *count; c++ {
		var cpu, gpu float64
		gpuN, n := 0, 0
		extra := map[string]float64{}
		for i := 0; i < *frames; i++ {
			if !e.Step(demo) {
				break
			}
			fs := e.Stats()
			rs := m.Stats()
			cpu += fs.RenderTime * 1000
			if rs.GPUValid {
				gpu += float64(rs.GPUNS) / 1e6
				gpuN++
			}
			draws += float64(rs.DrawCalls)
			sprites += float64(rs.Visible)
			if reporter != nil {
				buf = reporter.ReportMetrics(buf[:0])
				for _, mt := range buf {
					extra[mt.Name] += mt.Value
				}
			}
			n++
		}
		if n == 0 {
			fail(fmt.Errorf("no frames measured"))
		}
		rec.Metrics["cpuRenderMs"] = append(rec.Metrics["cpuRenderMs"], cpu/float64(n))
		if gpuN > 0 {
			rec.Metrics["gpuMs"] = append(rec.Metrics["gpuMs"], gpu/float64(gpuN))
		}
		for name, v := range extra {
			rec.Metrics[name] = append(rec.Metrics[name], v/float64(n))
		}
		total += n
	}
	runtime.ReadMemStats(&after)
	if *memprofile != "" {
		writeProfile(*memprofile)
	}
	e.Close()
	rec.Frames = total
	rec.AllocsPerFrame = int64(math.Round(float64(after.Mallocs-before.Mallocs) / float64(total)))
	rec.DrawCalls = draws / float64(total)
	rec.Sprites = sprites / float64(total)

	blob, _ := json.MarshalIndent(rec, "", "  ")
	fmt.Println(string(blob))
	if *write {
		update(*baseline, rec)
		return
	}
	check(*baseline, rec, *alpha, *tolerance, *timing)
}

func writeProfile(path string) {
	f, err := os.Create(path)
	if err != nil {
		fail(err)
	}
	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		f.Close()
		fail(err)
	}
	f.Close()
}

func load(path string) map[string]record {
	out := map[string]record{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	if json.Unmarshal(data, &out) != nil {
		return map[string]record{}
	}
	return out
}

func update(path string, rec record) {
	all := load(path)
	all[rec.key()] = rec
	blob, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		fail(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(path, append(blob, '\n'), 0o644); err != nil {
		fail(err)
	}
	fmt.Println("baseline updated for", rec.key())
}

// baselineFor finds rec's baseline; timings need the same renderer, allocations any of the demo.
func baselineFor(all map[string]record, rec record, timing bool) (record, string, bool) {
	if b, ok := all[rec.key()]; ok {
		return b, rec.key(), true
	}
	if timing {
		return record{}, "", false
	}
	keys := make([]string, 0, len(all))
	for k, b := range all {
		if b.Demo == rec.Demo {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return record{}, "", false
	}
	sort.Strings(keys)
	return all[keys[0]], keys[0], true
}

func check(path string, rec record, alpha, tolerance float64, timing bool) {
	base, key, ok := baselineFor(load(path), rec, timing)
	if !ok {
		fmt.Fprintf(os.Stderr, "no baseline for %q, run make bench-baseline\n", rec.key())
		return
	}
	if key != rec.key() {
		fmt.Printf("allocations compared with the baseline of %q\n", key)
	}
	failed := false
	if rec.AllocsPerFrame != base.AllocsPerFrame {
		fmt.Fprintf(os.Stderr, "FAIL allocations per frame: %d, baseline %d\n", rec.AllocsPerFrame, base.AllocsPerFrame)
		failed = true
	}
	fmt.Printf("draw calls %.2f (baseline %.2f), sprites %.0f (baseline %.0f)\n", rec.DrawCalls, base.DrawCalls, rec.Sprites, base.Sprites)
	if timing {
		names := make([]string, 0, len(rec.Metrics))
		for name := range rec.Metrics {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			b := base.Metrics[name]
			if len(b) == 0 {
				fmt.Printf("%-24s %10.3f  (no baseline)\n", name, stat.Median(rec.Metrics[name]))
				continue
			}
			v := stat.Compare(b, rec.Metrics[name], alpha, tolerance)
			mark := "ok"
			if v.Regressed {
				mark = "FAIL"
				failed = true
			}
			fmt.Printf("%-24s %10.3f -> %10.3f  %+6.1f%%  p=%.3f  %s\n", name, v.Base, v.Head, v.Delta, v.P, mark)
		}
	}
	if failed {
		os.Exit(1)
	}
	fmt.Println("bench: ok")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "bench:", err)
	os.Exit(2)
}
