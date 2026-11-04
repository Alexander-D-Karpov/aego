//go:build aego_gltest

package integration

import (
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"aego/app/run"
	"aego/core/log"
	"aego/core/mathx"
	"aego/modules/particles"
	"aego/project"
	"aego/scene"
	"aego/scene/frame"
	"aego/tests/particlefixture"
)

// This is a diagnostic GL profile on a generated Smoke project. It is kept
// separate from timed release acceptance and never changes any golden or user project.
func TestParticleSmokeOverdrawGPUProfile(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	root := t.TempDir()
	if err := particlefixture.Write(root); err != nil {
		t.Fatal(err)
	}
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	p.Window.Width, p.Window.Height = 1280, 720
	a, err := run.Open(p, run.Config{Hidden: true, FrameTime: 1. / 60, Seed: 42, ScenePath: filepath.Join(root, "scenes", "Smoke.aescene"), Logger: log.Nop(), NoProfile: true})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.Engine.SetLockstep(true)
	if !a.Engine.Caps().TimerQueries {
		t.Skip("GPU timer unavailable on " + a.Engine.Caps().Renderer)
	}
	h := a.Scene.Find(a.Scene.Root(), "Smoke")
	c, ok := scene.Get[particles.ParticleSystem2D](a.Scene, h)
	if !ok {
		t.Fatal("Smoke emitter missing")
	}
	// Keep the real texture, alpha gradient, shader, PMA ordering and emitter path.
	c.MaxParticles, c.Rate, c.Lifetime, c.Size = 96, 120, mathx.V2(8, 8), mathx.V2(.6, .6)
	d := a.Driver()
	for range 75 {
		if !a.Engine.Step(d) {
			t.Fatal(a.Engine.Err())
		}
	}
	w := a.Engine.MainWorld()
	w.Time.SetPaused(true)
	effects := particles.Of(w)
	if n := effects.Count(h); n != 96 {
		t.Fatalf("stress fixture needs 96 live particles, got %d", n)
	}
	tick := w.Time.Tick()
	for _, scale := range []float32{1, 4, 1} {
		var measurements []uint64
		var area float64
		a.Runner.SetPreviewExtract(func(r *frame.Runner) {
			area = 0
			for i, v := range r.Views() {
				b := r.Bucket(i)
				for j := range b.Sprites {
					dr := &b.Sprites[j]
					dr.Size = dr.Size.Scale(scale)
					sz := dr.WorldSize(v.PixelsPerUnit).Scale(v.EffectiveZoom())
					area += float64(sz.X * sz.Y)
				}
			}
		})
		var previous uint64
		for i := 0; i < 44; i++ {
			if !a.Engine.Step(d) {
				t.Fatal(a.Engine.Err())
			}
			st := a.RenderStats()
			if w.Time.Tick() != tick || effects.Stats().Submitted != 96 {
				t.Fatal("presentation probe changed simulation or culling")
			}
			// The ring may expose the old sample while its next query is pending.
			if i >= 12 && st.GPUValid && st.GPUNS != previous {
				measurements = append(measurements, st.GPUNS)
			}
			if st.GPUValid {
				previous = st.GPUNS
			}
		}
		if len(measurements) < 5 {
			t.Fatalf("insufficient distinct completed GPU timer results: %d", len(measurements))
		}
		slices.Sort(measurements)
		m := effects.Stats()
		t.Logf("%s 1280x720 seed 42 tick %d: quad scale %.0fx, sum of unscissored quad areas %.0f px (%.2f screen equivalents), draws %d, instances %d, uploaded bytes %d, GPU main-pass p50/p95 %.3f/%.3f ms (%d distinct completed samples)", a.Engine.Caps().Renderer, tick, scale, area, area/(1280*720), m.Draws, m.Submitted, m.UploadBytes, float64(measurements[len(measurements)/2])/1e6, float64(measurements[(len(measurements)*95-1)/100])/1e6, len(measurements))
	}
}
