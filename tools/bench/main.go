package main

import (
	"encoding/json"
	"flag"
	"fmt"
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
	rendermodule "aego/render/module"
)

type result struct {
	Demo         string             `json:"demo"`
	Renderer     string             `json:"renderer"`
	Frames       int                `json:"frames"`
	CPURenderMS  float64            `json:"cpuRenderMs"`
	GPUMS        float64            `json:"gpuMs"`
	FrameMS      float64            `json:"frameMs"`
	DrawCalls    float64            `json:"drawCalls"`
	Batches      float64            `json:"batches"`
	Sprites      float64            `json:"sprites"`
	AllocsPerFrm float64            `json:"allocsPerFrame"`
	Extra        map[string]float64 `json:"extra,omitempty"`
}

func (r result) key() string {
	return r.Demo + "|" + r.Renderer
}

func main() {
	demoName := flag.String("demo", "stress", "demo to benchmark")
	frames := flag.Int("frames", 600, "frames to run")
	warmup := flag.Int("warmup", 120, "frames excluded from measurement")
	baseline := flag.String("baseline", "tests/bench/baseline.json", "baseline file")
	write := flag.Bool("update", false, "write the current run into the baseline")
	tolerance := flag.Float64("tolerance", 20, "allowed regression in percent")
	maxDraws := flag.Float64("max-draws", 4, "maximum average draw calls per frame")
	memprofile := flag.String("memprofile", "", "write an allocation profile to this file")
	flag.Parse()

	if *memprofile != "" {
		runtime.MemProfileRate = 1
	}
	ctor, ok := demos.Registry[*demoName]
	if !ok {
		fail(fmt.Errorf("demo %q is not registered", *demoName))
	}
	demo := ctor()
	mod, ok := demo.(interface{ Module() *rendermodule.Module })
	if !ok {
		fail(fmt.Errorf("demo %q does not expose a render module", *demoName))
	}
	m := mod.Module()

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
	var buf, sums []demos.Metric
	for i := 0; i < *warmup; i++ {
		if !e.Step(demo) {
			fail(fmt.Errorf("engine stopped during warmup"))
		}
		if i == 0 {
			if o, ok := demo.(interface{ SetOverlay(bool) }); ok {
				o.SetOverlay(false)
			}
		}
		if reporter != nil {
			buf = reporter.ReportMetrics(buf[:0])
			for len(sums) < len(buf) {
				sums = append(sums, demos.Metric{Name: buf[len(sums)].Name})
			}
		}
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	res := result{Demo: *demoName, Renderer: e.Caps().Renderer}
	measured := 0
	for i := 0; i < *frames; i++ {
		if !e.Step(demo) {
			break
		}
		fs := e.Stats()
		rs := m.Stats()
		res.FrameMS += fs.FrameTime * 1000
		res.CPURenderMS += fs.RenderTime * 1000
		if rs.GPUValid {
			res.GPUMS += float64(rs.GPUNS) / 1e6
		}
		res.DrawCalls += float64(rs.DrawCalls)
		res.Batches += float64(rs.Batches)
		res.Sprites += float64(rs.Visible)
		if reporter != nil {
			buf = reporter.ReportMetrics(buf[:0])
			for k := range buf {
				if k < len(sums) {
					sums[k].Value += buf[k].Value
				}
			}
		}
		measured++
	}
	runtime.ReadMemStats(&after)

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
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
	e.Close()

	if measured == 0 {
		fail(fmt.Errorf("no frames measured"))
	}
	n := float64(measured)
	res.Frames = measured
	res.FrameMS /= n
	res.CPURenderMS /= n
	res.GPUMS /= n
	res.DrawCalls /= n
	res.Batches /= n
	res.Sprites /= n
	res.AllocsPerFrm = float64(after.Mallocs-before.Mallocs) / n
	if len(sums) > 0 {
		res.Extra = make(map[string]float64, len(sums))
		for _, s := range sums {
			res.Extra[s.Name] = s.Value / n
		}
	}

	blob, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(blob))

	if *write {
		updateBaseline(*baseline, res)
		return
	}
	check(*baseline, res, *tolerance, *maxDraws)
}

func loadBaseline(path string) map[string]result {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]result{}
	}
	out := map[string]result{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]result{}
	}
	return out
}

func updateBaseline(path string, res result) {
	all := loadBaseline(path)
	all[res.key()] = res
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
	fmt.Println("baseline updated for", res.key())
}

func check(path string, res result, tolerance, maxDraws float64) {
	failed := false
	// Mallocs is process-wide; background goroutines add sub-frame jitter. A real
	// per-frame alloc is >=1/frame, so fail there and warn in the band above noise.
	const allocNoiseFloor = 0.05
	switch {
	case res.AllocsPerFrm >= 1:
		fmt.Fprintf(os.Stderr, "FAIL allocations per frame: %.3f\n", res.AllocsPerFrm)
		failed = true
	case res.AllocsPerFrm > allocNoiseFloor:
		fmt.Fprintf(os.Stderr, "warn: allocations per frame %.3f above noise floor %.2f\n", res.AllocsPerFrm, allocNoiseFloor)
	}
	if res.DrawCalls > maxDraws {
		fmt.Fprintf(os.Stderr, "FAIL draw calls per frame: %.2f > %.2f\n", res.DrawCalls, maxDraws)
		failed = true
	}
	base, ok := loadBaseline(path)[res.key()]
	if !ok {
		fmt.Fprintf(os.Stderr, "no baseline for %q, run with -update\n", res.key())
		if failed {
			os.Exit(1)
		}
		return
	}
	cmp := func(name string, got, want float64) {
		if want <= 0 {
			return
		}
		if delta := (got - want) / want * 100; delta > tolerance {
			fmt.Fprintf(os.Stderr, "FAIL %s regressed %.1f%% (%.3f vs %.3f)\n", name, delta, got, want)
			failed = true
		}
	}
	cmp("cpu render", res.CPURenderMS, base.CPURenderMS)
	cmp("gpu", res.GPUMS, base.GPUMS)
	names := make([]string, 0, len(res.Extra))
	for name := range res.Extra {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		cmp(name, res.Extra[name], base.Extra[name])
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
