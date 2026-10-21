package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"aego/demos"
	_ "aego/demos/asset-hot-reload"
	_ "aego/demos/audio"
	_ "aego/demos/basic"
	_ "aego/demos/camera"
	_ "aego/demos/headless"
	_ "aego/demos/hierarchy"
	_ "aego/demos/input"
	_ "aego/demos/layers"
	_ "aego/demos/mesh"
	_ "aego/demos/mixed"
	_ "aego/demos/offscreen"
	_ "aego/demos/physics2d"
	_ "aego/demos/resize"
	_ "aego/demos/scene-load"
	_ "aego/demos/sprites"
	_ "aego/demos/stress"
	_ "aego/demos/stress-mesh"
	_ "aego/demos/stress-scene"
	_ "aego/demos/text"
	"aego/engine"
	"aego/platform"
	renderhost "aego/render/host"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("sandbox", flag.ContinueOnError)
	name := fs.String("demo", "basic", "demo to run")
	vsyncFlag := fs.String("vsync", "on", "off|on|adaptive")
	fullscreen := fs.Bool("fullscreen", false, "start fullscreen")
	headless := fs.Bool("headless", false, "run without a window on the null gpu backend")
	frames := fs.Int("frames", 0, "run a fixed number of frames and exit")
	warmup := fs.Int("warmup", 60, "frames excluded from the allocation measurement")
	debug := fs.Bool("debug", false, "enable gpu debug output and verbose logging")
	hidden := fs.Bool("hidden", false, "create the window without showing it (benchmarks)")
	res := fs.String("res", "demos/res", "directory mounted as res://")
	width := fs.Int("width", 1280, "window width")
	height := fs.Int("height", 720, "window height")
	strict := fs.Bool("strict-allocs", false, "exit non-zero if frames allocate after warmup")
	list := fs.Bool("list", false, "list demos and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *list {
		for _, n := range demos.Names() {
			fmt.Println(n)
		}
		return 0
	}
	ctor, ok := demos.Registry[*name]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown demo %q, available: %s\n", *name, strings.Join(demos.Names(), ", "))
		return 2
	}
	mode, err := platform.ParseVSync(*vsyncFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	demo := ctor()
	cfg := engine.Config{
		Title:      "aego sandbox: " + demo.Name(),
		Width:      *width,
		Height:     *height,
		Fullscreen: *fullscreen,
		VSync:      mode,
		Headless:   *headless,
		Debug:      *debug,
		Hidden:     *hidden,
		ResRoot:    *res,
		AppID:      "aego-sandbox",
	}
	if c, ok := demo.(demos.Configurer); ok {
		c.Configure(&cfg)
	}
	if cfg.Headless {
		if *frames <= 0 {
			*frames = 600
		}
		if cfg.HeadlessFrameTime <= 0 {
			cfg.HeadlessFrameTime = 1.0 / 60
		}
	}

	builder := engine.NewBuilder(cfg)
	if m, ok := demo.(interface{ Module() *renderhost.Module }); ok {
		builder.Use(m.Module())
	}
	eng, err := builder.Build()
	if err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		return 1
	}
	if *frames <= 0 {
		if err := eng.Run(demo); err != nil {
			fmt.Fprintln(os.Stderr, "run:", err)
			return 1
		}
		return 0
	}
	return runFrames(eng, demo, *frames, *warmup, *strict)
}

func runFrames(eng *engine.Engine, app engine.App, frames, warmup int, strict bool) int {
	if warmup >= frames {
		warmup = frames / 10
	}
	n := 0
	for ; n < warmup; n++ {
		if !eng.Step(app) {
			break
		}
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	measured := 0
	var draws, tris uint64
	var elapsed float64
	for ; n < frames; n++ {
		if !eng.Step(app) {
			break
		}
		s := eng.Stats()
		draws += uint64(s.DrawCalls)
		tris += uint64(s.Triangles)
		elapsed += s.FrameTime
		measured++
	}
	runtime.ReadMemStats(&after)
	err := eng.Close()
	allocs := after.Mallocs - before.Mallocs
	bytes := after.TotalAlloc - before.TotalAlloc
	perFrame := func(v uint64) float64 {
		if measured == 0 {
			return 0
		}
		return float64(v) / float64(measured)
	}
	fmt.Printf("frames=%d measured=%d sim_time=%.3fs draws/frame=%.2f tris/frame=%.2f allocs=%d alloc_bytes=%d gc=%d\n",
		n, measured, elapsed, perFrame(draws), perFrame(tris), allocs, bytes, after.NumGC-before.NumGC)
	if err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		return 1
	}
	if strict && allocs > 0 {
		fmt.Fprintf(os.Stderr, "allocations after warmup: %d\n", allocs)
		return 3
	}
	return 0
}
