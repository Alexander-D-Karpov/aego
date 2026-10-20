//go:build aego_golden

package golden

import (
	"strings"
	"testing"
	"time"

	"aego/core/mathx"
	"aego/core/stats"
	"aego/core/trace"
	"aego/editor/core/context"
	"aego/editor/core/play"
	"aego/editor/core/services"
	"aego/editor/panels/profiler"
	"aego/render"
	"aego/ui/widgets"
)

func fixedMemory() profiler.MemStats {
	return profiler.MemStats{Heap: 48 << 20, Goal: 96 << 20, Process: 180 << 20, Goroutines: 12, GC: 7}
}

func TestEditorProfilerPlay(t *testing.T) {
	env := newEditorEnvSize(t, editorW, 1200)
	secs := []context.ProfilerSection{{Title: "Physics", Play: true, Draw: func() {
		widgets.LabelDim(env.ui, "bodies 666   active 120   sleeping 546   shapes 667   joints 30")
		widgets.LabelDim(env.ui, "broad 0.21   narrow 0.35   solve 0.62   integrate 0.08 ms")
	}}}
	p := profiler.New(profiler.Deps{Sections: func() []context.ProfilerSection { return secs }, Memory: fixedMemory})
	for i := 0; i < 120; i++ {
		w := float64(i%30) / 30
		f := &stats.Frame{FrameTime: 1.0 / 60, UpdateTime: 0.003 + 0.002*w, RenderTime: 0.005 + 0.001*w, AllocBytes: uint64(4096 * (i % 4))}
		rs := render.Stats{DrawCalls: 14, Visible: 320}
		g := play.Stats{Update: 0.002 + 0.001*w, Render: 0.004, Sprites: 48, Nodes: 61, Scale: 1}
		sc := context.ScriptStats{
			Instances: 5, Update: 3, Fixed: 2,
			Instructions: uint64(12000 + 100*(i%10)), HostCalls: 900, HeapObjects: 40, HeapBytes: 64 << 10,
			Coroutines: context.ScriptCoroutineStats{Active: 2, Sleeping: 1, ResumesPerFrame: 2},
			Signals:    context.ScriptSignalStats{Connections: 2, EmitsPerFrame: 1, HandlersPerFrame: 1},
			Types:      []context.ScriptTypeStats{{Name: "mygame.player.PlayerController", Instances: 1, Calls: 120}},
		}
		p.Record(float64(i)/60, f, rs, g, sc, true, true)
	}
	img, err := env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui}, full) })
	if got := strings.Join(p.Sections(), ","); got != "Frame,Memory,Game,Scripts,Physics" {
		t.Fatalf("sections %s", got)
	}
	checkGolden(t, "editor_profiler_play", img, err)
}

func TestProfilerGolden(t *testing.T) {
	env := newEditorEnvSize(t, 800, 1240)
	p := profiler.New(profiler.Deps{Memory: fixedMemory})
	sc := context.ScriptStats{
		Instances: 3, Update: 2, Fixed: 1, Instructions: 12000, HostCalls: 340, HeapBytes: 96 << 10,
		Types: []context.ScriptTypeStats{{Name: "mygame.player.PlayerController", Instances: 1, Calls: 120, Time: 300 * time.Microsecond}},
	}
	for i := 0; i < 160; i++ {
		k := float64(i%17) / 17
		f := &stats.Frame{FrameTime: 0.012 + 0.006*k, UpdateTime: 0.004 + 0.002*k, RenderTime: 0.006, AllocBytes: uint64(20<<10 + i*64)}
		g := play.Stats{Update: 0.002 + 0.001*k, Render: 0.003, Nodes: 9, Sprites: 7, Scale: 1}
		p.Record(float64(i)*0.125, f, render.Stats{DrawCalls: 14, Visible: 120}, g, sc, true, i >= 100)
	}
	img, err := env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui}, full) })
	checkGolden(t, "editor_profiler", img, err)
}

func TestProfilerShortPanel(t *testing.T) {
	env := newEditorEnvSize(t, 800, 400)
	p := profiler.New(profiler.Deps{Memory: fixedMemory, Tracer: trace.New(trace.Options{})})
	img, err := env.draw(func(full mathx.Rect) {
		p.Draw(&services.Frame{UI: env.ui}, mathx.Rect{X: full.X, Y: full.Y, W: full.W, H: 250})
	})
	checkGolden(t, "editor_profiler_short", img, err)
}
